package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkflowBuildsAndPublishesExactSixAssets(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", "build.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"go test ./...",
		"go test -tags=integration ./internal/app",
		"go run ./cmd/geodata-build bootstrap",
		"go run ./cmd/geodata-build build",
		"install_passwall2_rules.sh",
		"install_passwall2_rules.sh.sha256sum",
		"persist-credentials: false",
		"permissions:\n      contents: read",
	} {
		if !bytes.Contains(data, []byte(required)) {
			t.Errorf("workflow missing %q", required)
		}
	}
	for _, name := range []string{
		"geoip.dat", "geoip.dat.sha256sum", "geosite.dat", "geosite.dat.sha256sum",
		"install_passwall2_rules.sh", "install_passwall2_rules.sh.sha256sum",
	} {
		if bytes.Count(data, []byte(name)) < 2 {
			t.Errorf("workflow does not verify and upload %q", name)
		}
	}
}

func TestWorkflowHasNoLegacyRuntime(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", "build.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"setup-python", "python ", "npm ", "node "} {
		if bytes.Contains(data, []byte(forbidden)) {
			t.Errorf("workflow contains legacy command %q", forbidden)
		}
	}
}
