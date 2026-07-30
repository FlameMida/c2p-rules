package manifest_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"clash-rules-srs/internal/manifest"
	"clash-rules-srs/internal/model"
)

func TestManifestUsesOutputTagsAndForbidsLegacyTags(t *testing.T) {
	google := model.Output{Tag: "google", Mode: model.MergeBase}
	netflixSite := model.Output{Tag: "netflix", Mode: model.MergeBase}
	netflixIP := model.Output{Tag: "netflix", Mode: model.MergeBase}
	sources := []model.Source{
		{ID: "loyalsoldier-google", Outputs: model.Outputs{GeoSite: &google}},
		{ID: "xiaolin-netflix", Outputs: model.Outputs{GeoSite: &netflixSite, GeoIP: &netflixIP}},
	}
	doc := manifest.Build(sources, nil)
	if !slices.Contains(doc.Required.GeoSite, "google") || slices.Contains(doc.Required.GeoSite, "loyalsoldier-google") {
		t.Fatalf("required=%v", doc.Required.GeoSite)
	}
	if !slices.Contains(doc.Forbidden.GeoSite, "loyalsoldier-google") || !slices.Contains(doc.Forbidden.GeoSite, "xiaolin-netflix") || !slices.Contains(doc.Forbidden.GeoIP, "xiaolin-netflix") {
		t.Fatalf("forbidden=%#v", doc.Forbidden)
	}
	if !slices.Contains(doc.Forbidden.GeoSite, "applications") || slices.Contains(doc.Forbidden.GeoIP, "loyalsoldier-google") {
		t.Fatalf("forbidden=%#v", doc.Forbidden)
	}
	record := doc.Sources["xiaolin-netflix"]
	if record.GeoSite == nil || record.GeoIP == nil || record.GeoSite.Tag != "netflix" || record.GeoIP.Mode != model.MergeBase {
		t.Fatalf("record=%#v", record)
	}
}

func TestManifestAddsGroupRefsAndBaselineRequirements(t *testing.T) {
	doc := manifest.Build(nil, []model.Group{{
		ID: "apple", GeoSite: []string{"icloud", "apple"}, GeoIP: []string{"private"},
	}})
	if !reflect.DeepEqual(doc.Required.GeoSite, []string{"apple", "cn", "icloud"}) {
		t.Fatalf("geosite=%v", doc.Required.GeoSite)
	}
	if !reflect.DeepEqual(doc.Required.GeoIP, []string{"cn", "private"}) {
		t.Fatalf("geoip=%v", doc.Required.GeoIP)
	}
}

func TestManifestWriteIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	doc := manifest.Document{
		SchemaVersion: 1,
		Sources: map[string]manifest.SourceRecord{
			"z": {},
			"a": {},
		},
	}
	one := filepath.Join(dir, "one.json")
	two := filepath.Join(dir, "two.json")
	if err := manifest.Write(one, doc); err != nil {
		t.Fatal(err)
	}
	if err := manifest.Write(two, doc); err != nil {
		t.Fatal(err)
	}
	oneBytes, _ := os.ReadFile(one)
	twoBytes, _ := os.ReadFile(two)
	if !bytes.Equal(oneBytes, twoBytes) || len(oneBytes) == 0 || oneBytes[len(oneBytes)-1] != '\n' {
		t.Fatalf("one=%q two=%q", oneBytes, twoBytes)
	}
}
