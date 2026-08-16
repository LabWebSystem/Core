package backend

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}
type OSRunner struct{ Timeout time.Duration }

func (r OSRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if r.Timeout == 0 {
		r.Timeout = 5 * time.Minute
	}
	c, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	cmd := exec.CommandContext(c, name, args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	return cmd.Output()
}
func CloneAndValidate(ctx context.Context, runner CommandRunner, url, ref, dest string) error {
	tmp := dest + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(filepath.Dir(tmp), 0700); err != nil {
		return err
	}
	if _, err := runner.Run(ctx, "git", "clone", "--no-tags", "--depth", "1", "--branch", ref, url, tmp); err != nil {
		return fmt.Errorf("リポジトリ取得に失敗しました")
	}
	data, err := os.ReadFile(filepath.Join(tmp, "lws.manifest.yaml"))
	if err != nil {
		return fmt.Errorf("manifestが見つかりません")
	}
	if _, err = ValidateManifest(data); err != nil {
		return err
	}
	compose, err := os.ReadFile(filepath.Join(tmp, "compose.yaml"))
	if err != nil {
		return fmt.Errorf("compose.yamlが見つかりません")
	}
	if err = ValidateComposeSource(tmp, compose); err != nil {
		return err
	}
	old := dest + ".old"
	_ = os.RemoveAll(old)
	if err = os.Rename(dest, old); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err = os.Rename(tmp, dest); err != nil {
		_ = os.Rename(old, dest)
		return err
	}
	_ = os.RemoveAll(old)
	return nil
}
func ValidateRef(ref string) error {
	if strings.TrimSpace(ref) == "" || strings.ContainsAny(ref, "\x00\n\r") {
		return fmt.Errorf("refが不正です")
	}
	return nil
}
