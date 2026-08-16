package geosite_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clash-rules-srs/internal/geosite"
	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/targets"
)

func TestMergeDeduplicatesExactRulesButPreservesKindAndAttrs(t *testing.T) {
	out := filepath.Join(t.TempDir(), "merged")
	inputs := []model.Contribution{{SourceID: "custom", Side: model.GeoSite, Tag: "google", Domains: []model.DomainRule{
		{Kind: "domain", Value: "example.com"},
		{Kind: "full", Value: "example.com"},
		{Kind: "domain", Value: "example.com", Attrs: []string{"@cn"}},
	}}}
	if err := geosite.Merge("testdata/community", out, inputs); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(filepath.Join(out, "google"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(text), "domain:example.com\n") != 1 || !bytes.Contains(text, []byte("full:example.com\n")) || !bytes.Contains(text, []byte("domain:example.com @cn\n")) {
		t.Fatalf("merged=%s", text)
	}
	if !bytes.Contains(text, []byte("include:youtube")) || !bytes.Contains(text, []byte("# keep comment")) {
		t.Fatalf("community directives lost: %s", text)
	}
}

func TestMergeKeepsGoogleYouTubeAndProxyTargetsIndependent(t *testing.T) {
	out := filepath.Join(t.TempDir(), "merged")
	inputs := []model.Contribution{
		{SourceID: "youtube", Side: model.GeoSite, Tag: "youtube", Domains: []model.DomainRule{{Kind: "full", Value: "youtubei.googleapis.com"}}},
		{SourceID: "google", Side: model.GeoSite, Tag: "google", Domains: []model.DomainRule{{Kind: "domain", Value: "googleapis.com"}}},
		{SourceID: "proxy", Side: model.GeoSite, Tag: "proxy", Domains: []model.DomainRule{{Kind: "domain", Value: "proxy-only.example"}}},
	}
	if err := geosite.Merge("testdata/community", out, inputs); err != nil {
		t.Fatal(err)
	}
	assertContainsOnly(t, out, "youtubei.googleapis.com", "youtube", "google")
	assertContainsOnly(t, out, "proxy-only.example", "proxy", "google")
	google, _ := os.ReadFile(filepath.Join(out, "google"))
	if !bytes.Contains(google, []byte("domain:googleapis.com")) {
		t.Fatalf("google=%s", google)
	}
}

func TestMergeIsDeterministicAcrossContributionOrder(t *testing.T) {
	contributions := []model.Contribution{
		{SourceID: "z", Side: model.GeoSite, Tag: "new-tag", Domains: []model.DomainRule{{Kind: "domain", Value: "z.example"}}},
		{SourceID: "a", Side: model.GeoSite, Tag: "new-tag", Domains: []model.DomainRule{{Kind: "domain", Value: "a.example"}}},
	}
	one := filepath.Join(t.TempDir(), "one")
	two := filepath.Join(t.TempDir(), "two")
	if err := geosite.Merge("testdata/community", one, contributions); err != nil {
		t.Fatal(err)
	}
	contributions[0], contributions[1] = contributions[1], contributions[0]
	if err := geosite.Merge("testdata/community", two, contributions); err != nil {
		t.Fatal(err)
	}
	oneText, _ := os.ReadFile(filepath.Join(one, "new-tag"))
	twoText, _ := os.ReadFile(filepath.Join(two, "new-tag"))
	if !bytes.Equal(oneText, twoText) || string(oneText) != "domain:a.example\ndomain:z.example\n" {
		t.Fatalf("one=%q two=%q", oneText, twoText)
	}
}

func TestMergeWritesGeneratedTargetsWithRepositoryFileMode(t *testing.T) {
	out := filepath.Join(t.TempDir(), "merged")
	contributions := []model.Contribution{{
		SourceID: "custom", Side: model.GeoSite, Tag: "new-tag",
		Domains: []model.DomainRule{{Kind: "domain", Value: "example.test"}},
	}}
	if err := geosite.Merge("testdata/community", out, contributions); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(out, "new-tag"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode=%o, want 644", info.Mode().Perm())
	}
}

func TestMergeRejectsNonGeositeAndUnsafeTags(t *testing.T) {
	for name, contribution := range map[string]model.Contribution{
		"wrong side": {SourceID: "x", Side: model.GeoIP, Tag: "x"},
		"unsafe tag": {SourceID: "x", Side: model.GeoSite, Tag: "../x"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := geosite.Merge("testdata/community", filepath.Join(t.TempDir(), "out"), []model.Contribution{contribution}); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestResidual标签安全覆写并删除合并标签(t *testing.T) {
	root := t.TempDir()
	for _, tag := range []string{"proxy", "gfw", "tld-not-cn"} {
		if err := os.WriteFile(filepath.Join(root, tag), []byte("domain:old.example\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rules := []model.DomainRule{
		{Kind: model.DomainSuffix, Value: "z.example", Attrs: []string{"@cn"}},
		{Kind: model.DomainFull, Value: "a.example"},
	}
	if err := geosite.WriteResidual(root, targets.ProxyResidualTag, rules, "gfw", "tld-not-cn"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, targets.ProxyResidualTag))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "domain:z.example @cn\nfull:a.example\n" {
		t.Fatalf("data=%q", data)
	}
	for _, removed := range []string{"gfw", "tld-not-cn"} {
		if _, err := os.Stat(filepath.Join(root, removed)); !os.IsNotExist(err) {
			t.Fatalf("retired %s remains: %v", removed, err)
		}
	}
}

func TestResidual标签拒绝空规则(t *testing.T) {
	err := geosite.WriteResidual(t.TempDir(), targets.DirectResidualTag, nil)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("err=%v", err)
	}
}

func TestResidual标签删除目标无效时不覆写原文件(t *testing.T) {
	for name, removed := range map[string]string{
		"不能删除自身":   targets.ProxyResidualTag,
		"待删除标签不存在": "missing",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, targets.ProxyResidualTag)
			original := []byte("domain:original.example\n")
			if err := os.WriteFile(path, original, 0o644); err != nil {
				t.Fatal(err)
			}
			err := geosite.WriteResidual(root, targets.ProxyResidualTag, []model.DomainRule{
				{Kind: model.DomainSuffix, Value: "replacement.example"},
			}, removed)
			if err == nil {
				t.Fatal("expected error")
			}
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !bytes.Equal(got, original) {
				t.Fatalf("target changed after validation failure: got=%q want=%q", got, original)
			}
		})
	}
}

func assertContainsOnly(t *testing.T, root, value, present, absent string) {
	t.Helper()
	presentText, err := os.ReadFile(filepath.Join(root, present))
	if err != nil {
		t.Fatal(err)
	}
	absentText, err := os.ReadFile(filepath.Join(root, absent))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(presentText, []byte(value)) || bytes.Contains(absentText, []byte(value)) {
		t.Fatalf("value=%q present=%s absent=%s", value, presentText, absentText)
	}
}
