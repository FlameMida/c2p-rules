package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"clash-rules-srs/internal/config"
	"clash-rules-srs/internal/fetch"
	"clash-rules-srs/internal/fileutil"
	"clash-rules-srs/internal/geodat"
	"clash-rules-srs/internal/geoip"
	"clash-rules-srs/internal/geosite"
	"clash-rules-srs/internal/manifest"
	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/passwall"
	"clash-rules-srs/internal/rules"
	"clash-rules-srs/internal/shadowrocket"
	"clash-rules-srs/internal/targets"
	"clash-rules-srs/internal/tools"
	"clash-rules-srs/internal/verify"
	"clash-rules-srs/internal/workspace"
)

const (
	maxSourceBytes  = int64(32 * 1024 * 1024)
	maxBaseGeoIPDat = int64(256 * 1024 * 1024)
)

type SourceFetcher interface {
	Get(context.Context, string, int64) ([]byte, error)
}

func ProductionDependencies(fetcher SourceFetcher, runner *tools.Runner) Dependencies {
	return Dependencies{
		Begin: workspace.Begin,
		LoadConfig: func(_ context.Context, state *buildState) error {
			sources, err := config.LoadSources(state.Options.Sources)
			if err != nil {
				return err
			}
			groups, err := config.LoadGroups(state.Options.Groups)
			if err != nil {
				return err
			}
			state.Sources = sources
			state.Groups = groups
			return nil
		},
		FetchBaseGeoIP: func(ctx context.Context, state *buildState) error {
			if fetcher == nil {
				return fmt.Errorf("source fetcher is nil")
			}
			templatePath := filepath.Join(state.Options.Root, "config", "geoip.base.json")
			template, err := geoip.LoadTemplate(templatePath)
			if err != nil {
				return err
			}
			state.GeoIPTemplate = template
			data, err := fetcher.Get(ctx, template.BaseURI(), maxBaseGeoIPDat)
			if err != nil {
				return fmt.Errorf("download base geoip: %w", err)
			}
			if len(data) == 0 {
				return fmt.Errorf("downloaded base geoip is empty")
			}
			state.BaseGeoIP = state.Tx.Layout().BaseGeoIP
			return fileutil.AtomicWrite(state.BaseGeoIP, data, 0o644)
		},
		BuildRegistry: func(ctx context.Context, state *buildState) error {
			if runner == nil {
				return fmt.Errorf("tool runner is nil")
			}
			baseIPProber := verify.NewProber(runner, state.BaseGeoIP, state.BaseGeoIP)
			lookup := func(side model.Side, tag string) (bool, error) {
				switch side {
				case model.GeoSite:
					info, err := os.Stat(filepath.Join(state.Options.Community, tag))
					if os.IsNotExist(err) {
						return false, nil
					}
					if err != nil {
						return false, err
					}
					return info.Mode().IsRegular(), nil
				case model.GeoIP:
					return baseIPProber.Has(ctx, model.GeoIP, tag)
				default:
					return false, fmt.Errorf("invalid side %q", side)
				}
			}
			registry, err := targets.New(state.Sources, lookup)
			if err != nil {
				return err
			}
			state.Registry = registry
			return nil
		},
		FetchRemote: func(ctx context.Context, state *buildState) error {
			if fetcher == nil {
				return fmt.Errorf("source fetcher is nil")
			}
			for _, source := range state.Sources {
				data, err := fetcher.Get(ctx, source.URL, maxSourceBytes)
				if err != nil {
					return fmt.Errorf("source %q: %w", source.ID, err)
				}
				buckets, err := rules.Parse(bytes.NewReader(data), source.Format, source.Behavior)
				if err != nil {
					return fmt.Errorf("source %q: %w", source.ID, err)
				}
				if source.Outputs.GeoSite == nil && len(buckets.Domains) != 0 || source.Outputs.GeoSite != nil && len(buckets.Domains) == 0 {
					return fmt.Errorf("source %q geosite payload does not match outputs declaration", source.ID)
				}
				if source.Outputs.GeoIP == nil && len(buckets.CIDRs) != 0 || source.Outputs.GeoIP != nil && len(buckets.CIDRs) == 0 {
					return fmt.Errorf("source %q geoip payload does not match outputs declaration", source.ID)
				}
				if source.Outputs.GeoSite != nil {
					state.Contributions = append(state.Contributions, model.Contribution{
						SourceID: source.ID, Side: model.GeoSite, Tag: source.Outputs.GeoSite.Tag, Domains: buckets.Domains,
					})
				}
				if source.Outputs.GeoIP != nil {
					state.Contributions = append(state.Contributions, model.Contribution{
						SourceID: source.ID, Side: model.GeoIP, Tag: source.Outputs.GeoIP.Tag, CIDRs: buckets.CIDRs,
					})
				}
			}
			return nil
		},
		LoadCustom: func(_ context.Context, state *buildState) error {
			custom, err := rules.LoadCustom(state.Options.Custom, state.Registry)
			if err != nil {
				return err
			}
			state.Contributions = append(state.Contributions, custom...)
			return nil
		},
		MergeGeoSite: func(_ context.Context, state *buildState) error {
			var site []model.Contribution
			for _, contribution := range state.Contributions {
				if contribution.Side == model.GeoSite {
					site = append(site, contribution)
				}
			}
			return geosite.Merge(state.Options.Community, state.Tx.Layout().DataMerged, site)
		},
		PrepareGeoIP: func(_ context.Context, state *buildState) error {
			var ip []model.Contribution
			for _, contribution := range state.Contributions {
				if contribution.Side == model.GeoIP {
					ip = append(ip, contribution)
				}
			}
			inputs, err := geoip.WriteInputs(state.Tx.Layout().IP, ip)
			if err != nil {
				return err
			}
			state.GeoInputs = inputs
			return geoip.WriteConfig(
				state.GeoIPTemplate,
				inputs,
				state.BaseGeoIP,
				state.Tx.Layout().Publish,
				state.Tx.Layout().GeoIPConfig,
			)
		},
		WriteManifest: func(_ context.Context, state *buildState) error {
			state.Manifest = manifest.Build(state.Sources, state.Groups)
			return manifest.Write(state.Tx.Layout().Manifest, state.Manifest)
		},
		CompileIntermediateGeoSite: func(ctx context.Context, state *buildState) error {
			return compileGeoSite(
				ctx,
				runner,
				state.Options.Root,
				state.Tx.Layout().DataMerged,
				filepath.Dir(state.Tx.Layout().IntermediateGeoSite),
			)
		},
		DeriveResidualGeoSite: func(_ context.Context, state *buildState) error {
			data, err := os.ReadFile(state.Tx.Layout().IntermediateGeoSite)
			if err != nil {
				return fmt.Errorf("read intermediate geosite: %w", err)
			}
			tags, err := geodat.DecodeGeoSite(data)
			if err != nil {
				return fmt.Errorf("decode intermediate geosite: %w", err)
			}
			state.IntermediateSiteTags = tags
			state.ResidualReports = make(map[string]geosite.ResidualReport, 2)

			proxySpec := geosite.ResidualSpec{
				OutputTag:         targets.ProxyResidualTag,
				BasisTags:         []string{"gfw", "proxy"},
				EarlierActiveTags: []string{"reject", "youtube", "netflix", "spotify", "tiktok", "ai", "apple", "icloud", "google"},
				LaterEquivalentTags: []string{
					"tld-not-cn",
				},
			}
			proxy, report, err := geosite.Derive(tags, proxySpec)
			if err != nil {
				return err
			}
			if err := geosite.WriteGenerated(state.Tx.Layout().DataMerged, proxySpec.OutputTag, proxy); err != nil {
				return err
			}
			state.ResidualReports[proxySpec.OutputTag] = report
			tags[strings.ToLower(proxySpec.OutputTag)] = geodat.SiteTag{Name: proxySpec.OutputTag, Domains: proxy}

			directSpec := geosite.ResidualSpec{
				OutputTag: targets.DirectResidualTag,
				BasisTags: []string{"direct"},
				EarlierActiveTags: []string{
					"reject", "youtube", "netflix", "spotify", "tiktok", "ai", "apple", "icloud", "google",
					targets.ProxyResidualTag, "tld-not-cn", "private", "cn",
				},
			}
			direct, report, err := geosite.Derive(tags, directSpec)
			if err != nil {
				return err
			}
			if err := geosite.WriteGenerated(state.Tx.Layout().DataMerged, directSpec.OutputTag, direct); err != nil {
				return err
			}
			state.ResidualReports[directSpec.OutputTag] = report
			return nil
		},
		CompileGeoSite: func(ctx context.Context, state *buildState) error {
			return compileGeoSite(ctx, runner, state.Options.Root, state.Tx.Layout().DataMerged, state.Tx.Layout().Publish)
		},
		CompileGeoIP: func(ctx context.Context, state *buildState) error {
			if runner == nil {
				return fmt.Errorf("tool runner is nil")
			}
			return runner.Run(ctx, "geoip", state.Options.Root, "convert", "-c", state.Tx.Layout().GeoIPConfig)
		},
		VerifyFullGeoSiteTags: func(_ context.Context, state *buildState) error {
			return verifyFullGeoSiteTags(filepath.Join(state.Tx.Layout().Publish, "geosite.dat"), state.IntermediateSiteTags)
		},
		ProbeTags: func(ctx context.Context, state *buildState) error {
			prober := finalProber(runner, state)
			if err := verify.Required(ctx, prober, state.Manifest); err != nil {
				return err
			}
			return verify.Forbidden(ctx, prober, state.Manifest)
		},
		ValidateGroups: func(ctx context.Context, state *buildState) error {
			return passwall.ValidateGroups(ctx, state.Groups, finalProber(runner, state))
		},
		RenderInstaller: func(_ context.Context, state *buildState) error {
			publish := state.Tx.Layout().Publish
			var err error
			state.GeoSiteSHA, err = verify.WriteSHA256(filepath.Join(publish, "geosite.dat"))
			if err != nil {
				return err
			}
			state.GeoIPSHA, err = verify.WriteSHA256(filepath.Join(publish, "geoip.dat"))
			if err != nil {
				return err
			}
			fragment, err := passwall.Render(state.Groups)
			if err != nil {
				return err
			}
			installer, err := passwall.RenderInstaller(passwall.InstallOptions{
				Repo: state.Options.Repo, ReleaseTag: state.Options.ReleaseTag,
				GeoSiteSHA: state.GeoSiteSHA, GeoIPSHA: state.GeoIPSHA, Fragment: fragment,
			})
			if err != nil {
				return err
			}
			installerPath := filepath.Join(publish, "install_passwall2_rules.sh")
			if err := fileutil.AtomicWrite(installerPath, installer, 0o755); err != nil {
				return err
			}
			_, err = verify.WriteSHA256(installerPath)
			return err
		},
		RenderShadowrocket: func(ctx context.Context, state *buildState) error {
			publish := state.Tx.Layout().Publish
			expanded, err := shadowrocket.Expand(
				ctx,
				filepath.Join(publish, "geosite.dat"),
				filepath.Join(publish, "geoip.dat"),
				state.Groups,
			)
			if err != nil {
				return err
			}
			conf, err := shadowrocket.Render(expanded, state.Options.Repo)
			if err != nil {
				return err
			}
			path := filepath.Join(publish, "shadowrocket.conf")
			if err := fileutil.AtomicWrite(path, conf, 0o644); err != nil {
				return err
			}
			_, err = verify.WriteSHA256(path)
			return err
		},
		VerifyReleaseAssets: func(_ context.Context, state *buildState) error {
			return verify.Assets(state.Tx.Layout().Publish, verify.ReleaseAssets())
		},
	}
}

