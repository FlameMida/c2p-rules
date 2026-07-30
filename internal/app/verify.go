package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"clash-rules-srs/internal/manifest"
	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/tools"
	verifytags "clash-rules-srs/internal/verify"
)

type VerifyOptions struct {
	Dat      string
	Manifest string
	Side     model.Side
	Forbid   bool
}

func Verify(ctx context.Context, options VerifyOptions, runner *tools.Runner) error {
	if options.Dat == "" || options.Manifest == "" {
		return fmt.Errorf("dat and manifest are required")
	}
	if options.Side != model.GeoSite && options.Side != model.GeoIP {
		return fmt.Errorf("invalid verify side %q", options.Side)
	}
	document, err := readManifest(options.Manifest)
	if err != nil {
		return err
	}

	selected := manifest.Document{}
	if options.Side == model.GeoSite {
		selected.Required.GeoSite = document.Required.GeoSite
		selected.Forbidden.GeoSite = document.Forbidden.GeoSite
	} else {
		selected.Required.GeoIP = document.Required.GeoIP
		selected.Forbidden.GeoIP = document.Forbidden.GeoIP
	}
	prober := verifytags.NewProber(runner, options.Dat, options.Dat)
	if options.Forbid {
		return verifytags.Forbidden(ctx, prober, selected)
	}
	return verifytags.Required(ctx, prober, selected)
}

func readManifest(path string) (manifest.Document, error) {
	file, err := os.Open(path)
	if err != nil {
		return manifest.Document{}, fmt.Errorf("open manifest %s: %w", path, err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var document manifest.Document
	if err := decoder.Decode(&document); err != nil {
		return manifest.Document{}, fmt.Errorf("decode manifest %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return manifest.Document{}, fmt.Errorf("manifest %s contains trailing JSON", path)
	}
	return document, nil
}
