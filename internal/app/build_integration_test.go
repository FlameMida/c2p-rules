//go:build integration

package app_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"clash-rules-srs/internal/app"
	"clash-rules-srs/internal/config"
	"clash-rules-srs/internal/fetch"
	"clash-rules-srs/internal/geodat"
	"clash-rules-srs/internal/manifest"
	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/targets"
	"clash-rules-srs/internal/tools"
	"clash-rules-srs/internal/verify"
)

func Test完整构建得到八文件且Checksum匹配(t *testing.T) {
	repositoryRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binRoot := filepath.Join(repositoryRoot, ".cache", "bin")
	requireIntegrationTools(t, binRoot)
	runner := &tools.Runner{BinRoot: binRoot}
	baseDat := buildSyntheticBaseGeoIP(t, runner)
	baseData, err := os.ReadFile(baseDat)
	if err != nil {
		t.Fatal(err)
	}

	sourcePayloads := integrationSourcePayloads()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/base.dat" {
			writer.Header().Set("Content-Type", "application/octet-stream")
			_, _ = writer.Write(baseData)
			return
		}
		payload, found := sourcePayloads[request.URL.Path]
		if !found {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write([]byte(payload))
	}))
	defer server.Close()

	root := newIntegrationRoot(t, server.URL)
	client := fetch.NewWithHTTPClient(fetch.Options{}, server.Client())
	options := app.BuildOptions{
		Root:       root,
		Sources:    filepath.Join(root, "sources.yaml"),
		Custom:     filepath.Join(root, "custom"),
		Groups:     filepath.Join(root, "groups.yaml"),
		Community:  filepath.Join(root, "community"),
		CacheRoot:  filepath.Join(repositoryRoot, ".cache"),
		Repo:       "example/clash-rules-srs",
		ReleaseTag: "integration-test",
	}
	if err := app.Build(context.Background(), options, app.ProductionDependencies(client, runner)); err != nil {
		t.Fatal(err)
	}

	document := readIntegrationManifest(t, filepath.Join(root, "build", "expected_tags.json"))
	for _, tag := range []string{
		"reject", "proxy", "direct", "ai",
		"youtube", "netflix", "spotify", "tiktok", "apple", "icloud", "google", "private", "cn",
	} {
		if !slices.Contains(document.Required.GeoSite, tag) {
			t.Fatalf("missing geosite:%s", tag)
		}
	}
	for _, tag := range []string{"cn", "google", "netflix", "private", "telegram"} {
		if !slices.Contains(document.Required.GeoIP, tag) {
			t.Fatalf("missing geoip:%s", tag)
		}
	}
	for _, side := range [][]string{document.Forbidden.GeoSite, document.Forbidden.GeoIP} {
		for _, forbidden := range []string{"BilibiliHMT", "xiaolin-bilibili"} {
			if !slices.Contains(side, forbidden) {
				t.Fatalf("legacy tag %s is not forbidden: %#v", forbidden, document.Forbidden)
			}
		}
	}
	for _, retired := range targets.RetiredGeoSiteTags() {
		if !slices.Contains(document.Forbidden.GeoSite, retired) {
			t.Fatalf("retired geosite:%s is not forbidden: %#v", retired, document.Forbidden)
		}
	}

	mergedRoot := filepath.Join(root, "build", "data-merged")
	assertFileContains(t, filepath.Join(mergedRoot, "proxy"), "domain:custom-proxy.example", 1)
	geoSiteDat := filepath.Join(root, "publish", "geosite.dat")
	geoIPDat := filepath.Join(root, "publish", "geoip.dat")

	prober := verify.NewProber(runner, geoSiteDat, geoIPDat)
	if err := verify.Required(context.Background(), prober, document); err != nil {
		t.Fatal(err)
	}
	if err := verify.Forbidden(context.Background(), prober, document); err != nil {
		t.Fatal(err)
	}

	intermediateBytes, err := os.ReadFile(filepath.Join(root, "build", "intermediate-geosite", "geosite.dat"))
	if err != nil {
		t.Fatal(err)
	}
	finalBytes, err := os.ReadFile(geoSiteDat)
	if err != nil {
		t.Fatal(err)
	}
	intermediate, err := geodat.DecodeGeoSite(intermediateBytes)
	if err != nil {
		t.Fatal(err)
	}
	final, err := geodat.DecodeGeoSite(finalBytes)
	if err != nil {
		t.Fatal(err)
	}
	finalIPBytes, err := os.ReadFile(geoIPDat)
	if err != nil {
		t.Fatal(err)
	}
	finalIP, err := geodat.DecodeGeoIP(finalIPBytes)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := sortedIPTagKeys(finalIP), []string{"cn", "google", "netflix", "private", "telegram"}; !slices.Equal(got, want) {
		t.Fatalf("final geoip tags=%v want=%v", got, want)
	}

	t.Run("最终DAT用proxy和direct原位承载残差", func(t *testing.T) {
		if len(finalBytes) >= len(intermediateBytes) {
			t.Fatalf("final geosite did not shrink: final=%d intermediate=%d", len(finalBytes), len(intermediateBytes))
		}
		for _, removed := range []string{"gfw", "tld-not-cn", "crs-proxy-residual", "crs-direct-residual"} {
			if _, exists := final[removed]; exists {
				t.Fatalf("removed geosite:%s remains in final DAT", removed)
			}
		}
		if _, exists := final["proxy"]; !exists {
			t.Fatal("final DAT is missing geosite:proxy")
		}
		if _, exists := final["direct"]; !exists {
			t.Fatal("final DAT is missing geosite:direct")
		}
		if reflect.DeepEqual(intermediate["proxy"].Domains, final["proxy"].Domains) {
			t.Fatal("geosite:proxy was not replaced by the in-place residual")
		}
		if !hasDomainRule(final["proxy"].Domains, model.DomainSuffix, "foreign", nil) {
			t.Fatal("geosite:proxy did not absorb tld-not-cn")
		}
	})

	t.Run("include 与属性在残差输入中已展开", func(t *testing.T) {
		google := intermediate["google"].Domains
		if !hasDomainRule(google, model.DomainSuffix, "youtube.example", nil) ||
			!hasDomainRule(google, model.DomainSuffix, "googleapis.com", []string{"@cn"}) {
			t.Fatalf("google=%#v", google)
		}
		for _, tag := range targets.ResidualGeoSiteTags() {
			want := readGeneratedRules(t, filepath.Join(mergedRoot, tag))
			if !sameDomainRules(final[tag].Domains, want) {
				t.Fatalf("in-place residual %s=%#v want=%#v", tag, final[tag].Domains, want)
			}
		}
		if direct := final[targets.DirectResidualTag].Domains; !hasDomainRule(direct, model.DomainFull, "attr-direct.example", []string{"@cn"}) ||
			!hasDomainRule(direct, model.DomainRegexp, "^regex-direct\\.example$", nil) {
			t.Fatalf("direct residual does not preserve attrs/regexp: %#v", direct)
		}
	})

	t.Run("reject 指纹不受优化影响", func(t *testing.T) {
		if !reflect.DeepEqual(intermediate["reject"].Domains, final["reject"].Domains) {
			t.Fatal("reject changed")
		}
		groups, err := config.LoadGroups(filepath.Join(root, "groups.yaml"))
		if err != nil || groups[0].ID != "reject" {
			t.Fatalf("groups=%#v err=%v", groups, err)
		}
	})

	t.Run("中间 DAT 不污染最终探测缓存", func(t *testing.T) {
		for _, tag := range targets.ResidualGeoSiteTags() {
			present, err := prober.Has(context.Background(), model.GeoSite, tag)
			if err != nil || !present {
				t.Fatalf("%s present=%v err=%v", tag, present, err)
			}
		}
	})

	t.Run("最终 DAT 不含 BilibiliHMT", func(t *testing.T) {
		for _, side := range []model.Side{model.GeoSite, model.GeoIP} {
			present, err := prober.Has(context.Background(), side, "BilibiliHMT")
			if err != nil || present {
				t.Fatalf("%s present=%v err=%v", side, present, err)
			}
		}
	})

	publish := filepath.Join(root, "publish")
	if err := verify.Assets(publish, verify.ReleaseAssets()); err != nil {
		t.Fatal(err)
	}
	confBytes, err := os.ReadFile(filepath.Join(publish, "shadowrocket.conf"))
	if err != nil {
		t.Fatal(err)
	}
	conf := string(confBytes)
	for _, want := range []string{
		"DOMAIN-SUFFIX,youtube.example,Google 服务",
		"DOMAIN-SUFFIX,foreign,代理规则",
		"IP-CIDR,8.8.8.0/24,Google 服务,no-resolve",
		"IP-CIDR,192.0.2.0/24,中国大陆,no-resolve",
		"IP-CIDR6,2001:db8::/32,中国大陆,no-resolve",
	} {
		if !strings.Contains(conf, want+"\n") {
			t.Fatalf("conf missing %q:\n%s", want, conf)
		}
	}
	checksum, err := os.ReadFile(filepath.Join(publish, "shadowrocket.conf.sha256sum"))
	if err != nil || !strings.HasSuffix(string(checksum), "  shadowrocket.conf\n") {
		t.Fatalf("checksum=%q err=%v", checksum, err)
	}
	assertAppearsBefore(t, conf, ",YouTube\n", ",Google 服务\n")
	proxyGroupStart := strings.Index(conf, "[Proxy Group]\n")
	ruleStart := strings.Index(conf, "\n[Rule]\n")
	if proxyGroupStart < 0 || ruleStart < proxyGroupStart || strings.Count(conf[proxyGroupStart:ruleStart], " = select,") != 13 {
		t.Fatalf("Shadowrocket policy group count is not 13:\n%s", conf)
	}
	if strings.Contains(conf, "GFW") || strings.Contains(conf, "BilibiliHMT") || strings.Contains(conf, "非中国域名") {
		t.Fatalf("Shadowrocket contains removed group/tag:\n%s", conf)
	}
	if strings.Contains(conf, options.ReleaseTag) {
		t.Fatalf("conf contains release tag %q", options.ReleaseTag)
	}
	installer, err := os.ReadFile(filepath.Join(root, "publish", "install_passwall2_rules.sh"))
	if err != nil {
		t.Fatal(err)
	}
	fragment := decodeInstallerFragment(t, string(installer))
	assertAppearsBefore(t, fragment, "YouTube", "Google 服务")
	for _, want := range []string{
		"config shunt_rules 'crs_proxy'",
		"config shunt_rules 'crs_direct'",
		"geoip:google",
		"geosite:" + targets.ProxyResidualTag,
		"geosite:" + targets.DirectResidualTag,
	} {
		if !strings.Contains(fragment, want) {
			t.Fatalf("installer fragment missing %q:\n%s", want, fragment)
		}
	}
	for _, removed := range []string{"crs_tld_not_cn", "geosite:tld-not-cn", "geosite:gfw", "geosite:crs-proxy-residual", "geosite:crs-direct-residual"} {
		if strings.Contains(fragment, removed) {
			t.Fatalf("installer fragment contains removed %q:\n%s", removed, fragment)
		}
	}
}

