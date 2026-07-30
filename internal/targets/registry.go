package targets

import (
	"fmt"
	"regexp"
	"sort"

	"clash-rules-srs/internal/model"
)

type BaseLookup func(model.Side, string) (bool, error)

type Registry struct {
	final  map[model.Side]map[string]struct{}
	lookup BaseLookup
}

type claim struct {
	mode     model.Mode
	sourceID string
}

var targetPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func New(sources []model.Source, lookup BaseLookup) (*Registry, error) {
	if lookup == nil {
		return nil, fmt.Errorf("base lookup is nil")
	}
	claims := map[model.Side]map[string]claim{
		model.GeoSite: {},
		model.GeoIP:   {},
	}
	for _, source := range sources {
		outputs := []struct {
			side   model.Side
			output *model.Output
		}{
			{model.GeoSite, source.Outputs.GeoSite},
			{model.GeoIP, source.Outputs.GeoIP},
		}
		for _, item := range outputs {
			if item.output == nil {
				continue
			}
			if !targetPattern.MatchString(item.output.Tag) {
				return nil, fmt.Errorf("source %q has invalid %s tag %q", source.ID, item.side, item.output.Tag)
			}
			if item.output.Mode != model.Create && item.output.Mode != model.MergeBase {
				return nil, fmt.Errorf("source %q %s:%s has invalid mode %q", source.ID, item.side, item.output.Tag, item.output.Mode)
			}
			previous, exists := claims[item.side][item.output.Tag]
			if exists && previous.mode != item.output.Mode {
				return nil, fmt.Errorf("conflicting modes for %s:%s: source %q uses %s, source %q uses %s", item.side, item.output.Tag, previous.sourceID, previous.mode, source.ID, item.output.Mode)
			}
			if !exists {
				claims[item.side][item.output.Tag] = claim{mode: item.output.Mode, sourceID: source.ID}
			}
		}
	}

	registry := &Registry{
		final: map[model.Side]map[string]struct{}{
			model.GeoSite: {},
			model.GeoIP:   {},
		},
		lookup: lookup,
	}
	for _, side := range []model.Side{model.GeoSite, model.GeoIP} {
		tags := make([]string, 0, len(claims[side]))
		for tag := range claims[side] {
			tags = append(tags, tag)
		}
		sort.Strings(tags)
		for _, tag := range tags {
			target := claims[side][tag]
			exists, err := lookup(side, tag)
			if err != nil {
				return nil, fmt.Errorf("source %q lookup %s:%s: %w", target.sourceID, side, tag, err)
			}
			switch target.mode {
			case model.Create:
				if exists {
					return nil, fmt.Errorf("source %q %s:%s violates create mode: target exists in base", target.sourceID, side, tag)
				}
			case model.MergeBase:
				if !exists {
					return nil, fmt.Errorf("source %q %s:%s violates merge-base mode: target is absent from base", target.sourceID, side, tag)
				}
			}
			registry.final[side][tag] = struct{}{}
		}
	}
	return registry, nil
}

func (r *Registry) Require(side model.Side, tag string) error {
	if r == nil || r.lookup == nil {
		return fmt.Errorf("target registry is not initialized")
	}
	if tags, ok := r.final[side]; ok {
		if _, exists := tags[tag]; exists {
			return nil
		}
	}
	exists, err := r.lookup(side, tag)
	if err != nil {
		return fmt.Errorf("lookup target %s:%s: %w", side, tag, err)
	}
	if !exists {
		return fmt.Errorf("unknown target %s:%s", side, tag)
	}
	return nil
}
