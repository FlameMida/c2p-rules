package geodat

import (
	"bytes"
	"strings"
	"testing"
)

func TestGeoSite规范化消除Tag规则与属性顺序差异(t *testing.T) {
	left := siteDat(
		siteEntry("Z-TAG",
			domainEntry(2, "z.example"),
			domainEntry(3, "a.example", attribute("z", 2, 1), attribute("a", 2, 1)),
		),
		siteEntry("A-TAG", domainEntry(0, "needle")),
	)
	right := siteDat(
		siteEntry("A-TAG", domainEntry(0, "needle")),
		siteEntry("Z-TAG",
			domainEntry(3, "a.example", attribute("a", 2, 1), attribute("z", 2, 1)),
			domainEntry(2, "z.example"),
		),
	)

	canonicalLeft, err := CanonicalizeGeoSite(left)
	if err != nil {
		t.Fatal(err)
	}
	canonicalRight, err := CanonicalizeGeoSite(right)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonicalLeft, canonicalRight) {
		t.Fatalf("canonical bytes differ\nleft=%x\nright=%x", canonicalLeft, canonicalRight)
	}
	again, err := CanonicalizeGeoSite(canonicalLeft)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonicalLeft, again) {
		t.Fatal("canonicalization is not idempotent")
	}
}

func TestGeoSite规范化拒绝未知字段(t *testing.T) {
	domainWithUnknown := append(domainEntry(2, "kept.example"), fieldVarint(99, 7)...)
	tagWithUnknown := append(siteEntry("TEST", domainEntry(2, "kept.example")), fieldVarint(99, 7)...)
	documentWithUnknown := append(siteDat(siteEntry("TEST", domainEntry(2, "kept.example"))), fieldVarint(99, 7)...)
	tests := map[string][]byte{
		"document": documentWithUnknown,
		"tag":      siteDat(tagWithUnknown),
		"domain":   siteDat(siteEntry("TEST", domainWithUnknown)),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := CanonicalizeGeoSite(data)
			if err == nil || !strings.Contains(err.Error(), "unknown") || !strings.Contains(err.Error(), "99") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
