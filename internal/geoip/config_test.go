package geoip_test

import (
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clash-rules-srs/internal/geoip"
	"clash-rules-srs/internal/model"
)

func TestWriteConfigKeepsBaseFirstAndSortsTargets(t *testing.T) {
	dir := t.TempDir()
	inputs, err := geoip.WriteInputs(filepath.Join(dir, "ip"), []model.Contribution{
		{SourceID: "z", Side: model.GeoIP, Tag: "netflix", CIDRs: []netip.Prefix{netip.MustParsePrefix("23.246.0.0/18")}},
		{SourceID: "a", Side: model.GeoIP, Tag: "BilibiliHMT", CIDRs: []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "base.dat")
	if err := os.WriteFile(base, []byte("base"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	template, err := geoip.LoadTemplate("../../config/geoip.base.json")
	if err != nil {
		t.Fatal(err)
	}
	if template.BaseURI() == "" {
		t.Fatal("validated template did not expose its base URI")
	}
	if err := geoip.WriteConfig(template, inputs, base, filepath.Join(dir, "publish"), path); err != nil {
		t.Fatal(err)
	}
	var got converterConfig
	decodeJSON(t, path, &got)
	if len(got.Input) != 3 || got.Input[0].Type != "v2rayGeoIPDat" || got.Input[0].Args.URI != base {
		t.Fatalf("input=%#v", got.Input)
	}
	if got.Input[1].Args.Name != "BilibiliHMT" || got.Input[2].Args.Name != "netflix" {
		t.Fatalf("input=%#v", got.Input)
	}
	if !filepath.IsAbs(got.Input[1].Args.URI) || got.Output[0].Args.OutputName != "geoip.dat" || !filepath.IsAbs(got.Output[0].Args.OutputDir) {
		t.Fatalf("config=%#v", got)
	}
}

func TestCIDRsAreMaskedSortedAndDeduplicated(t *testing.T) {
	dir := t.TempDir()
	inputs, err := geoip.WriteInputs(dir, []model.Contribution{
		{SourceID: "one", Side: model.GeoIP, Tag: "netflix", CIDRs: []netip.Prefix{
			netip.MustParsePrefix("23.246.0.1/18"),
			netip.MustParsePrefix("23.246.0.0/18"),
			netip.MustParsePrefix("23.246.0.0/19"),
			netip.MustParsePrefix("2001:db8::1/32"),
		}},
		{SourceID: "two", Side: model.GeoIP, Tag: "netflix", CIDRs: []netip.Prefix{
			netip.MustParsePrefix("23.246.0.0/19"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 1 || inputs[0].Tag != "netflix" {
		t.Fatalf("inputs=%#v", inputs)
	}
	got, err := os.ReadFile(inputs[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	want := "23.246.0.0/18\n23.246.0.0/19\n2001:db8::/32\n"
	if string(got) != want {
		t.Fatalf("got=%q want=%q", got, want)
	}
}

func TestWriteInputsRejectsWrongSideDomainsAndUnsafeTags(t *testing.T) {
	for name, contribution := range map[string]model.Contribution{
		"wrong side":  {Side: model.GeoSite, Tag: "x", CIDRs: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}},
		"domain data": {Side: model.GeoIP, Tag: "x", Domains: []model.DomainRule{{Kind: "domain", Value: "example.com"}}},
		"unsafe tag":  {Side: model.GeoIP, Tag: "../x", CIDRs: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := geoip.WriteInputs(t.TempDir(), []model.Contribution{contribution}); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestWriteConfigPreservesTemplateOptions(t *testing.T) {
	dir := t.TempDir()
	template := filepath.Join(dir, "template.json")
	document := `{
  "input": [{"type":"v2rayGeoIPDat","action":"add","args":{"uri":"old","wantedList":["cn"]}}],
  "output": [{"type":"v2rayGeoIPDat","action":"output","args":{"outputDir":"old","outputName":"old.dat","onlyIPType":"IPv4"}}]
}`
	if err := os.WriteFile(template, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "base.dat")
	if err := os.WriteFile(base, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "generated.json")
	loaded, err := geoip.LoadTemplate(template)
	if err != nil {
		t.Fatal(err)
	}
	if err := geoip.WriteConfig(loaded, nil, base, filepath.Join(dir, "publish"), output); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	decodeJSON(t, output, &got)
	inputArgs := got["input"].([]any)[0].(map[string]any)["args"].(map[string]any)
	outputArgs := got["output"].([]any)[0].(map[string]any)["args"].(map[string]any)
	if _, ok := inputArgs["wantedList"]; !ok || outputArgs["onlyIPType"] != "IPv4" {
		t.Fatalf("got=%#v", got)
	}
}

func TestWriteConfigRejectsInvalidTemplates(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.dat")
	if err := os.WriteFile(base, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, document := range map[string]string{
		"missing input":       `{"output":[{"type":"v2rayGeoIPDat","action":"output","args":{}}]}`,
		"empty input":         `{"input":[],"output":[{"type":"v2rayGeoIPDat","action":"output","args":{}}]}`,
		"wrong base type":     `{"input":[{"type":"text","action":"add","args":{}}],"output":[{"type":"v2rayGeoIPDat","action":"output","args":{}}]}`,
		"missing base args":   `{"input":[{"type":"v2rayGeoIPDat","action":"add"}],"output":[{"type":"v2rayGeoIPDat","action":"output","args":{}}]}`,
		"missing output":      `{"input":[{"type":"v2rayGeoIPDat","action":"add","args":{}}]}`,
		"missing output args": `{"input":[{"type":"v2rayGeoIPDat","action":"add","args":{}}],"output":[{"type":"v2rayGeoIPDat","action":"output"}]}`,
		"wrong output type":   `{"input":[{"type":"v2rayGeoIPDat","action":"add","args":{}}],"output":[{"type":"text","action":"output","args":{}}]}`,
		"trailing json":       `{"input":[],"output":[]} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			template := filepath.Join(dir, strings.ReplaceAll(name, " ", "-")+".json")
			if err := os.WriteFile(template, []byte(document), 0o600); err != nil {
				t.Fatal(err)
			}
			loaded, loadErr := geoip.LoadTemplate(template)
			if loadErr == nil {
				loadErr = geoip.WriteConfig(loaded, nil, base, filepath.Join(dir, "publish"), filepath.Join(dir, name+"-out.json"))
			}
			if loadErr == nil {
				t.Fatal("expected error")
			}
		})
	}
}

type converterConfig struct {
	Input  []converterEntry `json:"input"`
	Output []converterEntry `json:"output"`
}

type converterEntry struct {
	Type   string `json:"type"`
	Action string `json:"action"`
	Args   struct {
		Name       string `json:"name"`
		URI        string `json:"uri"`
		OutputDir  string `json:"outputDir"`
		OutputName string `json:"outputName"`
	} `json:"args"`
}

func decodeJSON(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
}
