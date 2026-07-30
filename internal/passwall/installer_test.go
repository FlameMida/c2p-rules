package passwall_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"clash-rules-srs/internal/passwall"
)

func TestInstallerIsIdempotentAndPreservesUserRulesAndNodes(t *testing.T) {
	harness := newHarness(t)
	harness.seedConfig(`config nodes 'node1'
	option remarks 'Keep Node'

config shunt_rules 'user_rule'
	option remarks 'Keep Rule'

config shunt_rules 'c2p_Proxy'
	option remarks 'Legacy'

config shunt_rules 'crs_old'
	option managed_by 'clash-rules-srs'
`)
	script := harness.render(validInstallOptions())
	firstOutput := harness.run(script, true)
	first := harness.readConfig()
	secondOutput := harness.run(script, true)
	second := harness.readConfig()
	if first != second {
		t.Fatalf("not idempotent\nfirst=%s\nsecond=%s", first, second)
	}
	for _, want := range []string{"Keep Node", "Keep Rule", "crs_apple_services", "releases/latest/download/geosite.dat"} {
		if !strings.Contains(second, want) {
			t.Fatalf("missing %s in %s", want, second)
		}
	}
	for _, gone := range []string{"c2p_Proxy", "crs_old"} {
		if strings.Contains(second, gone) {
			t.Fatalf("legacy remains: %s", gone)
		}
	}
	if strings.Count(second, "config shunt_rules 'crs_apple_services'") != 1 {
		t.Fatalf("managed group count wrong: %s", second)
	}
	for _, output := range []string{firstOutput, secondOutput} {
		backup := backupPath(t, output)
		if _, err := os.Stat(backup); err != nil {
			t.Fatalf("backup %s: %v", backup, err)
		}
	}
}

func TestInstallerRollsBackWhenUpdaterReturnsSuccessWithWrongHash(t *testing.T) {
	harness := newHarness(t)
	harness.seedConfig("")
	beforeConfig, beforeSite, beforeIP := harness.snapshot()
	harness.siteContent = "wrong-site"
	harness.ipContent = "wrong-ip"
	harness.run(harness.render(validInstallOptions()), false)
	harness.assertSnapshot(beforeConfig, beforeSite, beforeIP)
}

func TestInstallerRollsBackWhenOnlyOneOldDatExists(t *testing.T) {
	harness := newHarness(t)
	harness.seedConfig("")
	if err := os.Remove(harness.ipPath); err != nil {
		t.Fatal(err)
	}
	beforeConfig, beforeSite, _ := harness.snapshot()
	harness.siteContent = "wrong-site"
	harness.ipContent = "wrong-ip"
	harness.run(harness.render(validInstallOptions()), false)
	if harness.readConfig() != beforeConfig || string(mustRead(t, harness.sitePath)) != beforeSite {
		t.Fatal("existing config/geosite were not restored")
	}
	if _, err := os.Stat(harness.ipPath); !os.IsNotExist(err) {
		t.Fatalf("previously absent geoip was not removed: %v", err)
	}
}

func TestInstallerRollsBackStageAndLiveCommitFailures(t *testing.T) {
	for _, failCommit := range []int{1, 2} {
		t.Run(fmt.Sprintf("commit-%d", failCommit), func(t *testing.T) {
			harness := newHarness(t)
			harness.seedConfig("")
			beforeConfig, beforeSite, beforeIP := harness.snapshot()
			harness.failCommit = failCommit
			harness.run(harness.render(validInstallOptions()), false)
			harness.assertSnapshot(beforeConfig, beforeSite, beforeIP)
		})
	}
}

func TestInstallerPreflightRejectsDirtyUCIAndMissingUpdater(t *testing.T) {
	for name, mutate := range map[string]func(*installerHarness){
		"dirty": func(h *installerHarness) { h.uciChanges = "passwall2.changed=1" },
		"missing updater": func(h *installerHarness) {
			if err := os.Remove(h.updater); err != nil {
				h.t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			harness := newHarness(t)
			harness.seedConfig("")
			beforeConfig, beforeSite, beforeIP := harness.snapshot()
			mutate(harness)
			harness.run(harness.render(validInstallOptions()), false)
			harness.assertSnapshot(beforeConfig, beforeSite, beforeIP)
		})
	}
}

