package verify_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clash-rules-srs/internal/verify"
)

var sixAssets = []string{
	"geoip.dat",
	"geoip.dat.sha256sum",
	"geosite.dat",
	"geosite.dat.sha256sum",
	"install_passwall2_rules.sh",
	"install_passwall2_rules.sh.sha256sum",
}

func TestWriteSHA256AndValidateExactSixAssets(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"geosite.dat":                "site",
		"geoip.dat":                  "ip",
		"install_passwall2_rules.sh": "#!/bin/sh\n",
	} {
		path := filepath.Join(dir, name)
		mustWrite(t, path, content)
		digest, err := verify.WriteSHA256(path)
		if err != nil || len(digest) != 64 {
			t.Fatalf("name=%s digest=%q err=%v", name, digest, err)
		}
		checksum, _ := os.ReadFile(path + ".sha256sum")
		if string(checksum) != digest+"  "+name+"\n" {
			t.Fatalf("checksum=%q", checksum)
		}
	}
	if err := verify.Assets(dir, sixAssets); err != nil {
		t.Fatal(err)
	}
}

func TestAssetsRejectsUnexpectedMissingAndMismatchedFiles(t *testing.T) {
	for name, mutate := range map[string]func(string){
		"unexpected": func(dir string) { mustWrite(t, filepath.Join(dir, "extra"), "x") },
		"missing":    func(dir string) { _ = os.Remove(filepath.Join(dir, "geoip.dat")) },
		"mismatch":   func(dir string) { mustWrite(t, filepath.Join(dir, "geosite.dat"), "tampered") },
	} {
		t.Run(name, func(t *testing.T) {
			dir := validAssets(t)
			mutate(dir)
			if err := verify.Assets(dir, sixAssets); err == nil || !strings.Contains(err.Error(), name[:3]) && name != "mismatch" {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func validAssets(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"geosite.dat", "geoip.dat", "install_passwall2_rules.sh"} {
		path := filepath.Join(dir, name)
		mustWrite(t, path, name)
		if _, err := verify.WriteSHA256(path); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