func compileGeoSite(ctx context.Context, runner *tools.Runner, root, dataPath, outputPath string) error {
	if runner == nil {
		return fmt.Errorf("tool runner is nil")
	}
	if err := os.MkdirAll(outputPath, 0o755); err != nil {
		return fmt.Errorf("create geosite output: %w", err)
	}
	if err := runner.Run(ctx, "domain-list-custom", root,
		"--datapath="+dataPath,
		"--datname=geosite.dat",
		"--outputpath="+outputPath,
		"--exportlists=",
		"--togfwlist=",
	); err != nil {
		return err
	}
	// The pinned compiler creates an empty gfwlist.txt even when
	// --togfwlist is empty. It is not part of the release contract.
	if err := os.Remove(filepath.Join(outputPath, "gfwlist.txt")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove compiler side asset: %w", err)
	}
	return nil
}

func verifyFullGeoSiteTags(finalPath string, intermediate map[string]geodat.SiteTag) error {
	data, err := os.ReadFile(finalPath)
	if err != nil {
		return fmt.Errorf("read final geosite: %w", err)
	}
	final, err := geodat.DecodeGeoSite(data)
	if err != nil {
		return fmt.Errorf("decode final geosite: %w", err)
	}
	for _, tag := range []string{"gfw", "proxy", "direct", "reject"} {
		before, beforeOK := intermediate[tag]
		after, afterOK := final[tag]
		if !beforeOK || !afterOK || !reflect.DeepEqual(before.Domains, after.Domains) {
			return fmt.Errorf("full geosite:%s fingerprint changed", tag)
		}
	}
	for _, tag := range targets.GeneratedGeoSiteTags() {
		record, ok := final[tag]
		if !ok || len(record.Domains) == 0 {
			return fmt.Errorf("generated geosite:%s is missing or empty", tag)
		}
	}
	return nil
}

func finalProber(runner *tools.Runner, state *buildState) verify.TagLookup {
	if state.FinalLookup == nil {
		state.FinalLookup = &cachedTagLookup{
			delegate: verify.NewProber(
				runner,
				filepath.Join(state.Tx.Layout().Publish, "geosite.dat"),
				filepath.Join(state.Tx.Layout().Publish, "geoip.dat"),
			),
			results: make(map[tagLookupKey]tagLookupResult),
		}
	}
	return state.FinalLookup
}

type tagLookupKey struct {
	side model.Side
	tag  string
}

type tagLookupResult struct {
	present bool
	err     error
}

type cachedTagLookup struct {
	delegate verify.TagLookup
	results  map[tagLookupKey]tagLookupResult
}

func (c *cachedTagLookup) Has(ctx context.Context, side model.Side, tag string) (bool, error) {
	key := tagLookupKey{side: side, tag: tag}
	if result, exists := c.results[key]; exists {
		return result.present, result.err
	}
	present, err := c.delegate.Has(ctx, side, tag)
	c.results[key] = tagLookupResult{present: present, err: err}
	return present, err
}

var _ SourceFetcher = (*fetch.Client)(nil)
