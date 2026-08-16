package geosite

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/targets"
)

func WriteGenerated(root, tag string, rules []model.DomainRule) error {
	if !targets.IsGenerated(model.GeoSite, tag) {
		return fmt.Errorf("geosite:%s is not a generated-only target", tag)
	}
	if len(rules) == 0 {
		return fmt.Errorf("generated geosite:%s is empty", tag)
	}
	path := filepath.Join(root, tag)
	exists, err := generatedPathExists(root, tag)
	if err != nil {
		return fmt.Errorf("inspect generated geosite:%s: %w", tag, err)
	}
	if exists {
		return fmt.Errorf("generated geosite:%s already exists", tag)
	}
	if err := mergeTarget(path, rules); err != nil {
		return fmt.Errorf("write generated geosite:%s: %w", tag, err)
	}
	return nil
}

func generatedPathExists(root, tag string) (bool, error) {
	found := false
	err := filepath.WalkDir(root, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type().IsRegular() && strings.EqualFold(entry.Name(), tag) {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found, err
}
