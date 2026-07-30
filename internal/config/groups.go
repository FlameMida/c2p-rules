package config

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"clash-rules-srs/internal/model"
)

var groupIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`)

type groupsDocument struct {
	Groups *[]groupYAML `yaml:"groups"`
}

type groupYAML struct {
	ID      string    `yaml:"id"`
	Remarks string    `yaml:"remarks"`
	GeoSite *[]string `yaml:"geosite"`
	GeoIP   *[]string `yaml:"geoip"`
}

func ParseGroups(r io.Reader) ([]model.Group, error) {
	var document groupsDocument
	if err := DecodeStrict(r, &document); err != nil {
		return nil, err
	}
	if document.Groups == nil {
		return nil, fmt.Errorf("groups field is required")
	}

	seen := make(map[string]struct{}, len(*document.Groups))
	groups := make([]model.Group, 0, len(*document.Groups))
	for index, raw := range *document.Groups {
		if !groupIDPattern.MatchString(raw.ID) {
			return nil, fmt.Errorf("group %d has invalid id %q", index, raw.ID)
		}
		if _, exists := seen[raw.ID]; exists {
			return nil, fmt.Errorf("duplicate group id %q", raw.ID)
		}
		seen[raw.ID] = struct{}{}
		if raw.Remarks == "" || strings.TrimSpace(raw.Remarks) != raw.Remarks || hasForbiddenControl(raw.Remarks) {
			return nil, fmt.Errorf("group %q has invalid remarks", raw.ID)
		}
		if raw.GeoSite == nil {
			return nil, fmt.Errorf("group %q must declare geosite", raw.ID)
		}
		if raw.GeoIP == nil {
			return nil, fmt.Errorf("group %q must declare geoip", raw.ID)
		}
		geoSite, err := validateTags(raw.ID, model.GeoSite, *raw.GeoSite)
		if err != nil {
			return nil, err
		}
		geoIP, err := validateTags(raw.ID, model.GeoIP, *raw.GeoIP)
		if err != nil {
			return nil, err
		}
		if len(geoSite) == 0 && len(geoIP) == 0 {
			return nil, fmt.Errorf("group %q must reference at least one tag", raw.ID)
		}
		groups = append(groups, model.Group{ID: raw.ID, Remarks: raw.Remarks, GeoSite: geoSite, GeoIP: geoIP})
	}
	return groups, nil
}

func validateTags(groupID string, side model.Side, tags []string) ([]string, error) {
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, len(tags))
	copy(result, tags)
	for _, tag := range tags {
		if !targetNamePattern.MatchString(tag) {
			return nil, fmt.Errorf("group %q has invalid %s tag %q", groupID, side, tag)
		}
		if _, exists := seen[tag]; exists {
			return nil, fmt.Errorf("group %q repeats %s:%s", groupID, side, tag)
		}
		seen[tag] = struct{}{}
	}
	return result, nil
}

func LoadGroups(path string) ([]model.Group, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open groups %s: %w", path, err)
	}
	defer file.Close()
	groups, err := ParseGroups(file)
	if err != nil {
		return nil, fmt.Errorf("parse groups %s: %w", path, err)
	}
	return groups, nil
}
