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
	"slices"
	"strings"
	"testing"

	"clash-rules-srs/internal/app"
	"clash-rules-srs/internal/fetch"
	"clash-rules-srs/internal/manifest"
	"clash-rules-srs/internal/tools"
	"clash-rules-srs/internal/verify"
)

func TestSyntheticFullBuildMergesAndSeparatesApprovedTags(t *testing.T) {
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

	sourcePayloads := map[string]string{
		"/google":   "payload:\n  - '+.googleapis.com'\n  - '+.google-source.example'\n",
		"/youtube":  "payload:\n  - DOMAIN,youtubei.googleapis.com\n",
		"/netflix":  "payload:\n  - DOMAIN-SUFFIX,netflix-source.example\n  - IP-CIDR,198.51.100.0/24\n  - IP-CIDR,198.51.100.0/25,no-resolve\n",
		"/bilibili": "payload:\n  - DOMAIN-SUFFIX,hmt-source.example\n  - IP-CIDR,203.0.113.0/24\n",
	}
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
	for _, tag := range []string{"google", "youtube", "netflix", "BilibiliHMT"} {
		if !slices.Contains(document.Required.GeoSite, tag) {
			t.Fatalf("missing geosite:%s", tag)
		}
	}
	for _, tag := range []string{"netflix", "BilibiliHMT"} {
		if !slices.Contains(document.Required.GeoIP, tag) {
			t.Fatalf("missing geoip:%s", tag)
		}
	}
	for _, old := range []string{"loyalsoldier-google", "xiaolin-youtube", "xiaolin-bilibili"} {
		if !slices.Contains(document.Forbidden.GeoSite, old) {
			t.Fatalf("legacy geosite tag is not forbidden: %s", old)
		}
	}
	if !slices.Contains(document.Forbidden.GeoIP, "xiaolin-bilibili") {
		t.Fatal("legacy geoip tag is not forbidden")
	}

	mergedRoot := filepath.Join(root, "build", "data-merged")
	assertFileContains(t, filepath.Join(mergedRoot, "google"), "domain:googleapis.com", 1)
	assertFileContains(t, filepath.Join(mergedRoot, "youtube"), "full:youtubei.googleapis.com", 1)
	assertOnlyFileContains(t, mergedRoot, "hmt-custom.example", "BilibiliHMT", "bilibili")
	assertFileContains(t, filepath.Join(root, "build", "ip", "netflix.txt"), "198.51.100.0/24", 1)
	assertFileContains(t, filepath.Join(root, "build", "ip", "netflix.txt"), "198.51.100.0/25", 1)
	geoSiteDat := filepath.Join(root, "publish", "geosite.dat")
	geoIPDat := filepath.Join(root, "publish", "geoip.dat")
	assertLookupContains(t, runner, "geosite", geoSiteDat, "hmt-custom.example", "BilibiliHMT", "bilibili")
	assertLookupContains(t, runner, "geoip", geoIPDat, "198.51.100.1", "netflix")
	assertLookupContains(t, runner, "geoip", geoIPDat, "198.51.100.200", "netflix")
	assertLookupContains(t, runner, "geoip", geoIPDat, "203.0.113.1", "BilibiliHMT", "bilibili")

	prober := verify.NewProber(runner, geoSiteDat, geoIPDat)
	if err := verify.Required(context.Background(), prober, document); err != nil {
		t.Fatal(err)
	}
	if err := verify.Forbidden(context.Background(), prober, document); err != nil {
		t.Fatal(err)
	}
	if err := verify.Assets(filepath.Join(root, "publish"), []string{
		"geoip.dat", "geoip.dat.sha256sum", "geosite.dat", "geosite.dat.sha256sum",
		"install_passwall2_rules.sh", "install_passwall2_rules.sh.sha256sum",
	}); err != nil {
		t.Fatal(err)
	}
	installer, err := os.ReadFile(filepath.Join(root, "publish", "install_passwall2_rules.sh"))
	if err != nil {
		t.Fatal(err)
	}
	fragment := decodeInstallerFragment(t, string(installer))
	if strings.Index(fragment, "YouTube") >= strings.Index(fragment, "Google 服务") {
		t.Fatalf("YouTube does not precede Google 服务")
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
	payloads := map[string]string{
		"/google":   "payload:\n  - '+.google-source.example'\n",
		"/youtube":  "payload:\n  - DOMAIN,youtubei.googleapis.com\n",
		"/netflix":  "payload:\n  - DOMAIN-SUFFIX,netflix-source.example\n  - IP-CIDR,198.51.100.0/24\n",
		"/bilibili": "payload:\n  - DOMAIN-SUFFIX,hmt-source.example\n  - IP-CIDR,203.0.113.0/24\n",
	}
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
			fail: "/google",
			want: []string{"loyalsoldier-google", "404"},
		},
		{
			name: "strict custom schema",
			mutate: func(t *testing.T, root string) {
				path := filepath.Join(root, "custom", "geosite", "BilibiliHMT.yaml")
				if err := os.WriteFile(path, []byte("paylaod:\n  - DOMAIN-SUFFIX,example.test\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"BilibiliHMT.yaml", "unknown field"},
		},
		{
			name: "create collision",
			mutate: func(t *testing.T, root string) {
				path := filepath.Join(root, "sources.yaml")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				data = []byte(strings.Replace(string(data), "{tag: google, mode: merge-base}", "{tag: google, mode: create}", 1))
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"loyalsoldier-google", "geosite:google", "create"},
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
		"cn":      "192.0.2.0/24\n",
		"private": "10.0.0.0/8\n",
		"netflix": "198.51.100.0/24\n",
	} {
		if err := os.WriteFile(filepath.Join(inputRoot, name+".txt"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	inputs := make([]map[string]any, 0, 3)
	for _, name := range []string{"cn", "private", "netflix"} {
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
		"testdata/sources.yaml":                    "sources.yaml",
		"testdata/groups.yaml":                     "groups.yaml",
		"testdata/custom/geosite/BilibiliHMT.yaml": "custom/geosite/BilibiliHMT.yaml",
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
