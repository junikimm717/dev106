package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"os/user"
	"strings"
	"syscall"

	"github.com/containerd/errdefs"
	"github.com/junikimm717/dev106/internal/shared"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	dockerClient "github.com/moby/moby/client"
	"golang.org/x/term"
)

type DevClient struct {
	client *dockerClient.Client
	ctx    context.Context
}

func NewClient(ctx context.Context) (*DevClient, error) {
	// Docker Desktop rewrites bind mount paths in an API proxy behind the
	// distro's unix socket; tcp:// skips that proxy and the bind silently
	// degrades to an empty volume rather than failing.
	if host := os.Getenv("DOCKER_HOST"); strings.HasPrefix(host, "tcp://") && IsWSL() {
		fmt.Fprintf(os.Stderr, `warning: DOCKER_HOST is %s

  On WSL, connecting over TCP bypasses Docker Desktop's bind mount
  translation, so /workspace would come up empty instead of erroring.
  Unless you mean to use a remote daemon, unset it:
      unset DOCKER_HOST

`, host)
	}

	client, err := dockerClient.New(dockerClient.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("could not connect to Docker; is the daemon running?\n%w", err)
	}
	return &DevClient{
		client: client,
		ctx:    ctx,
	}, nil
}

func (d *DevClient) Run(config *DevConfig, containerName string, binds []string, root string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	platform := config.linuxPlatform()
	resp, err := d.client.ContainerCreate(d.ctx, dockerClient.ContainerCreateOptions{
		Image: config.Image,
		Name:  containerName,
		Config: &container.Config{
			Env: []string{
				fmt.Sprintf("DEV_UID=%s", u.Uid),
				fmt.Sprintf("DEV_GID=%s", u.Gid),
			},
			Labels: ContainerLabels(root),
		},
		Platform: &platform,
		HostConfig: &container.HostConfig{
			Binds: binds,
		},
	})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return fmt.Errorf("image %s is not installed locally.\nRun `dev106 pull` first.", config.Image)
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

func resizeExecTTY(client *dockerClient.Client, ctx context.Context, execID string, fd int) error {
	width, height, err := term.GetSize(fd)
	if err != nil {
		return err
	}

	_, err = client.ExecResize(ctx, execID, dockerClient.ExecResizeOptions{
		Height: uint(height),
		Width:  uint(width),
	})

	return err
}

func (d *DevClient) Exec(containerName string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}

	userSpec := fmt.Sprintf("%s:%s", u.Uid, u.Gid)

	execResp, err := d.client.ExecCreate(
		d.ctx,
		containerName,
		dockerClient.ExecCreateOptions{
			User:         userSpec,
			Cmd:          []string{"/bin/bash", "-l"},
			TTY:          true,
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
		},
	)
	if err != nil {
		return err
	}

	attachResp, err := d.client.ExecAttach(
		d.ctx,
		execResp.ID,
		dockerClient.ExecAttachOptions{
			TTY: true,
		},
	)
	if err != nil {
		return err
	}
	defer attachResp.Close()

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("dev106 shell needs an interactive terminal (stdin is not a TTY)")
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("dev106 shell needs an interactive terminal: %w", err)
	}

	// terminal resizing
	defer term.Restore(fd, oldState)

	resizeCh := make(chan os.Signal, 1)
	signal.Notify(resizeCh, syscall.SIGWINCH)
	defer signal.Stop(resizeCh)

	// initial resize
	_ = resizeExecTTY(d.client, d.ctx, execResp.ID, fd)

	// dynamically handle window resizes
	go func() {
		for range resizeCh {
			_ = resizeExecTTY(d.client, d.ctx, execResp.ID, fd)
		}
	}()

	// pipe stdin → container
	go func() {
		_, _ = io.Copy(attachResp.Conn, os.Stdin)
	}()

	// pipe container → stdout
	_, copyErr := io.Copy(os.Stdout, attachResp.Reader)

	inspectResp, err := d.client.ExecInspect(d.ctx, execResp.ID, dockerClient.ExecInspectOptions{})
	if err != nil {
		if copyErr != nil {
			return copyErr
		}
		return err
	}
	if inspectResp.ExitCode != 0 {
		return &ExitError{Code: inspectResp.ExitCode}
	}
	return copyErr
}

func (d *DevClient) ExecCmd(containerName string, cmd []string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}

	userSpec := fmt.Sprintf("%s:%s", u.Uid, u.Gid)

	execResp, err := d.client.ExecCreate(
		d.ctx,
		containerName,
		dockerClient.ExecCreateOptions{
			User:         userSpec,
			Cmd:          cmd,
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
		},
	)
	if err != nil {
		return err
	}

	attachResp, err := d.client.ExecAttach(
		d.ctx,
		execResp.ID,
		dockerClient.ExecAttachOptions{},
	)
	if err != nil {
		return err
	}
	defer attachResp.Close()

	go func() {
		_, _ = io.Copy(attachResp.Conn, os.Stdin)
		_ = attachResp.CloseWrite()
	}()

	if _, err := stdcopy.StdCopy(os.Stdout, os.Stderr, attachResp.Reader); err != nil {
		return err
	}

	inspectResp, err := d.client.ExecInspect(d.ctx, execResp.ID, dockerClient.ExecInspectOptions{})
	if err != nil {
		return err
	}
	if inspectResp.ExitCode != 0 {
		return &ExitError{Code: inspectResp.ExitCode}
	}

	return nil
}

func (d *DevClient) Delete(containerName string) (bool, error) {
	_, err := d.client.ContainerRemove(
		d.ctx,
		containerName,
		dockerClient.ContainerRemoveOptions{
			Force: true, // kill if running
		},
	)
	if errdefs.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (d *DevClient) ContainerExists(containerName string) (bool, error) {
	result, err := d.client.ContainerInspect(d.ctx, containerName, dockerClient.ContainerInspectOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if !result.Container.State.Running {
		fmt.Printf("Removing stopped container %s\n", containerName)
		if _, err := d.Delete(containerName); err != nil {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

type ManagedContainer struct {
	Name   string
	Image  string
	Status string
	Root   string
}

func managedLabelFilter() dockerClient.Filters {
	return make(dockerClient.Filters).Add("label", shared.LabelManaged+"=true")
}

func (d *DevClient) ListManaged() ([]ManagedContainer, error) {
	result, err := d.client.ContainerList(d.ctx, dockerClient.ContainerListOptions{
		All:     true,
		Filters: managedLabelFilter(),
	})
	if err != nil {
		return nil, err
	}

	out := make([]ManagedContainer, 0, len(result.Items))
	for _, item := range result.Items {
		name := ""
		if len(item.Names) > 0 {
			name = strings.TrimPrefix(item.Names[0], "/")
		}
		root := ""
		if item.Labels != nil {
			root = item.Labels[shared.LabelRoot]
		}
		out = append(out, ManagedContainer{
			Name:   name,
			Image:  item.Image,
			Status: item.Status,
			Root:   root,
		})
	}
	return out, nil
}

func (d *DevClient) NukeManaged() ([]string, error) {
	items, err := d.ListManaged()
	if err != nil {
		return nil, err
	}
	removed := make([]string, 0, len(items))
	for _, item := range items {
		ok, err := d.Delete(item.Name)
		if err != nil {
			return removed, err
		}
		if ok {
			removed = append(removed, item.Name)
		}
	}
	return removed, nil
}
