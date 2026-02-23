package cli

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"os/user"
	"syscall"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	dockerClient "github.com/moby/moby/client"
	"github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/term"
)

type DevClient struct {
	client *dockerClient.Client
	ctx    context.Context
}

func NewClient(ctx context.Context) *DevClient {
	client, err := dockerClient.New(dockerClient.FromEnv)
	if err != nil {
		log.Fatalln("Could not connect to docker client!", err)
	}
	return &DevClient{
		client: client,
		ctx:    ctx,
	}
}

func (d *DevClient) Run(config *DevConfig, containerName string, binds []string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	resp, err := d.client.ContainerCreate(d.ctx, dockerClient.ContainerCreateOptions{
		Image: config.Image,
		Name:  containerName,
		Config: &container.Config{
			Env: []string{
				fmt.Sprintf("DEV_UID=%s", u.Uid),
				fmt.Sprintf("DEV_GID=%s", u.Gid),
			},
		},
		Platform: &v1.Platform{
			Architecture: "amd64",
			OS:           "linux",
		},
		HostConfig: &container.HostConfig{
			Binds: binds,
		},
	})
	if err != nil {
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
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return err
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
	_, err = io.Copy(os.Stdout, attachResp.Reader)

	return err
}

func (d *DevClient) Delete(containerName string) error {
	_, err := d.client.ContainerRemove(
		d.ctx,
		containerName,
		dockerClient.ContainerRemoveOptions{
			Force: true, // kill if running
		},
	)
	if errdefs.IsNotFound(err) {
		return nil
	}
	return err
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
		d.Delete(containerName)
		return false, nil
	}
	return true, nil
}
