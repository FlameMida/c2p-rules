package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDocsDescribeGoOnlyWorkflowAndManagedInstall(t *testing.T) {
	root := repositoryRoot(t)
	for _, name := range []string{"README.md", "context.md"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, required := range []string{
			"geodata-build bootstrap",
			"geodata-build build",
			"geodata-build verify",
			"custom/geosite",
			"custom/geoip",
			"config/passwall2-groups.yaml",
			"install_passwall2_rules.sh.sha256sum",
			"managed_by=clash-rules-srs",
			"回滚",
		} {
			if !bytes.Contains(data, []byte(required)) {
				t.Errorf("%s missing %q", name, required)
			}
		}
		for _, forbidden := range []string{"python scripts/", "npm --prefix", "clash2passwall.js"} {
			if bytes.Contains(data, []byte(forbidden)) {
				t.Errorf("%s contains legacy instruction %q", name, forbidden)
			}
		}
	}
}
