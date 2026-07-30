package workspace_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clash-rules-srs/internal/workspace"
)

func TestAbortAfterFinalProbePreservesOldBuildAndPublish(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "build", "expected_tags.json"), "old-manifest")
	writeFile(t, filepath.Join(root, "publish", "geosite.dat"), "old-site")
	transaction, err := workspace.Begin(root)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(transaction.Layout().Publish, "geosite.dat"), "new-site")
	if err := transaction.Abort(); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(root, "build", "expected_tags.json"), "old-manifest")
	assertFile(t, filepath.Join(root, "publish", "geosite.dat"), "old-site")
	if _, err := os.Stat(transaction.Layout().Staging); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging remains: %v", err)
	}
}

func TestCommitSwitchesBothDirectories(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "build", "old"), "old")
	writeFile(t, filepath.Join(root, "publish", "old"), "old")
	transaction, err := workspace.Begin(root)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(transaction.Layout().Build, "new"), "new")
	writeFile(t, filepath.Join(transaction.Layout().Publish, "new"), "new")
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(root, "build", "new"), "new")
	assertFile(t, filepath.Join(root, "publish", "new"), "new")
	if _, err := os.Stat(filepath.Join(root, "build", "old")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old build remains: %v", err)
	}
}

func TestCommitRollsBackBothDirectoriesWhenSecondSwitchFails(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "build", "old"), "old-build")
	writeFile(t, filepath.Join(root, "publish", "old"), "old-publish")
	failing := &failRenameFS{FS: workspace.OSFS{}, failAt: 4}
	transaction, err := workspace.BeginWithFS(root, failing)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(transaction.Layout().Build, "new"), "new-build")
	writeFile(t, filepath.Join(transaction.Layout().Publish, "new"), "new-publish")
	err = transaction.Commit()
	if err == nil || !strings.Contains(err.Error(), "injected rename failure") {
		t.Fatalf("err=%v", err)
	}
	assertFile(t, filepath.Join(root, "build", "old"), "old-build")
	assertFile(t, filepath.Join(root, "publish", "old"), "old-publish")
	if err := transaction.Abort(); err != nil {
		t.Fatal(err)
	}
}

func TestCommitWithNoPreviousDirectoriesRollsBackNewDirectory(t *testing.T) {
	root := t.TempDir()
	failing := &failRenameFS{FS: workspace.OSFS{}, failAt: 2}
	transaction, err := workspace.BeginWithFS(root, failing)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(transaction.Layout().Build, "new"), "new-build")
	writeFile(t, filepath.Join(transaction.Layout().Publish, "new"), "new-publish")
	if err := transaction.Commit(); err == nil {
		t.Fatal("expected failure")
	}
	for _, name := range []string{"build", "publish"} {
		if _, err := os.Stat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s unexpectedly exists: %v", name, err)
		}
	}
}

func TestLayoutKeepsAllIntermediatesInsideStaging(t *testing.T) {
	transaction, err := workspace.Begin(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Abort()
	layout := transaction.Layout()
	for name, path := range map[string]string{
		"build": layout.Build, "publish": layout.Publish, "data": layout.DataMerged,
		"ip": layout.IP, "manifest": layout.Manifest, "geoip config": layout.GeoIPConfig,
	} {
		relative, err := filepath.Rel(layout.Staging, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			t.Fatalf("%s path escaped staging: %s", name, path)
		}
	}
}

type failRenameFS struct {
	workspace.FS
	calls  int
	failAt int
}

func (f *failRenameFS) Rename(oldPath, newPath string) error {
	f.calls++
	if f.calls == f.failAt {
		return errors.New("injected rename failure")
	}
	return f.FS.Rename(oldPath, newPath)
}

var _ workspace.FS = (*failRenameFS)(nil)

func writeFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s=%q want %q", path, got, want)
	}
}
