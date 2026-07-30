package verify_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clash-rules-srs/internal/manifest"
	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/tools"
	"clash-rules-srs/internal/verify"
)

func TestProberRequiresNonemptyOutputAndPreservesMixedCase(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
set -eu
list=''
output=''
while [ "$#" -gt 0 ]; do
  case "$1" in
    -list) list="$2"; shift 2 ;;
    -output) output="$2"; shift 2 ;;
    *) shift ;;
  esac
done
[ "$list" = 'BilibiliHMT' ] && printf 'present' > "$output"
exit 0
`
	writeExecutable(t, filepath.Join(bin, "geoview"), script)
	site := filepath.Join(root, "geosite.dat")
	ip := filepath.Join(root, "geoip.dat")
	mustWrite(t, site, "site")
	mustWrite(t, ip, "ip")
	runner := &tools.Runner{BinRoot: bin, Timeout: time.Second, MaxLogBytes: 1024}
	prober := verify.NewProber(runner, site, ip)
	present, err := prober.Has(context.Background(), model.GeoSite, "BilibiliHMT")
	if err != nil || !present {
		t.Fatalf("present=%v err=%v", present, err)
	}
	present, err = prober.Has(context.Background(), model.GeoSite, "bilibilihmt")
	if err != nil || present {
		t.Fatalf("present=%v err=%v", present, err)
	}
}

func TestProberPropagatesToolFailureAndRejectsInvalidSide(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(bin, "geoview"), "#!/bin/sh\nexit 7\n")
	site := filepath.Join(root, "site.dat")
	mustWrite(t, site, "site")
	prober := verify.NewProber(&tools.Runner{BinRoot: bin, Timeout: time.Second}, site, site)
	if _, err := prober.Has(context.Background(), model.GeoSite, "google"); err == nil {
		t.Fatal("expected tool error")
	}
	if _, err := prober.Has(context.Background(), model.Side("bad"), "google"); err == nil {
		t.Fatal("expected side error")
	}
}

type fakeLookup struct {
	present map[string]bool
	fail    string
}

func (f fakeLookup) Has(_ context.Context, side model.Side, tag string) (bool, error) {
	key := string(side) + ":" + tag
	if key == f.fail {
		return false, fmt.Errorf("probe failed")
	}
	return f.present[key], nil
}

func TestRequiredForbiddenAndGroupRefs(t *testing.T) {
	doc := manifest.Document{
		Required:  manifest.Tags{GeoSite: []string{"google"}, GeoIP: []string{"cn"}},
		Forbidden: manifest.Tags{GeoSite: []string{"old"}},
	}
	lookup := fakeLookup{present: map[string]bool{"geosite:google": true, "geoip:cn": true}}
	if err := verify.Required(context.Background(), lookup, doc); err != nil {
		t.Fatal(err)
	}
	if err := verify.Forbidden(context.Background(), lookup, doc); err != nil {
		t.Fatal(err)
	}
	if err := verify.GroupRefs(context.Background(), lookup, []model.Group{{Remarks: "中国大陆", GeoSite: []string{"google"}, GeoIP: []string{"missing"}}}); err == nil || !strings.Contains(err.Error(), "中国大陆") || !strings.Contains(err.Error(), "geoip:missing") {
		t.Fatalf("err=%v", err)
	}
	lookup.present["geosite:old"] = true
	if err := verify.Forbidden(context.Background(), lookup, doc); err == nil || !strings.Contains(err.Error(), "geosite:old") {
		t.Fatalf("err=%v", err)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
