package verify

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"clash-rules-srs/internal/manifest"
	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/tools"
)

type TagLookup interface {
	Has(context.Context, model.Side, string) (bool, error)
}

type Prober struct {
	runner      *tools.Runner
	geoSitePath string
	geoIPPath   string
}

var tagPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func NewProber(runner *tools.Runner, geoSitePath, geoIPPath string) *Prober {
	return &Prober{runner: runner, geoSitePath: geoSitePath, geoIPPath: geoIPPath}
}

func (p *Prober) Has(ctx context.Context, side model.Side, tag string) (bool, error) {
	if p == nil || p.runner == nil {
		return false, fmt.Errorf("geoview prober is not initialized")
	}
	if !tagPattern.MatchString(tag) {
		return false, fmt.Errorf("invalid %s tag %q", side, tag)
	}
	var datPath string
	switch side {
	case model.GeoSite:
		datPath = p.geoSitePath
	case model.GeoIP:
		datPath = p.geoIPPath
	default:
		return false, fmt.Errorf("invalid probe side %q", side)
	}
	info, err := os.Stat(datPath)
	if err != nil {
		return false, fmt.Errorf("open %s dat %s: %w", side, datPath, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%s dat is not a regular file: %s", side, datPath)
	}
	temporary, err := os.MkdirTemp("", "geoview-probe-*")
	if err != nil {
		return false, fmt.Errorf("create geoview probe directory: %w", err)
	}
	defer os.RemoveAll(temporary)
	output := filepath.Join(temporary, "probe.srs")
	if err := p.runner.Run(ctx, "geoview", "",
		"-type", string(side),
		"-action", "convert",
		"-input", datPath,
		"-list", tag,
		"-output", output,
		"-lowmem=true",
	); err != nil {
		return false, fmt.Errorf("probe %s:%s: %w", side, tag, err)
	}
	outputInfo, err := os.Stat(output)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat probe output for %s:%s: %w", side, tag, err)
	}
	return outputInfo.Mode().IsRegular() && outputInfo.Size() > 0, nil
}

func Required(ctx context.Context, lookup TagLookup, document manifest.Document) error {
	if lookup == nil {
		return fmt.Errorf("required tag lookup is nil")
	}
	return checkTags(ctx, lookup, document.Required, true)
}

func Forbidden(ctx context.Context, lookup TagLookup, document manifest.Document) error {
	if lookup == nil {
		return fmt.Errorf("forbidden tag lookup is nil")
	}
	return checkTags(ctx, lookup, document.Forbidden, false)
}

func GroupRefs(ctx context.Context, lookup TagLookup, groups []model.Group) error {
	if lookup == nil {
		return fmt.Errorf("group tag lookup is nil")
	}
	for _, group := range groups {
		for _, item := range []struct {
			side model.Side
			tags []string
		}{
			{model.GeoSite, group.GeoSite},
			{model.GeoIP, group.GeoIP},
		} {
			for _, tag := range item.tags {
				present, err := lookup.Has(ctx, item.side, tag)
				if err != nil {
					return fmt.Errorf("group %q probe %s:%s: %w", group.Remarks, item.side, tag, err)
				}
				if !present {
					return fmt.Errorf("group %q references missing %s:%s", group.Remarks, item.side, tag)
				}
			}
		}
	}
	return nil
}

func checkTags(ctx context.Context, lookup TagLookup, tags manifest.Tags, required bool) error {
	for _, item := range []struct {
		side model.Side
		tags []string
	}{
		{model.GeoSite, tags.GeoSite},
		{model.GeoIP, tags.GeoIP},
	} {
		for _, tag := range item.tags {
			present, err := lookup.Has(ctx, item.side, tag)
			if err != nil {
				return fmt.Errorf("probe %s:%s: %w", item.side, tag, err)
			}
			if required && !present {
				return fmt.Errorf("required tag is missing or empty: %s:%s", item.side, tag)
			}
			if !required && present {
				return fmt.Errorf("forbidden tag exists: %s:%s", item.side, tag)
			}
		}
	}
	return nil
}
