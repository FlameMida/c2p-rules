package rules_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/rules"
)

type checker map[model.Side]map[string]bool

func (c checker) Require(side model.Side, tag string) error {
	if c[side][tag] {
		return nil
	}
	return fmt.Errorf("unknown target %s:%s", side, tag)
}

func TestCustomCanExtendCreatedBilibiliHMT(t *testing.T) {
	got, err := rules.LoadCustom("testdata/custom", checker{
		model.GeoSite: {"BilibiliHMT": true},
		model.GeoIP:   {"netflix": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got=%#v", got)
	}
	if got[0].Tag != "BilibiliHMT" || got[0].Domains[0].Value != "example.test" {
		t.Fatalf("got=%#v", got)
	}
	if got[1].Tag != "netflix" || len(got[1].CIDRs) != 1 {
		t.Fatalf("got=%#v", got)
	}
}

func TestCustomRejectsUnknownTargetBeforeEmission(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "geosite", "googel.yaml"), "payload:\n  - DOMAIN-SUFFIX,example.test\n")
	_, err := rules.LoadCustom(dir, checker{})
	if err == nil || !strings.Contains(err.Error(), filepath.Join("geosite", "googel.yaml")) || !strings.Contains(err.Error(), "geosite:googel") {
		t.Fatalf("err=%v", err)
	}
}

func TestCustomRejectsRulesOnWrongSideAndUnsupportedTypes(t *testing.T) {
	tests := []struct {
		side, tag, rule string
	}{
		{"geosite", "apple", "IP-CIDR,10.0.0.0/8"},
		{"geoip", "cn", "DOMAIN-SUFFIX,example.cn"},
		{"geosite", "apple", "PROCESS-NAME,Chrome"},
	}
	for _, tc := range tests {
		t.Run(tc.side+"-"+tc.tag, func(t *testing.T) {
			dir := t.TempDir()
			mustWrite(t, filepath.Join(dir, tc.side, tc.tag+".yaml"), "payload:\n  - "+tc.rule+"\n")
			allowed := checker{
				model.GeoSite: {tc.tag: true},
				model.GeoIP:   {tc.tag: true},
			}
			if _, err := rules.LoadCustom(dir, allowed); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestCustomEmptyTemplateIsSemanticNoOp(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "geosite", "apple.yaml"), "payload:\n  # - DOMAIN-SUFFIX,example.com\n")
	got, err := rules.LoadCustom(dir, checker{model.GeoSite: {"apple": true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got=%#v", got)
	}
}

func TestCustomRejectsUnknownDuplicateAliasMergeAndControlYAML(t *testing.T) {
	tests := map[string]string{
		"unknown field": "paylaod:\n  - DOMAIN-SUFFIX,example.test\n",
		"duplicate key": "metadata: one\nmetadata: two\npayload: []\n",
		"alias":         "metadata: &value one\nother: *value\npayload: []\n",
		"merge key":     "defaults: &defaults\n  payload: []\n<<: *defaults\npayload: []\n",
		"control":       "metadata: \"bad\\u0085value\"\npayload: []\n",
		"second doc":    "payload: []\n---\npayload: []\n",
	}
	for name, document := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "geosite", "apple.yaml")
			mustWrite(t, path, document)
			_, err := rules.LoadCustom(dir, checker{model.GeoSite: {"apple": true}})
			if err == nil || !strings.Contains(err.Error(), path) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func mustWrite(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}
