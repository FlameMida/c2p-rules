package verify_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"clash-rules-srs/internal/verify"
)

var releaseAssets = []string{
	"geoip.dat",
	"geoip.dat.sha256sum",
	"geosite.dat",
	"geosite.dat.sha256sum",
	"install_passwall2_rules.sh",
	"install_passwall2_rules.sh.sha256sum",
	"shadowrocket.conf",
	"shadowrocket.conf.sha256sum",
}

func TestReleaseAssetsReturnsIndependentExactEightNames(t *testing.T) {
	first := verify.ReleaseAssets()
	if !slices.Equal(first, releaseAssets) {
		t.Fatalf("assets=%v", first)
	}
	first[0] = "mutated"
	if got := verify.ReleaseAssets(); !slices.Equal(got, releaseAssets) {
		t.Fatalf("shared mutable assets=%v", got)
	}
}

func TestWriteSHA256AndValidateExactEightAssets(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"geosite.dat":                "site",
		"geoip.dat":                  "ip",
		"install_passwall2_rules.sh": "#!/bin/sh\n",
		"shadowrocket.conf":          "[General]\n",
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
	if err := verify.Assets(dir, releaseAssets); err != nil {
		t.Fatal(err)
	}
}

func Test缺少Conf时资产校验失败(t *testing.T) {
	dir := validAssets(t)
	if err := os.Remove(filepath.Join(dir, "shadowrocket.conf")); err != nil {
		t.Fatal(err)
	}
	err := verify.Assets(dir, verify.ReleaseAssets())
	if err == nil || !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "shadowrocket.conf") {
		t.Fatalf("err=%v", err)
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
			if err := verify.Assets(dir, releaseAssets); err == nil || !strings.Contains(err.Error(), name[:3]) && name != "mismatch" {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func validAssets(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"geosite.dat", "geoip.dat", "install_passwall2_rules.sh", "shadowrocket.conf"} {
		path := filepath.Join(dir, name)
		mustWrite(t, path, name)
		if _, err := verify.WriteSHA256(path); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
