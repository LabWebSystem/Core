package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func (a *application) run(args []string) error {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return exitError{code: 2}
	}
	if args[0] == "-h" || args[0] == "--help" {
		printUsage(os.Stdout)
		return nil
	}

	command, options := args[0], args[1:]
	if len(options) == 1 && (options[0] == "-h" || options[0] == "--help") {
		printCommandUsage(os.Stdout, command)
		return nil
	}
	switch command {
	case "start":
		return a.start(options)
	case "stop":
		if len(options) != 0 {
			return fmt.Errorf("stopにはオプションを指定できません")
		}
		return a.stop()
	case "down":
		return a.down(options)
	case "status":
		if len(options) != 0 {
			return fmt.Errorf("statusにはオプションを指定できません")
		}
		return a.status()
	case "rebuild":
		if len(options) != 0 {
			return fmt.Errorf("rebuildにはオプションを指定できません")
		}
		return a.rebuild()
	case "update":
		if len(options) != 0 {
			return fmt.Errorf("updateにはオプションを指定できません")
		}
		return a.update()
	default:
		printUsage(os.Stderr)
		return exitError{code: 2}
	}
}

func (a *application) start(options []string) error {
	requested, force, err := parseStartOptions(options)
	if err != nil {
		return err
	}
	if requested != "" && !domainPattern.MatchString(requested) {
		return fmt.Errorf("ドメインが不正です: %s", requested)
	}

	if err := a.loadConfig(); err == nil {
		current := a.domain
		if requested != "" && requested != current {
			if !force {
				confirmed, confirmErr := confirm(fmt.Sprintf("ベースドメインを%sから%sへ変更し、ルーティング設定を再生成します。続行しますか?", current, requested))
				if confirmErr != nil {
					return confirmErr
				}
				if !confirmed {
					return errors.New("ドメインの変更をキャンセルしました")
				}
			}
			if err := a.writeConfig(requested); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	} else {
		if requested == "" {
			var promptErr error
			requested, promptErr = prompt("ベースドメイン: ")
			if promptErr != nil {
				return promptErr
			}
		}
		if requested == "" {
			return errors.New("ベースドメインが必要です")
		}
		if !domainPattern.MatchString(requested) {
			return fmt.Errorf("ドメインが不正です: %s", requested)
		}
		if err := a.writeConfig(requested); err != nil {
			return err
		}
	}

	if err := a.compose("up", "-d", "--remove-orphans"); err != nil {
		return err
	}
	fmt.Printf("LWSを%s向けに起動しました\n", a.domain)
	return nil
}

func parseStartOptions(options []string) (string, bool, error) {
	var domain string
	force := false
	for len(options) > 0 {
		switch options[0] {
		case "-d", "--domain":
			if len(options) < 2 {
				return "", false, errors.New("--domainには値が必要です")
			}
			domain, options = options[1], options[2:]
		case "-f", "--force":
			force, options = true, options[1:]
		default:
			return "", false, fmt.Errorf("startの不明なオプションです: %s", options[0])
		}
	}
	return domain, force, nil
}

func (a *application) stop() error {
	if _, err := os.Stat(a.paths.composeFile); errors.Is(err, os.ErrNotExist) {
		fmt.Println("LWSはインストールされていません")
		return nil
	} else if err != nil {
		return err
	}
	if err := a.loadConfig(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := a.compose("stop"); err != nil {
		return err
	}
	fmt.Println("LWSを停止しました")
	return nil
}

func (a *application) status() error {
	if err := a.loadConfig(); err == nil {
		fmt.Printf("設定済み: YES\nドメイン: %s\n", a.domain)
	} else if errors.Is(err, os.ErrNotExist) {
		fmt.Println("設定済み: NO")
		fmt.Println("先にlwsctl startを実行してください")
		return nil
	} else {
		return err
	}
	if _, err := os.Stat(a.paths.composeFile); errors.Is(err, os.ErrNotExist) {
		fmt.Println("実行環境: 未インストール")
		return nil
	} else if err != nil {
		return err
	}
	return a.compose("ps")
}

func (a *application) rebuild() error {
	if err := a.loadConfig(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("設定を作成するため、先にstartを実行してください")
		}
		return err
	}
	if err := a.compose("config"); err != nil {
		return err
	}
	if err := a.compose("up", "-d", "--force-recreate", "--remove-orphans"); err != nil {
		return err
	}
	fmt.Println("LWSを再構成しました")
	return nil
}

func (a *application) update() error {
	if err := a.loadConfig(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("設定を作成するため、先にstartを実行してください")
		}
		return err
	}
	if os.Getenv("LWS_SKIP_PACKAGE_UPDATE") != "1" {
		wasRunning, err := a.hasRunningServices()
		if err != nil {
			return err
		}
		if info, err := os.Stat(a.paths.installerPath); err != nil || info.Mode()&0o111 == 0 {
			return fmt.Errorf("インストーラーが見つかりません: %s", a.paths.installerPath)
		}
		command := exec.Command(a.paths.installerPath)
		command.Env = removeEnvironment(os.Environ(), "LWS_VERSION")
		command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := command.Run(); err != nil {
			return err
		}
		executable, err := os.Executable()
		if err != nil {
			return fmt.Errorf("lwsctlを再実行できません: %w", err)
		}
		environment := setEnvironment(os.Environ(), "LWS_SKIP_PACKAGE_UPDATE", "1")
		if wasRunning {
			environment = setEnvironment(environment, "LWS_RESTART_AFTER_UPDATE", "1")
		}
		return syscall.Exec(executable, []string{executable, "update"}, environment)
	}
	if err := a.writeConfig(a.domain); err != nil {
		return err
	}
	if err := a.compose("pull"); err != nil {
		return err
	}
	if os.Getenv("LWS_RESTART_AFTER_UPDATE") == "1" {
		if err := a.compose("up", "-d", "--remove-orphans"); err != nil {
			return err
		}
	}
	fmt.Println("LWSを更新しました")
	return nil
}

