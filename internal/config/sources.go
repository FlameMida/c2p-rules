package config

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"

	"clash-rules-srs/internal/model"
)

var targetNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type sourcesDocument struct {
	Sources *[]sourceYAML `yaml:"sources"`
}

type sourceYAML struct {
	ID       string         `yaml:"id"`
	Behavior model.Behavior `yaml:"behavior"`
	Format   model.Format   `yaml:"format,omitempty"`
	URL      string         `yaml:"url"`
	Outputs  *outputsYAML   `yaml:"outputs"`
}

type outputsYAML struct {
	GeoSite *outputYAML `yaml:"geosite,omitempty"`
	GeoIP   *outputYAML `yaml:"geoip,omitempty"`
}

type outputYAML struct {
	Tag  string     `yaml:"tag"`
	Mode model.Mode `yaml:"mode"`
}

func ParseSources(r io.Reader) ([]model.Source, error) {
	var document sourcesDocument
	if err := DecodeStrict(r, &document); err != nil {
		return nil, err
	}
	if document.Sources == nil {
		return nil, fmt.Errorf("sources field is required")
	}

	seen := make(map[string]struct{}, len(*document.Sources))
	sources := make([]model.Source, 0, len(*document.Sources))
	for index, raw := range *document.Sources {
		if !targetNamePattern.MatchString(raw.ID) {
			return nil, fmt.Errorf("source %d has invalid id %q", index, raw.ID)
		}
		if _, exists := seen[raw.ID]; exists {
			return nil, fmt.Errorf("duplicate source id %q", raw.ID)
		}
		seen[raw.ID] = struct{}{}

		if raw.Format == "" {
			raw.Format = model.YAML
		}
		if raw.Format != model.YAML && raw.Format != model.Text {
			return nil, fmt.Errorf("source %q has invalid format %q", raw.ID, raw.Format)
		}
		if raw.Behavior != model.Domain && raw.Behavior != model.IPCIDR && raw.Behavior != model.Classical {
			return nil, fmt.Errorf("source %q has invalid behavior %q", raw.ID, raw.Behavior)
		}
		parsedURL, err := url.ParseRequestURI(raw.URL)
		if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" || parsedURL.User != nil {
			return nil, fmt.Errorf("source %q URL must be absolute HTTPS", raw.ID)
		}
		if raw.Outputs == nil || raw.Outputs.GeoSite == nil && raw.Outputs.GeoIP == nil {
			return nil, fmt.Errorf("source %q must declare at least one output", raw.ID)
		}
		if raw.Behavior == model.Domain && raw.Outputs.GeoIP != nil {
			return nil, fmt.Errorf("source %q behavior domain cannot declare geoip", raw.ID)
		}
		if raw.Behavior == model.IPCIDR && raw.Outputs.GeoSite != nil {
			return nil, fmt.Errorf("source %q behavior ipcidr cannot declare geosite", raw.ID)
		}

		source := model.Source{ID: raw.ID, Behavior: raw.Behavior, Format: raw.Format, URL: raw.URL}
		if raw.Outputs.GeoSite != nil {
			output, err := validateOutput(raw.ID, model.GeoSite, raw.Outputs.GeoSite)
			if err != nil {
				return nil, err
			}
			source.Outputs.GeoSite = output
		}
		if raw.Outputs.GeoIP != nil {
			output, err := validateOutput(raw.ID, model.GeoIP, raw.Outputs.GeoIP)
			if err != nil {
				return nil, err
			}
			source.Outputs.GeoIP = output
		}
		sources = append(sources, source)
	}
	return sources, nil
}

func validateOutput(sourceID string, side model.Side, raw *outputYAML) (*model.Output, error) {
	if !targetNamePattern.MatchString(raw.Tag) {
		return nil, fmt.Errorf("source %q %s output has invalid tag %q", sourceID, side, raw.Tag)
	}
	if raw.Mode != model.Create && raw.Mode != model.MergeBase {
		return nil, fmt.Errorf("source %q %s:%s has invalid mode %q", sourceID, side, raw.Tag, raw.Mode)
	}
	return &model.Output{Tag: raw.Tag, Mode: raw.Mode}, nil
}

func LoadSources(path string) ([]model.Source, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open sources %s: %w", path, err)
	}
	defer file.Close()
	sources, err := ParseSources(file)
	if err != nil {
		return nil, fmt.Errorf("parse sources %s: %w", path, err)
	}
	return sources, nil
}
