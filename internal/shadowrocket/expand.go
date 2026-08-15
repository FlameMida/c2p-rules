package shadowrocket

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"unicode"

	"clash-rules-srs/internal/geodat"
	"clash-rules-srs/internal/model"
)

type DomainLine struct {
	Kind  model.DomainKind
	Value string
}

type GroupRules struct {
	Group   model.Group
	Domains []DomainLine
	CIDRs   []netip.Prefix
}

func Expand(ctx context.Context, siteDat, ipDat string, groups []model.Group) ([]GroupRules, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	siteBytes, err := os.ReadFile(siteDat)
	if err != nil {
		return nil, fmt.Errorf("read geosite dat %s: %w", siteDat, err)
	}
	ipBytes, err := os.ReadFile(ipDat)
	if err != nil {
		return nil, fmt.Errorf("read geoip dat %s: %w", ipDat, err)
	}
	sites, err := geodat.DecodeGeoSite(siteBytes)
	if err != nil {
		return nil, fmt.Errorf("decode geosite dat %s: %w", siteDat, err)
	}
	ips, err := geodat.DecodeGeoIP(ipBytes)
	if err != nil {
		return nil, fmt.Errorf("decode geoip dat %s: %w", ipDat, err)
	}

	result := make([]GroupRules, 0, len(groups))
	for _, group := range groups {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		expanded := GroupRules{Group: group}
		domainSeen := make(map[string]struct{})
		for _, tag := range group.GeoSite {
			record, ok := sites[strings.ToLower(tag)]
			if !ok {
				return nil, fmt.Errorf("group %q references missing geosite:%s", group.Remarks, tag)
			}
			for _, rule := range record.Domains {
				if rule.Kind == model.DomainRegexp {
					continue
				}
				line := DomainLine{Kind: rule.Kind, Value: rule.Value}
				key := string(line.Kind) + "\x00" + line.Value
				if _, duplicate := domainSeen[key]; duplicate {
					continue
				}
				domainSeen[key] = struct{}{}
				expanded.Domains = append(expanded.Domains, line)
			}
		}
		cidrSeen := make(map[netip.Prefix]struct{})
		for _, tag := range group.GeoIP {
			record, ok := ips[strings.ToLower(tag)]
			if !ok {
				return nil, fmt.Errorf("group %q references missing geoip:%s", group.Remarks, tag)
			}
			for _, prefix := range record.Prefixes {
				if _, duplicate := cidrSeen[prefix]; duplicate {
					continue
				}
				cidrSeen[prefix] = struct{}{}
				expanded.CIDRs = append(expanded.CIDRs, prefix)
			}
		}
		if len(expanded.Domains) == 0 && len(expanded.CIDRs) == 0 {
			return nil, fmt.Errorf("group %q (%s) expands to zero rules", group.Remarks, group.ID)
		}
		result = append(result, expanded)
	}
	return result, nil
}

func validRuleValue(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsRune(value, ',') {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
