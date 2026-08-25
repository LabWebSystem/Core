package backend

import (
	"bytes"
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
type StreamingCommandRunner interface {
	RunStreaming(context.Context, string, []string, io.Writer, io.Writer) error
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

func (r OSRunner) RunStreaming(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
	if r.Timeout == 0 {
		r.Timeout = 5 * time.Minute
	}
	streamCtx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	cmd := exec.CommandContext(streamCtx, name, args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd.Run()
}

func runLogged(ctx context.Context, runner CommandRunner, task, name string, args ...string) ([]byte, error) {
	if streaming, ok := runner.(StreamingCommandRunner); ok {
		var stdout, stderr bytes.Buffer
		outWriter := &operationLineWriter{task: task, level: "info", report: func(message string) { reportOperationOutput(ctx, task, message, "info") }}
		errWriter := &operationLineWriter{task: task, level: "error", report: func(message string) { reportOperationOutput(ctx, task, message, "error") }}
		err := streaming.RunStreaming(ctx, name, args, io.MultiWriter(&stdout, outWriter), io.MultiWriter(&stderr, errWriter))
		outWriter.Flush()
		errWriter.Flush()
		return append(stdout.Bytes(), stderr.Bytes()...), err
	}
	out, err := runner.Run(ctx, name, args...)
	if len(out) > 0 {
		reportOperationOutput(ctx, task, strings.TrimSpace(string(out)), "info")
	}
	return out, err
}

type operationLineWriter struct {
	task, level string
	pending     string
	report      func(string)
}

func (w *operationLineWriter) Write(data []byte) (int, error) {
	w.pending += string(data)
	for {
		i := strings.IndexByte(w.pending, '\n')
		if i < 0 {
			break
		}
		w.report(w.pending[:i])
		w.pending = w.pending[i+1:]
	}
	return len(data), nil
}
func (w *operationLineWriter) Flush() {
	if w.pending != "" {
		w.report(w.pending)
		w.pending = ""
	}
}

func CloneAndValidate(ctx context.Context, runner CommandRunner, url, ref, dest string) error {
	tmp := dest + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(filepath.Dir(tmp), 0700); err != nil {
		return err
	}
	reportOperationPhase(ctx, "source_clone", "GitHubリポジトリを取得しています")
	out, err := runLogged(ctx, runner, "git取得", "git", "clone", "--no-tags", "--depth", "1", "--branch", ref, url, tmp)
	if err != nil {
		output := strings.ToLower(string(out))
		if strings.Contains(output, "remote branch") || strings.Contains(output, "remote ref") || strings.Contains(output, "couldn't find") {
			return fmt.Errorf("指定したブランチまたはタグがリポジトリに見つかりません")
		}
		return fmt.Errorf("リポジトリ取得に失敗しました")
	}
	reportOperationPhase(ctx, "source_validate", "取得したファイルを検証しています")
	if err := ValidateSourceTree(tmp); err != nil {
		return err
	}
	reportOperationPhase(ctx, "manifest_validate", "アプリ定義を検証しています")
	data, err := os.ReadFile(filepath.Join(tmp, "lws.manifest.yaml"))
	if err != nil {
		return fmt.Errorf("manifestが見つかりません")
	}
	if _, err = ValidateManifest(data); err != nil {
		return err
	}
	reportOperationPhase(ctx, "compose_validate", "Compose定義を検証しています")
	compose, err := os.ReadFile(filepath.Join(tmp, "compose.yaml"))
	if err != nil {
		return fmt.Errorf("compose.yamlが見つかりません")
	}
	if err = ValidateComposeSource(tmp, compose); err != nil {
		return err
	}
	reportOperationPhase(ctx, "source_activate", "検証済みのアプリ定義を反映しています")
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
