package releasecmp_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"clash-rules-srs/internal/passwall"
	"clash-rules-srs/internal/releasecmp"
	"clash-rules-srs/internal/verify"
)

func Test相同内容重复运行(t *testing.T) {
	candidate := payload(t, "candidate-tag", "same-site", "same-ip", "logic=v1")
	baseline := payload(t, "baseline-tag", "same-site", "same-ip", "logic=v1")

	decision, err := releasecmp.Decide(compareInput(candidate, "candidate-tag", baseline, "baseline-tag"))
	if err != nil {
		t.Fatal(err)
	}
	if decision.ShouldPublish || decision.Reason != "unchanged" {
		t.Fatalf("decision=%+v", decision)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(decision.BaselineFingerprint) {
		t.Fatalf("fingerprint=%q", decision.BaselineFingerprint)
	}
}

func Test任一Dat发生变化(t *testing.T) {
	for _, name := range []string{"geosite.dat", "geoip.dat"} {
		t.Run(name, func(t *testing.T) {
			candidate := payload(t, "candidate", "same-site", "same-ip", "logic=v1")
			baseline := payload(t, "baseline", "same-site", "same-ip", "logic=v1")
			rewriteMain(t, candidate, name, "changed")

			decision, err := releasecmp.Decide(compareInput(candidate, "candidate", baseline, "baseline"))
			if err != nil {
				t.Fatal(err)
			}
			if !decision.ShouldPublish || decision.Reason != "changed" {
				t.Fatalf("decision=%+v", decision)
			}
		})
	}
}

func Test安装器逻辑发生变化(t *testing.T) {
	candidate := payload(t, "candidate", "same-site", "same-ip", "logic=v2")
	baseline := payload(t, "baseline", "same-site", "same-ip", "logic=v1")
	decision, err := releasecmp.Decide(compareInput(candidate, "candidate", baseline, "baseline"))
	if err != nil {
		t.Fatal(err)
	}
	if !decision.ShouldPublish || decision.Reason != "changed" {
		t.Fatalf("decision=%+v", decision)
	}
}

func Test安装器仅ReleaseTag不同(t *testing.T) {
	left, err := releasecmp.NormalizeInstaller(installer("left", "logic=v1"))
	if err != nil {
		t.Fatal(err)
	}
	right, err := releasecmp.NormalizeInstaller(installer("right", "logic=v1"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left, right) {
		t.Fatalf("normalized installers differ\nleft=%s\nright=%s", left, right)
	}
}

func Test安装器发布绑定字段异常(t *testing.T) {
	cases := map[string]string{
		"missing":               "#!/bin/sh\nlogic=v1\n",
		"duplicate":             "RELEASE_TAG='one'\nRELEASE_TAG='two'\n",
		"invalid syntax":        "RELEASE_TAG = 'one'\n",
		"invalid empty":         "RELEASE_TAG=''\n",
		"invalid value":         "RELEASE_TAG='bad tag'\n",
		"valid plus malformed":  "RELEASE_TAG='one'\nRELEASE_TAG=two\n",
		"over maximum tag size": "RELEASE_TAG='" + strings.Repeat("a", 129) + "'\n",
	}
	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := releasecmp.NormalizeInstaller([]byte(script)); err == nil || !strings.Contains(err.Error(), "RELEASE_TAG") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func Test内容相同但人工强制发布(t *testing.T) {
	candidate := payload(t, "candidate", "site", "ip", "logic=v1")
	decision, err := releasecmp.Decide(modeInput(candidate, "candidate", releasecmp.Force))
	if err != nil {
		t.Fatal(err)
	}
	if !decision.ShouldPublish || decision.Reason != "forced" || decision.BaselineFingerprint != "" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestLatest六资产损坏(t *testing.T) {
	mutations := map[string]func(*testing.T, string){
		"missing": func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "geoip.dat")); err != nil {
				t.Fatal(err)
			}
		},
		"extra": func(t *testing.T, dir string) {
			mustWrite(t, filepath.Join(dir, "extra.asset"), "extra")
		},
		"bad checksum": func(t *testing.T, dir string) {
			mustWrite(t, filepath.Join(dir, "geosite.dat.sha256sum"), strings.Repeat("0", 64)+"  geosite.dat\n")
		},
		"bad installer": func(t *testing.T, dir string) {
			rewriteMain(t, dir, "install_passwall2_rules.sh", "#!/bin/sh\nlogic=v1\n")
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := payload(t, "candidate", "site", "ip", "logic=v1")
			baseline := payload(t, "baseline", "site", "ip", "logic=v1")
			mutate(t, baseline)
			_, err := releasecmp.Decide(compareInput(candidate, "candidate", baseline, "baseline"))
			if err == nil || !strings.Contains(err.Error(), "baseline payload") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func Test所有模式下候选损坏(t *testing.T) {
	for _, mode := range []releasecmp.Mode{releasecmp.Compare, releasecmp.FirstRelease, releasecmp.Force} {
		for _, mutation := range []string{"missing", "extra", "bad checksum", "bad installer"} {
			t.Run(fmt.Sprintf("mode-%d/%s", mode, mutation), func(t *testing.T) {
				candidate := payload(t, "candidate", "site", "ip", "logic=v1")
				switch mutation {
				case "missing":
					if err := os.Remove(filepath.Join(candidate, "geoip.dat")); err != nil {
						t.Fatal(err)
					}
				case "extra":
					mustWrite(t, filepath.Join(candidate, "extra.asset"), "extra")
				case "bad checksum":
					mustWrite(t, filepath.Join(candidate, "geoip.dat.sha256sum"), strings.Repeat("0", 64)+"  geoip.dat\n")
				case "bad installer":
					rewriteMain(t, candidate, "install_passwall2_rules.sh", "#!/bin/sh\nlogic=v1\n")
				}
				input := modeInput(candidate, "candidate", mode)
				if mode == releasecmp.Compare {
					input.BaselineDir = payload(t, "baseline", "site", "ip", "logic=v1")
					input.BaselineTag = "baseline"
				}
				_, err := releasecmp.Decide(input)
				if err == nil || !strings.Contains(err.Error(), "candidate payload") {
					t.Fatalf("err=%v", err)
				}
			})
		}
	}
}

func Test尚无LatestRelease(t *testing.T) {
	candidate := payload(t, "candidate", "site", "ip", "logic=v1")
	decision, err := releasecmp.Decide(modeInput(candidate, "candidate", releasecmp.FirstRelease))
	if err != nil {
		t.Fatal(err)
	}
	if !decision.ShouldPublish || decision.Reason != "first-release" || decision.BaselineFingerprint != "" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestFingerprintFramesNamesAndLengths(t *testing.T) {
	candidate := payload(t, "candidate", "a", "bc", "logic=v1")
	baseline := payload(t, "baseline", "ab", "c", "logic=v1")
	decision, err := releasecmp.Decide(compareInput(candidate, "candidate", baseline, "baseline"))
	if err != nil {
		t.Fatal(err)
	}
	if !decision.ShouldPublish || decision.Reason != "changed" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestRenderedInstallerMatchesComparisonProtocol(t *testing.T) {
	candidate := renderedPayload(t, "candidate-tag", "same-site", "same-ip")
	baseline := renderedPayload(t, "baseline-tag", "same-site", "same-ip")
	decision, err := releasecmp.Decide(compareInput(candidate, "candidate-tag", baseline, "baseline-tag"))
	if err != nil {
		t.Fatal(err)
	}
	if decision.ShouldPublish || decision.Reason != "unchanged" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestReleaseTagMustMatchPayloadContext(t *testing.T) {
	candidate := payload(t, "actual-candidate", "site", "ip", "logic=v1")
	baseline := payload(t, "actual-baseline", "site", "ip", "logic=v1")
	for name, test := range map[string]struct {
		input   releasecmp.Input
		payload string
	}{
		"compare candidate": {compareInput(candidate, "wrong-candidate", baseline, "actual-baseline"), "candidate"},
		"first candidate":   {releasecmp.Input{CandidateDir: candidate, CandidateTag: "wrong-candidate", Mode: releasecmp.FirstRelease}, "candidate"},
		"force candidate":   {releasecmp.Input{CandidateDir: candidate, CandidateTag: "wrong-candidate", Mode: releasecmp.Force}, "candidate"},
		"compare baseline":  {compareInput(candidate, "actual-candidate", baseline, "wrong-baseline"), "baseline"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := releasecmp.Decide(test.input)
			if err == nil || !strings.Contains(err.Error(), test.payload+" payload") || !strings.Contains(err.Error(), "release tag mismatch") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func payload(t *testing.T, tag, geosite, geoip, logic string) string {
	t.Helper()
	dir := t.TempDir()
	rewriteMain(t, dir, "geosite.dat", geosite)
	rewriteMain(t, dir, "geoip.dat", geoip)
	rewriteMain(t, dir, "install_passwall2_rules.sh", string(installer(tag, logic)))
	return dir
}

func renderedPayload(t *testing.T, tag, geosite, geoip string) string {
	t.Helper()
	script, err := passwall.RenderInstaller(passwall.InstallOptions{
		Repo:       "flame/clash-rules-srs",
		ReleaseTag: tag,
		GeoSiteSHA: strings.Repeat("a", 64),
		GeoIPSHA:   strings.Repeat("b", 64),
		Fragment:   []byte("config shunt_rules 'crs_test'\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	rewriteMain(t, dir, "geosite.dat", geosite)
	rewriteMain(t, dir, "geoip.dat", geoip)
	rewriteMain(t, dir, "install_passwall2_rules.sh", string(script))
	return dir
}

func compareInput(candidate, candidateTag, baseline, baselineTag string) releasecmp.Input {
	return releasecmp.Input{
		CandidateDir: candidate,
		CandidateTag: candidateTag,
		BaselineDir:  baseline,
		BaselineTag:  baselineTag,
		Mode:         releasecmp.Compare,
	}
}

func modeInput(candidate, candidateTag string, mode releasecmp.Mode) releasecmp.Input {
	return releasecmp.Input{CandidateDir: candidate, CandidateTag: candidateTag, Mode: mode}
}

func installer(tag, logic string) []byte {
	return []byte(fmt.Sprintf("#!/bin/sh\nRELEASE_TAG='%s'\n%s\n", tag, logic))
}

func rewriteMain(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	mustWrite(t, path, content)
	if _, err := verify.WriteSHA256(path); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
