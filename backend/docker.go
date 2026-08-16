package backend

import (
	"context"
	"encoding/json"
	"fmt"
)

type DockerResource struct {
	ID     string
	Name   string
	Labels ResourceLabels
}
type DockerResources struct {
	Runner         CommandRunner
	InstallationID string
}

func NewDockerResources(runner CommandRunner, installationID string) *DockerResources {
	return &DockerResources{Runner: runner, InstallationID: installationID}
}
func (d *DockerResources) inspect(ctx context.Context, kind, name string) (DockerResource, error) {
	out, err := d.Runner.Run(ctx, "docker", kind, "inspect", name)
	if err != nil {
		return DockerResource{}, fmt.Errorf("Docker資源を確認できません")
	}
	var raw []struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Config struct {
			Labels ResourceLabels `json:"Labels"`
		} `json:"Config"`
		Labels ResourceLabels `json:"Labels"`
	}
	if err = json.Unmarshal(out, &raw); err != nil || len(raw) != 1 {
		return DockerResource{}, fmt.Errorf("Docker資源の応答が不正です")
	}
	labels := raw[0].Labels
	if len(labels) == 0 {
		labels = raw[0].Config.Labels
	}
	return DockerResource{ID: raw[0].ID, Name: raw[0].Name, Labels: labels}, nil
}
func (d *DockerResources) EnsureNetwork(ctx context.Context, app string) error {
	name := EdgeNetworkName(app)
	r, err := d.inspect(ctx, "network", name)
	if err == nil {
		return VerifyOwnership(r.Labels, d.InstallationID, app)
	}
	_, err = d.Runner.Run(ctx, "docker", "network", "create", "--label", "com.labwebsystem.owner=lws", "--label", "com.labwebsystem.installation-id="+d.InstallationID, "--label", "com.labwebsystem.app-id="+app, name)
	return err
}
func (d *DockerResources) RemoveNetwork(ctx context.Context, app, name string) error {
	r, err := d.inspect(ctx, "network", name)
	if err != nil {
		return err
	}
	if err = VerifyOwnership(r.Labels, d.InstallationID, app); err != nil {
		return err
	}
	_, err = d.Runner.Run(ctx, "docker", "network", "rm", name)
	return err
}
func (d *DockerResources) RemoveVolume(ctx context.Context, app, name string) error {
	r, err := d.inspect(ctx, "volume", name)
	if err != nil {
		return err
	}
	if err = VerifyOwnership(r.Labels, d.InstallationID, app); err != nil {
		return err
	}
	_, err = d.Runner.Run(ctx, "docker", "volume", "rm", name)
	return err
}
