package passwall_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"clash-rules-srs/internal/config"
	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/passwall"
)

func TestRenderAppleAndChinaWithStableOrder(t *testing.T) {
	groups := []model.Group{
		{ID: "apple_services", Remarks: "苹果服务", GeoSite: []string{"apple", "icloud"}},
		{ID: "china", Remarks: "中国大陆", GeoSite: []string{"cn"}, GeoIP: []string{"cn"}},
	}
	got, err := passwall.Render(groups)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/rules.golden")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

type fakeLookup struct {
	missing string
}

func (f fakeLookup) Has(_ context.Context, side model.Side, tag string) (bool, error) {
	return string(side)+":"+tag != f.missing, nil
}

func TestMissingGroupTagNamesGroupAndReference(t *testing.T) {
	lookup := fakeLookup{missing: "geoip:not-exist"}
	err := passwall.ValidateGroups(context.Background(), []model.Group{{ID: "broken", Remarks: "坏组", GeoIP: []string{"not-exist"}}}, lookup)
	if err == nil || !strings.Contains(err.Error(), "坏组") || !strings.Contains(err.Error(), "geoip:not-exist") {
		t.Fatalf("err=%v", err)
	}
}

func TestYouTubePrecedesGoogle(t *testing.T) {
	groups, err := config.LoadGroups("testdata/groups.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if index(groups, "youtube") >= index(groups, "google") {
		t.Fatalf("order=%v", groups)
	}
}

func TestRenderEscapesQuotesAndRejectsControlCharacters(t *testing.T) {
	got, err := passwall.Render([]model.Group{{ID: "quoted", Remarks: "Bob's 服务", GeoSite: []string{"google"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(`option remarks 'Bob'\''s 服务'`)) {
		t.Fatalf("got=%s", got)
	}
	for name, group := range map[string]model.Group{
		"remarks": {ID: "x", Remarks: "bad\nremark", GeoSite: []string{"google"}},
		"tag":     {ID: "x", Remarks: "X", GeoSite: []string{"bad\rtag"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := passwall.Render([]model.Group{group}); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestRenderRejectsDuplicateOrOversizedSectionIDs(t *testing.T) {
	for name, groups := range map[string][]model.Group{
		"duplicate": {
			{ID: "same", Remarks: "A", GeoSite: []string{"a"}},
			{ID: "same", Remarks: "B", GeoSite: []string{"b"}},
		},
		"oversized": {{ID: strings.Repeat("a", 61), Remarks: "A", GeoSite: []string{"a"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := passwall.Render(groups); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func index(groups []model.Group, id string) int {
	for position, group := range groups {
		if group.ID == id {
			return position
		}
	}
	panic(fmt.Sprintf("missing group %s", id))
}
