package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"clash-rules-srs/internal/geodat"
	"clash-rules-srs/internal/manifest"
	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/targets"
	"clash-rules-srs/internal/verify"
	"clash-rules-srs/internal/workspace"
)

func TestBuildEmitsAndProbesEveryOutput(t *testing.T) {
	fixture := newBuildFixture(t)
	google := model.Output{Tag: "google", Mode: model.MergeBase}
	netflixSite := model.Output{Tag: "netflix", Mode: model.MergeBase}
	netflixIP := model.Output{Tag: "netflix", Mode: model.MergeBase}
	fixture.sources = []model.Source{
		{ID: "loyalsoldier-google", Outputs: model.Outputs{GeoSite: &google}},
		{ID: "xiaolin-netflix", Outputs: model.Outputs{GeoSite: &netflixSite, GeoIP: &netflixIP}},
	}
	if err := Build(context.Background(), fixture.options(), fixture.dependencies()); err != nil {
		t.Fatal(err)
	}
	for _, call := range []string{"geosite:google", "geosite:netflix", "geoip:netflix"} {
		if !slices.Contains(fixture.probes, call) {
			t.Fatalf("missing probe %s in %v", call, fixture.probes)
		}
	}
	if slices.Contains(fixture.required.GeoSite, "loyalsoldier-google") {
		t.Fatalf("legacy tag in manifest: %#v", fixture.required)
	}
	if err := verify.Assets(filepath.Join(fixture.root, "publish"), verify.ReleaseAssets()); err != nil {
		t.Fatal(err)
	}
}

func TestBuildSourceFailureDoesNotSwitchPublish(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.seedPublished("old")
	fixture.failStage = "fetch and parse sources"
	fixture.failError = errors.New("HTTP 404")
	err := Build(context.Background(), fixture.options(), fixture.dependencies())
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("err=%v", err)
	}
	fixture.assertBuildAndPublished("old")
}

func TestBuildUnknownCustomTargetDoesNotSwitchPublish(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.seedPublished("old")
	fixture.failStage = "load custom rules"
	fixture.failError = errors.New("custom/geosite/googel.yaml: unknown target geosite:googel")
	err := Build(context.Background(), fixture.options(), fixture.dependencies())
	if err == nil || !strings.Contains(err.Error(), "geosite:googel") {
		t.Fatalf("err=%v", err)
	}
	fixture.assertBuildAndPublished("old")
}

func TestBuildMissingGroupTagDoesNotPublishInstaller(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.seedPublished("old")
	fixture.failStage = "validate passwall groups"
	fixture.failError = errors.New("group 坏组 references missing geoip:not-exist")
	err := Build(context.Background(), fixture.options(), fixture.dependencies())
	if err == nil || !strings.Contains(err.Error(), "坏组") || !strings.Contains(err.Error(), "geoip:not-exist") {
		t.Fatalf("err=%v", err)
	}
	fixture.assertBuildAndPublished("old")
}

func TestBuildCreateCollisionDoesNotSwitchPublish(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.seedPublished("old")
	fixture.failStage = "validate target registry"
	fixture.failError = errors.New("source-a geosite:collision violates create mode")
	err := Build(context.Background(), fixture.options(), fixture.dependencies())
	if err == nil || !strings.Contains(err.Error(), "source-a") || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("err=%v", err)
	}
	fixture.assertBuildAndPublished("old")
}

func TestBuildForbiddenProbeFailurePreservesOldOutputs(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.seedPublished("old")
	fixture.failStage = "probe required and forbidden tags"
	fixture.failError = errors.New("forbidden tag exists: geosite:loyalsoldier-google")
	err := Build(context.Background(), fixture.options(), fixture.dependencies())
	if err == nil || !strings.Contains(err.Error(), "loyalsoldier-google") {
		t.Fatalf("err=%v", err)
	}
	fixture.assertBuildAndPublished("old")
}

func Test展开失败则不发布(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.seedPublished("old")
	fixture.failStage = "render shadowrocket conf and checksum"
	fixture.failError = errors.New("group \"坏组\" references missing geosite:not-exist")
	err := Build(context.Background(), fixture.options(), fixture.dependencies())
	if err == nil || !strings.Contains(err.Error(), "坏组") || !strings.Contains(err.Error(), "geosite:not-exist") {
		t.Fatalf("err=%v", err)
	}
	fixture.assertBuildAndPublished("old")
}

