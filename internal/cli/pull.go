package cli

import (
	"encoding/json"
	"fmt"
	"io"

	dockerClient "github.com/moby/moby/client"
	"github.com/opencontainers/image-spec/specs-go/v1"
)

type pullMessage struct {
	Status   string `json:"status,omitempty"`
	ID       string `json:"id,omitempty"`
	Progress string `json:"progress,omitempty"`
	Error    string `json:"error,omitempty"`
}

func (d *DevClient) Pull(config *DevConfig) error {
	resp, err := d.client.ImagePull(
		d.ctx,
		config.Image,
		dockerClient.ImagePullOptions{
			Platforms: []v1.Platform{
				{OS: "linux", Architecture: "amd64"},
			},
		},
	)
	if err != nil {
		return err
	}
	defer resp.Close()

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
			return fmt.Errorf("docker pull failed: %s", msg.Error)
		}

		// Match docker CLI's non-TTY behavior
		switch {
		case msg.ID != "" && msg.Progress != "":
			fmt.Printf("%s: %s %s\n", msg.ID, msg.Status, msg.Progress)
		case msg.ID != "":
			fmt.Printf("%s: %s\n", msg.ID, msg.Status)
		case msg.Status != "":
			fmt.Println(msg.Status)
		}
	}

	return nil
}
