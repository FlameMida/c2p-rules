package yamlutil_test

import (
	"strings"
	"testing"

	"clash-rules-srs/internal/yamlutil"
)

func TestDecodeStrictAllowsOnlyExplicitNullPolicy(t *testing.T) {
	type document struct {
		Payload []string `yaml:"payload"`
	}
	var strict document
	if err := yamlutil.DecodeStrict(strings.NewReader("payload:\n"), &strict, yamlutil.StrictOptions{}); err == nil {
		t.Fatal("strict decoder accepted null without policy")
	}
	var custom document
	if err := yamlutil.DecodeStrict(strings.NewReader("payload:\n"), &custom, yamlutil.StrictOptions{AllowedNullPaths: []string{"payload"}}); err != nil {
		t.Fatal(err)
	}
	var invalid document
	if err := yamlutil.DecodeStrict(strings.NewReader("payload: [null]\n"), &invalid, yamlutil.StrictOptions{AllowedNullPaths: []string{"payload"}}); err == nil {
		t.Fatal("path policy leaked into payload elements")
	}
}
