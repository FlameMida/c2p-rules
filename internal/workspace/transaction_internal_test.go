package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCommitIgnoresLockReleaseFailureAfterPublishing(t *testing.T) {
	originalAcquireCommitLock := acquireCommitLock
	acquireCommitLock = func(string) (commitLock, error) {
		return failingReleaseLock{}, nil
	}
	t.Cleanup(func() { acquireCommitLock = originalAcquireCommitLock })

	root := t.TempDir()
	transaction, err := Begin(root)
	if err != nil {
		t.Fatal(err)
	}
	writeInternalTestFile(t, filepath.Join(transaction.Layout().Build, "generation"), "new")
	writeInternalTestFile(t, filepath.Join(transaction.Layout().Publish, "generation"), "new")

	if err := transaction.Commit(); err != nil {
		t.Fatalf("published generation must not be reported as failed: %v", err)
	}
	for _, directory := range []string{"build", "publish"} {
		data, err := os.ReadFile(filepath.Join(root, directory, "generation"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "new" {
			t.Fatalf("%s generation=%q want new", directory, data)
		}
	}
}

type failingReleaseLock struct{}

func (failingReleaseLock) release() error { return errors.New("injected release failure") }

func writeInternalTestFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}
