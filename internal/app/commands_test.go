package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"clash-rules-srs/internal/cli"
	"clash-rules-srs/internal/verify"
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

func Test发布判定模式缺失或冲突(t *testing.T) {
	for name, args := range map[string][]string{
		"missing":                     {"--candidate", "publish"},
		"baseline and first":          {"--candidate", "publish", "--baseline", "old", "--first-release"},
		"baseline and force":          {"--candidate", "publish", "--baseline", "old", "--force"},
		"first and force":             {"--candidate", "publish", "--first-release", "--force"},
		"all three":                   {"--candidate", "publish", "--baseline", "old", "--first-release", "--force"},
		"unknown flag":                {"--candidate", "publish", "--force", "--unknown"},
		"unexpected positional value": {"--candidate", "publish", "--force", "extra"},
	} {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := runReleaseDecision(args)
			if code != 2 || stdout != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			if name != "unknown flag" && name != "unexpected positional value" && !strings.Contains(stderr, "exactly one") {
				t.Fatalf("stderr=%q", stderr)
			}
		})
	}
}

func TestReleaseDecisionRequiresCandidate(t *testing.T) {
	for name, args := range map[string][]string{
		"baseline": {"--baseline", "old"},
		"first":    {"--first-release"},
		"force":    {"--force"},
	} {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := runReleaseDecision(args)
			if code != 2 || stdout != "" || !strings.Contains(stderr, "--candidate is required") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func TestReleaseDecisionPrintsStableOutput(t *testing.T) {
	candidate := releasePayload(t, "candidate", "same-site", "same-ip")
	baseline := releasePayload(t, "baseline", "same-site", "same-ip")
	code, stdout, stderr := runReleaseDecision([]string{"--candidate", candidate, "--baseline", baseline})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	pattern := regexp.MustCompile(`^should_publish=false\nreason=unchanged\nbaseline_fingerprint=[0-9a-f]{64}\n$`)
	if !pattern.MatchString(stdout) {
		t.Fatalf("stdout=%q", stdout)
	}
}

func Test首次或强制模式下候选损坏CLI(t *testing.T) {
	for _, mode := range []string{"--first-release", "--force"} {
		t.Run(mode, func(t *testing.T) {
			code, stdout, stderr := runReleaseDecision([]string{"--candidate", t.TempDir(), mode})
			if code != 1 || stdout != "" || !strings.Contains(stderr, "candidate payload") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func runReleaseDecision(args []string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), append([]string{"release-decision"}, args...), &out, &errOut, Commands())
	return code, out.String(), errOut.String()
}

func releasePayload(t *testing.T, tag, geosite, geoip string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"geosite.dat":                geosite,
		"geoip.dat":                  geoip,
		"install_passwall2_rules.sh": fmt.Sprintf("#!/bin/sh\nRELEASE_TAG='%s'\nlogic=v1\n", tag),
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := verify.WriteSHA256(path); err != nil {
			t.Fatal(err)
		}
	}
	return dir
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
	options, err := parseBuildOptions([]string{"--work-root", root, "--repo", "flame/repo", "--release-tag", "v1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{options.Sources, options.Custom, options.Groups, options.Community, options.CacheRoot} {
		if len(path) <= len(root) || path[:len(root)] != root {
			t.Fatalf("path %q not under %q", path, root)
		}
	}
}

func TestBuildRejectsUndocumentedRootFlag(t *testing.T) {
	_, err := parseBuildOptions([]string{"--root", t.TempDir(), "--repo", "flame/repo", "--release-tag", "v1"})
	if err == nil {
		t.Fatal("legacy --root flag unexpectedly accepted")
	}
}
