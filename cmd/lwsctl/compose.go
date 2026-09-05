package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func (a *application) compose(arguments ...string) error {
	command, err := a.composeCommand(arguments...)
	if err != nil {
		return err
	}
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command.Run()
}

func (a *application) composeOutput(arguments ...string) ([]byte, error) {
	command, err := a.composeCommand(arguments...)
	if err != nil {
		return nil, err
	}
	return command.Output()
}

func (a *application) dockerOutput(arguments ...string) ([]byte, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, errors.New("必要なコマンドが見つかりません: docker")
	}
	return exec.Command("docker", arguments...).Output()
}

func (a *application) docker(arguments ...string) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return errors.New("必要なコマンドが見つかりません: docker")
	}
	command := exec.Command("docker", arguments...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command.Run()
}

func (a *application) purgeOwnedAppResources() error {
	owner := "label=com.labwebsystem.owner=lws"
	installation := "label=com.labwebsystem.installation-id=" + a.installationID
	app := "label=com.labwebsystem.app-id"

	containers, err := a.dockerOutput("ps", "-aq", "--filter", owner, "--filter", installation, "--filter", app)
	if err != nil {
		return errors.New("LWSアプリcontainer一覧を取得できません")
	}
	if ids := strings.Fields(string(containers)); len(ids) > 0 {
		args := append([]string{"rm", "-f"}, ids...)
		if err := a.docker(args...); err != nil {
			return errors.New("LWSアプリcontainerを削除できません")
		}
	}

	networks, err := a.dockerOutput("network", "ls", "-q", "--filter", owner, "--filter", installation, "--filter", app)
	if err != nil {
		return errors.New("LWSアプリnetwork一覧を取得できません")
	}
	if ids := strings.Fields(string(networks)); len(ids) > 0 {
		args := append([]string{"network", "rm"}, ids...)
		if err := a.docker(args...); err != nil {
			return errors.New("LWSアプリnetworkを削除できません")
		}
	}

	volumes, err := a.dockerOutput("volume", "ls", "-q", "--filter", owner, "--filter", installation, "--filter", app)
	if err != nil {
		return errors.New("LWSアプリvolume一覧を取得できません")
	}
	if ids := strings.Fields(string(volumes)); len(ids) > 0 {
		args := append([]string{"volume", "rm"}, ids...)
		if err := a.docker(args...); err != nil {
			return errors.New("LWSアプリvolumeを削除できません")
		}
	}
	return nil
}

func (a *application) composeCommand(arguments ...string) (*exec.Cmd, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, errors.New("必要なコマンドが見つかりません: docker")
	}
	if _, err := os.Stat(a.paths.composeFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("Composeファイルが見つかりません: %s", a.paths.composeFile)
		}
		return nil, err
	}
	args := append([]string{"compose", "--project-name", a.paths.project, "--file", a.paths.composeFile}, arguments...)
	command := exec.Command("docker", args...)
	command.Env = setEnvironment(os.Environ(), "LWS_VERSION", a.version)
	if a.domain != "" {
		command.Env = setEnvironment(command.Env, "LWS_BASE_DOMAIN", a.domain)
	}
	if a.installationID != "" {
		command.Env = setEnvironment(command.Env, "LWS_INSTALLATION_ID", a.installationID)
	}
	if a.publicAddress != "" {
		command.Env = setEnvironment(command.Env, "LWS_PUBLIC_ADDRESS", a.publicAddress)
	}
	command.Env = setEnvironment(command.Env, "LWS_STATE_DIR", a.paths.stateDir)
	command.Env = setEnvironment(command.Env, "LWS_CADDY_CONTAINER", a.paths.project+"-caddy-1")
	command.Env = setEnvironment(command.Env, "LWS_COREDNS_CONTAINER", a.paths.project+"-coredns-1")
	return command, nil
}

func removeEnvironment(environment []string, name string) []string {
	prefix := name + "="
	result := make([]string, 0, len(environment))
	for _, item := range environment {
		if !strings.HasPrefix(item, prefix) {
			result = append(result, item)
		}
	}
	return result
}

func setEnvironment(environment []string, name, value string) []string {
	return append(removeEnvironment(environment, name), name+"="+value)
}
