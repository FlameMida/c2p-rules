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
	"slices"
	"strings"
	"testing"

	"clash-rules-srs/internal/config"
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
	for _, want := range []string{
		"config nodes 'node1'\n\toption remarks 'Keep Node'",
		"config shunt_rules 'user_rule'\n\toption remarks 'Keep Rule'",
		"crs_apple_services",
		"releases/latest/download/geosite.dat",
	} {
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

func TestInstallerSuccessInstallsAllRepositoryGroupsAndValidData(t *testing.T) {
	groups, err := config.LoadGroups("../../config/passwall2-groups.yaml")
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := passwall.Render(groups)
	if err != nil {
		t.Fatal(err)
	}
	options := validInstallOptions()
	options.Fragment = fragment
	harness := newHarness(t)
	harness.seedConfig("config nodes 'node1'\n\toption remarks 'Keep Node'\n")
	output := harness.run(harness.render(options), true)
	installed := harness.readConfig()
	if strings.Count(installed, "option managed_by 'clash-rules-srs'") != 16 {
		t.Fatalf("managed group count is not 16:\n%s", installed)
	}
	for _, group := range groups {
		if !strings.Contains(installed, "config shunt_rules 'crs_"+group.ID+"'") {
			t.Fatalf("missing managed group %s", group.ID)
		}
	}
	for _, required := range []string{"Keep Node", "releases/latest/download/geosite.dat", "releases/latest/download/geoip.dat"} {
		if !strings.Contains(installed, required) {
			t.Fatalf("missing %s in installed config", required)
		}
	}
	if string(mustRead(t, harness.sitePath)) != "new-site" || string(mustRead(t, harness.ipPath)) != "new-ip" {
		t.Fatal("installed dat bytes do not match validated payload")
	}
	backup := backupPath(t, output)
	backupInfo, err := os.Stat(backup)
	if err != nil {
		t.Fatalf("success backup is not recoverable: %v", err)
	}
	if backupInfo.Mode().Perm() != 0o600 || !strings.Contains(string(mustRead(t, backup)), "Keep Node") {
		t.Fatalf("backup mode/content invalid: mode=%o content=%q", backupInfo.Mode().Perm(), mustRead(t, backup))
	}
}

func TestInstallerFallsBackFromTimeoutAndHashMismatchThenPersistsOfficialLatest(t *testing.T) {
	harness := newHarness(t)
	harness.seedConfig("")
	harness.ruleTimeout = "1"
	harness.timeoutSources = "gh-proxy.com"
	harness.badSHASources = "ghfast.top"

	output := harness.run(harness.render(validInstallOptions()), true)
	if got, want := harness.readAttempts(), "gh-proxy.com\nghfast.top\ngithub.com\n"; got != want {
		t.Fatalf("attempt order=%q want=%q", got, want)
	}
	for _, want := range []string{
		"[4/5] 尝试源 1/3：gh-proxy.com",
		"gh-proxy.com 超时（1 秒），切换下一个源。",
		"[4/5] 尝试源 2/3：ghfast.top",
		"ghfast.top SHA-256 校验失败，切换下一个源。",
		"[4/5] 尝试源 3/3：GitHub 官方",
		"GitHub 官方 SHA-256 校验通过。",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing fallback progress %q in output:\n%s", want, output)
		}
	}
	installed := harness.readConfig()
	for _, want := range []string{
		"option geosite_url 'https://github.com/flame/clash-rules-srs/releases/latest/download/geosite.dat'",
		"option geoip_url 'https://github.com/flame/clash-rules-srs/releases/latest/download/geoip.dat'",
	} {
		if !strings.Contains(installed, want) {
			t.Fatalf("missing official persistent URL %q in config:\n%s", want, installed)
		}
	}
	if strings.Contains(installed, "gh-proxy.com") || strings.Contains(installed, "ghfast.top") {
		t.Fatalf("mirror URL persisted after install:\n%s", installed)
	}
}

func TestInstallerStopsAfterFirstMirrorPassesHashValidation(t *testing.T) {
	harness := newHarness(t)
	harness.seedConfig("")

	harness.run(harness.render(validInstallOptions()), true)
	if got, want := harness.readAttempts(), "gh-proxy.com\n"; got != want {
		t.Fatalf("attempts=%q want=%q", got, want)
	}
}