func TestSkipCompile不生成残差发布物(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.seedPublished("old")
	options := fixture.options()
	options.SkipCompile = true
	if err := Build(context.Background(), options, fixture.dependencies()); err != nil {
		t.Fatal(err)
	}
	fixture.assertBuildAndPublished("old")
	for _, forbidden := range []string{
		"compile intermediate geosite",
		"derive residual geosite",
		"compile final geosite",
		"verify full geosite tags",
		"render shadowrocket conf and checksum",
	} {
		if slices.Contains(fixture.calls, forbidden) {
			t.Fatalf("%s ran: %v", forbidden, fixture.calls)
		}
	}
}

func Test中间DAT不污染最终探测缓存(t *testing.T) {
	fixture := newBuildFixture(t)
	if err := Build(context.Background(), fixture.options(), fixture.dependencies()); err != nil {
		t.Fatal(err)
	}
	wantOrder := []string{
		"compile intermediate geosite",
		"derive residual geosite",
		"compile final geosite",
		"compile geoip",
		"verify full geosite tags",
		"validate passwall groups",
		"probe required and forbidden tags",
	}
	last := -1
	for _, name := range wantOrder {
		index := slices.Index(fixture.calls, name)
		if index <= last {
			t.Fatalf("stage %q out of order: %v", name, fixture.calls)
		}
		last = index
	}
}

