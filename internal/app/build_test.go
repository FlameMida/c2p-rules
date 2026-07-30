package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"clash-rules-srs/internal/manifest"
	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/verify"
	"clash-rules-srs/internal/workspace"
)

func TestBuildEmitsAndProbesEveryOutput(t *testing.T) {
	fixture := newBuildFixture(t)
	google := model.Output{Tag: "google", Mode: model.MergeBase}
	netflixSite := model.Output{Tag: "netflix", Mode: model.MergeBase}
	netflixIP := model.Output{Tag: "netflix", Mode: model.MergeBase}
	fixture.sources = []model.Source{
		{ID: "loyalsoldier-google", Outputs: model.Outputs{GeoSite: &google}},
		{ID: "xiaolin-netflix", Outputs: model.Outputs{GeoSite: &netflixSite, GeoIP: &netflixIP}},
	}
	if err := Build(context.Background(), fixture.options(), fixture.dependencies()); err != nil {
		t.Fatal(err)
	}
	for _, call := range []string{"geosite:google", "geosite:netflix", "geoip:netflix"} {
		if !slices.Contains(fixture.probes, call) {
			t.Fatalf("missing probe %s in %v", call, fixture.probes)
		}
	}
	if slices.Contains(fixture.required.GeoSite, "loyalsoldier-google") {
		t.Fatalf("legacy tag in manifest: %#v", fixture.required)
	}
	if err := verify.Assets(filepath.Join(fixture.root, "publish"), releaseAssets); err != nil {
		t.Fatal(err)
	}
}

func TestBuildSourceFailureDoesNotSwitchPublish(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.seedPublished("old")
	fixture.failStage = "fetch and parse sources"
	fixture.failError = errors.New("HTTP 404")
	err := Build(context.Background(), fixture.options(), fixture.dependencies())
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("err=%v", err)
	}
	fixture.assertBuildAndPublished("old")
}

func TestBuildUnknownCustomTargetDoesNotSwitchPublish(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.seedPublished("old")
	fixture.failStage = "load custom rules"
	fixture.failError = errors.New("custom/geosite/googel.yaml: unknown target geosite:googel")
	err := Build(context.Background(), fixture.options(), fixture.dependencies())
	if err == nil || !strings.Contains(err.Error(), "geosite:googel") {
		t.Fatalf("err=%v", err)
	}
	fixture.assertBuildAndPublished("old")
}

func TestBuildMissingGroupTagDoesNotPublishInstaller(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.seedPublished("old")
	fixture.failStage = "validate passwall groups"
	fixture.failError = errors.New("group 坏组 references missing geoip:not-exist")
	err := Build(context.Background(), fixture.options(), fixture.dependencies())
	if err == nil || !strings.Contains(err.Error(), "坏组") || !strings.Contains(err.Error(), "geoip:not-exist") {
		t.Fatalf("err=%v", err)
	}
	fixture.assertBuildAndPublished("old")
}

func TestBuildCreateCollisionDoesNotSwitchPublish(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.seedPublished("old")
	fixture.failStage = "validate target registry"
	fixture.failError = errors.New("source-a geosite:collision violates create mode")
	err := Build(context.Background(), fixture.options(), fixture.dependencies())
	if err == nil || !strings.Contains(err.Error(), "source-a") || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("err=%v", err)
	}
	fixture.assertBuildAndPublished("old")
}

func TestBuildForbiddenProbeFailurePreservesOldOutputs(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.seedPublished("old")
	fixture.failStage = "probe required and forbidden tags"
	fixture.failError = errors.New("forbidden tag exists: geosite:loyalsoldier-google")
	err := Build(context.Background(), fixture.options(), fixture.dependencies())
	if err == nil || !strings.Contains(err.Error(), "loyalsoldier-google") {
		t.Fatalf("err=%v", err)
	}
	fixture.assertBuildAndPublished("old")
}

func TestBuildSkipCompileDoesNotSwitchOutputs(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.seedPublished("old")
	options := fixture.options()
	options.SkipCompile = true
	if err := Build(context.Background(), options, fixture.dependencies()); err != nil {
		t.Fatal(err)
	}
	fixture.assertBuildAndPublished("old")
	if slices.Contains(fixture.calls, "compile geosite") {
		t.Fatalf("compile ran: %v", fixture.calls)
	}
}

