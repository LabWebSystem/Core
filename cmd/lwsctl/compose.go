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
