package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"clash-rules-srs/internal/cli"
	"clash-rules-srs/internal/fetch"
	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/releasecmp"
	"clash-rules-srs/internal/tools"
)

func Commands() cli.Commands {
	return cli.Commands{
		Bootstrap: func(ctx context.Context, args []string, _, _ io.Writer) error {
			options, err := parseBootstrapOptions(args)
			if err != nil {
				return err
			}
			return Bootstrap(ctx, options, tools.OSExecutor{})
		},
		Build: func(ctx context.Context, args []string, _, _ io.Writer) error {
			options, err := parseBuildOptions(args)
			if err != nil {
				return err
			}
			client := fetch.New(fetch.Options{})
			runner := &tools.Runner{BinRoot: filepath.Join(options.CacheRoot, "bin")}
			return Build(ctx, options, ProductionDependencies(client, runner))
		},
		Verify: func(ctx context.Context, args []string, _, _ io.Writer) error {
			options, err := parseVerifyOptions(args)
			if err != nil {
				return err
			}
			runner := &tools.Runner{BinRoot: filepath.Join(".cache", "bin")}
			return Verify(ctx, options, runner)
		},
		ReleaseDecision: func(_ context.Context, args []string, out, _ io.Writer) error {
			options, err := parseReleaseDecisionOptions(args)
			if err != nil {
				return err
			}
			return ReleaseDecision(options, out)
		},
	}
}

func parseReleaseDecisionOptions(args []string) (ReleaseDecisionOptions, error) {
	set := newFlagSet("release-decision")
	var options ReleaseDecisionOptions
	var firstRelease, force bool
	set.StringVar(&options.CandidateDir, "candidate", "", "candidate six-asset directory")
	set.StringVar(&options.CandidateTag, "candidate-tag", "", "expected candidate installer release tag")
	set.StringVar(&options.BaselineDir, "baseline", "", "baseline six-asset directory")
	set.StringVar(&options.BaselineTag, "baseline-tag", "", "expected baseline installer release tag")
	set.BoolVar(&firstRelease, "first-release", false, "publish when latest is explicitly absent")
	set.BoolVar(&force, "force", false, "publish a verified candidate without a baseline")
	if err := parseFlags(set, args); err != nil {
		return ReleaseDecisionOptions{}, err
	}
	if options.CandidateDir == "" {
		return ReleaseDecisionOptions{}, usageError(fmt.Errorf("--candidate is required"))
	}
	if options.CandidateTag == "" {
		return ReleaseDecisionOptions{}, usageError(fmt.Errorf("--candidate-tag is required"))
	}
	selected := 0
	if options.BaselineDir != "" {
		selected++
	}
	if firstRelease {
		selected++
	}
	if force {
		selected++
	}
	if selected != 1 {
		return ReleaseDecisionOptions{}, usageError(fmt.Errorf("exactly one of --baseline, --first-release, or --force is required"))
	}
	if options.BaselineDir != "" && options.BaselineTag == "" {
		return ReleaseDecisionOptions{}, usageError(fmt.Errorf("--baseline-tag is required with --baseline"))
	}
	if options.BaselineDir == "" && options.BaselineTag != "" {
		return ReleaseDecisionOptions{}, usageError(fmt.Errorf("--baseline-tag requires --baseline"))
	}
	switch {
	case options.BaselineDir != "":
		options.Mode = releasecmp.Compare
	case firstRelease:
		options.Mode = releasecmp.FirstRelease
	case force:
		options.Mode = releasecmp.Force
	}
	options.CandidateDir = filepath.Clean(options.CandidateDir)
	if options.BaselineDir != "" {
		options.BaselineDir = filepath.Clean(options.BaselineDir)
	}
	return options, nil
}

func parseBootstrapOptions(args []string) (BootstrapOptions, error) {
	set := newFlagSet("bootstrap")
	var options BootstrapOptions
	set.StringVar(&options.CacheRoot, "cache-root", ".cache", "tool and upstream cache root")
	if err := parseFlags(set, args); err != nil {
		return BootstrapOptions{}, err
	}
	options.CacheRoot = filepath.Clean(options.CacheRoot)
	return options, nil
}

func parseBuildOptions(args []string) (BuildOptions, error) {
	set := newFlagSet("build")
	var options BuildOptions
	set.StringVar(&options.Root, "work-root", ".", "repository working root")
	set.StringVar(&options.Sources, "sources", "sources.yaml", "source configuration")
	set.StringVar(&options.Custom, "custom", "custom", "custom rule root")
	set.StringVar(&options.Groups, "groups", "config/passwall2-groups.yaml", "PassWall2 group configuration")
	set.StringVar(&options.Community, "community", ".cache/upstream/domain-list-community/data", "domain-list-community data root")
	set.StringVar(&options.CacheRoot, "cache-root", ".cache", "tool and upstream cache root")
	set.StringVar(&options.Repo, "repo", "", "GitHub OWNER/REPO")
	set.StringVar(&options.ReleaseTag, "release-tag", "", "immutable release tag")
	set.BoolVar(&options.SkipCompile, "skip-compile", false, "prepare inputs without compiling or publishing")
	if err := parseFlags(set, args); err != nil {
		return BuildOptions{}, err
	}
	if options.Repo == "" {
		return BuildOptions{}, usageError(fmt.Errorf("--repo is required"))
	}
	if options.ReleaseTag == "" {
		return BuildOptions{}, usageError(fmt.Errorf("--release-tag is required"))
	}
	options.Root = filepath.Clean(options.Root)
	options.Sources = resolveUnder(options.Root, options.Sources)
	options.Custom = resolveUnder(options.Root, options.Custom)
	options.Groups = resolveUnder(options.Root, options.Groups)
	options.Community = resolveUnder(options.Root, options.Community)
	options.CacheRoot = resolveUnder(options.Root, options.CacheRoot)
	return options, nil
}

func parseVerifyOptions(args []string) (VerifyOptions, error) {
	set := newFlagSet("verify")
	var options VerifyOptions
	var side string
	set.StringVar(&options.Dat, "dat", "", "GeoSite or GeoIP dat file")
	set.StringVar(&options.Manifest, "manifest", "", "expected tag manifest")
	set.StringVar(&side, "side", "", "geosite or geoip")
	set.BoolVar(&options.Forbid, "forbid", false, "verify forbidden tags are absent")
	if err := parseFlags(set, args); err != nil {
		return VerifyOptions{}, err
	}
	if options.Dat == "" {
		return VerifyOptions{}, usageError(fmt.Errorf("--dat is required"))
	}
	if options.Manifest == "" {
		return VerifyOptions{}, usageError(fmt.Errorf("--manifest is required"))
	}
	options.Side = model.Side(side)
	if options.Side != model.GeoSite && options.Side != model.GeoIP {
		return VerifyOptions{}, usageError(fmt.Errorf("--side must be geosite or geoip"))
	}
	options.Dat = filepath.Clean(options.Dat)
	options.Manifest = filepath.Clean(options.Manifest)
	return options, nil
}

func newFlagSet(name string) *flag.FlagSet {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	return set
}

func parseFlags(set *flag.FlagSet, args []string) error {
	if err := set.Parse(args); err != nil {
		return usageError(err)
	}
	if set.NArg() != 0 {
		return usageError(fmt.Errorf("unexpected positional arguments: %v", set.Args()))
	}
	return nil
}

func usageError(err error) error {
	return &cli.UsageError{Err: err}
}

func resolveUnder(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(root, path))
}
