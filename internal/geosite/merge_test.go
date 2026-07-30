package geosite_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clash-rules-srs/internal/geosite"
	"clash-rules-srs/internal/model"
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

func TestMergeKeepsGoogleYouTubeAndBilibiliTargetsIndependent(t *testing.T) {
	out := filepath.Join(t.TempDir(), "merged")
	inputs := []model.Contribution{
		{SourceID: "youtube", Side: model.GeoSite, Tag: "youtube", Domains: []model.DomainRule{{Kind: "full", Value: "youtubei.googleapis.com"}}},
		{SourceID: "google", Side: model.GeoSite, Tag: "google", Domains: []model.DomainRule{{Kind: "domain", Value: "googleapis.com"}}},
		{SourceID: "hmt", Side: model.GeoSite, Tag: "BilibiliHMT", Domains: []model.DomainRule{{Kind: "domain", Value: "hmt-only.example"}}},
	}
	if err := geosite.Merge("testdata/community", out, inputs); err != nil {
		t.Fatal(err)
	}
	assertContainsOnly(t, out, "youtubei.googleapis.com", "youtube", "google")
	assertContainsOnly(t, out, "hmt-only.example", "BilibiliHMT", "bilibili")
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
