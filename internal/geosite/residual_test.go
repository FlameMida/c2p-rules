package geosite_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"clash-rules-srs/internal/geodat"
	"clash-rules-srs/internal/geosite"
	"clash-rules-srs/internal/model"
)

func Test残差推导响应Context取消(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := geosite.DeriveContext(ctx, siteTags(map[string][]model.DomainRule{
		"basis": {{Kind: model.DomainFull, Value: "kept.example"}},
	}), geosite.ResidualSpec{OutputTag: "out", BasisTags: []string{"basis"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

func Test大规则集在固定期限内完成(t *testing.T) {
	const ruleCount = 50_000
	basis := make([]model.DomainRule, 0, ruleCount)
	coverage := make([]model.DomainRule, 0, ruleCount)
	for index := 0; index < ruleCount; index++ {
		basis = append(basis, model.DomainRule{Kind: model.DomainFull, Value: fmt.Sprintf("basis-%05d.example", index)})
		coverage = append(coverage, model.DomainRule{Kind: model.DomainSuffix, Value: fmt.Sprintf("covered-%05d.invalid", index)})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, _, err := geosite.DeriveContext(ctx, siteTags(map[string][]model.DomainRule{
		"basis": basis, "earlier": coverage,
	}), geosite.ResidualSpec{OutputTag: "out", BasisTags: []string{"basis"}, EarlierActiveTags: []string{"earlier"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != ruleCount {
		t.Fatalf("got %d rules, want %d", len(got), ruleCount)
	}
}

func Test编译器覆盖去重保持规则集语义(t *testing.T) {
	expected := []model.DomainRule{
		{Kind: model.DomainSuffix, Value: "example.com"},
		{Kind: model.DomainSuffix, Value: "www.example.com"},
		{Kind: model.DomainFull, Value: "attr.example", Attrs: []string{"@cn"}},
	}
	compiled := []model.DomainRule{
		{Kind: model.DomainSuffix, Value: "example.com"},
		{Kind: model.DomainFull, Value: "attr.example", Attrs: []string{"@cn"}},
	}
	if !geosite.EquivalentCoverage(expected, compiled) {
		t.Fatal("covered child suffix should be a semantic no-op")
	}
	for name, changed := range map[string][]model.DomainRule{
		"uncovered drop": {{Kind: model.DomainFull, Value: "attr.example", Attrs: []string{"@cn"}}},
		"changed attrs": {
			{Kind: model.DomainSuffix, Value: "example.com"},
			{Kind: model.DomainFull, Value: "attr.example", Attrs: []string{"@other"}},
		},
		"invented rule": append(append([]model.DomainRule(nil), compiled...), model.DomainRule{Kind: model.DomainFull, Value: "invented.example"}),
	} {
		t.Run(name, func(t *testing.T) {
			if geosite.EquivalentCoverage(expected, changed) {
				t.Fatalf("unexpected equivalence for %#v", changed)
			}
		})
	}
}

func TestGFW与Proxy合并且服务组优先(t *testing.T) {
	tags := siteTags(map[string][]model.DomainRule{
		"gfw": {{Kind: model.DomainSuffix, Value: "shared.example"}},
		"proxy": {
			{Kind: model.DomainSuffix, Value: "shared.example"},
			{Kind: model.DomainFull, Value: "www.youtube.example"},
			{Kind: model.DomainSuffix, Value: "kept.example"},
		},
		"youtube": {{Kind: model.DomainSuffix, Value: "youtube.example"}},
	})
	got, report, err := geosite.Derive(tags, geosite.ResidualSpec{
		OutputTag: "proxy", BasisTags: []string{"gfw", "proxy"}, EarlierActiveTags: []string{"youtube"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []model.DomainRule{
		{Kind: model.DomainSuffix, Value: "kept.example"},
		{Kind: model.DomainSuffix, Value: "shared.example"},
	}
	if !reflect.DeepEqual(got, want) || report != (geosite.ResidualReport{Basis: 3, Kept: 2, Removed: 1}) {
		t.Fatalf("got=%#v report=%#v", got, report)
	}
}

func Test不借用后序直连组裁剪Proxy(t *testing.T) {
	tags := siteTags(map[string][]model.DomainRule{
		"proxy": {{Kind: model.DomainFull, Value: "only-cn.example"}},
		"cn":    {{Kind: model.DomainSuffix, Value: "example"}},
	})
	got, _, err := geosite.Derive(tags, geosite.ResidualSpec{
		OutputTag: "proxy", BasisTags: []string{"proxy"},
	})
	if err != nil || len(got) != 1 || got[0].Value != "only-cn.example" {
		t.Fatalf("got=%#v err=%v", got, err)
	}
}

func TestTLDNotCN与CN使Direct规则不可达(t *testing.T) {
	tags := siteTags(map[string][]model.DomainRule{
		"direct": {
			{Kind: model.DomainFull, Value: "covered.foreign"},
			{Kind: model.DomainFull, Value: "covered.cn"},
			{Kind: model.DomainFull, Value: "kept.example"},
		},
		"tld-not-cn": {{Kind: model.DomainSuffix, Value: "foreign"}},
		"cn":         {{Kind: model.DomainSuffix, Value: "cn"}},
	})
	got, _, err := geosite.Derive(tags, geosite.ResidualSpec{
		OutputTag: "direct", BasisTags: []string{"direct"}, EarlierActiveTags: []string{"tld-not-cn", "cn"},
	})
	if err != nil || len(got) != 1 || got[0].Value != "kept.example" {
		t.Fatalf("got=%#v err=%v", got, err)
	}
}

func TestGeoIP不作为域名覆盖证据(t *testing.T) {
	tags := siteTags(map[string][]model.DomainRule{
		"direct": {{Kind: model.DomainFull, Value: "resolves-to-cn.example"}},
	})
	got, _, err := geosite.Derive(tags, geosite.ResidualSpec{
		OutputTag: "direct", BasisTags: []string{"direct"},
	})
	if err != nil || len(got) != 1 {
		t.Fatalf("got=%#v err=%v", got, err)
	}
}

func TestSuffix标签边界不产生假覆盖(t *testing.T) {
	tags := siteTags(map[string][]model.DomainRule{
		"basis":   {{Kind: model.DomainFull, Value: "example.com"}},
		"earlier": {{Kind: model.DomainSuffix, Value: "ample.com"}},
	})
	got, _, err := geosite.Derive(tags, geosite.ResidualSpec{
		OutputTag: "out", BasisTags: []string{"basis"}, EarlierActiveTags: []string{"earlier"},
	})
	if err != nil || len(got) != 1 {
		t.Fatalf("got=%#v err=%v", got, err)
	}
}

func Test正则只做同类等值去重(t *testing.T) {
	tags := siteTags(map[string][]model.DomainRule{
		"basis": {
			{Kind: model.DomainRegexp, Value: "^x.+$", Attrs: []string{"@b", "@a"}},
			{Kind: model.DomainRegexp, Value: "^same$", Attrs: []string{"@a"}},
		},
		"earlier": {
			{Kind: model.DomainRegexp, Value: "^x.*$", Attrs: []string{"@a", "@b"}},
			{Kind: model.DomainRegexp, Value: "^same$", Attrs: []string{"@a"}},
		},
	})
	got, _, err := geosite.Derive(tags, geosite.ResidualSpec{
		OutputTag: "out", BasisTags: []string{"basis"}, EarlierActiveTags: []string{"earlier"},
	})
	if err != nil || len(got) != 1 || got[0].Value != "^x.+$" {
		t.Fatalf("got=%#v err=%v", got, err)
	}
}

func Test输出顺序可重现(t *testing.T) {
	one := siteTags(map[string][]model.DomainRule{"basis": {
		{Kind: model.DomainSuffix, Value: "z.example", Attrs: []string{"@b", "@a"}},
		{Kind: model.DomainFull, Value: "a.example"},
	}})
	two := siteTags(map[string][]model.DomainRule{"basis": {
		{Kind: model.DomainFull, Value: "a.example"},
		{Kind: model.DomainSuffix, Value: "z.example", Attrs: []string{"@a", "@b"}},
	}})
	left, _, leftErr := geosite.Derive(one, geosite.ResidualSpec{OutputTag: "out", BasisTags: []string{"basis"}})
	right, _, rightErr := geosite.Derive(two, geosite.ResidualSpec{OutputTag: "out", BasisTags: []string{"basis"}})
	if leftErr != nil || rightErr != nil || !reflect.DeepEqual(left, right) {
		t.Fatalf("left=%#v right=%#v errors=%v/%v", left, right, leftErr, rightErr)
	}
}

func Test覆盖矩阵与Attribute隔离(t *testing.T) {
	tests := map[string]struct {
		candidate model.DomainRule
		existing  model.DomainRule
		covered   bool
	}{
		"full by full":       {candidate: model.DomainRule{Kind: model.DomainFull, Value: "www.example"}, existing: model.DomainRule{Kind: model.DomainFull, Value: "www.example"}, covered: true},
		"full by suffix":     {candidate: model.DomainRule{Kind: model.DomainFull, Value: "www.example"}, existing: model.DomainRule{Kind: model.DomainSuffix, Value: "example"}, covered: true},
		"full by keyword":    {candidate: model.DomainRule{Kind: model.DomainFull, Value: "www.example"}, existing: model.DomainRule{Kind: model.DomainKeyword, Value: "example"}, covered: true},
		"suffix by suffix":   {candidate: model.DomainRule{Kind: model.DomainSuffix, Value: "www.example"}, existing: model.DomainRule{Kind: model.DomainSuffix, Value: "example"}, covered: true},
		"suffix by keyword":  {candidate: model.DomainRule{Kind: model.DomainSuffix, Value: "www.example"}, existing: model.DomainRule{Kind: model.DomainKeyword, Value: "example"}, covered: true},
		"keyword by keyword": {candidate: model.DomainRule{Kind: model.DomainKeyword, Value: "www.example"}, existing: model.DomainRule{Kind: model.DomainKeyword, Value: "example"}, covered: true},
		"attrs differ":       {candidate: model.DomainRule{Kind: model.DomainFull, Value: "www.example", Attrs: []string{"@cn"}}, existing: model.DomainRule{Kind: model.DomainFull, Value: "www.example"}, covered: false},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			tags := siteTags(map[string][]model.DomainRule{
				"basis":   {test.candidate, model.DomainRule{Kind: model.DomainFull, Value: "always-kept.invalid"}},
				"earlier": {test.existing},
			})
			got, _, err := geosite.Derive(tags, geosite.ResidualSpec{
				OutputTag: "out", BasisTags: []string{"basis"}, EarlierActiveTags: []string{"earlier"},
			})
			if err != nil {
				t.Fatal(err)
			}
			wantLength := 2
			if test.covered {
				wantLength = 1
			}
			if len(got) != wantLength {
				t.Fatalf("got=%#v want length %d", got, wantLength)
			}
		})
	}
}

func Test缺失基础与空结果失败(t *testing.T) {
	tests := map[string]struct {
		tags map[string]geodat.SiteTag
		spec geosite.ResidualSpec
	}{
		"missing basis": {
			tags: siteTags(nil),
			spec: geosite.ResidualSpec{OutputTag: "out", BasisTags: []string{"missing"}},
		},
		"empty result": {
			tags: siteTags(map[string][]model.DomainRule{
				"basis":   {{Kind: model.DomainFull, Value: "covered.example"}},
				"earlier": {{Kind: model.DomainSuffix, Value: "example"}},
			}),
			spec: geosite.ResidualSpec{OutputTag: "out", BasisTags: []string{"basis"}, EarlierActiveTags: []string{"earlier"}},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := geosite.Derive(test.tags, test.spec)
			if err == nil || !strings.Contains(err.Error(), "out") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func siteTags(input map[string][]model.DomainRule) map[string]geodat.SiteTag {
	result := make(map[string]geodat.SiteTag, len(input))
	for name, rules := range input {
		result[strings.ToLower(name)] = geodat.SiteTag{Name: name, Domains: append([]model.DomainRule(nil), rules...)}
	}
	return result
}
