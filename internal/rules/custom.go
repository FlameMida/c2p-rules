package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"clash-rules-srs/internal/model"
)

type TargetChecker interface {
	Require(model.Side, string) error
}

func LoadCustom(root string, checker TargetChecker) ([]model.Contribution, error) {
	if checker == nil {
		return nil, fmt.Errorf("custom target checker is nil")
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("open custom root %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("custom root %s is not a directory", root)
	}

	var contributions []model.Contribution
	for _, side := range []model.Side{model.GeoSite, model.GeoIP} {
		dir := filepath.Join(root, string(side))
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read custom directory %s: %w", dir, err)
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			if entry.Type()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("custom rule symlink is not allowed: %s", path)
			}
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
				continue
			}
			tag := strings.TrimSuffix(entry.Name(), ".yaml")
			if tag == "" {
				return nil, fmt.Errorf("custom rule has empty target: %s", path)
			}
			if err := checker.Require(side, tag); err != nil {
				return nil, fmt.Errorf("custom rule %s: %w", path, err)
			}
			file, err := os.Open(path)
			if err != nil {
				return nil, fmt.Errorf("open custom rule %s: %w", path, err)
			}
			buckets, parseErr := parse(file, model.YAML, model.Classical, true)
			closeErr := file.Close()
			if parseErr != nil {
				return nil, fmt.Errorf("parse custom rule %s: %w", path, parseErr)
			}
			if closeErr != nil {
				return nil, fmt.Errorf("close custom rule %s: %w", path, closeErr)
			}
			if len(buckets.Skipped) != 0 {
				return nil, fmt.Errorf("custom rule %s contains unsupported rule %q", path, buckets.Skipped[0])
			}
			if side == model.GeoSite && len(buckets.CIDRs) != 0 {
				return nil, fmt.Errorf("custom geosite rule %s contains an IP rule", path)
			}
			if side == model.GeoIP && len(buckets.Domains) != 0 {
				return nil, fmt.Errorf("custom geoip rule %s contains a domain rule", path)
			}
			if len(buckets.Domains) == 0 && len(buckets.CIDRs) == 0 {
				continue
			}
			contributions = append(contributions, model.Contribution{
				SourceID: "custom/" + string(side) + "/" + tag,
				Side:     side,
				Tag:      tag,
				Domains:  buckets.Domains,
				CIDRs:    buckets.CIDRs,
			})
		}
	}
	return contributions, nil
}