type buildFixture struct {
	t         *testing.T
	root      string
	sources   []model.Source
	groups    []model.Group
	calls     []string
	probes    []string
	required  manifest.Tags
	failStage string
	failError error
}

func newBuildFixture(t *testing.T) *buildFixture {
	t.Helper()
	return &buildFixture{t: t, root: t.TempDir()}
}

func (f *buildFixture) options() BuildOptions {
	return BuildOptions{Root: f.root, Repo: "flame/repo", ReleaseTag: "v1"}
}

func (f *buildFixture) dependencies() Dependencies {
	stage := func(name string, action func(*buildState) error) buildStage {
		return func(_ context.Context, state *buildState) error {
			f.calls = append(f.calls, name)
			if f.failStage == name {
				return f.failError
			}
			if action != nil {
				return action(state)
			}
			return nil
		}
	}
	return Dependencies{
		Begin: workspace.Begin,
		LoadConfig: stage("load config", func(state *buildState) error {
			state.Sources = f.sources
			state.Groups = f.groups
			return nil
		}),
		FetchBaseGeoIP: stage("fetch base geoip", nil),
		BuildRegistry:  stage("validate target registry", nil),
		FetchRemote:    stage("fetch and parse sources", nil),
		LoadCustom:     stage("load custom rules", nil),
		MergeGeoSite:   stage("merge geosite", nil),
		PrepareGeoIP:   stage("prepare geoip", nil),
		WriteManifest: stage("write manifest", func(state *buildState) error {
			state.Manifest = manifest.Build(state.Sources, state.Groups)
			f.required = state.Manifest.Required
			return manifest.Write(state.Tx.Layout().Manifest, state.Manifest)
		}),
		CompileGeoSite: stage("compile geosite", func(state *buildState) error {
			return os.WriteFile(filepath.Join(state.Tx.Layout().Publish, "geosite.dat"), []byte("site"), 0o600)
		}),
		CompileGeoIP: stage("compile geoip", func(state *buildState) error {
			return os.WriteFile(filepath.Join(state.Tx.Layout().Publish, "geoip.dat"), []byte("ip"), 0o600)
		}),
		ProbeTags: stage("probe required and forbidden tags", func(state *buildState) error {
			for _, tag := range state.Manifest.Required.GeoSite {
				f.probes = append(f.probes, "geosite:"+tag)
			}
			for _, tag := range state.Manifest.Required.GeoIP {
				f.probes = append(f.probes, "geoip:"+tag)
			}
			return nil
		}),
		ValidateGroups: stage("validate passwall groups", nil),
		RenderInstaller: stage("render installer and checksums", func(state *buildState) error {
			script := filepath.Join(state.Tx.Layout().Publish, "install_passwall2_rules.sh")
			if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o700); err != nil {
				return err
			}
			for _, name := range []string{"geosite.dat", "geoip.dat", "install_passwall2_rules.sh"} {
				if _, err := verify.WriteSHA256(filepath.Join(state.Tx.Layout().Publish, name)); err != nil {
					return err
				}
			}
			return nil
		}),
		VerifySixAssets: stage("verify six assets", func(state *buildState) error {
			return verify.Assets(state.Tx.Layout().Publish, releaseAssets)
		}),
	}
}

func (f *buildFixture) seedPublished(value string) {
	f.t.Helper()
	for _, directory := range []string{"build", "publish"} {
		path := filepath.Join(f.root, directory)
		if err := os.MkdirAll(path, 0o755); err != nil {
			f.t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "marker"), []byte(value), 0o600); err != nil {
			f.t.Fatal(err)
		}
	}
}

func (f *buildFixture) assertBuildAndPublished(value string) {
	f.t.Helper()
	for _, directory := range []string{"build", "publish"} {
		data, err := os.ReadFile(filepath.Join(f.root, directory, "marker"))
		if err != nil || string(data) != value {
			f.t.Fatalf("%s marker=%q err=%v", directory, data, err)
		}
	}
}
