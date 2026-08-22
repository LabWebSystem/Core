package backend

import (
	"bufio"
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

// TailLogsは所有確認済みのCompose projectだけをargv形式で追跡する。
// channelをboundedにして、HTTP購読者の速度をDocker側へ伝播させない。
func (d *DockerResources) TailLogs(ctx context.Context, app, envFile, composeFile, overrideFile string, redactions []string) (<-chan string, error) {
	if err := d.VerifyProjectOwnership(ctx, app); err != nil {
		return nil, err
	}
	streamer, ok := d.Runner.(StreamRunner)
	if !ok {
		return nil, fmt.Errorf("コンテナログ取得器を利用できません")
	}
	args := []string{"compose", "--project-name", ProjectName(app), "--env-file", envFile, "-f", composeFile, "-f", overrideFile, "logs", "--follow", "--no-color", "--tail", "100"}
	stdout, err := streamer.Stream(ctx, "docker", args...)
	if err != nil {
		return nil, fmt.Errorf("コンテナログを開始できません")
	}
	lines := make(chan string, 32)
	go func() {
		defer close(lines)
		defer stdout.Close()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			for _, secret := range redactions {
				if secret != "" {
					line = strings.ReplaceAll(line, secret, "[REDACTED]")
				}
			}
			select {
			case lines <- line:
			case <-ctx.Done():
				return
			}
		}
	}()
	return lines, nil
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
	_, err := d.Runner.Run(ctx, "docker", "network", "connect", "--alias", "lws-"+app, EdgeNetworkName(app), d.CaddyContainer)
	message := strings.ToLower(errString(err))
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
	_, err := d.Runner.Run(ctx, "docker", "network", "disconnect", "-f", EdgeNetworkName(app), d.CaddyContainer)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "not connected") {
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
