package backend

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}
type StreamRunner interface {
	Stream(context.Context, string, ...string) (io.ReadCloser, error)
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
	// Dockerは資源未存在などの判定情報を標準エラーへ出力する。
	// 呼び出し側が安全に状態を分類できるよう、失敗時も出力を回収する。
	return cmd.CombinedOutput()
}

func (r OSRunner) Stream(ctx context.Context, name string, args ...string) (io.ReadCloser, error) {
	if r.Timeout == 0 {
		r.Timeout = 5 * time.Minute
	}
	streamCtx, cancel := context.WithTimeout(ctx, r.Timeout)
	cmd := exec.CommandContext(streamCtx, name, args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	return &waitReadCloser{ReadCloser: stdout, wait: func() error {
		err := cmd.Wait()
		cancel()
		return err
	}}, nil
}

type waitReadCloser struct {
	io.ReadCloser
	wait func() error
}

func (r *waitReadCloser) Close() error {
	closeErr := r.ReadCloser.Close()
	if err := r.wait(); closeErr == nil {
		closeErr = err
	}
	return closeErr
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
	if err := ValidateSourceTree(tmp); err != nil {
		return err
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
	return nil
}

func FinalizeSourceSwap(dest string) error {
	return os.RemoveAll(dest + ".old")
}

func RestoreSourceSwap(dest string) error {
	old := dest + ".old"
	if _, err := os.Stat(old); err != nil {
		if os.IsNotExist(err) {
			return os.RemoveAll(dest)
		}
		return err
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	return os.Rename(old, dest)
}

func ValidateSourceTree(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return NewValidationError("source", "source treeにsymlinkは許可されていません", "SYMLINK_FORBIDDEN")
		}
		if entry.Name() == ".env" {
			return NewValidationError("source", "sourceの.envは使用できません", "EXTERNAL_ENVIRONMENT_FORBIDDEN")
		}
		return nil
	})
}
func ValidateRef(ref string) error {
	if strings.TrimSpace(ref) == "" || strings.ContainsAny(ref, "\x00\n\r") {
		return fmt.Errorf("refが不正です")
	}
	return nil
}
