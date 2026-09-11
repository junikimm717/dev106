package docker

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"strings"
	"sync"
	"time"

	"github.com/containerd/errdefs"
	"github.com/junikimm717/dev106/internal/config"
	"github.com/junikimm717/dev106/internal/host"
	"github.com/junikimm717/dev106/internal/repo"
	"github.com/junikimm717/dev106/internal/shared"
	"github.com/moby/moby/api/types/container"
	dockerClient "github.com/moby/moby/client"
)

type DevClient struct {
	client *dockerClient.Client
	ctx    context.Context

	usbIDs  []host.USBID
	usbOnce sync.Once
	usb     host.USBDevices
}

func New(ctx context.Context, cfg *config.DevConfig) (*DevClient, error) {
	// Docker Desktop rewrites bind mount paths in an API proxy behind the
	// distro's unix socket; tcp:// skips that proxy and the bind silently
	// degrades to an empty volume rather than failing.
	if warning := host.DockerHostWarning(os.Getenv("DOCKER_HOST")); warning != "" {
		fmt.Fprint(os.Stderr, warning)
	}

	client, err := dockerClient.New(dockerClient.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("could not connect to Docker; is the daemon running?\n%w", err)
	}

	// A typo here must not stop a shell opening, so report it and carry on
	// with whatever parsed.
	var ids []host.USBID
	if cfg != nil {
		var bad []string
		ids, bad = host.ParseUSBIDs(cfg.USBIDs)
		for _, entry := range bad {
			fmt.Fprintf(os.Stderr, "warning: ignoring malformed usb_ids entry %q; expected \"vid:pid\"\n", entry)
		}
	}

	return &DevClient{
		client: client,
		ctx:    ctx,
		usbIDs: ids,
	}, nil
}

// daemonIdentity treats a daemon that will not answer as unknown rather than
// as an error; USB must never be why a shell fails to open.
func (d *DevClient) daemonIdentity() host.DaemonIdentity {
	id := host.DaemonIdentity{}
	if host := os.Getenv("DOCKER_HOST"); strings.HasPrefix(host, "tcp://") || strings.HasPrefix(host, "ssh://") {
		id.Remote = true
	}

	ctx, cancel := context.WithTimeout(d.ctx, 3*time.Second)
	defer cancel()
	result, err := d.client.Info(ctx, dockerClient.InfoOptions{})
	if err != nil {
		return id
	}
	id.OperatingSystem = result.Info.OperatingSystem
	id.KernelVersion = result.Info.KernelVersion
	return id
}

// USB reports what the daemon can hand a container.
func (d *DevClient) USB() host.USBDevices {
	d.usbOnce.Do(func() {
		id := d.daemonIdentity()
		d.usb = host.Detect(id, d.usbIDs)
	})
	return d.usb
}

func (d *DevClient) Run(cfg *config.DevConfig, containerName string, binds []string, root string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	platform := cfg.LinuxPlatform()
	hostConfig := &container.HostConfig{
		Binds: binds,
	}
	if cfg.USB {
		applyUSB(hostConfig, d.USB())
	}
	resp, err := d.client.ContainerCreate(d.ctx, dockerClient.ContainerCreateOptions{
		Image: cfg.Image,
		Name:  containerName,
		Config: &container.Config{
			Env: []string{
				fmt.Sprintf("DEV_UID=%s", u.Uid),
				fmt.Sprintf("DEV_GID=%s", u.Gid),
			},
			Labels:     repo.ContainerLabels(root),
			WorkingDir: shared.CONTAINER_WORKSPACE,
		},
		Platform:   &platform,
		HostConfig: hostConfig,
	})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return fmt.Errorf("image %s is not installed locally.\nRun `dev106 pull` first.", cfg.Image)
		}
		if errdefs.IsConflict(err) {
			return fmt.Errorf("container %s already exists.\nUse `dev106` to attach, or `dev106 restart` to recreate it.", containerName)
		}
		return err
	}
	if _, err := d.client.ContainerStart(d.ctx, resp.ID, dockerClient.ContainerStartOptions{}); err != nil {
		return err
	}
	return nil
}

// StaleUSBWarning warns when a running container predates the board.
func (d *DevClient) StaleUSBWarning(containerName string) string {
	u := d.USB()
	result, err := d.client.ContainerInspect(d.ctx, containerName, dockerClient.ContainerInspectOptions{})
	if err != nil || result.Container.HostConfig == nil {
		return ""
	}
	return host.StaleContainerAdvice(u, host.ContainerHasUSBPassthrough(result.Container.HostConfig.Binds))
}
