package shadowrocket_test

import (
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clash-rules-srs/internal/config"
	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/shadowrocket"
)

func Test四种可映射类型与CIDR(t *testing.T) {
	groups := []shadowrocket.GroupRules{{
		Group: model.Group{ID: "cn", Remarks: "中国大陆"},
		Domains: []shadowrocket.DomainLine{
			{Kind: model.DomainSuffix, Value: "suffix.example"},
			{Kind: model.DomainFull, Value: "full.example"},
			{Kind: model.DomainKeyword, Value: "needle"},
		},
		CIDRs: []netip.Prefix{
			netip.MustParsePrefix("192.0.2.0/24"),
			netip.MustParsePrefix("2001:db8::/32"),
		},
	}}
	rendered := mustRender(t, groups, "FlameMida/c2p-rules")
	rules := section(rendered, "[Rule]")
	golden, err := os.ReadFile(filepath.Join("testdata", "rules.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if rules != string(golden) {
		t.Fatalf("rules mismatch\nwant:\n%s\ngot:\n%s", golden, rules)
	}
}

func Test组名与顺序对齐YAML(t *testing.T) {
	groups, err := config.LoadGroups("../../config/passwall2-groups.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 16 {
		t.Fatalf("groups=%d", len(groups))
	}
	expanded := make([]shadowrocket.GroupRules, len(groups))
	for index, group := range groups {
		expanded[index] = shadowrocket.GroupRules{
			Group: group,
			Domains: []shadowrocket.DomainLine{{
				Kind: model.DomainSuffix, Value: group.ID + ".example",
			}},
		}
	}
	rendered := mustRender(t, expanded, "FlameMida/c2p-rules")
	proxyGroups := section(rendered, "[Proxy Group]")
	rules := section(rendered, "[Rule]")
	for _, group := range groups {
		if !strings.Contains(proxyGroups, group.Remarks+" = select,") {
			t.Fatalf("missing policy group %q", group.Remarks)
		}
		wantRule := "DOMAIN-SUFFIX," + group.ID + ".example," + group.Remarks + "\n"
		if !strings.Contains(rules, wantRule) {
			t.Fatalf("missing rule policy %q", group.Remarks)
		}
	}
	assertAppearsBefore(t, proxyGroups, "YouTube =", "Google 服务 =")
	assertAppearsBefore(t, rules, ",YouTube\n", ",Google 服务\n")
	if strings.Contains(rendered, "[Proxy]\n") {
		t.Fatal("renderer emitted node definitions")
	}
}

func Test广告与国内默认(t *testing.T) {
	groups := []shadowrocket.GroupRules{
		oneRule("reject", "广告拦截"),
		oneRule("youtube", "YouTube"),
		oneRule("private", "私有网络"),
		oneRule("cn", "中国大陆"),
		oneRule("direct", "直连规则"),
	}
	rendered := mustRender(t, groups, "FlameMida/c2p-rules")
	for _, want := range []string{
		"广告拦截 = select,REJECT,PROXY,DIRECT,policy-select-name=REJECT",
		"YouTube = select,PROXY,DIRECT,policy-select-name=PROXY",
		"私有网络 = select,DIRECT,PROXY,policy-select-name=DIRECT",
		"中国大陆 = select,DIRECT,PROXY,policy-select-name=DIRECT",
		"直连规则 = select,DIRECT,PROXY,policy-select-name=DIRECT",
	} {
		if !strings.Contains(rendered, want+"\n") {
			t.Fatalf("missing %q in\n%s", want, rendered)
		}
	}
}

func Test非中国域名跟随代理规则(t *testing.T) {
	rendered := mustRender(t, []shadowrocket.GroupRules{
		oneRule("proxy", "代理规则"),
		oneRule("tld_not_cn", "非中国域名"),
		oneRule("cn", "中国大陆"),
	}, "FlameMida/c2p-rules")
	want := "非中国域名 = select,代理规则,policy-select-name=代理规则\n"
	if !strings.Contains(rendered, want) {
		t.Fatalf("missing %q in\n%s", want, rendered)
	}
	if strings.Contains(rendered, "非中国域名 = select,PROXY,DIRECT") {
		t.Fatal("tld_not_cn remains independently selectable")
	}
}

func TestRemarks变化时按ID建立引用(t *testing.T) {
	rendered := mustRender(t, []shadowrocket.GroupRules{
		oneRule("tld_not_cn", "海外域名"),
		oneRule("proxy", "我的代理规则"),
		oneRule("cn", "中国大陆"),
	}, "FlameMida/c2p-rules")
	if !strings.Contains(rendered, "海外域名 = select,我的代理规则,policy-select-name=我的代理规则\n") {
		t.Fatalf("rendered=%s", rendered)
	}
	for name, groups := range map[string][]shadowrocket.GroupRules{
		"missing proxy": {
			oneRule("tld_not_cn", "海外域名"),
			oneRule("cn", "中国大陆"),
		},
		"duplicate proxy id": {
			oneRule("proxy", "代理一"),
			oneRule("proxy", "代理二"),
			oneRule("tld_not_cn", "海外域名"),
			oneRule("cn", "中国大陆"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := shadowrocket.Render(groups, "FlameMida/c2p-rules")
			if err == nil || !strings.Contains(err.Error(), "proxy") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func Test空组失败(t *testing.T) {
	_, err := shadowrocket.Render([]shadowrocket.GroupRules{{
		Group: model.Group{ID: "cn", Remarks: "中国大陆"},
	}}, "FlameMida/c2p-rules")
	if err == nil || !strings.Contains(err.Error(), "中国大陆") {
		t.Fatalf("err=%v", err)
	}
}

func Test策略组名称拒绝重复与保留名(t *testing.T) {
	tests := map[string][]shadowrocket.GroupRules{
		"duplicate": {
			oneRule("youtube", "同名策略"),
			oneRule("google", "同名策略"),
			oneRule("cn", "中国大陆"),
		},
		"reserved proxy": {
			oneRule("proxy_group", "PROXY"),
			oneRule("cn", "中国大陆"),
		},
		"reserved direct case variant": {
			oneRule("direct_group", "direct"),
			oneRule("cn", "中国大陆"),
		},
		"reserved reject": {
			oneRule("reject_group", "REJECT"),
			oneRule("cn", "中国大陆"),
		},
	}
	for name, groups := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := shadowrocket.Render(groups, "FlameMida/c2p-rules")
			if err == nil || !strings.Contains(err.Error(), "policy group name") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestLatestUpdateURL(t *testing.T) {
	rendered := mustRender(t, []shadowrocket.GroupRules{oneRule("cn", "中国大陆")}, "FlameMida/c2p-rules")
	want := "update-url = https://github.com/FlameMida/c2p-rules/releases/latest/download/shadowrocket.conf"
	if !strings.Contains(rendered, want+"\n") {
		t.Fatalf("rendered=%s", rendered)
	}
	if strings.Contains(rendered, "release-test-tag") {
		t.Fatal("conf contains release tag")
	}
	for _, repo := range []string{"", "owner/repo", "bad repo/name", "one-part"} {
		if _, err := shadowrocket.Render([]shadowrocket.GroupRules{oneRule("cn", "中国大陆")}, repo); err == nil {
			t.Fatalf("repo %q accepted", repo)
		}
	}
}

func Test兜底顺序(t *testing.T) {
	rendered := mustRender(t, []shadowrocket.GroupRules{oneRule("cn", "中国大陆")}, "FlameMida/c2p-rules")
	lines := strings.Split(strings.TrimSpace(section(rendered, "[Rule]")), "\n")
	if len(lines) < 2 || lines[len(lines)-2] != "GEOIP,CN,中国大陆" || lines[len(lines)-1] != "FINAL,PROXY" {
		t.Fatalf("rule tail=%v", lines)
	}
	if strings.Count(rendered, "GEOIP,CN,中国大陆\n") != 1 || strings.Count(rendered, "FINAL,PROXY\n") != 1 {
		t.Fatalf("fallback count wrong:\n%s", rendered)
	}
}

func oneRule(id, remarks string) shadowrocket.GroupRules {
	return shadowrocket.GroupRules{
		Group: model.Group{ID: id, Remarks: remarks},
		Domains: []shadowrocket.DomainLine{{
			Kind: model.DomainSuffix, Value: id + ".example",
		}},
	}
}

func mustRender(t *testing.T, groups []shadowrocket.GroupRules, repo string) string {
	t.Helper()
	data, err := shadowrocket.Render(groups, repo)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func section(document, header string) string {
	start := strings.Index(document, header+"\n")
	if start < 0 {
		return ""
	}
	value := document[start+len(header)+1:]
	if end := strings.Index(value, "\n["); end >= 0 {
		value = value[:end+1]
	}
	return value
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