func TestInstallerReportsPhaseProgressWhileUpdatingRules(t *testing.T) {
	harness := newHarness(t)
	harness.seedConfig("")
	harness.updaterDelaySeconds = 2

	output := harness.run(harness.render(validInstallOptions()), true)
	for _, want := range []string{
		"[1/5] 检查 PassWall2 运行环境...",
		"[2/5] 备份现有配置和 GeoIP/GeoSite 数据...",
		"[3/5] 写入托管分流和本次 Release 更新地址...",
		"[4/5] 正在更新 GeoIP/GeoSite（下载可能需要一些时间）...",
		"[4/5] 仍在更新（已等待 1 秒）...",
		"[4/5] GeoIP/GeoSite 更新完成。",
		"[5/5] 校验规则数据并切换后续更新地址...",
		"[5/5] 校验完成，后续更新将使用 latest 地址。",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing progress %q in output:\n%s", want, output)
		}
	}
}

func TestInstallerReportsRollbackProgressAfterUpdateFailure(t *testing.T) {
	harness := newHarness(t)
	harness.seedConfig("")
	harness.siteContent = "wrong-site"
	harness.ipContent = "wrong-ip"

	output := harness.run(harness.render(validInstallOptions()), false)
	if !strings.Contains(output, "[回滚] 安装未完成，正在恢复原有配置和 GeoIP/GeoSite 数据...") {
		t.Fatalf("missing rollback progress in output:\n%s", output)
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
	for _, failCommit := range []int{1, 2, 3} {
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

func TestInstallerUsesIsolatedSavedirForStagingUCI(t *testing.T) {
	harness := newHarness(t)
	harness.seedConfig("")
	harness.requireIsolatedSavedir = true
	harness.run(harness.render(validInstallOptions()), true)
}

func TestInstallerPreservesManualRecoveryFilesWhenRestoreFails(t *testing.T) {
	harness := newHarness(t)
	harness.seedConfig("")
	harness.siteContent = "wrong-site"
	harness.ipContent = "wrong-ip"
	harness.failRestoreSourceSuffix = "geosite.dat.old"
	output := harness.run(harness.render(validInstallOptions()), false)
	if !strings.Contains(output, "automatic restore incomplete") {
		t.Fatalf("missing restore failure: %s", output)
	}
	for _, label := range []string{"config backup", "geosite backup", "geoip backup"} {
		match := regexp.MustCompile(`(?m)` + label + `: (.+)$`).FindStringSubmatch(output)
		if len(match) != 2 {
			t.Fatalf("%s path missing from %q", label, output)
		}
		if _, err := os.Stat(strings.TrimSpace(match[1])); err != nil {
			t.Fatalf("%s was not preserved: %v", label, err)
		}
	}
}

func TestInstallerClearsLiveUCIDeltaBeforeRestoringConfig(t *testing.T) {
	harness := newHarness(t)
	harness.seedConfig("")
	harness.siteContent = "wrong-site"
	harness.ipContent = "wrong-ip"
	harness.requireRevertBeforeRestore = true
	beforeConfig, beforeSite, beforeIP := harness.snapshot()
	harness.run(harness.render(validInstallOptions()), false)
	harness.assertSnapshot(beforeConfig, beforeSite, beforeIP)
}

func TestInstallerPreflightRejectsDirtyUCIExistingUpdaterAndMissingCommands(t *testing.T) {
	for name, mutate := range map[string]func(*installerHarness){
		"dirty": func(h *installerHarness) { h.uciChanges = "passwall2.changed=1" },
		"existing updater lock": func(h *installerHarness) {
			mustWrite(h.t, h.ruleLock, "locked")
		},
		"missing updater": func(h *installerHarness) {
			if err := os.Remove(h.updater); err != nil {
				h.t.Fatal(err)
			}
		},
		"missing timeout": func(h *installerHarness) {
			if err := os.Remove(filepath.Join(h.bin, "timeout")); err != nil {
				h.t.Fatal(err)
			}
			for _, name := range []string{"sha256sum", "base64"} {
				path, err := exec.LookPath(name)
				if err != nil {
					h.t.Fatal(err)
				}
				if err := os.Symlink(path, filepath.Join(h.bin, name)); err != nil {
					h.t.Fatal(err)
				}
			}
			h.isolatePath = true
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

func TestInstallerPreflightRejectsInvalidRuleTimeout(t *testing.T) {
	for _, value := range []string{"0", "bad", "-1"} {
		t.Run(value, func(t *testing.T) {
			harness := newHarness(t)
			harness.seedConfig("")
			harness.ruleTimeout = value
			beforeConfig, beforeSite, beforeIP := harness.snapshot()

			output := harness.run(harness.render(validInstallOptions()), false)
			if !strings.Contains(output, "ERROR: PASSWALL2_RULE_TIMEOUT must be a positive integer") {
				t.Fatalf("missing timeout validation error:\n%s", output)
			}
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
	for _, command := range []string{"command -v uci", "command -v lua", "command -v sha256sum", "command -v base64", "command -v timeout"} {
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

func TestValidateReleaseTagMatchesRendererContract(t *testing.T) {
	valid := []string{"release-1_A.b", "a", strings.Repeat("a", 128)}
	invalid := []string{"", "bad tag", "-leading", strings.Repeat("a", 129)}
	for _, tag := range append(valid, invalid...) {
		t.Run(tag, func(t *testing.T) {
			validateErr := passwall.ValidateReleaseTag(tag)
			options := validInstallOptions()
			options.ReleaseTag = tag
			_, renderErr := passwall.RenderInstaller(options)
			if (validateErr == nil) != (renderErr == nil) {
				t.Fatalf("tag=%q validate=%v render=%v", tag, validateErr, renderErr)
			}
			wantValid := len(tag) > 0 && !slices.Contains(invalid, tag)
			if wantValid != (validateErr == nil) {
				t.Fatalf("tag=%q err=%v", tag, validateErr)
			}
		})
	}
}

type installerHarness struct {
	t                          *testing.T
	root                       string
	bin                        string
	config                     string
	assets                     string
	sitePath                   string
	ipPath                     string
	updater                    string
	siteContent                string
	ipContent                  string
	updaterDelaySeconds        int
	ruleTimeout                string
	timeoutSources             string
	badSHASources              string
	attemptsPath               string
	ruleLock                   string
	isolatePath                bool
	uciChanges                 string
	failCommit                 int
	requireIsolatedSavedir     bool
	failRestoreSourceSuffix    string
	requireRevertBeforeRestore bool
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
	copyExecutable(t, "testdata/fake-cp.sh", filepath.Join(bin, "cp"))
	copyExecutable(t, "testdata/fake-timeout.sh", filepath.Join(bin, "timeout"))
	updater := filepath.Join(root, "fake-rule-update.lua")
	copyExecutable(t, "testdata/fake-rule-update.lua", updater)
	harness := &installerHarness{
		t: t, root: root, bin: bin, config: filepath.Join(root, "passwall2"), assets: assets,
		sitePath: filepath.Join(assets, "geosite.dat"), ipPath: filepath.Join(assets, "geoip.dat"), updater: updater,
		siteContent: "new-site", ipContent: "new-ip", ruleTimeout: "60",
		attemptsPath: filepath.Join(root, "updater-attempts"),
		ruleLock:     filepath.Join(root, "passwall2_rule_update.lock"),
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
	commandPath := h.bin + ":" + os.Getenv("PATH")
	if h.isolatePath {
		commandPath = h.bin
	}
	command.Env = append(os.Environ(),
		"PATH="+commandPath,
		"PASSWALL2_CONF="+h.config,
		"PASSWALL2_RULE_UPDATER="+h.updater,
		"PASSWALL2_ASSET_DIR="+h.assets,
		"FAKE_SITE_CONTENT="+h.siteContent,
		"FAKE_IP_CONTENT="+h.ipContent,
		fmt.Sprintf("FAKE_UPDATER_DELAY=%d", h.updaterDelaySeconds),
		"FAKE_UPDATER_TIMEOUT_SOURCES="+h.timeoutSources,
		"FAKE_UPDATER_BAD_SHA_SOURCES="+h.badSHASources,
		"FAKE_UPDATER_ATTEMPTS="+h.attemptsPath,
		"FAKE_EXPECTED_TAG=v1.2.3",
		"PASSWALL2_RULE_TIMEOUT="+h.ruleTimeout,
		"PASSWALL2_RULE_LOCK="+h.ruleLock,
		"FAKE_UCI_CHANGES="+h.uciChanges,
		fmt.Sprintf("FAKE_UCI_FAIL_COMMIT=%d", h.failCommit),
		fmt.Sprintf("FAKE_UCI_REQUIRE_ISOLATED_SAVEDIR=%t", h.requireIsolatedSavedir),
		"FAKE_UCI_COUNTER="+filepath.Join(h.root, "uci-counter"),
		"FAKE_UCI_LIVE_SAVEDIR="+filepath.Join(h.root, "uci-live-saved"),
		"FAKE_UCI_REVERT_MARKER="+filepath.Join(h.root, "uci-reverted"),
		"FAKE_CP_FAIL_SOURCE_SUFFIX="+h.failRestoreSourceSuffix,
		fmt.Sprintf("FAKE_CP_REQUIRE_REVERT_FOR_BACKUP=%t", h.requireRevertBeforeRestore),
		"FAKE_CP_REVERT_MARKER="+filepath.Join(h.root, "uci-reverted"),
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

func (h *installerHarness) readAttempts() string {
	h.t.Helper()
	data, err := os.ReadFile(h.attemptsPath)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		h.t.Fatal(err)
	}
	return string(data)
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
