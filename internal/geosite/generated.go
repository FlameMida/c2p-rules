package geosite

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"clash-rules-srs/internal/fileutil"
	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/targets"
)

func WriteResidual(root, tag string, rules []model.DomainRule, removeTags ...string) error {
	valid := false
	for _, residual := range targets.ResidualGeoSiteTags() {
		if tag == residual {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("geosite:%s is not a residual target", tag)
	}
	if len(rules) == 0 {
		return fmt.Errorf("residual geosite:%s is empty", tag)
	}
	path := filepath.Join(root, tag)
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect residual geosite:%s: %w", tag, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("residual geosite:%s target is not a regular file", tag)
	}
	removePaths := make([]string, 0, len(removeTags))
	seenRemovals := make(map[string]struct{}, len(removeTags))
	for _, removed := range removeTags {
		if removed == tag {
			return fmt.Errorf("residual geosite:%s cannot remove itself", tag)
		}
		if removed == "" || filepath.Base(removed) != removed {
			return fmt.Errorf("remove merged geosite:%s has unsafe tag", removed)
		}
		if _, exists := seenRemovals[removed]; exists {
			continue
		}
		removedPath := filepath.Join(root, removed)
		removedInfo, err := os.Stat(removedPath)
		if err != nil {
			return fmt.Errorf("inspect merged geosite:%s: %w", removed, err)
		}
		if !removedInfo.Mode().IsRegular() {
			return fmt.Errorf("merged geosite:%s target is not a regular file", removed)
		}
		seenRemovals[removed] = struct{}{}
		removePaths = append(removePaths, removedPath)
	}
	seen := make(map[string]struct{}, len(rules))
	lines := make([]string, 0, len(rules))
	for _, rule := range rules {
		line, key, err := encodeRule(rule)
		if err != nil {
			return fmt.Errorf("write residual geosite:%s: %w", tag, err)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		lines = append(lines, line)
	}
	content := []byte(strings.Join(lines, "\n") + "\n")
	if err := fileutil.AtomicWrite(path, content, 0o644); err != nil {
		return fmt.Errorf("write residual geosite:%s: %w", tag, err)
	}
	for _, removedPath := range removePaths {
		if err := os.Remove(removedPath); err != nil {
			return fmt.Errorf("remove merged geosite:%s: %w", filepath.Base(removedPath), err)
		}
	}
	return nil
}
