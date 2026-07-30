package fileutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"clash-rules-srs/internal/fileutil"
)

func TestAtomicWriteReplacesContentAndMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "asset")
	if err := fileutil.AtomicWrite(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.AtomicWrite(path, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "two" || info.Mode().Perm() != 0o644 {
		t.Fatalf("content=%q mode=%o", data, info.Mode().Perm())
	}
}

func TestAtomicWriteCleansTemporaryFileAfterRenameFailure(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.AtomicWrite(target, []byte("data"), 0o644); err == nil {
		t.Fatal("expected rename failure")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "target" {
		t.Fatalf("temporary file leaked: %#v", entries)
	}
}
