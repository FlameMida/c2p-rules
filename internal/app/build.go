package app

import (
	"context"
	"errors"
	"fmt"

	"clash-rules-srs/internal/geodat"
	"clash-rules-srs/internal/geoip"
	"clash-rules-srs/internal/manifest"
	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/targets"
	"clash-rules-srs/internal/verify"
	"clash-rules-srs/internal/workspace"
)

type BuildOptions struct {
	Root        string
	Sources     string
	Custom      string
	Groups      string
	Community   string
	CacheRoot   string
	Repo        string
	ReleaseTag  string
	SkipCompile bool
}

type buildState struct {
	Options          BuildOptions
	Tx               *workspace.Transaction
	Sources          []model.Source
	Groups           []model.Group
	Registry         *targets.Registry
	Contributions    []model.Contribution
	BaseGeoIP        string
	GeoIPTemplate    *geoip.Template
	GeoInputs        []geoip.Input
	Manifest         manifest.Document
	GeoSiteSHA       string
	GeoIPSHA         string
	ExpectedSiteTags map[string]geodat.SiteTag
	FinalLookup      verify.TagLookup
}

type buildStage func(context.Context, *buildState) error

type Dependencies struct {
	Begin                      func(string) (*workspace.Transaction, error)
	LoadConfig                 buildStage
	FetchBaseGeoIP             buildStage
	BuildRegistry              buildStage
	FetchRemote                buildStage
	LoadCustom                 buildStage
	MergeGeoSite               buildStage
	PrepareGeoIP               buildStage
	WriteManifest              buildStage
	CompileIntermediateGeoSite buildStage
	DeriveResidualGeoSite      buildStage
	CompileGeoSite             buildStage
	CompileGeoIP               buildStage
	VerifyFullGeoSiteTags      buildStage
	ProbeTags                  buildStage
	ValidateGroups             buildStage
	RenderInstaller            buildStage
	RenderShadowrocket         buildStage
	VerifyReleaseAssets        buildStage
}

type namedStage struct {
	name string
	run  buildStage
}

func Build(ctx context.Context, options BuildOptions, dependencies Dependencies) (err error) {
	if dependencies.Begin == nil {
		return fmt.Errorf("begin staging: dependency is nil")
	}
	transaction, err := dependencies.Begin(options.Root)
	if err != nil {
		return fmt.Errorf("begin staging: %w", err)
	}
	state := &buildState{Options: options, Tx: transaction}
	committed := false
	defer func() {
		if committed {
			return
		}
		if abortErr := transaction.Abort(); abortErr != nil {
			err = errors.Join(err, fmt.Errorf("abort staging: %w", abortErr))
		}
	}()

	prepare := []namedStage{
		{"load config", dependencies.LoadConfig},
		{"fetch base geoip", dependencies.FetchBaseGeoIP},
		{"validate target registry", dependencies.BuildRegistry},
		{"fetch and parse sources", dependencies.FetchRemote},
		{"load custom rules", dependencies.LoadCustom},
		{"merge geosite", dependencies.MergeGeoSite},
		{"prepare geoip", dependencies.PrepareGeoIP},
		{"write manifest", dependencies.WriteManifest},
	}
	if err := runStages(ctx, state, prepare); err != nil {
		return err
	}
	if options.SkipCompile {
		return nil
	}

	finish := []namedStage{
		{"compile intermediate geosite", dependencies.CompileIntermediateGeoSite},
		{"derive residual geosite", dependencies.DeriveResidualGeoSite},
		{"compile final geosite", dependencies.CompileGeoSite},
		{"compile geoip", dependencies.CompileGeoIP},
		{"verify full geosite tags", dependencies.VerifyFullGeoSiteTags},
		{"validate passwall groups", dependencies.ValidateGroups},
		{"probe required and forbidden tags", dependencies.ProbeTags},
		{"render installer and checksums", dependencies.RenderInstaller},
		{"render shadowrocket conf and checksum", dependencies.RenderShadowrocket},
		{"verify release assets", dependencies.VerifyReleaseAssets},
	}
	if err := runStages(ctx, state, finish); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit build and publish: %w", err)
	}
	committed = true
	return nil
}

func runStages(ctx context.Context, state *buildState, stages []namedStage) error {
	for _, stage := range stages {
		if stage.run == nil {
			return fmt.Errorf("%s: dependency is nil", stage.name)
		}
		if err := stage.run(ctx, state); err != nil {
			return fmt.Errorf("%s: %w", stage.name, err)
		}
	}
	return nil
}
