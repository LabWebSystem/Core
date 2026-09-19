package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type dockerNotFoundError struct{ message string }

func (e *dockerNotFoundError) Error() string { return e.message }

type DockerResource struct {
	ID     string
	Name   string
	Labels ResourceLabels
}

type DockerResources struct {
	Runner         CommandRunner
	InstallationID string
	CaddyContainer string
}

func NewDockerResources(runner CommandRunner, installationID string) *DockerResources {
	return &DockerResources{Runner: runner, InstallationID: installationID, CaddyContainer: "lws-caddy-1"}
}

func (d *DockerResources) EnsureCaddyConnected(ctx context.Context, app string) error {
	if d.CaddyContainer == "" {
		return fmt.Errorf("Caddy containerが設定されていません")
	}
	out, err := d.Runner.Run(ctx, "docker", "network", "connect", "--alias", "lws-"+app, EdgeNetworkName(app), d.CaddyContainer)
	message := strings.ToLower(string(out) + "\n" + errString(err))
	if err != nil && !strings.Contains(message, "already connected") && !strings.Contains(message, "already exists") {
		return fmt.Errorf("Caddyをedge networkへ接続できません")
	}
	return nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (d *DockerResources) DisconnectCaddy(ctx context.Context, app string) error {
	if d.CaddyContainer == "" {
		return fmt.Errorf("Caddy containerが設定されていません")
	}
	out, err := d.Runner.Run(ctx, "docker", "network", "disconnect", "-f", EdgeNetworkName(app), d.CaddyContainer)
	message := strings.ToLower(string(out) + "\n" + errString(err))
	if err != nil && !strings.Contains(message, "not connected") && !strings.Contains(message, "not found") && !strings.Contains(message, "no such network") {
		return fmt.Errorf("Caddyをedge networkから切断できません")
	}
	return nil
}

func (d *DockerResources) ValidateCaddyfile(ctx context.Context) error {
	if d.CaddyContainer == "" {
		return fmt.Errorf("Caddy containerが設定されていません")
	}
	if _, err := d.Runner.Run(ctx, "docker", "exec", d.CaddyContainer, "caddy", "validate", "--config", "/var/lib/lws/generated/Caddyfile", "--adapter", "caddyfile"); err != nil {
		return fmt.Errorf("Caddyfileの検証に失敗しました")
	}
	return nil
}

func (d *DockerResources) VerifyInfrastructureContainer(ctx context.Context, container string) error {
	resource, err := d.inspect(ctx, "container", container)
	if err != nil {
		return err
	}
	if resource.Labels["com.labwebsystem.owner"] != "lws" || resource.Labels["com.labwebsystem.installation-id"] != d.InstallationID {
		return fmt.Errorf("LWS所有確認に失敗しました")
	}
	return nil
}

func (d *DockerResources) ReloadCaddy(ctx context.Context) error {
	if d.CaddyContainer == "" {
		return fmt.Errorf("Caddy containerが設定されていません")
	}
	if _, err := d.Runner.Run(ctx, "docker", "exec", d.CaddyContainer, "caddy", "reload", "--config", "/var/lib/lws/generated/Caddyfile", "--adapter", "caddyfile", "--address", "localhost:2019"); err != nil {
		return fmt.Errorf("Caddyを再読込できません")
	}
	return nil
}

func (d *DockerResources) ReloadCoreDNS(ctx context.Context, container string) error {
	if container == "" {
		return fmt.Errorf("CoreDNS containerが設定されていません")
	}
	if _, err := d.Runner.Run(ctx, "docker", "kill", "--signal", "HUP", container); err != nil {
		return fmt.Errorf("CoreDNSを再読込できません")
	}
	return nil
}

func (d *DockerResources) RemoveOwnedVolumes(ctx context.Context, app string) error {
	out, err := d.Runner.Run(ctx, "docker", "volume", "ls", "--filter", "label=com.labwebsystem.owner=lws", "--filter", "label=com.labwebsystem.installation-id="+d.InstallationID, "--filter", "label=com.labwebsystem.app-id="+app, "--format", "{{.Name}}")
	if err != nil {
		return fmt.Errorf("アプリvolume一覧を取得できません")
	}
	for _, name := range strings.Fields(string(out)) {
		if err := d.RemoveVolume(ctx, app, name); err != nil {
			return err
		}
	}
	return nil
}

func (d *DockerResources) VerifyProjectOwnership(ctx context.Context, app string) error {
	out, err := d.Runner.Run(ctx, "docker", "ps", "-a", "--filter", "label=com.docker.compose.project="+ProjectName(app), "--format", "{{.ID}}")
	if err != nil {
		return fmt.Errorf("アプリcontainer一覧を取得できません")
	}
	for _, id := range strings.Fields(string(out)) {
		resource, err := d.inspect(ctx, "container", id)
		if err != nil {
			return err
		}
		if err := VerifyOwnership(resource.Labels, d.InstallationID, app); err != nil {
			return err
		}
	}
	return nil
}
func (d *DockerResources) inspect(ctx context.Context, kind, name string) (DockerResource, error) {
	out, err := d.Runner.Run(ctx, "docker", kind, "inspect", name)
	if err != nil {
		message := strings.ToLower(string(out) + "\n" + err.Error())
		if strings.Contains(message, "no such") || strings.Contains(message, "not found") {
			return DockerResource{}, &dockerNotFoundError{message: "Docker資源が見つかりません"}
		}
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
	var notFound *dockerNotFoundError
	if !errors.As(err, &notFound) {
		return err
	}
	_, err = d.Runner.Run(ctx, "docker", "network", "create", "--label", "com.labwebsystem.owner=lws", "--label", "com.labwebsystem.installation-id="+d.InstallationID, "--label", "com.labwebsystem.app-id="+app, name)
	return err
}
func (d *DockerResources) RemoveNetwork(ctx context.Context, app, name string) error {
	r, err := d.inspect(ctx, "network", name)
	if err != nil {
		var notFound *dockerNotFoundError
		if errors.As(err, &notFound) {
			return nil
		}
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

func (d *DockerResources) RemoveOwnedVolume(ctx context.Context, name string) error {
	r, err := d.inspect(ctx, "volume", name)
	if err != nil {
		return err
	}
	if r.Labels["com.labwebsystem.owner"] != "lws" || r.Labels["com.labwebsystem.installation-id"] != d.InstallationID || r.Labels["com.labwebsystem.app-id"] == "" {
		return fmt.Errorf("LWS所有確認に失敗しました")
	}
	_, err = d.Runner.Run(ctx, "docker", "volume", "rm", name)
	return err
}

// VolumeInUseは、停止済みコンテナを含めてVolumeの接続状態を確認します。
func (d *DockerResources) VolumeInUse(ctx context.Context, name string) (bool, error) {
	out, err := d.Runner.Run(ctx, "docker", "ps", "-a", "--filter", "volume="+name, "--format", "{{.ID}}")
	if err != nil {
		return false, fmt.Errorf("Volumeの使用状況を確認できません")
	}
	return len(strings.Fields(string(out))) > 0, nil
}
