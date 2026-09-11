package docker

import (
	"fmt"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/junikimm717/dev106/internal/shared"
	dockerClient "github.com/moby/moby/client"
)

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
