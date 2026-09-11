package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"os/user"
	"syscall"

	"github.com/junikimm717/dev106/internal/shared"
	"github.com/moby/moby/api/pkg/stdcopy"
	dockerClient "github.com/moby/moby/client"
	"golang.org/x/term"
)

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
			WorkingDir:   shared.CONTAINER_WORKSPACE,
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
			WorkingDir:   shared.CONTAINER_WORKSPACE,
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
