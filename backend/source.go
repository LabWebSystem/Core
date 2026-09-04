package backend

import (
	"bytes"
	"context"
	"encoding/json"
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
		outWriter := &operationLineWriter{task: task, level: func(string) string { return "info" }, report: func(message, level string) { reportOperationOutput(ctx, task, message, level) }}
		errWriter := &operationLineWriter{task: task, level: func(message string) string { return classifyLogLevel(message, "info") }, report: func(message, level string) { reportOperationOutput(ctx, task, message, level) }}
		err := streaming.RunStreaming(ctx, name, args, io.MultiWriter(&stdout, outWriter), io.MultiWriter(&stderr, errWriter))
		outWriter.Flush()
		errWriter.Flush()
		if err != nil {
			reportOperationOutput(ctx, task, "外部コマンドが失敗しました", "error")
		}
		return append(stdout.Bytes(), stderr.Bytes()...), err
	}
	out, err := runner.Run(ctx, name, args...)
	if len(out) > 0 {
		reportOperationOutput(ctx, task, strings.TrimSpace(string(out)), "info")
	}
	if err != nil {
		reportOperationOutput(ctx, task, "外部コマンドが失敗しました", "error")
	}
	return out, err
}

// runLoggedJSONは、構造化された外部コマンド出力を1レコードの未整形JSONとして記録する。
// Compose検証のJSONは行単位で記録すると、Dashboardで追跡しづらくなるためである。
func runLoggedJSON(ctx context.Context, runner CommandRunner, task, name string, args ...string) ([]byte, error) {
	out, err := runner.Run(ctx, name, args...)
	if len(out) > 0 {
		message := strings.TrimSpace(string(out))
		var compact bytes.Buffer
		if json.Compact(&compact, bytes.TrimSpace(out)) == nil {
			message = compact.String()
		}
		reportOperationOutput(ctx, task, message, "info")
	}
	if err != nil {
		reportOperationOutput(ctx, task, "外部コマンドが失敗しました", "error")
	}
	return out, err
}

type operationLineWriter struct {
	task    string
	level   func(string) string
	pending string
	report  func(string, string)
}

func (w *operationLineWriter) Write(data []byte) (int, error) {
	w.pending += string(data)
	for {
		i := strings.IndexByte(w.pending, '\n')
		if i < 0 {
			break
		}
		message := w.pending[:i]
		w.report(message, w.level(message))
		w.pending = w.pending[i+1:]
	}
	return len(data), nil
}
func (w *operationLineWriter) Flush() {
	if w.pending != "" {
		w.report(w.pending, w.level(w.pending))
		w.pending = ""
	}
}

func classifyLogLevel(message, fallback string) string {
	value := strings.ToLower(message)
	switch {
	case strings.Contains(value, "[emerg]") || strings.Contains(value, "[alert]") || strings.Contains(value, "[crit]") || strings.Contains(value, "[error]"):
		return "error"
	case strings.Contains(value, "[warn]") || strings.Contains(value, "[warning]"):
		return "warn"
	case strings.Contains(value, "[debug]"):
		return "debug"
	case strings.Contains(value, "[info]") || strings.Contains(value, "[notice]"):
		return "info"
	default:
		return fallback
	}
}

var composeFilePriority = []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}

func CloneAndValidate(ctx context.Context, runner CommandRunner, url, ref, dest string) error {
	_, err := CloneAndValidateWithCompose(ctx, runner, url, ref, "", dest)
	return err
}

func CloneAndValidateWithCompose(ctx context.Context, runner CommandRunner, url, ref, requestedCompose, dest string) (string, error) {
	url, ref = RepositoryURLAndRef(url, ref)
	tmp := dest + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(filepath.Dir(tmp), 0700); err != nil {
		return "", err
	}
	reportOperationPhase(ctx, "source_clone", "GitHubリポジトリを取得しています")
	out, err := runLogged(ctx, runner, "git取得", "git", "clone", "--no-tags", "--depth", "1", "--branch", ref, url, tmp)
	if err != nil {
		output := strings.ToLower(string(out))
		if strings.Contains(output, "remote branch") || strings.Contains(output, "remote ref") || strings.Contains(output, "couldn't find") {
			return "", fmt.Errorf("指定したブランチまたはタグがリポジトリに見つかりません")
		}
		return "", fmt.Errorf("リポジトリ取得に失敗しました")
	}
	reportOperationPhase(ctx, "source_validate", "取得したファイルを検証しています")
	if err := ValidateSourceTree(tmp); err != nil {
		return "", err
	}
	reportOperationPhase(ctx, "manifest_validate", "アプリ定義を検証しています")
	data, err := os.ReadFile(filepath.Join(tmp, "lws.manifest.yaml"))
	if err != nil {
		return "", fmt.Errorf("manifestが見つかりません")
	}
	if _, err = ValidateManifest(data); err != nil {
		return "", err
	}
	reportOperationPhase(ctx, "compose_validate", "Compose定義を検証しています")
	composeFile := requestedCompose
	if composeFile == "" {
		for _, candidate := range composeFilePriority {
			if _, e := os.Stat(filepath.Join(tmp, candidate)); e == nil {
				composeFile = candidate
				break
			}
		}
	}
	if !containsComposeFile(composeFile) {
		return "", fmt.Errorf("使用できるComposeファイルが見つかりません")
	}
	compose, err := os.ReadFile(filepath.Join(tmp, composeFile))
	if err != nil {
		return "", fmt.Errorf("指定したComposeファイルが見つかりません")
	}
	if err = ValidateComposeSource(tmp, compose); err != nil {
		return "", err
	}
	reportOperationPhase(ctx, "source_activate", "検証済みのアプリ定義を反映しています")
	old := dest + ".old"
	_ = os.RemoveAll(old)
	if err = os.Rename(dest, old); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err = os.Rename(tmp, dest); err != nil {
		_ = os.Rename(old, dest)
		return "", err
	}
	return composeFile, nil
}

func containsComposeFile(value string) bool {
	for _, candidate := range composeFilePriority {
		if value == candidate {
			return true
		}
	}
	return false
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
