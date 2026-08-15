package shadowrocket

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"unicode"

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

type siteRecord struct {
	tag     string
	domains []DomainLine
}

type ipRecord struct {
	tag   string
	cidrs []netip.Prefix
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
	sites, err := decodeGeoSiteList(siteBytes)
	if err != nil {
		return nil, fmt.Errorf("decode geosite dat %s: %w", siteDat, err)
	}
	ips, err := decodeGeoIPList(ipBytes)
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
			for _, line := range record.domains {
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
			for _, prefix := range record.cidrs {
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

func decodeGeoSiteList(data []byte) (map[string]siteRecord, error) {
	result := make(map[string]siteRecord)
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, _, err := cursor.next()
		if err != nil {
			return nil, err
		}
		if field != 1 {
			continue
		}
		if wire != 2 {
			return nil, fmt.Errorf("geosite entry has wire type %d", wire)
		}
		record, err := decodeGeoSite(payload)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(record.tag)
		if _, duplicate := result[key]; duplicate {
			return nil, fmt.Errorf("duplicate geosite tag %q", record.tag)
		}
		result[key] = record
	}
	return result, nil
}

func decodeGeoSite(data []byte) (siteRecord, error) {
	var record siteRecord
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, _, err := cursor.next()
		if err != nil {
			return siteRecord{}, err
		}
		switch field {
		case 1:
			if wire != 2 {
				return siteRecord{}, fmt.Errorf("geosite tag has wire type %d", wire)
			}
			record.tag = string(payload)
		case 2:
			if wire != 2 {
				return siteRecord{}, fmt.Errorf("geosite domain has wire type %d", wire)
			}
			line, regexp, err := decodeDomain(payload)
			if err != nil {
				return siteRecord{}, fmt.Errorf("tag %q: %w", record.tag, err)
			}
			if !regexp {
				record.domains = append(record.domains, line)
			}
		}
	}
	if record.tag == "" {
		return siteRecord{}, fmt.Errorf("geosite entry has empty tag")
	}
	return record, nil
}

func decodeDomain(data []byte) (DomainLine, bool, error) {
	var kind uint64
	var value string
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, scalar, err := cursor.next()
		if err != nil {
			return DomainLine{}, false, err
		}
		switch field {
		case 1:
			if wire != 0 {
				return DomainLine{}, false, fmt.Errorf("domain kind has wire type %d", wire)
			}
			kind = scalar
		case 2:
			if wire != 2 {
				return DomainLine{}, false, fmt.Errorf("domain value has wire type %d", wire)
			}
			value = string(payload)
		}
	}
	if kind == 1 {
		return DomainLine{}, true, nil
	}
	if !validRuleValue(value) {
		return DomainLine{}, false, fmt.Errorf("invalid domain value %q", value)
	}
	switch kind {
	case 0:
		return DomainLine{Kind: model.DomainKeyword, Value: value}, false, nil
	case 2:
		return DomainLine{Kind: model.DomainSuffix, Value: value}, false, nil
	case 3:
		return DomainLine{Kind: model.DomainFull, Value: value}, false, nil
	default:
		return DomainLine{}, false, fmt.Errorf("unsupported domain kind %d", kind)
	}
}

func decodeGeoIPList(data []byte) (map[string]ipRecord, error) {
	result := make(map[string]ipRecord)
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, _, err := cursor.next()
		if err != nil {
			return nil, err
		}
		if field != 1 {
			continue
		}
		if wire != 2 {
			return nil, fmt.Errorf("geoip entry has wire type %d", wire)
		}
		record, err := decodeGeoIP(payload)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(record.tag)
		if _, duplicate := result[key]; duplicate {
			return nil, fmt.Errorf("duplicate geoip tag %q", record.tag)
		}
		result[key] = record
	}
	return result, nil
}

func decodeGeoIP(data []byte) (ipRecord, error) {
	var record ipRecord
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, _, err := cursor.next()
		if err != nil {
			return ipRecord{}, err
		}
		switch field {
		case 1:
			if wire != 2 {
				return ipRecord{}, fmt.Errorf("geoip tag has wire type %d", wire)
			}
			record.tag = string(payload)
		case 2:
			if wire != 2 {
				return ipRecord{}, fmt.Errorf("geoip cidr has wire type %d", wire)
			}
			prefix, err := decodeCIDR(payload)
			if err != nil {
				return ipRecord{}, fmt.Errorf("tag %q: %w", record.tag, err)
			}
			record.cidrs = append(record.cidrs, prefix)
		}
	}
	if record.tag == "" {
		return ipRecord{}, fmt.Errorf("geoip entry has empty tag")
	}
	return record, nil
}

func decodeCIDR(data []byte) (netip.Prefix, error) {
	var addressBytes []byte
	var bits uint64
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, scalar, err := cursor.next()
		if err != nil {
			return netip.Prefix{}, err
		}
		switch field {
		case 1:
			if wire != 2 {
				return netip.Prefix{}, fmt.Errorf("cidr address has wire type %d", wire)
			}
			addressBytes = append([]byte(nil), payload...)
		case 2:
			if wire != 0 {
				return netip.Prefix{}, fmt.Errorf("cidr prefix has wire type %d", wire)
			}
			bits = scalar
		}
	}
	address, ok := netip.AddrFromSlice(addressBytes)
	if !ok || address.Is4In6() {
		return netip.Prefix{}, fmt.Errorf("invalid cidr address %x", addressBytes)
	}
	if bits > uint64(address.BitLen()) {
		return netip.Prefix{}, fmt.Errorf("invalid /%d for %s", bits, address)
	}
	return netip.PrefixFrom(address, int(bits)).Masked(), nil
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

type wireCursor struct {
	data []byte
}

func (c *wireCursor) more() bool {
	return len(c.data) != 0
}

func (c *wireCursor) next() (field, wire uint64, payload []byte, scalar uint64, err error) {
	key, n := binary.Uvarint(c.data)
	if n <= 0 {
		return 0, 0, nil, 0, fmt.Errorf("invalid protobuf field key")
	}
	c.data = c.data[n:]
	field, wire = key>>3, key&7
	if field == 0 {
		return 0, 0, nil, 0, fmt.Errorf("protobuf field number is zero")
	}
	switch wire {
	case 0:
		scalar, n = binary.Uvarint(c.data)
		if n <= 0 {
			return 0, 0, nil, 0, fmt.Errorf("invalid protobuf varint")
		}
		c.data = c.data[n:]
	case 1:
		if len(c.data) < 8 {
			return 0, 0, nil, 0, fmt.Errorf("truncated protobuf fixed64")
		}
		payload, c.data = c.data[:8], c.data[8:]
	case 2:
		length, lengthBytes := binary.Uvarint(c.data)
		if lengthBytes <= 0 {
			return 0, 0, nil, 0, fmt.Errorf("invalid protobuf byte length")
		}
		c.data = c.data[lengthBytes:]
		if length > uint64(len(c.data)) {
			return 0, 0, nil, 0, fmt.Errorf("truncated protobuf bytes")
		}
		payload, c.data = c.data[:int(length)], c.data[int(length):]
	case 5:
		if len(c.data) < 4 {
			return 0, 0, nil, 0, fmt.Errorf("truncated protobuf fixed32")
		}
		payload, c.data = c.data[:4], c.data[4:]
	default:
		return 0, 0, nil, 0, fmt.Errorf("unsupported protobuf wire type %d", wire)
	}
	return field, wire, payload, scalar, nil
}
