package config_test

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"

	"clash-rules-srs/internal/config"
	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/rules"
)

func TestRepositorySourcesMatchApprovedTargets(t *testing.T) {
	sources, err := config.LoadSources("../../sources.yaml")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"loyalsoldier-reject":       "geosite:reject:create",
		"loyalsoldier-icloud":       "geosite:icloud:merge-base",
		"loyalsoldier-apple":        "geosite:apple:merge-base",
		"loyalsoldier-google":       "geosite:google:merge-base",
		"loyalsoldier-proxy":        "geosite:proxy:create",
		"loyalsoldier-direct":       "geosite:direct:create",
		"loyalsoldier-private":      "geosite:private:merge-base",
		"loyalsoldier-gfw":          "geosite:gfw:create",
		"loyalsoldier-tld-not-cn":   "geosite:tld-not-cn:create",
		"loyalsoldier-telegramcidr": "geoip:telegram:merge-base",
		"loyalsoldier-cncidr":       "geoip:cn:merge-base",
		"loyalsoldier-lancidr":      "geoip:private:merge-base",
		"xiaolin-youtube":           "geosite:youtube:merge-base",
		"xiaolin-netflix":           "geosite:netflix:merge-base,geoip:netflix:merge-base",
		"xiaolin-spotify":           "geosite:spotify:merge-base",
		"xiaolin-bilibili":          "geosite:BilibiliHMT:create,geoip:BilibiliHMT:create",
		"xiaolin-tiktok":            "geosite:tiktok:merge-base",
		"sukka-ai":                  "geosite:ai:create",
	}
	got := make(map[string]string, len(sources))
	for _, source := range sources {
		var targets []string
		if source.Outputs.GeoSite != nil {
			targets = append(targets, fmt.Sprintf("geosite:%s:%s", source.Outputs.GeoSite.Tag, source.Outputs.GeoSite.Mode))
		}
		if source.Outputs.GeoIP != nil {
			targets = append(targets, fmt.Sprintf("geoip:%s:%s", source.Outputs.GeoIP.Tag, source.Outputs.GeoIP.Mode))
		}
		got[source.ID] = joinTargets(targets)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sources mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestRepositoryDefaultGroupsMatchApprovedOrder(t *testing.T) {
	groups, err := config.LoadGroups("../../config/passwall2-groups.yaml")
	if err != nil {
		t.Fatal(err)
	}
	want := []model.Group{
		{ID: "reject", Remarks: "广告拦截", GeoSite: []string{"reject"}, GeoIP: []string{}},
		{ID: "bilibili_hmt", Remarks: "哔哩哔哩港澳台", GeoSite: []string{"BilibiliHMT"}, GeoIP: []string{"BilibiliHMT"}},
		{ID: "youtube", Remarks: "YouTube", GeoSite: []string{"youtube"}, GeoIP: []string{}},
		{ID: "netflix", Remarks: "Netflix", GeoSite: []string{"netflix"}, GeoIP: []string{"netflix"}},
		{ID: "spotify", Remarks: "Spotify", GeoSite: []string{"spotify"}, GeoIP: []string{}},
		{ID: "tiktok", Remarks: "TikTok", GeoSite: []string{"tiktok"}, GeoIP: []string{}},
		{ID: "ai", Remarks: "AI 服务", GeoSite: []string{"ai"}, GeoIP: []string{}},
		{ID: "apple_services", Remarks: "苹果服务", GeoSite: []string{"apple", "icloud"}, GeoIP: []string{}},
		{ID: "telegram", Remarks: "Telegram", GeoSite: []string{}, GeoIP: []string{"telegram"}},
		{ID: "google", Remarks: "Google 服务", GeoSite: []string{"google"}, GeoIP: []string{}},
		{ID: "gfw", Remarks: "GFW", GeoSite: []string{"gfw"}, GeoIP: []string{}},
		{ID: "proxy", Remarks: "代理规则", GeoSite: []string{"proxy"}, GeoIP: []string{}},
		{ID: "tld_not_cn", Remarks: "非中国域名", GeoSite: []string{"tld-not-cn"}, GeoIP: []string{}},
		{ID: "private", Remarks: "私有网络", GeoSite: []string{"private"}, GeoIP: []string{"private"}},
		{ID: "cn", Remarks: "中国大陆", GeoSite: []string{"cn"}, GeoIP: []string{"cn"}},
		{ID: "direct", Remarks: "直连规则", GeoSite: []string{"direct"}, GeoIP: []string{}},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Fatalf("groups mismatch\n got: %#v\nwant: %#v", groups, want)
	}
}

func TestEveryDefaultGroupTargetHasEmptyDocumentedTemplate(t *testing.T) {
	for _, test := range defaultTemplateCases(t) {
		data, err := os.ReadFile(test.path)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range test.markers {
			if !bytes.Contains(data, []byte(marker)) {
				t.Fatalf("%s does not document %s", test.path, marker)
			}
		}

		file, err := os.Open(test.path)
		if err != nil {
			t.Fatal(err)
		}
		buckets, parseErr := rules.Parse(file, model.YAML, model.Classical)
		closeErr := file.Close()
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		if len(buckets.Domains) != 0 || len(buckets.CIDRs) != 0 || len(buckets.Skipped) != 0 {
			t.Fatalf("%s injects rules: %#v", test.path, buckets)
		}

		commentedExample := []byte("#   - " + test.example)
		if !bytes.Contains(data, commentedExample) {
			t.Fatalf("%s does not contain directly usable example %q", test.path, commentedExample)
		}
		enabled := bytes.Replace(data, commentedExample, []byte("  - "+test.example), 1)
		buckets, err = rules.Parse(bytes.NewReader(enabled), model.YAML, model.Classical)
		if err != nil {
			t.Fatalf("parse enabled example in %s: %v", test.path, err)
		}
		if len(buckets.Domains) != test.wantDomains || len(buckets.CIDRs) != test.wantCIDRs || len(buckets.Skipped) != 0 {
			t.Fatalf("%s enabled example produced unexpected rules: %#v", test.path, buckets)
		}
	}
}

type defaultTemplateCase struct {
	path        string
	markers     []string
	example     string
	wantDomains int
	wantCIDRs   int
}

func defaultTemplateCases(t *testing.T) []defaultTemplateCase {
	t.Helper()
	groups, err := config.LoadGroups("../../config/passwall2-groups.yaml")
	if err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]struct{})
	var cases []defaultTemplateCase
	add := func(side model.Side, tags []string, markers []string, example string, wantDomains, wantCIDRs int) {
		for _, tag := range tags {
			path := fmt.Sprintf("../../custom/%s/%s.yaml", side, tag)
			if _, exists := seen[path]; exists {
				continue
			}
			seen[path] = struct{}{}
			cases = append(cases, defaultTemplateCase{
				path: path, markers: markers, example: example,
				wantDomains: wantDomains, wantCIDRs: wantCIDRs,
			})
		}
	}
	for _, group := range groups {
		add(model.GeoSite, group.GeoSite, []string{"payload:", "DOMAIN-SUFFIX", "DOMAIN,", "DOMAIN-KEYWORD", "DOMAIN-REGEX"}, "DOMAIN-SUFFIX,example.com", 1, 0)
		add(model.GeoIP, group.GeoIP, []string{"payload:", "IP-CIDR,", "IP-CIDR6", "no-resolve"}, "IP-CIDR,192.0.2.0/24", 0, 1)
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].path < cases[j].path })
	return cases
}

func joinTargets(values []string) string {
	if len(values) == 1 {
		return values[0]
	}
	return values[0] + "," + values[1]
}
