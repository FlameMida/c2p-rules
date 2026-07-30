package workspace_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestConcurrentTransactionsNeverPublishMixedGenerations(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "build", "generation"), "old")
	writeFile(t, filepath.Join(root, "publish", "generation"), "old")
	paused := make(chan struct{})
	release := make(chan struct{})
	aFS := &barrierRenameFS{
		FS: workspace.OSFS{}, pauseOldPath: filepath.Join(root, "publish"),
		paused: paused, release: release,
	}
	a, err := workspace.BeginWithFS(root, aFS)
	if err != nil {
		t.Fatal(err)
	}
	b, err := workspace.Begin(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		tx         *workspace.Transaction
		generation string
	}{
		{a, "A"},
		{b, "B"},
	} {
		writeFile(t, filepath.Join(item.tx.Layout().Build, "generation"), item.generation)
		writeFile(t, filepath.Join(item.tx.Layout().Publish, "generation"), item.generation)
	}

	aDone := make(chan error, 1)
	go func() { aDone <- a.Commit() }()
	select {
	case <-paused:
	case <-time.After(2 * time.Second):
		t.Fatal("transaction A did not reach publish switch")
	}
	bDone := make(chan error, 1)
	go func() { bDone <- b.Commit() }()
	var bErr error
	bFinished := false
	select {
	case bErr = <-bDone:
		bFinished = true
		// An unlocked implementation lets B finish inside A's switch window.
	case <-time.After(200 * time.Millisecond):
		// A root lock keeps B waiting until A releases the switch window.
	}
	close(release)
	if err := <-aDone; err != nil {
		t.Fatalf("transaction A: %v", err)
	}
	if !bFinished {
		bErr = <-bDone
	}
	if bErr != nil {
		t.Fatalf("transaction B: %v", bErr)
	}
	buildGeneration := string(mustReadFile(t, filepath.Join(root, "build", "generation")))
	publishGeneration := string(mustReadFile(t, filepath.Join(root, "publish", "generation")))
	if buildGeneration != publishGeneration {
		t.Fatalf("mixed generation: build=%s publish=%s", buildGeneration, publishGeneration)
	}
}

type barrierRenameFS struct {
	workspace.FS
	pauseOldPath string
	paused       chan struct{}
	release      chan struct{}
	once         sync.Once
}

func (f *barrierRenameFS) Rename(oldPath, newPath string) error {
	if oldPath == f.pauseOldPath {
		f.once.Do(func() {
			close(f.paused)
			<-f.release
		})
	}
	return f.FS.Rename(oldPath, newPath)
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

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
