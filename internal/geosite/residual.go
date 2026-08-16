package geosite

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"clash-rules-srs/internal/geodat"
	"clash-rules-srs/internal/model"
)

type ResidualSpec struct {
	OutputTag           string
	BasisTags           []string
	EarlierActiveTags   []string
	LaterEquivalentTags []string
}

type ResidualReport struct {
	Basis   int
	Kept    int
	Removed int
}

func Derive(tags map[string]geodat.SiteTag, spec ResidualSpec) ([]model.DomainRule, ResidualReport, error) {
	return DeriveContext(context.Background(), tags, spec)
}

func DeriveContext(ctx context.Context, tags map[string]geodat.SiteTag, spec ResidualSpec) ([]model.DomainRule, ResidualReport, error) {
	if spec.OutputTag == "" || len(spec.BasisTags) == 0 {
		return nil, ResidualReport{}, fmt.Errorf("residual %q has no basis tags", spec.OutputTag)
	}
	if err := ctx.Err(); err != nil {
		return nil, ResidualReport{}, fmt.Errorf("residual %s: %w", spec.OutputTag, err)
	}
	basis, err := collect(tags, spec.BasisTags)
	if err != nil {
		return nil, ResidualReport{}, fmt.Errorf("residual %s basis: %w", spec.OutputTag, err)
	}
	coverageNames := append(append([]string(nil), spec.EarlierActiveTags...), spec.LaterEquivalentTags...)
	coverage, err := collect(tags, coverageNames)
	if err != nil {
		return nil, ResidualReport{}, fmt.Errorf("residual %s coverage: %w", spec.OutputTag, err)
	}
	index := newCoverageIndex(coverage)

	kept := make([]model.DomainRule, 0, len(basis))
	for _, candidate := range basis {
		if err := ctx.Err(); err != nil {
			return nil, ResidualReport{}, fmt.Errorf("residual %s: %w", spec.OutputTag, err)
		}
		if !index.covers(candidate) {
			kept = append(kept, candidate)
		}
	}
	if len(kept) == 0 {
		return nil, ResidualReport{}, fmt.Errorf("residual %s is empty", spec.OutputTag)
	}
	report := ResidualReport{Basis: len(basis), Kept: len(kept), Removed: len(basis) - len(kept)}
	return kept, report, nil
}

func collect(tags map[string]geodat.SiteTag, names []string) ([]model.DomainRule, error) {
	byKey := make(map[string]model.DomainRule)
	for _, name := range names {
		record, exists := tags[strings.ToLower(name)]
		if !exists {
			return nil, fmt.Errorf("missing geosite:%s", name)
		}
		for _, rule := range record.Domains {
			normalized := normalize(rule)
			byKey[ruleKey(normalized)] = normalized
		}
	}
	result := make([]model.DomainRule, 0, len(byKey))
	for _, rule := range byKey {
		result = append(result, rule)
	}
	sort.Slice(result, func(i, j int) bool {
		return ruleKey(result[i]) < ruleKey(result[j])
	})
	return result, nil
}

func normalize(rule model.DomainRule) model.DomainRule {
	rule.Attrs = append([]string(nil), rule.Attrs...)
	sort.Strings(rule.Attrs)
	if len(rule.Attrs) > 1 {
		out := rule.Attrs[:1]
		for _, attr := range rule.Attrs[1:] {
			if attr != out[len(out)-1] {
				out = append(out, attr)
			}
		}
		rule.Attrs = out
	}
	return rule
}

func ruleKey(rule model.DomainRule) string {
	return string(rule.Kind) + "\x00" + rule.Value + "\x00" + strings.Join(rule.Attrs, "\x00")
}

// EquivalentCoverage reports whether compiled contains no invented rules and
// covers exactly the same domain universe as expected. It permits a compiler
// to remove a more-specific rule only when another compiled rule with the same
// attributes conservatively covers it.
func EquivalentCoverage(expected, compiled []model.DomainRule) bool {
	expected = uniqueNormalizedRules(expected)
	compiled = uniqueNormalizedRules(compiled)
	expectedKeys := make(map[string]struct{}, len(expected))
	for _, rule := range expected {
		expectedKeys[ruleKey(rule)] = struct{}{}
	}
	for _, rule := range compiled {
		if _, exists := expectedKeys[ruleKey(rule)]; !exists {
			return false
		}
	}
	index := newCoverageIndex(compiled)
	for _, rule := range expected {
		if !index.covers(rule) {
			return false
		}
	}
	return true
}

func uniqueNormalizedRules(rules []model.DomainRule) []model.DomainRule {
	unique := make(map[string]model.DomainRule, len(rules))
	for _, rule := range rules {
		rule = normalize(rule)
		unique[ruleKey(rule)] = rule
	}
	result := make([]model.DomainRule, 0, len(unique))
	for _, rule := range unique {
		result = append(result, rule)
	}
	return result
}

type coverageIndex map[string]*coverageBucket

type coverageBucket struct {
	full    map[string]struct{}
	suffix  map[string]struct{}
	keyword map[string]struct{}
	regexp  map[string]struct{}
}

func newCoverageIndex(rules []model.DomainRule) coverageIndex {
	index := make(coverageIndex)
	for _, rule := range rules {
		attributeKey := strings.Join(rule.Attrs, "\x00")
		bucket := index[attributeKey]
		if bucket == nil {
			bucket = &coverageBucket{
				full: make(map[string]struct{}), suffix: make(map[string]struct{}),
				keyword: make(map[string]struct{}), regexp: make(map[string]struct{}),
			}
			index[attributeKey] = bucket
		}
		switch rule.Kind {
		case model.DomainFull:
			bucket.full[rule.Value] = struct{}{}
		case model.DomainSuffix:
			bucket.suffix[rule.Value] = struct{}{}
		case model.DomainKeyword:
			bucket.keyword[rule.Value] = struct{}{}
		case model.DomainRegexp:
			bucket.regexp[rule.Value] = struct{}{}
		}
	}
	return index
}

func (index coverageIndex) covers(candidate model.DomainRule) bool {
	bucket := index[strings.Join(candidate.Attrs, "\x00")]
	if bucket == nil {
		return false
	}
	switch candidate.Kind {
	case model.DomainFull:
		if _, ok := bucket.full[candidate.Value]; ok || bucket.suffixCovers(candidate.Value) {
			return true
		}
	case model.DomainSuffix:
		if bucket.suffixCovers(candidate.Value) {
			return true
		}
	case model.DomainKeyword:
		return bucket.keywordCovers(candidate.Value)
	case model.DomainRegexp:
		_, ok := bucket.regexp[candidate.Value]
		return ok
	}
	return bucket.keywordCovers(candidate.Value)
}

func (bucket *coverageBucket) suffixCovers(value string) bool {
	for {
		if _, ok := bucket.suffix[value]; ok {
			return true
		}
		separator := strings.IndexByte(value, '.')
		if separator < 0 {
			return false
		}
		value = value[separator+1:]
	}
}

func (bucket *coverageBucket) keywordCovers(value string) bool {
	for keyword := range bucket.keyword {
		if strings.Contains(value, keyword) {
			return true
		}
	}
	return false
}