func TestRenderInstallerValidatesInputsAndEmbedsOnlyBase64Fragment(t *testing.T) {
	options := validInstallOptions()
	script, err := passwall.RenderInstaller(options)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(script, options.Fragment) || bytes.Contains(script, []byte("<<")) {
		t.Fatalf("fragment/heredoc leaked into template:\n%s", script)
	}
	for _, command := range []string{"command -v uci", "command -v lua", "command -v sha256sum", "command -v base64"} {
		if !bytes.Contains(script, []byte(command)) {
			t.Fatalf("missing preflight %q", command)
		}
	}
	path := filepath.Join(t.TempDir(), "installer.sh")
	if err := os.WriteFile(path, script, 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("/bin/sh", "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("sh -n: %v\n%s", err, output)
	}

	invalid := []passwall.InstallOptions{
		{Repo: "OWNER/REPO", ReleaseTag: "v1", GeoSiteSHA: strings.Repeat("a", 64), GeoIPSHA: strings.Repeat("b", 64), Fragment: []byte("x")},
		{Repo: "flame/repo", ReleaseTag: "bad tag", GeoSiteSHA: strings.Repeat("a", 64), GeoIPSHA: strings.Repeat("b", 64), Fragment: []byte("x")},
		{Repo: "flame/repo", ReleaseTag: "v1", GeoSiteSHA: "bad", GeoIPSHA: strings.Repeat("b", 64), Fragment: []byte("x")},
		{Repo: "flame/repo", ReleaseTag: "v1", GeoSiteSHA: strings.Repeat("a", 64), GeoIPSHA: strings.Repeat("b", 64)},
	}
	for _, option := range invalid {
		if _, err := passwall.RenderInstaller(option); err == nil {
			t.Fatalf("accepted invalid options: %#v", option)
		}
	}
}

type installerHarness struct {
	t           *testing.T
	root        string
	bin         string
	config      string
	assets      string
	sitePath    string
	ipPath      string
	updater     string
	siteContent string
	ipContent   string
	uciChanges  string
	failCommit  int
}

func newHarness(t *testing.T) *installerHarness {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	assets := filepath.Join(root, "assets")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	copyExecutable(t, "testdata/fake-uci.sh", filepath.Join(bin, "uci"))
	copyExecutable(t, "testdata/fake-lua.sh", filepath.Join(bin, "lua"))
	updater := filepath.Join(root, "fake-rule-update.lua")
	copyExecutable(t, "testdata/fake-rule-update.lua", updater)
	harness := &installerHarness{
		t: t, root: root, bin: bin, config: filepath.Join(root, "passwall2"), assets: assets,
		sitePath: filepath.Join(assets, "geosite.dat"), ipPath: filepath.Join(assets, "geoip.dat"), updater: updater,
		siteContent: "new-site", ipContent: "new-ip",
	}
	mustWrite(t, harness.sitePath, "old-site")
	mustWrite(t, harness.ipPath, "old-ip")
	return harness
}

func (h *installerHarness) seedConfig(extra string) {
	h.t.Helper()
	content := fmt.Sprintf(`config global_rules 'global'
	option v2ray_location_asset '%s'
	option geosite_url 'old-site-url'
	option geoip_url 'old-ip-url'

%s`, h.assets, extra)
	mustWrite(h.t, h.config, content)
}

func (h *installerHarness) render(options passwall.InstallOptions) []byte {
	h.t.Helper()
	script, err := passwall.RenderInstaller(options)
	if err != nil {
		h.t.Fatal(err)
	}
	return script
}

func (h *installerHarness) run(script []byte, wantSuccess bool) string {
	h.t.Helper()
	path := filepath.Join(h.root, "install.sh")
	if err := os.WriteFile(path, script, 0o700); err != nil {
		h.t.Fatal(err)
	}
	command := exec.Command("/bin/sh", path)
	command.Env = append(os.Environ(),
		"PATH="+h.bin+":"+os.Getenv("PATH"),
		"PASSWALL2_CONF="+h.config,
		"PASSWALL2_RULE_UPDATER="+h.updater,
		"PASSWALL2_ASSET_DIR="+h.assets,
		"FAKE_SITE_CONTENT="+h.siteContent,
		"FAKE_IP_CONTENT="+h.ipContent,
		"FAKE_EXPECTED_TAG=v1.2.3",
		"FAKE_UCI_CHANGES="+h.uciChanges,
		fmt.Sprintf("FAKE_UCI_FAIL_COMMIT=%d", h.failCommit),
		"FAKE_UCI_COUNTER="+filepath.Join(h.root, "uci-counter"),
	)
	output, err := command.CombinedOutput()
	if wantSuccess && err != nil {
		h.t.Fatalf("installer failed: %v\n%s", err, output)
	}
	if !wantSuccess && err == nil {
		h.t.Fatalf("installer unexpectedly succeeded:\n%s", output)
	}
	return string(output)
}

func (h *installerHarness) snapshot() (string, string, string) {
	h.t.Helper()
	config := h.readConfig()
	site := string(mustRead(h.t, h.sitePath))
	ip := ""
	if data, err := os.ReadFile(h.ipPath); err == nil {
		ip = string(data)
	} else if !os.IsNotExist(err) {
		h.t.Fatal(err)
	}
	return config, site, ip
}

func (h *installerHarness) assertSnapshot(config, site, ip string) {
	h.t.Helper()
	gotConfig, gotSite, gotIP := h.snapshot()
	if gotConfig != config || gotSite != site || gotIP != ip {
		h.t.Fatalf("snapshot changed\nconfig=%q want=%q\nsite=%q want=%q\nip=%q want=%q", gotConfig, config, gotSite, site, gotIP, ip)
	}
}

func (h *installerHarness) readConfig() string {
	h.t.Helper()
	return string(mustRead(h.t, h.config))
}

func validInstallOptions() passwall.InstallOptions {
	return passwall.InstallOptions{
		Repo: "flame/clash-rules-srs", ReleaseTag: "v1.2.3",
		GeoSiteSHA: digest("new-site"), GeoIPSHA: digest("new-ip"),
		Fragment: []byte("config shunt_rules 'crs_apple_services'\n\toption remarks '苹果服务'\n\toption managed_by 'clash-rules-srs'\n\toption network 'tcp,udp'\n\toption domain_list 'geosite:apple\ngeosite:icloud'\n"),
	}
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func backupPath(t *testing.T, output string) string {
	t.Helper()
	match := regexp.MustCompile(`(?m)^备份: (.+)$`).FindStringSubmatch(output)
	if len(match) != 2 {
		t.Fatalf("backup path missing from %q", output)
	}
	return strings.TrimSpace(match[1])
}

func copyExecutable(t *testing.T, source, destination string) {
	t.Helper()
	data := mustRead(t, source)
	if err := os.WriteFile(destination, data, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
