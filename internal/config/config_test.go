package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clash-rules-srs/internal/config"
	"clash-rules-srs/internal/model"
)

func TestSourcesRejectLegacySidesAndDuplicateKeys(t *testing.T) {
	for name, document := range map[string]string{
		"legacy":    "sources:\n- id: x\n  behavior: domain\n  url: https://e.test/x\n  sides: [geosite]\n",
		"duplicate": "sources:\n- id: x\n  id: y\n  behavior: domain\n  url: https://e.test/x\n  outputs:\n    geosite: {tag: x, mode: create}\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := config.ParseSources(strings.NewReader(document))
			if err == nil {
				t.Fatal("expected strict YAML error")
			}
		})
	}
}

func TestDecodeStrictRejectsControlCharacters(t *testing.T) {
	var out struct {
		Value string `yaml:"value"`
	}
	err := config.DecodeStrict(strings.NewReader("value: \"bad\\u0085value\"\n"), &out)
	if err == nil || !strings.Contains(err.Error(), "control") {
		t.Fatalf("err=%v value=%q", err, out.Value)
	}
}

func TestGoogleUsesExplicitMergeBaseTarget(t *testing.T) {
	sources, err := config.ParseSources(strings.NewReader(`sources:
- id: loyalsoldier-google
  behavior: domain
  url: https://example.test/google
  outputs:
    geosite: {tag: google, mode: merge-base}
`))
	if err != nil {
		t.Fatal(err)
	}
	got := sources[0]
	if got.ID != "loyalsoldier-google" || got.Format != model.YAML || got.Outputs.GeoSite == nil || got.Outputs.GeoSite.Tag != "google" || got.Outputs.GeoSite.Mode != model.MergeBase {
		t.Fatalf("unexpected source: %#v", got)
	}
}

func TestSourcesRejectInvalidDocuments(t *testing.T) {
	tests := map[string]string{
		"missing sources": `{}`,
		"non-string scalar": `sources:
- id: 123
  behavior: domain
  url: https://example.test/x
  outputs: {geosite: {tag: x, mode: create}}
`,
		"unknown field": `sources:
- id: x
  behavior: domain
  url: https://example.test/x
  extra: true
  outputs: {geosite: {tag: x, mode: create}}
`,
		"second document": `sources: []
---
sources: []
`,
		"invalid id": `sources:
- id: /x
  behavior: domain
  url: https://example.test/x
  outputs: {geosite: {tag: x, mode: create}}
`,
		"invalid tag": `sources:
- id: x
  behavior: domain
  url: https://example.test/x
  outputs: {geosite: {tag: "bad tag", mode: create}}
`,
		"invalid mode": `sources:
- id: x
  behavior: domain
  url: https://example.test/x
  outputs: {geosite: {tag: x, mode: append}}
`,
		"invalid behavior": `sources:
- id: x
  behavior: magic
  url: https://example.test/x
  outputs: {geosite: {tag: x, mode: create}}
`,
		"invalid format": `sources:
- id: x
  behavior: domain
  format: json
  url: https://example.test/x
  outputs: {geosite: {tag: x, mode: create}}
`,
		"http url": `sources:
- id: x
  behavior: domain
  url: http://example.test/x
  outputs: {geosite: {tag: x, mode: create}}
`,
		"no output": `sources:
- id: x
  behavior: domain
  url: https://example.test/x
  outputs: {}
`,
		"domain with geoip": `sources:
- id: x
  behavior: domain
  url: https://example.test/x
  outputs: {geoip: {tag: x, mode: create}}
`,
		"ipcidr with geosite": `sources:
- id: x
  behavior: ipcidr
  url: https://example.test/x
  outputs: {geosite: {tag: x, mode: create}}
`,
		"duplicate id": `sources:
- id: x
  behavior: domain
  url: https://example.test/x
  outputs: {geosite: {tag: x, mode: create}}
- id: x
  behavior: ipcidr
  url: https://example.test/y
  outputs: {geoip: {tag: x, mode: create}}
`,
	}
	for name, document := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := config.ParseSources(strings.NewReader(document)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestClassicalMayDeclareOneOrBothSides(t *testing.T) {
	for _, outputs := range []string{
		"geosite: {tag: one, mode: create}",
		"geoip: {tag: one, mode: create}",
		"geosite: {tag: one, mode: create}\n    geoip: {tag: one, mode: create}",
	} {
		document := "sources:\n- id: one\n  behavior: classical\n  format: text\n  url: https://example.test/one\n  outputs:\n    " + outputs + "\n"
		if _, err := config.ParseSources(strings.NewReader(document)); err != nil {
			t.Fatalf("outputs=%q: %v", outputs, err)
		}
	}
}

func TestGroupsPreserveOrderAndRequireBothSides(t *testing.T) {
	groups, err := config.ParseGroups(strings.NewReader(`groups:
- id: youtube
  remarks: YouTube
  geosite: [youtube]
  geoip: []
- id: google
  remarks: Google 服务
  geosite: [google]
  geoip: []
`))
	if err != nil {
		t.Fatal(err)
	}
	if groups[0].ID != "youtube" || groups[1].ID != "google" {
		t.Fatalf("order=%v", groups)
	}

	_, err = config.ParseGroups(strings.NewReader("groups:\n- id: x\n  remarks: X\n  geosite: [x]\n"))
	if err == nil || !strings.Contains(err.Error(), "geoip") {
		t.Fatalf("missing geoip error=%v", err)
	}
}

func TestGroupsRejectInvalidValues(t *testing.T) {
	tests := map[string]string{
		"missing groups":  "{}\n",
		"invalid id":      "groups:\n- id: Bad-ID\n  remarks: X\n  geosite: []\n  geoip: []\n",
		"empty remarks":   "groups:\n- id: x\n  remarks: ''\n  geosite: []\n  geoip: []\n",
		"duplicate id":    "groups:\n- id: x\n  remarks: X\n  geosite: []\n  geoip: []\n- id: x\n  remarks: Y\n  geosite: []\n  geoip: []\n",
		"duplicate tag":   "groups:\n- id: x\n  remarks: X\n  geosite: [google, google]\n  geoip: []\n",
		"invalid tag":     "groups:\n- id: x\n  remarks: X\n  geosite: ['bad tag']\n  geoip: []\n",
		"missing geosite": "groups:\n- id: x\n  remarks: X\n  geoip: []\n",
		"unknown field":   "groups:\n- id: x\n  remarks: X\n  geosite: []\n  geoip: []\n  route: proxy\n",
	}
	for name, document := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := config.ParseGroups(strings.NewReader(document)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadSourcesAndGroups(t *testing.T) {
	dir := t.TempDir()
	sourcesPath := filepath.Join(dir, "sources.yaml")
	groupsPath := filepath.Join(dir, "groups.yaml")
	sources, err := os.ReadFile("testdata/sources-valid.yaml")
	if err != nil {
		t.Fatal(err)
	}
	groups, err := os.ReadFile("testdata/groups-valid.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcesPath, sources, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(groupsPath, groups, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := config.LoadSources(sourcesPath); err != nil || len(got) != 1 {
		t.Fatalf("sources=%v err=%v", got, err)
	}
	if got, err := config.LoadGroups(groupsPath); err != nil || len(got) != 1 {
		t.Fatalf("groups=%v err=%v", got, err)
	}
}
