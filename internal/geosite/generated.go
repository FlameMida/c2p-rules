package geosite

import (
	"fmt"
	"os"
	"path/filepath"

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
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("generated geosite:%s already exists", tag)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect generated geosite:%s: %w", tag, err)
	}
	if err := mergeTarget(path, rules); err != nil {
		return fmt.Errorf("write generated geosite:%s: %w", tag, err)
	}
	return nil
}
