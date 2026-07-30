package app

import (
	"bytes"
	"context"
	"testing"

	"clash-rules-srs/internal/cli"
)

func TestCommandUsageErrorsReturnExitTwo(t *testing.T) {
	commands := Commands()
	for name, args := range map[string][]string{
		"build missing required": {"build"},
		"build unknown flag":     {"build", "--unknown"},
		"verify invalid side":    {"verify", "--dat", "x", "--manifest", "y", "--side", "bad"},
	} {
		t.Run(name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := cli.Run(context.Background(), args, &out, &errOut, commands); code != 2 {
				t.Fatalf("code=%d stderr=%q", code, errOut.String())
			}
		})
	}
}

func TestBootstrapDefaultsToDotCache(t *testing.T) {
	options, err := parseBootstrapOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	if options.CacheRoot != ".cache" {
		t.Fatalf("cache=%q", options.CacheRoot)
	}
}

func TestBuildPathFlagsResolveUnderRoot(t *testing.T) {
	root := t.TempDir()
	options, err := parseBuildOptions([]string{"--root", root, "--repo", "flame/repo", "--release-tag", "v1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{options.Sources, options.Custom, options.Groups, options.Community, options.CacheRoot} {
		if len(path) <= len(root) || path[:len(root)] != root {
			t.Fatalf("path %q not under %q", path, root)
		}
	}
}
