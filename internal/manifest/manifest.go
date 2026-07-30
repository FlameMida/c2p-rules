package manifest

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"clash-rules-srs/internal/fileutil"
	"clash-rules-srs/internal/model"
)

type Tags struct {
	GeoSite []string `json:"geosite"`
	GeoIP   []string `json:"geoip"`
}

type Target struct {
	Tag  string     `json:"tag"`
	Mode model.Mode `json:"mode"`
}

type SourceRecord struct {
	GeoSite *Target `json:"geosite,omitempty"`
	GeoIP   *Target `json:"geoip,omitempty"`
}

type Document struct {
	SchemaVersion int                     `json:"schema_version"`
	Required      Tags                    `json:"required"`
	Forbidden     Tags                    `json:"forbidden"`
	Sources       map[string]SourceRecord `json:"sources"`
}

var legacyForbidden = Tags{
	GeoSite: []string{
		"applications",
		"loyalsoldier-reject",
		"loyalsoldier-icloud",
		"loyalsoldier-apple",
		"loyalsoldier-google",
		"loyalsoldier-proxy",
		"loyalsoldier-direct",
		"loyalsoldier-private",
		"loyalsoldier-gfw",
		"loyalsoldier-tld-not-cn",
		"xiaolin-youtube",
		"xiaolin-netflix",
		"xiaolin-spotify",
		"xiaolin-bilibili",
		"xiaolin-tiktok",
		"sukka-ai",
	},
	GeoIP: []string{
		"loyalsoldier-telegramcidr",
		"loyalsoldier-cncidr",
		"loyalsoldier-lancidr",
		"xiaolin-netflix",
		"xiaolin-bilibili",
	},
}

func Build(sources []model.Source, groups []model.Group) Document {
	requiredSite := map[string]struct{}{"cn": {}}
	requiredIP := map[string]struct{}{"cn": {}, "private": {}}
	forbiddenSite := stringSet(legacyForbidden.GeoSite)
	forbiddenIP := stringSet(legacyForbidden.GeoIP)
	records := make(map[string]SourceRecord, len(sources))
	for _, source := range sources {
		var record SourceRecord
		if source.Outputs.GeoSite != nil {
			output := source.Outputs.GeoSite
			requiredSite[output.Tag] = struct{}{}
			record.GeoSite = &Target{Tag: output.Tag, Mode: output.Mode}
		}
		if source.Outputs.GeoIP != nil {
			output := source.Outputs.GeoIP
			requiredIP[output.Tag] = struct{}{}
			record.GeoIP = &Target{Tag: output.Tag, Mode: output.Mode}
		}
		records[source.ID] = record
	}
	for _, group := range groups {
		for _, tag := range group.GeoSite {
			requiredSite[tag] = struct{}{}
		}
		for _, tag := range group.GeoIP {
			requiredIP[tag] = struct{}{}
		}
	}
	return Document{
		SchemaVersion: 1,
		Required: Tags{
			GeoSite: sortedKeys(requiredSite),
			GeoIP:   sortedKeys(requiredIP),
		},
		Forbidden: Tags{
			GeoSite: sortedKeys(forbiddenSite),
			GeoIP:   sortedKeys(forbiddenIP),
		},
		Sources: records,
	}
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func Write(path string, document Document) error {
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	data = append(data, '\n')
	if err := fileutil.AtomicWrite(path, data, 0o644); err != nil {
		return fmt.Errorf("write manifest %s: %w", filepath.Clean(path), err)
	}
	return nil
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	sort.Strings(keys)
	return keys
}
