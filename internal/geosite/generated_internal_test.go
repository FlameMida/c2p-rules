package geosite

import (
	"os"
	"path/filepath"
	"testing"

	"clash-rules-srs/internal/targets"
)

func TestGenerated路径占用检查不区分大小写(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "CRS-PROXY-RESIDUAL"), []byte("domain:collision.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	exists, err := generatedPathExists(root, targets.ProxyResidualTag)
	if err != nil || !exists {
		t.Fatalf("exists=%v err=%v", exists, err)
	}
}
