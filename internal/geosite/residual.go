package geosite

import (
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
	if spec.OutputTag == "" || len(spec.BasisTags) == 0 {
		return nil, ResidualReport{}, fmt.Errorf("residual %q has no basis tags", spec.OutputTag)
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

	kept := make([]model.DomainRule, 0, len(basis))
	for _, candidate := range basis {
		covered := false
		for _, existing := range coverage {
			if covers(existing, candidate) {
				covered = true
				break
			}
		}
		if !covered {
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

func covers(existing, candidate model.DomainRule) bool {
	if strings.Join(existing.Attrs, "\x00") != strings.Join(candidate.Attrs, "\x00") {
		return false
	}
	switch candidate.Kind {
	case model.DomainFull:
		switch existing.Kind {
		case model.DomainFull:
			return existing.Value == candidate.Value
		case model.DomainSuffix:
			return sameOrParent(existing.Value, candidate.Value)
		case model.DomainKeyword:
			return strings.Contains(candidate.Value, existing.Value)
		}
	case model.DomainSuffix:
		switch existing.Kind {
		case model.DomainSuffix:
			return sameOrParent(existing.Value, candidate.Value)
		case model.DomainKeyword:
			return strings.Contains(candidate.Value, existing.Value)
		}
	case model.DomainKeyword:
		return existing.Kind == model.DomainKeyword && strings.Contains(candidate.Value, existing.Value)
	case model.DomainRegexp:
		return existing.Kind == model.DomainRegexp && existing.Value == candidate.Value
	}
	return false
}

func sameOrParent(parent, value string) bool {
	return value == parent || strings.HasSuffix(value, "."+parent)
}
