package rules_test

import (
	"net/netip"
	"reflect"
	"strings"
	"testing"

	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/rules"
)

func TestParseClassicalNetflixSplitsDomainAndCIDR(t *testing.T) {
	in := `payload:
  - DOMAIN-SUFFIX,netflix.com
  - DOMAIN,api.netflix.com
  - DOMAIN-KEYWORD,nflx
  - DOMAIN-REGEX,^.+\.nflxvideo\.net$
  - IP-CIDR,23.246.0.0/18,no-resolve
  - IP-CIDR6,2001:db8::/32,no-resolve
`
	b, err := rules.Parse(strings.NewReader(in), model.YAML, model.Classical)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Domains) != 4 || len(b.CIDRs) != 2 || len(b.Skipped) != 0 {
		t.Fatalf("buckets=%#v", b)
	}
	if !reflect.DeepEqual(b.Domains[0], model.DomainRule{Kind: "domain", Value: "netflix.com"}) {
		t.Fatalf("domain=%#v", b.Domains[0])
	}
	if b.CIDRs[0] != netip.MustParsePrefix("23.246.0.0/18") {
		t.Fatalf("cidr=%v", b.CIDRs)
	}
}

func TestParseDomainBehaviorNormalizesSuffixExactAndGlob(t *testing.T) {
	in := "payload:\n  - '+.example.com'\n  - www.example.com\n  - 'foo*bar?.com'\n"
	b, err := rules.Parse(strings.NewReader(in), model.YAML, model.Domain)
	if err != nil {
		t.Fatal(err)
	}
	want := []model.DomainRule{
		{Kind: "domain", Value: "example.com"},
		{Kind: "full", Value: "www.example.com"},
		{Kind: "regexp", Value: `^foo[^.]*bar[^.]\.com$`},
	}
	if len(b.Domains) != len(want) {
		t.Fatalf("domains=%#v", b.Domains)
	}
	for index := range want {
		if !reflect.DeepEqual(b.Domains[index], want[index]) {
			t.Fatalf("domain[%d]=%#v want %#v", index, b.Domains[index], want[index])
		}
	}
}

func TestParseYAMLToleratesMetadataAndNullPayload(t *testing.T) {
	for _, document := range []string{
		"payload: null\nupdated: 2026-07-31\n",
		"metadata:\n  count: 1\npayload: []\n",
	} {
		b, err := rules.Parse(strings.NewReader(document), model.YAML, model.Domain)
		if err != nil || len(b.Domains) != 0 || len(b.CIDRs) != 0 {
			t.Fatalf("document=%q buckets=%#v err=%v", document, b, err)
		}
	}
}

func TestParseRejectsMalformedPayloadsAndCIDRs(t *testing.T) {
	tests := []struct {
		name     string
		format   model.Format
		behavior model.Behavior
		input    string
	}{
		{"root sequence", model.YAML, model.Domain, "- example.com\n"},
		{"payload scalar", model.YAML, model.Domain, "payload: example.com\n"},
		{"payload object", model.YAML, model.Domain, "payload: {domain: example.com}\n"},
		{"non-string item", model.YAML, model.Domain, "payload:\n  - 123\n"},
		{"second document", model.YAML, model.Domain, "payload: []\n---\npayload: []\n"},
		{"invalid cidr", model.YAML, model.IPCIDR, "payload:\n  - 999.1.1.0/24\n"},
		{"bad no-resolve", model.YAML, model.Classical, "payload:\n  - IP-CIDR,10.0.0.0/8,resolve\n"},
		{"too many fields", model.YAML, model.Classical, "payload:\n  - IP-CIDR,10.0.0.0/8,no-resolve,extra\n"},
		{"unknown format", model.Format("json"), model.Domain, "{}"},
		{"unknown behavior", model.YAML, model.Behavior("magic"), "payload: []\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := rules.Parse(strings.NewReader(tc.input), tc.format, tc.behavior); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParseTextSkipsCommentsAndClassifiesUnsupportedRules(t *testing.T) {
	in := "# comment\n\nDOMAIN-SUFFIX,example.com\nPROCESS-NAME,Chrome\nIP-SUFFIX,8.8.8.0/24\n"
	b, err := rules.Parse(strings.NewReader(in), model.Text, model.Classical)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Domains) != 1 || len(b.Skipped) != 2 {
		t.Fatalf("buckets=%#v", b)
	}
}
