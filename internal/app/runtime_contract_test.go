package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryHasNoPythonOrNodeBuildRuntime(t *testing.T) {
	root := repositoryRoot(t)
	for _, path := range []string{
		"requirements.txt",
		"scripts/build.py",
		"scripts/probe_tags.py",
		"scripts/bootstrap_vendor.sh",
		"scripts/lib",
		"tests",
		"tools/clash2passwall",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("legacy runtime remains: %s", path)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}
