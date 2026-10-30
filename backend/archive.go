package backend

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	MaxArchiveBytes    = 100 << 20
	MaxArchiveExpanded = 512 << 20
	MaxArchiveFiles    = 10000
)

// ExtractArchiveは空の隔離ディレクトリへZIPを展開し、アプリルートを返す。
func ExtractArchive(reader io.ReaderAt, size int64, dest string) (string, error) {
	if size < 1 || size > MaxArchiveBytes {
		return "", fmt.Errorf("ZIPのサイズが上限を超えているか空です")
	}
	archive, err := zip.NewReader(reader, size)
	if err != nil {
		return "", fmt.Errorf("ZIPを読み取れません")
	}
	if len(archive.File) > MaxArchiveFiles {
		return "", fmt.Errorf("ZIPのファイル数が上限を超えています")
	}
	existing, err := os.ReadDir(dest)
	if err != nil || len(existing) != 0 {
		return "", fmt.Errorf("ZIPの展開先は空のディレクトリである必要があります")
	}
	seen := make(map[string]bool, len(archive.File))
	var expanded uint64
	for _, entry := range archive.File {
		if entry.Name == "./" {
			if seen["."] || entry.Mode().Type() != os.ModeDir || entry.UncompressedSize64 != 0 {
				return "", fmt.Errorf("ZIPに重複または不正なパスがあります")
			}
			seen["."] = true
			continue
		}
		name := strings.TrimSuffix(entry.Name, "/")
		for strings.HasPrefix(name, "./") {
			name = strings.TrimPrefix(name, "./")
		}
		if name == "" || strings.ContainsAny(name, "\\\x00") || strings.Contains(name, ":") || strings.HasPrefix(name, "/") || path.Clean(name) != name {
			return "", fmt.Errorf("ZIPに不正なパスがあります")
		}
		for _, part := range strings.Split(name, "/") {
			if part == "." || part == ".." || part == "" {
				return "", fmt.Errorf("ZIPに不正なパスがあります")
			}
		}
		if seen[name] {
			return "", fmt.Errorf("ZIPに重複するパスがあります")
		}
		seen[name] = true
		mode := entry.Mode()
		if mode.Type() != 0 && mode.Type() != os.ModeDir {
			return "", fmt.Errorf("ZIPに通常ファイル以外が含まれています")
		}
		if mode.IsDir() && entry.UncompressedSize64 != 0 {
			return "", fmt.Errorf("ZIPのディレクトリに不正なデータがあります")
		}
		if entry.UncompressedSize64 > MaxArchiveExpanded-expanded {
			return "", fmt.Errorf("ZIPの展開サイズが上限を超えています")
		}
		expanded += entry.UncompressedSize64
		target := filepath.Join(dest, filepath.FromSlash(name))
		if mode.IsDir() {
			if err := os.MkdirAll(target, 0700); err != nil {
				return "", fmt.Errorf("ZIPのディレクトリを作成できません")
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return "", fmt.Errorf("ZIPのディレクトリを作成できません")
		}
		input, err := entry.Open()
		if err != nil {
			return "", fmt.Errorf("ZIPエントリを開けません")
		}
		permissions := os.FileMode(0600)
		if mode.Perm()&0111 != 0 {
			permissions = 0700
		}
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, permissions)
		if err != nil {
			_ = input.Close()
			return "", fmt.Errorf("ZIPエントリを作成できません")
		}
		n, copyErr := io.Copy(output, io.LimitReader(input, int64(entry.UncompressedSize64)+1))
		inputErr := input.Close()
		outputErr := output.Close()
		if copyErr != nil || inputErr != nil || outputErr != nil || n != int64(entry.UncompressedSize64) {
			return "", fmt.Errorf("ZIPエントリを展開できません")
		}
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		return "", fmt.Errorf("ZIPの内容を読み取れません")
	}
	root := dest
	if len(entries) == 1 && entries[0].IsDir() {
		root = filepath.Join(dest, entries[0].Name())
	}
	if err := ValidateSourceTree(root); err != nil {
		return "", err
	}
	return root, nil
}
