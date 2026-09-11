package docker

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/junikimm717/dev106/internal/config"
	dockerClient "github.com/moby/moby/client"
	"github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/term"
)

type pullMessage struct {
	Status   string `json:"status,omitempty"`
	ID       string `json:"id,omitempty"`
	Progress string `json:"progress,omitempty"`
	Error    string `json:"error,omitempty"`
}

type pullDisplay struct {
	out    io.Writer
	tty    bool
	order  []string
	lines  map[string]string
	status map[string]string
	height int
}

func newPullDisplay(out *os.File) *pullDisplay {
	return &pullDisplay{
		out:    out,
		tty:    term.IsTerminal(int(out.Fd())),
		lines:  make(map[string]string),
		status: make(map[string]string),
	}
}

func layerLine(msg pullMessage) string {
	id := shortID(msg.ID)
	if msg.Progress != "" {
		return fmt.Sprintf("%s: %s %s", id, msg.Status, strings.TrimSpace(msg.Progress))
	}
	if msg.Status != "" {
		return fmt.Sprintf("%s: %s", id, msg.Status)
	}
	return id
}

func (d *pullDisplay) handle(msg pullMessage) {
	if msg.ID == "" {
		if msg.Status == "" {
			return
		}
		d.reset()
		fmt.Fprintln(d.out, msg.Status)
		return
	}

	line := layerLine(msg)
	if _, ok := d.lines[msg.ID]; !ok {
		d.order = append(d.order, msg.ID)
	}
	d.lines[msg.ID] = line

	if d.tty {
		d.redraw()
		return
	}

	// Logs/CI: one line per status change, never the per-byte progress ticks.
	if d.status[msg.ID] == msg.Status {
		return
	}
	d.status[msg.ID] = msg.Status
	if msg.Status != "" {
		fmt.Fprintf(d.out, "%s: %s\n", shortID(msg.ID), msg.Status)
	}
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func (d *pullDisplay) redraw() {
	if d.height > 0 {
		fmt.Fprintf(d.out, "\033[%dA", d.height)
	}
	width := 80
	if f, ok := d.out.(*os.File); ok {
		if w, _, err := term.GetSize(int(f.Fd())); err == nil && w > 0 {
			width = w
		}
	}
	for _, id := range d.order {
		line := d.lines[id]
		if len(line) > width {
			line = line[:width]
		}
		fmt.Fprintf(d.out, "\r\033[K%s\n", line)
	}
	d.height = len(d.order)
}

// reset stops the next redraw from cursor-up over lines that are now
// permanent, whether because a non-layer message interrupted the block or
// because the pull is over.
func (d *pullDisplay) reset() {
	d.height = 0
}

func (d *DevClient) Pull(cfg *config.DevConfig) error {
	resp, err := d.client.ImagePull(
		d.ctx,
		cfg.Image,
		dockerClient.ImagePullOptions{
			Platforms: []v1.Platform{cfg.LinuxPlatform()},
		},
	)
	if err != nil {
		return fmt.Errorf("could not pull %s: %w", cfg.Image, err)
	}
	defer resp.Close()

	display := newPullDisplay(os.Stdout)
	if display.tty {
		fmt.Fprint(display.out, "\033[?25l")
		defer fmt.Fprint(display.out, "\033[?25h")
	}

	dec := json.NewDecoder(resp)
	for {
		var msg pullMessage
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if msg.Error != "" {
			display.reset()
			return fmt.Errorf("docker pull failed: %s", msg.Error)
		}
		display.handle(msg)
	}
	display.reset()

	platform := cfg.LinuxPlatform()
	fmt.Printf("Pulled %s (%s/%s)\n", cfg.Image, platform.OS, platform.Architecture)
	return nil
}
