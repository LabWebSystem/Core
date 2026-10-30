package backend

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type archiveEntry struct {
	name string
	body string
	mode os.FileMode
}

func testArchive(t *testing.T, entries ...archiveEntry) []byte {
	t.Helper()
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		file, err := w.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestExtractArchiveRoots(t *testing.T) {
	for _, prefix := range []string{"", "product/", "./"} {
		t.Run(prefix, func(t *testing.T) {
			data := testArchive(t, archiveEntry{name: prefix + "compose.yaml", body: "services: {}"})
			dir := t.TempDir()
			root, err := ExtractArchive(bytes.NewReader(data), int64(len(data)), dir)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(root, "compose.yaml")); err != nil {
				t.Fatal(err)
			}
		})
	}
	data := testArchive(t, archiveEntry{name: "./", mode: os.ModeDir | 0700}, archiveEntry{name: "./compose.yaml", body: "services: {}"})
	if _, err := ExtractArchive(bytes.NewReader(data), int64(len(data)), t.TempDir()); err != nil {
		t.Fatalf("ZIPのrootディレクトリを受理できません: %v", err)
	}
}

func TestExtractArchiveRejectsUnsafeEntries(t *testing.T) {
	for _, entry := range []archiveEntry{
		{name: "../escape"},
		{name: "/absolute"},
		{name: "C:/escape"},
		{name: "dir/../../escape"},
		{name: "dir\\escape"},
		{name: "link", body: "../outside", mode: os.ModeSymlink | 0777},
		{name: "pipe", mode: os.ModeNamedPipe | 0600},
	} {
		t.Run(strings.ReplaceAll(entry.name, "/", "_"), func(t *testing.T) {
			data := testArchive(t, entry)
			if _, err := ExtractArchive(bytes.NewReader(data), int64(len(data)), t.TempDir()); err == nil {
				t.Fatalf("危険なZIPエントリを受理しました: %q", entry.name)
			}
		})
	}
	data := testArchive(t, archiveEntry{name: "compose.yaml"}, archiveEntry{name: "compose.yaml"})
	if _, err := ExtractArchive(bytes.NewReader(data), int64(len(data)), t.TempDir()); err == nil {
		t.Fatal("重複パスを受理しました")
	}
	data = testArchive(t, archiveEntry{name: "./compose.yaml"}, archiveEntry{name: "compose.yaml"})
	if _, err := ExtractArchive(bytes.NewReader(data), int64(len(data)), t.TempDir()); err == nil {
		t.Fatal("同一パスの異なる表記を受理しました")
	}
}

func TestExtractArchiveLimits(t *testing.T) {
	data := testArchive(t, archiveEntry{name: "compose.yaml", body: "services: {}"})
	if _, err := ExtractArchive(bytes.NewReader(data), MaxArchiveBytes+1, t.TempDir()); err == nil {
		t.Fatal("ZIPサイズの上限を超過しました")
	}
	entries := make([]archiveEntry, MaxArchiveFiles+1)
	for i := range entries {
		entries[i] = archiveEntry{name: "file" + strings.Repeat("x", i%10)}
	}
	data = testArchive(t, entries...)
	if _, err := ExtractArchive(bytes.NewReader(data), int64(len(data)), t.TempDir()); err == nil {
		t.Fatal("ファイル数の上限を超過しました")
	}
	data = testArchive(t, archiveEntry{name: "compose.yaml", body: "services: {}"})
	central := bytes.LastIndex(data, []byte{'P', 'K', 1, 2})
	if central < 0 {
		t.Fatal("central directoryが見つかりません")
	}
	binary.LittleEndian.PutUint32(data[central+24:central+28], MaxArchiveExpanded+1)
	if _, err := ExtractArchive(bytes.NewReader(data), int64(len(data)), t.TempDir()); err == nil {
		t.Fatal("展開サイズの上限を超過しました")
	}
}
