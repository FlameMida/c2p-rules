package model

import "net/netip"

type Side string

const (
	GeoSite Side = "geosite"
	GeoIP   Side = "geoip"
)

type Mode string

const (
	Create    Mode = "create"
	MergeBase Mode = "merge-base"
)

type Behavior string

const (
	Domain    Behavior = "domain"
	IPCIDR    Behavior = "ipcidr"
	Classical Behavior = "classical"
)

type Format string

const (
	YAML Format = "yaml"
	Text Format = "text"
)

type DomainKind string

const (
	DomainSuffix  DomainKind = "domain"
	DomainFull    DomainKind = "full"
	DomainKeyword DomainKind = "keyword"
	DomainRegexp  DomainKind = "regexp"
)

func (kind DomainKind) Valid() bool {
	switch kind {
	case DomainSuffix, DomainFull, DomainKeyword, DomainRegexp:
		return true
	default:
		return false
	}
}

type Output struct {
	Tag  string
	Mode Mode
}

type Outputs struct {
	GeoSite *Output
	GeoIP   *Output
}

type Source struct {
	ID       string
	Behavior Behavior
	Format   Format
	URL      string
	Outputs  Outputs
}

type Group struct {
	ID      string
	Remarks string
	GeoSite []string
	GeoIP   []string
}

type DomainRule struct {
	Kind  DomainKind
	Value string
	Attrs []string
}

type Buckets struct {
	Domains []DomainRule
	CIDRs   []netip.Prefix
	Skipped []string
}

type Contribution struct {
	SourceID string
	Side     Side
	Tag      string
	Domains  []DomainRule
	CIDRs    []netip.Prefix
}