func (a *application) hasRunningServices() (bool, error) {
	output, err := a.composeOutput("ps", "--status", "running", "--services")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(output)) != "", nil
}

func (a *application) down(options []string) error {
	purge, force, err := parseDownOptions(options)
	if err != nil {
		return err
	}
	if err := a.loadConfig(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("設定を作成するため、先にstartを実行してください")
		}
		return err
	}
	if purge && !force {
		confirmed, err := confirm("LWSの設定と永続データを削除します。続行しますか?")
		if err != nil {
			return err
		}
		if !confirmed {
			return errors.New("完全削除をキャンセルしました")
		}
	}
	if purge {
		if err := a.stopOwnedContainers(); err != nil {
			return err
		}
	}
	downArgs := []string{"down", "--remove-orphans"}
	if purge {
		downArgs = append(downArgs, "--volumes")
	}
	if err := a.compose(downArgs...); err != nil {
		return err
	}
	if purge {
		if err := a.purgeOwnedAppResources(); err != nil {
			return err
		}
		if err := os.RemoveAll(a.paths.configDir); err != nil {
			return err
		}
		if err := os.RemoveAll(a.paths.stateDir); err != nil {
			return err
		}
	}
	if purge {
		fmt.Println("LWSの実行環境、設定、状態、永続データを削除しました")
		return nil
	}
	fmt.Println("LWSの実行環境を削除しました")
	return nil
}

func (a *application) stopOwnedContainers() error {
	containers, err := a.dockerOutput("ps", "-q", "--filter", "label=com.labwebsystem.owner=lws", "--filter", "label=com.labwebsystem.installation-id="+a.installationID)
	if err != nil {
		return errors.New("LWS管理container一覧を取得できません")
	}
	if ids := strings.Fields(string(containers)); len(ids) > 0 {
		args := append([]string{"stop"}, ids...)
		if err := a.docker(args...); err != nil {
			return errors.New("LWS管理containerを停止できません")
		}
	}
	return nil
}

func parseDownOptions(options []string) (bool, bool, error) {
	purge, force := false, false
	for len(options) > 0 {
		switch options[0] {
		case "--purge":
			purge, options = true, options[1:]
		case "-f", "--force":
			force, options = true, options[1:]
		default:
			return false, false, fmt.Errorf("downの不明なオプションです: %s", options[0])
		}
	}
	return purge, force, nil
}
