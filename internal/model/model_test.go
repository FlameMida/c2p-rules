package model_test

import (
	"testing"

	"clash-rules-srs/internal/model"
)

func TestDomainKindValidityIsCentralized(t *testing.T) {
	for _, kind := range []model.DomainKind{
		model.DomainSuffix,
		model.DomainFull,
		model.DomainKeyword,
		model.DomainRegexp,
	} {
		if !kind.Valid() {
			t.Fatalf("kind %q is not valid", kind)
		}
	}
	if model.DomainKind("suffix").Valid() {
		t.Fatal("unknown domain kind is valid")
	}
}