func TestProductionDependenciesFailuresPreservePreviousGeneration(t *testing.T) {
	repositoryRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binRoot := filepath.Join(repositoryRoot, ".cache", "bin")
	requireIntegrationTools(t, binRoot)
	runner := &tools.Runner{BinRoot: binRoot}
	baseData, err := os.ReadFile(buildSyntheticBaseGeoIP(t, runner))
	if err != nil {
		t.Fatal(err)
	}
	payloads := integrationSourcePayloads()
	failPath := ""
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == failPath {
			http.NotFound(writer, request)
			return
		}
		if request.URL.Path == "/base.dat" {
			_, _ = writer.Write(baseData)
			return
		}
		payload, found := payloads[request.URL.Path]
		if !found {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write([]byte(payload))
	}))
	defer server.Close()
	client := fetch.NewWithHTTPClient(fetch.Options{}, server.Client())
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
		fail   string
		want   []string
	}{
		{
			name: "source HTTP failure",
			fail: "/proxy",
			want: []string{"fixture-proxy", "404"},
		},
		{
			name: "strict custom schema",
			mutate: func(t *testing.T, root string) {
				path := filepath.Join(root, "custom", "geosite", "proxy.yaml")
				if err := os.WriteFile(path, []byte("paylaod:\n  - DOMAIN-SUFFIX,example.test\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"proxy.yaml", "field paylaod not found"},
		},
		{
			name: "retired mixed-case community tag",
			mutate: func(t *testing.T, root string) {
				path := filepath.Join(root, "community", "CRS-DIRECT-RESIDUAL")
				if err := os.WriteFile(path, []byte("domain:collision.example\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"retired geosite:crs-direct-residual remains"},
		},
		{
			name: "retired nested mixed-case community tag",
			mutate: func(t *testing.T, root string) {
				directory := filepath.Join(root, "community", "nested")
				if err := os.Mkdir(directory, 0o700); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(directory, "CRS-DIRECT-RESIDUAL")
				if err := os.WriteFile(path, []byte("domain:collision.example\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"retired geosite:crs-direct-residual remains"},
		},
		{
			name: "retired mixed-case custom tag",
			mutate: func(t *testing.T, root string) {
				path := filepath.Join(root, "custom", "geosite", "Crs-Proxy-Residual.yaml")
				if err := os.WriteFile(path, []byte("payload:\n  - DOMAIN-SUFFIX,collision.example\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"Crs-Proxy-Residual.yaml", "unknown target"},
		},
		{
			name: "create collision",
			mutate: func(t *testing.T, root string) {
				path := filepath.Join(root, "community", "proxy")
				if err := os.WriteFile(path, []byte("domain:base-collision.example\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"fixture-proxy", "geosite:proxy", "create"},
		},
		{
			name: "missing group tag",
			mutate: func(t *testing.T, root string) {
				path := filepath.Join(root, "groups.yaml")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				data = append(data, []byte("  - id: broken\n    remarks: 坏组\n    geosite: []\n    geoip: [not-exist]\n")...)
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"坏组", "geoip:not-exist"},
		},
		{
			name: "final forbidden probe",
			mutate: func(t *testing.T, root string) {
				path := filepath.Join(root, "community", "loyalsoldier-google")
				if err := os.WriteFile(path, []byte("domain:legacy.example\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"forbidden tag exists", "geosite:loyalsoldier-google"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			failPath = tc.fail
			root := newIntegrationRoot(t, server.URL)
			if tc.mutate != nil {
				tc.mutate(t, root)
			}
			seedIntegrationGeneration(t, root, "old")
			err := app.Build(context.Background(), integrationBuildOptions(repositoryRoot, root), app.ProductionDependencies(client, runner))
			if err == nil {
				t.Fatal("build unexpectedly succeeded")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not contain %q", err, want)
				}
			}
			assertIntegrationGeneration(t, root, "old")
		})
	}
}

func integrationSourcePayloads() map[string]string {
	return map[string]string{
		"/reject":     "payload:\n  - '+.ads.example'\n",
		"/gfw":        "payload:\n  - '+.shared.proxy.example'\n  - '+.blocked.example'\n",
		"/proxy":      "payload:\n  - '+.shared.proxy.example'\n  - 'www.youtube.example'\n  - '+.kept-proxy.example'\n",
		"/direct":     "payload:\n  - 'covered.foreign'\n  - 'covered.cn'\n  - 'kept-direct.example'\n",
		"/tld-not-cn": "payload:\n  - '+.foreign'\n",
		"/ai":         "payload:\n  - '+.ai-source.example'\n",
	}
}

func hasDomainRule(rules []model.DomainRule, kind model.DomainKind, value string, attrs []string) bool {
	for _, rule := range rules {
		if rule.Kind == kind && rule.Value == value && reflect.DeepEqual(rule.Attrs, attrs) {
			return true
		}
	}
	return false
}

func readGeneratedRules(t *testing.T, path string) []model.DomainRule {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result []model.DomainRule
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		kind, value, found := strings.Cut(fields[0], ":")
		if !found {
			t.Fatalf("invalid generated rule %q", line)
		}
		result = append(result, model.DomainRule{Kind: model.DomainKind(kind), Value: value, Attrs: append([]string(nil), fields[1:]...)})
	}
	return result
}

func sameDomainRules(left, right []model.DomainRule) bool {
	key := func(rule model.DomainRule) string {
		return string(rule.Kind) + "\x00" + rule.Value + "\x00" + strings.Join(rule.Attrs, "\x00")
	}
	left = append([]model.DomainRule(nil), left...)
	right = append([]model.DomainRule(nil), right...)
	slices.SortFunc(left, func(a, b model.DomainRule) int { return strings.Compare(key(a), key(b)) })
	slices.SortFunc(right, func(a, b model.DomainRule) int { return strings.Compare(key(a), key(b)) })
	return reflect.DeepEqual(left, right)
}

func integrationBuildOptions(repositoryRoot, root string) app.BuildOptions {
	return app.BuildOptions{
		Root:       root,
		Sources:    filepath.Join(root, "sources.yaml"),
		Custom:     filepath.Join(root, "custom"),
		Groups:     filepath.Join(root, "groups.yaml"),
		Community:  filepath.Join(root, "community"),
		CacheRoot:  filepath.Join(repositoryRoot, ".cache"),
		Repo:       "example/clash-rules-srs",
		ReleaseTag: "integration-failure-test",
	}
}

func seedIntegrationGeneration(t *testing.T, root, value string) {
	t.Helper()
	for _, directory := range []string{"build", "publish"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, directory, "marker"), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func assertIntegrationGeneration(t *testing.T, root, value string) {
	t.Helper()
	for _, directory := range []string{"build", "publish"} {
		data, err := os.ReadFile(filepath.Join(root, directory, "marker"))
		if err != nil || string(data) != value {
			t.Fatalf("%s marker=%q err=%v", directory, data, err)
		}
	}
}

func requireIntegrationTools(t *testing.T, binRoot string) {
	t.Helper()
	for _, name := range []string{"domain-list-custom", "geoip", "geoview"} {
		path := filepath.Join(binRoot, name)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("integration tool %s missing; run: go run ./cmd/geodata-build bootstrap --cache-root .cache", path)
		}
	}
}

func buildSyntheticBaseGeoIP(t *testing.T, runner *tools.Runner) string {
	t.Helper()
	root := t.TempDir()
	inputRoot := filepath.Join(root, "input")
	if err := os.MkdirAll(inputRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"cn":       "192.0.2.0/24\n2001:db8::/32\n",
		"google":   "8.8.8.0/24\n",
		"netflix":  "198.51.100.0/24\n",
		"private":  "10.0.0.0/8\n",
		"telegram": "149.154.160.0/20\n",
		"us":       "203.0.113.0/24\n",
	} {
		if err := os.WriteFile(filepath.Join(inputRoot, name+".txt"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	inputs := make([]map[string]any, 0, 6)
	for _, name := range []string{"cn", "google", "netflix", "private", "telegram", "us"} {
		inputs = append(inputs, map[string]any{
			"type": "text", "action": "add",
			"args": map[string]any{"name": name, "uri": filepath.Join(inputRoot, name+".txt")},
		})
	}
	configuration := map[string]any{
		"input": inputs,
		"output": []map[string]any{{
			"type": "v2rayGeoIPDat", "action": "output",
			"args": map[string]any{"outputDir": root, "outputName": "base.dat"},
		}},
	}
	data, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "geoip.json")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(context.Background(), "geoip", root, "convert", "-c", configPath); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "base.dat")
}

func newIntegrationRoot(t *testing.T, serverURL string) string {
	t.Helper()
	root := t.TempDir()
	for _, directory := range []string{"community", "custom/geosite", "custom/geoip", "config"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := filepath.WalkDir("testdata/community", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, "community", filepath.Base(path)), data, 0o600)
	}); err != nil {
		t.Fatal(err)
	}
	for source, destination := range map[string]string{
		"testdata/sources.yaml":              "sources.yaml",
		"testdata/groups.yaml":               "groups.yaml",
		"testdata/custom/geosite/proxy.yaml": "custom/geosite/proxy.yaml",
	} {
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		data = []byte(strings.ReplaceAll(string(data), "{{SERVER}}", serverURL))
		if err := os.WriteFile(filepath.Join(root, destination), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	template := fmt.Sprintf(`{"input":[{"type":"v2rayGeoIPDat","action":"add","args":{"uri":%q}}],"output":[{"type":"v2rayGeoIPDat","action":"output","args":{"outputDir":"publish","outputName":"geoip.dat"}}]}`, serverURL+"/base.dat")
	if err := os.WriteFile(filepath.Join(root, "config", "geoip.base.json"), []byte(template), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func readIntegrationManifest(t *testing.T, path string) manifest.Document {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document manifest.Document
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func sortedIPTagKeys(tags map[string]geodat.IPTag) []string {
	keys := make([]string, 0, len(tags))
	for tag := range tags {
		keys = append(keys, tag)
	}
	slices.Sort(keys)
	return keys
}

func assertFileContains(t *testing.T, path, value string, count int) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), value) != count {
		t.Fatalf("%s count in %s = %d, want %d\n%s", value, path, strings.Count(string(data), value), count, data)
	}
}

func assertAppearsBefore(t *testing.T, text, first, second string) {
	t.Helper()
	firstIndex := strings.Index(text, first)
	secondIndex := strings.Index(text, second)
	if firstIndex < 0 || secondIndex < 0 {
		t.Fatalf("missing order marker first=%q(%d) second=%q(%d)", first, firstIndex, second, secondIndex)
	}
	if firstIndex >= secondIndex {
		t.Fatalf("%q does not precede %q", first, second)
	}
}

func assertOnlyFileContains(t *testing.T, root, value, included, excluded string) {
	t.Helper()
	assertFileContains(t, filepath.Join(root, included), value, 1)
	data, err := os.ReadFile(filepath.Join(root, excluded))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), value) {
		t.Fatalf("%s unexpectedly appears in %s", value, excluded)
	}
}

func decodeInstallerFragment(t *testing.T, installer string) string {
	t.Helper()
	const prefix = "FRAGMENT_B64='"
	start := strings.Index(installer, prefix)
	if start < 0 {
		t.Fatal("installer has no FRAGMENT_B64")
	}
	encoded := installer[start+len(prefix):]
	end := strings.IndexByte(encoded, '\'')
	if end < 0 {
		t.Fatal("installer FRAGMENT_B64 is not terminated")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded[:end])
	if err != nil {
		t.Fatal(err)
	}
	return string(decoded)
}

func assertLookupContains(t *testing.T, runner *tools.Runner, side, dat, value, required string, forbidden ...string) {
	t.Helper()
	output, err := runner.Output(context.Background(), "geoview", "",
		"-type", side, "-action", "lookup", "-input", dat, "-value", value,
	)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Fields(output)
	contains := func(tag string) bool {
		return slices.ContainsFunc(lines, func(line string) bool { return strings.EqualFold(line, tag) })
	}
	if !contains(required) {
		t.Fatalf("lookup %s %s does not contain %s: %q", side, value, required, output)
	}
	for _, tag := range forbidden {
		if contains(tag) {
			t.Fatalf("lookup %s %s unexpectedly contains %s: %q", side, value, tag, output)
		}
	}
}