func TestCompilerGeoSite输出在发布前规范化(t *testing.T) {
	left := map[string]geodat.SiteTag{"test": {
		Name: "TEST",
		Domains: []model.DomainRule{
			{Kind: model.DomainSuffix, Value: "z.example"},
			{Kind: model.DomainFull, Value: "a.example"},
		},
	}}
	right := cloneSiteTags(left)
	right["test"] = geodat.SiteTag{Name: "TEST", Domains: []model.DomainRule{
		{Kind: model.DomainFull, Value: "a.example"},
		{Kind: model.DomainSuffix, Value: "z.example"},
	}}
	paths := []string{filepath.Join(t.TempDir(), "left.dat"), filepath.Join(t.TempDir(), "right.dat")}
	for index, tags := range []map[string]geodat.SiteTag{left, right} {
		if err := os.WriteFile(paths[index], encodeAppTestSiteDAT(tags), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := canonicalizeCompiledGeoSite(paths[index]); err != nil {
			t.Fatal(err)
		}
	}
	leftBytes, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	rightBytes, err := os.ReadFile(paths[1])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(leftBytes, rightBytes) {
		t.Fatal("compiler output was not canonicalized")
	}
}

func TestCompilerGeoSite未知字段拒绝且文件不变(t *testing.T) {
	path := filepath.Join(t.TempDir(), "geosite.dat")
	data := encodeAppTestSiteDAT(map[string]geodat.SiteTag{"test": {
		Name: "TEST", Domains: []model.DomainRule{{Kind: model.DomainSuffix, Value: "kept.example"}},
	}})
	data = binary.AppendUvarint(data, 99<<3)
	data = binary.AppendUvarint(data, 7)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	err := canonicalizeCompiledGeoSite(path)
	if err == nil || !strings.Contains(err.Error(), "unknown") || !strings.Contains(err.Error(), "99") {
		t.Fatalf("err=%v", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(after, data) {
		t.Fatal("failed canonicalization replaced compiler output")
	}
}

func Test未知GeoSite字段不切换Publish(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.seedPublished("old")
	fixture.failStage = "compile final geosite"
	fixture.failError = errors.New("canonicalize compiled geosite: geosite document contains unknown field 99")
	err := Build(context.Background(), fixture.options(), fixture.dependencies())
	if err == nil || !strings.Contains(err.Error(), "unknown field 99") {
		t.Fatalf("err=%v", err)
	}
	fixture.assertBuildAndPublished("old")
}

func Test完整标签与Reject指纹门禁(t *testing.T) {
	baseline := map[string]geodat.SiteTag{}
	for _, tag := range []string{"gfw", "proxy", "tld-not-cn", "direct", "reject"} {
		baseline[tag] = geodat.SiteTag{Name: strings.ToUpper(tag), Domains: []model.DomainRule{{
			Kind: model.DomainSuffix, Value: tag + ".example", Attrs: []string{"@cn"},
		}}}
	}
	baseline["reject"] = geodat.SiteTag{Name: "REJECT", Domains: []model.DomainRule{
		{Kind: model.DomainSuffix, Value: "ads-one.example", Attrs: []string{"@cn"}},
		{Kind: model.DomainFull, Value: "ads-two.example"},
	}}
	baseline[targets.ProxyResidualTag] = geodat.SiteTag{
		Name: targets.ProxyResidualTag, Domains: []model.DomainRule{
			{Kind: model.DomainFull, Value: "proxy-residual.example", Attrs: []string{"@cn"}},
			{Kind: model.DomainRegexp, Value: "^proxy-residual\\.example$"},
		},
	}
	baseline[targets.DirectResidualTag] = geodat.SiteTag{
		Name: targets.DirectResidualTag, Domains: []model.DomainRule{
			{Kind: model.DomainFull, Value: "direct-residual.example", Attrs: []string{"@cn"}},
			{Kind: model.DomainRegexp, Value: "^direct-residual\\.example$"},
		},
	}

	writeFinal := func(tags map[string]geodat.SiteTag) string {
		path := filepath.Join(t.TempDir(), "geosite.dat")
		if err := os.WriteFile(path, encodeAppTestSiteDAT(tags), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	if err := verifyFullGeoSiteTags(writeFinal(baseline), baseline); err != nil {
		t.Fatal(err)
	}

	for _, mutation := range []struct {
		name string
		tag  string
		rule model.DomainRule
	}{
		{name: "kind", tag: "gfw", rule: model.DomainRule{Kind: model.DomainFull, Value: "gfw.example", Attrs: []string{"@cn"}}},
		{name: "value", tag: "proxy", rule: model.DomainRule{Kind: model.DomainSuffix, Value: "changed.example", Attrs: []string{"@cn"}}},
		{name: "tld value", tag: "tld-not-cn", rule: model.DomainRule{Kind: model.DomainSuffix, Value: "changed.example", Attrs: []string{"@cn"}}},
		{name: "attrs", tag: "direct", rule: model.DomainRule{Kind: model.DomainSuffix, Value: "direct.example", Attrs: []string{"@other"}}},
		{name: "order", tag: "reject"},
		{name: "generated kind", tag: targets.ProxyResidualTag, rule: model.DomainRule{Kind: model.DomainSuffix, Value: "proxy-residual.example", Attrs: []string{"@cn"}}},
		{name: "generated attrs", tag: targets.DirectResidualTag, rule: model.DomainRule{Kind: model.DomainFull, Value: "direct-residual.example", Attrs: []string{"@other"}}},
		{name: "generated missing", tag: targets.ProxyResidualTag, rule: model.DomainRule{Kind: model.DomainFull, Value: "proxy-residual.example", Attrs: []string{"@cn"}}},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			changed := cloneSiteTags(baseline)
			if strings.Contains(mutation.name, "order") {
				rules := changed[mutation.tag].Domains
				changed[mutation.tag] = geodat.SiteTag{Name: strings.ToUpper(mutation.tag), Domains: []model.DomainRule{rules[1], rules[0]}}
			} else {
				changed[mutation.tag] = geodat.SiteTag{Name: strings.ToUpper(mutation.tag), Domains: []model.DomainRule{mutation.rule}}
			}
			err := verifyFullGeoSiteTags(writeFinal(changed), baseline)
			if err == nil || !strings.Contains(err.Error(), mutation.tag) {
				t.Fatalf("err=%v", err)
			}
		})
	}

	for _, tag := range []string{"gfw", "tld-not-cn"} {
		t.Run("missing "+tag, func(t *testing.T) {
			changed := cloneSiteTags(baseline)
			delete(changed, tag)
			err := verifyFullGeoSiteTags(writeFinal(changed), baseline)
			if err == nil || !strings.Contains(err.Error(), tag) {
				t.Fatalf("err=%v", err)
			}
		})
	}

	t.Run("generated compiler order", func(t *testing.T) {
		changed := cloneSiteTags(baseline)
		rules := changed[targets.ProxyResidualTag].Domains
		changed[targets.ProxyResidualTag] = geodat.SiteTag{Name: targets.ProxyResidualTag, Domains: []model.DomainRule{rules[1], rules[0]}}
		if err := verifyFullGeoSiteTags(writeFinal(changed), baseline); err != nil {
			t.Fatalf("compiler order should not change the semantic fingerprint: %v", err)
		}
	})
}

func Test残差拓扑跟随实际Groups顺序(t *testing.T) {
	groups := []model.Group{
		{ID: "reject", GeoSite: []string{"reject"}},
		{ID: "google", GeoSite: []string{"google", "youtube"}},
		{ID: "proxy", GeoSite: []string{targets.ProxyResidualTag, "tld-not-cn"}},
		{ID: "private", GeoSite: []string{"private"}},
		{ID: "direct", GeoSite: []string{targets.DirectResidualTag}},
	}
	proxy, direct, err := residualSpecsForGroups(groups)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"reject", "google", "youtube"}; !slices.Equal(proxy.EarlierActiveTags, want) {
		t.Fatalf("proxy earlier=%v want=%v", proxy.EarlierActiveTags, want)
	}
	if want := []string{"tld-not-cn"}; !slices.Equal(proxy.LaterEquivalentTags, want) {
		t.Fatalf("proxy equivalent=%v want=%v", proxy.LaterEquivalentTags, want)
	}
	if want := []string{"reject", "google", "youtube", targets.ProxyResidualTag, "tld-not-cn", "private"}; !slices.Equal(direct.EarlierActiveTags, want) {
		t.Fatalf("direct earlier=%v want=%v", direct.EarlierActiveTags, want)
	}
}

func TestProxy同组标签才是等价覆盖(t *testing.T) {
	t.Run("独立后序组不算等价覆盖", func(t *testing.T) {
		groups := []model.Group{
			{ID: "proxy", GeoSite: []string{targets.ProxyResidualTag}},
			{ID: "tld_not_cn", GeoSite: []string{"tld-not-cn"}},
			{ID: "direct", GeoSite: []string{targets.DirectResidualTag}},
		}
		proxy, _, err := residualSpecsForGroups(groups)
		if err != nil {
			t.Fatal(err)
		}
		if len(proxy.LaterEquivalentTags) != 0 {
			t.Fatalf("proxy must not borrow coverage from an independent group: %v", proxy.LaterEquivalentTags)
		}
	})

	t.Run("同组伴随标签算等价覆盖", func(t *testing.T) {
		groups := []model.Group{
			{ID: "proxy", GeoSite: []string{targets.ProxyResidualTag, "tld-not-cn"}},
			{ID: "direct", GeoSite: []string{targets.DirectResidualTag}},
		}
		proxy, _, err := residualSpecsForGroups(groups)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"tld-not-cn"}; !slices.Equal(proxy.LaterEquivalentTags, want) {
			t.Fatalf("proxy equivalent=%v want=%v", proxy.LaterEquivalentTags, want)
		}
	})
}

func TestCommunity标签查找不区分大小写(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "CRS-DIRECT-RESIDUAL"), []byte("domain:collision.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	exists, err := regularFileEqualFold(root, targets.DirectResidualTag)
	if err != nil || !exists {
		t.Fatalf("exists=%v err=%v", exists, err)
	}
}

func cloneSiteTags(input map[string]geodat.SiteTag) map[string]geodat.SiteTag {
	result := make(map[string]geodat.SiteTag, len(input))
	for key, tag := range input {
		tag.Domains = append([]model.DomainRule(nil), tag.Domains...)
		result[key] = tag
	}
	return result
}

func encodeAppTestSiteDAT(tags map[string]geodat.SiteTag) []byte {
	names := make([]string, 0, len(tags))
	for name := range tags {
		names = append(names, name)
	}
	sort.Strings(names)
	var document []byte
	for _, name := range names {
		tag := tags[name]
		entry := appTestFieldBytes(1, []byte(tag.Name))
		for _, rule := range tag.Domains {
			kind := map[model.DomainKind]uint64{
				model.DomainKeyword: 0,
				model.DomainRegexp:  1,
				model.DomainSuffix:  2,
				model.DomainFull:    3,
			}[rule.Kind]
			domain := appTestFieldVarint(1, kind)
			domain = append(domain, appTestFieldBytes(2, []byte(rule.Value))...)
			for _, attr := range rule.Attrs {
				attribute := appTestFieldBytes(1, []byte(strings.TrimPrefix(attr, "@")))
				attribute = append(attribute, appTestFieldVarint(2, 1)...)
				domain = append(domain, appTestFieldBytes(3, attribute)...)
			}
			entry = append(entry, appTestFieldBytes(2, domain)...)
		}
		document = append(document, appTestFieldBytes(1, entry)...)
	}
	return document
}

func appTestFieldBytes(number uint64, value []byte) []byte {
	result := binary.AppendUvarint(nil, number<<3|2)
	result = binary.AppendUvarint(result, uint64(len(value)))
	return append(result, value...)
}

func appTestFieldVarint(number, value uint64) []byte {
	result := binary.AppendUvarint(nil, number<<3)
	return binary.AppendUvarint(result, value)
}

func TestCachedTagLookupProbesEachExactTagOnce(t *testing.T) {
	delegate := &countingTagLookup{}
	lookup := &cachedTagLookup{delegate: delegate, results: make(map[tagLookupKey]tagLookupResult)}
	for range 2 {
		present, err := lookup.Has(context.Background(), model.GeoSite, "BilibiliHMT")
		if err != nil || !present {
			t.Fatalf("present=%v err=%v", present, err)
		}
	}
	if delegate.calls != 1 {
		t.Fatalf("calls=%d, want 1", delegate.calls)
	}
	if _, err := lookup.Has(context.Background(), model.GeoSite, "bilibilihmt"); err != nil {
		t.Fatal(err)
	}
	if delegate.calls != 2 {
		t.Fatalf("case-distinct tag reused cache: calls=%d", delegate.calls)
	}
}

func TestProductionDependenciesReuseOneValidatedGeoIPTemplate(t *testing.T) {
	root := t.TempDir()
	templatePath := filepath.Join(root, "config", "geoip.base.json")
	writeAppTestFile(t, templatePath, `{
  "input": [{"type":"v2rayGeoIPDat","action":"add","args":{"uri":"https://example.test/base.dat"}}],
  "output": [{"type":"v2rayGeoIPDat","action":"output","args":{"outputDir":"unused","outputName":"unused.dat"}}]
}`)
	tx, err := workspace.Begin(root)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Abort()
	state := &buildState{Options: BuildOptions{Root: root}, Tx: tx}
	dependencies := ProductionDependencies(staticSourceFetcher{data: []byte("base")}, nil)
	if err := dependencies.FetchBaseGeoIP(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	writeAppTestFile(t, templatePath, "not valid JSON")
	if err := dependencies.PrepareGeoIP(context.Background(), state); err != nil {
		t.Fatalf("template was parsed more than once: %v", err)
	}
}

type staticSourceFetcher struct {
	data []byte
}

func (f staticSourceFetcher) Get(context.Context, string, int64) ([]byte, error) {
	return append([]byte(nil), f.data...), nil
}

func writeAppTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

type countingTagLookup struct {
	calls int
}

func (l *countingTagLookup) Has(context.Context, model.Side, string) (bool, error) {
	l.calls++
	return true, nil
}

type buildFixture struct {
	t         *testing.T
	root      string
	sources   []model.Source
	groups    []model.Group
	calls     []string
	probes    []string
	required  manifest.Tags
	failStage string
	failError error
}

func newBuildFixture(t *testing.T) *buildFixture {
	t.Helper()
	return &buildFixture{t: t, root: t.TempDir()}
}

func (f *buildFixture) options() BuildOptions {
	return BuildOptions{Root: f.root, Repo: "flame/repo", ReleaseTag: "v1"}
}

func (f *buildFixture) dependencies() Dependencies {
	stage := func(name string, action func(*buildState) error) buildStage {
		return func(_ context.Context, state *buildState) error {
			f.calls = append(f.calls, name)
			if f.failStage == name {
				return f.failError
			}
			if action != nil {
				return action(state)
			}
			return nil
		}
	}
	return Dependencies{
		Begin: workspace.Begin,
		LoadConfig: stage("load config", func(state *buildState) error {
			state.Sources = f.sources
			state.Groups = f.groups
			return nil
		}),
		FetchBaseGeoIP: stage("fetch base geoip", nil),
		BuildRegistry:  stage("validate target registry", nil),
		FetchRemote:    stage("fetch and parse sources", nil),
		LoadCustom:     stage("load custom rules", nil),
		MergeGeoSite:   stage("merge geosite", nil),
		PrepareGeoIP:   stage("prepare geoip", nil),
		WriteManifest: stage("write manifest", func(state *buildState) error {
			state.Manifest = manifest.Build(state.Sources, state.Groups)
			f.required = state.Manifest.Required
			return manifest.Write(state.Tx.Layout().Manifest, state.Manifest)
		}),
		CompileIntermediateGeoSite: stage("compile intermediate geosite", func(state *buildState) error {
			return os.WriteFile(state.Tx.Layout().IntermediateGeoSite, []byte("intermediate-site"), 0o600)
		}),
		DeriveResidualGeoSite: stage("derive residual geosite", nil),
		CompileGeoSite: stage("compile final geosite", func(state *buildState) error {
			return os.WriteFile(filepath.Join(state.Tx.Layout().Publish, "geosite.dat"), []byte("site"), 0o600)
		}),
		CompileGeoIP: stage("compile geoip", func(state *buildState) error {
			return os.WriteFile(filepath.Join(state.Tx.Layout().Publish, "geoip.dat"), []byte("ip"), 0o600)
		}),
		VerifyFullGeoSiteTags: stage("verify full geosite tags", nil),
		ProbeTags: stage("probe required and forbidden tags", func(state *buildState) error {
			for _, tag := range state.Manifest.Required.GeoSite {
				f.probes = append(f.probes, "geosite:"+tag)
			}
			for _, tag := range state.Manifest.Required.GeoIP {
				f.probes = append(f.probes, "geoip:"+tag)
			}
			return nil
		}),
		ValidateGroups: stage("validate passwall groups", nil),
		RenderInstaller: stage("render installer and checksums", func(state *buildState) error {
			script := filepath.Join(state.Tx.Layout().Publish, "install_passwall2_rules.sh")
			if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o700); err != nil {
				return err
			}
			for _, name := range []string{"geosite.dat", "geoip.dat", "install_passwall2_rules.sh"} {
				if _, err := verify.WriteSHA256(filepath.Join(state.Tx.Layout().Publish, name)); err != nil {
					return err
				}
			}
			return nil
		}),
		RenderShadowrocket: stage("render shadowrocket conf and checksum", func(state *buildState) error {
			path := filepath.Join(state.Tx.Layout().Publish, "shadowrocket.conf")
			if err := os.WriteFile(path, []byte("[General]\n"), 0o600); err != nil {
				return err
			}
			_, err := verify.WriteSHA256(path)
			return err
		}),
		VerifyReleaseAssets: stage("verify release assets", func(state *buildState) error {
			return verify.Assets(state.Tx.Layout().Publish, verify.ReleaseAssets())
		}),
	}
}

func (f *buildFixture) seedPublished(value string) {
	f.t.Helper()
	for _, directory := range []string{"build", "publish"} {
		path := filepath.Join(f.root, directory)
		if err := os.MkdirAll(path, 0o755); err != nil {
			f.t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "marker"), []byte(value), 0o600); err != nil {
			f.t.Fatal(err)
		}
	}
}

func (f *buildFixture) assertBuildAndPublished(value string) {
	f.t.Helper()
	for _, directory := range []string{"build", "publish"} {
		data, err := os.ReadFile(filepath.Join(f.root, directory, "marker"))
		if err != nil || string(data) != value {
			f.t.Fatalf("%s marker=%q err=%v", directory, data, err)
		}
	}
}
