package targets_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/targets"
)

func TestCreateAndMergeBasePreconditions(t *testing.T) {
	base := func(side model.Side, tag string) (bool, error) {
		return side == model.GeoSite && tag == "google", nil
	}
	for name, output := range map[string]model.Output{
		"create-collision": {Tag: "google", Mode: model.Create},
		"merge-missing":    {Tag: "missing", Mode: model.MergeBase},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := targets.New([]model.Source{{ID: name, Outputs: model.Outputs{GeoSite: &output}}}, base)
			if err == nil || !strings.Contains(err.Error(), name) || !strings.Contains(err.Error(), string(output.Mode)) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestRegistryAllowsSharedTargetOnlyWithSameMode(t *testing.T) {
	base := func(model.Side, string) (bool, error) { return false, nil }
	createA := model.Output{Tag: "ai", Mode: model.Create}
	createB := model.Output{Tag: "ai", Mode: model.Create}
	registry, err := targets.New([]model.Source{
		{ID: "a", Outputs: model.Outputs{GeoSite: &createA}},
		{ID: "b", Outputs: model.Outputs{GeoSite: &createB}},
	}, base)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Require(model.GeoSite, "ai"); err != nil {
		t.Fatal(err)
	}

	merge := model.Output{Tag: "ai", Mode: model.MergeBase}
	_, err = targets.New([]model.Source{
		{ID: "a", Outputs: model.Outputs{GeoSite: &createA}},
		{ID: "b", Outputs: model.Outputs{GeoSite: &merge}},
	}, func(model.Side, string) (bool, error) { return true, nil })
	if err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("err=%v", err)
	}
}

func TestRegistryRequireAcceptsUnreferencedBaseAndRejectsUnknown(t *testing.T) {
	registry, err := targets.New(nil, func(side model.Side, tag string) (bool, error) {
		return side == model.GeoSite && tag == "apple", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Require(model.GeoSite, "apple"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Require(model.GeoIP, "apple"); err == nil || !strings.Contains(err.Error(), "geoip:apple") {
		t.Fatalf("err=%v", err)
	}
}

func TestRegistryPropagatesBaseLookupErrors(t *testing.T) {
	want := errors.New("probe failed")
	output := model.Output{Tag: "google", Mode: model.MergeBase}
	_, err := targets.New([]model.Source{{ID: "source", Outputs: model.Outputs{GeoSite: &output}}}, func(model.Side, string) (bool, error) {
		return false, want
	})
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "geosite:google") {
		t.Fatalf("err=%v", err)
	}
}

func TestRegistryLooksUpTargetsInStableOrder(t *testing.T) {
	z := model.Output{Tag: "z", Mode: model.Create}
	a := model.Output{Tag: "a", Mode: model.Create}
	b := model.Output{Tag: "b", Mode: model.Create}
	sources := []model.Source{
		{ID: "z", Outputs: model.Outputs{GeoSite: &z}},
		{ID: "a", Outputs: model.Outputs{GeoSite: &a}},
		{ID: "b", Outputs: model.Outputs{GeoIP: &b}},
	}
	want := []string{"geosite:a", "geosite:z", "geoip:b"}
	for attempt := 0; attempt < 32; attempt++ {
		var calls []string
		_, err := targets.New(sources, func(side model.Side, tag string) (bool, error) {
			calls = append(calls, string(side)+":"+tag)
			return false, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(calls, want) {
			t.Fatalf("calls=%v want=%v", calls, want)
		}
	}
}
