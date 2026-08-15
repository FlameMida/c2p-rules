package geodat

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"unicode"

	"clash-rules-srs/internal/model"
)

// SiteTag is one fully expanded geosite entry from a compiled DAT file.
type SiteTag struct {
	Name    string
	Domains []model.DomainRule
}

// IPTag is one geoip entry from a compiled DAT file.
type IPTag struct {
	Name     string
	Prefixes []netip.Prefix
}

// DecodeGeoSite decodes a compiled geosite DAT without discarding domain
// kinds or true boolean attributes. The returned map is keyed by lowercase
// tag name while SiteTag.Name preserves the spelling from the DAT.
func DecodeGeoSite(data []byte) (map[string]SiteTag, error) {
	result := make(map[string]SiteTag)
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
		record, err := decodeSiteTag(payload)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(record.Name)
		if _, duplicate := result[key]; duplicate {
			return nil, fmt.Errorf("duplicate geosite tag %q", record.Name)
		}
		result[key] = record
	}
	return result, nil
}

func decodeSiteTag(data []byte) (SiteTag, error) {
	var record SiteTag
	var encodedDomains [][]byte
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, _, err := cursor.next()
		if err != nil {
			return SiteTag{}, err
		}
		switch field {
		case 1:
			if wire != 2 {
				return SiteTag{}, fmt.Errorf("geosite tag has wire type %d", wire)
			}
			record.Name = string(payload)
		case 2:
			if wire != 2 {
				return SiteTag{}, fmt.Errorf("geosite domain has wire type %d", wire)
			}
			encodedDomains = append(encodedDomains, payload)
		}
	}
	if record.Name == "" {
		return SiteTag{}, fmt.Errorf("geosite entry has empty tag")
	}
	for _, encoded := range encodedDomains {
		rule, err := decodeDomain(encoded)
		if err != nil {
			return SiteTag{}, fmt.Errorf("tag %q: %w", record.Name, err)
		}
		record.Domains = append(record.Domains, rule)
	}
	return record, nil
}

func decodeDomain(data []byte) (model.DomainRule, error) {
	var kind uint64
	var value string
	var encodedAttributes [][]byte
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, scalar, err := cursor.next()
		if err != nil {
			return model.DomainRule{}, err
		}
		switch field {
		case 1:
			if wire != 0 {
				return model.DomainRule{}, fmt.Errorf("domain kind has wire type %d", wire)
			}
			kind = scalar
		case 2:
			if wire != 2 {
				return model.DomainRule{}, fmt.Errorf("domain value has wire type %d", wire)
			}
			value = string(payload)
		case 3:
			if wire != 2 {
				return model.DomainRule{}, fmt.Errorf("domain attribute has wire type %d", wire)
			}
			encodedAttributes = append(encodedAttributes, payload)
		}
	}

	domainKind, err := domainKind(kind)
	if err != nil {
		return model.DomainRule{}, err
	}
	if !validDomainValue(value) {
		return model.DomainRule{}, fmt.Errorf("invalid domain value %q", value)
	}

	attributes := make(map[string]struct{}, len(encodedAttributes))
	for _, encoded := range encodedAttributes {
		attribute, err := decodeAttribute(encoded)
		if err != nil {
			return model.DomainRule{}, fmt.Errorf("domain %q attribute: %w", value, err)
		}
		attributes[attribute] = struct{}{}
	}
	var attrs []string
	if len(attributes) != 0 {
		attrs = make([]string, 0, len(attributes))
	}
	for attribute := range attributes {
		attrs = append(attrs, attribute)
	}
	sort.Strings(attrs)

	return model.DomainRule{Kind: domainKind, Value: value, Attrs: attrs}, nil
}

func domainKind(kind uint64) (model.DomainKind, error) {
	switch kind {
	case 0:
		return model.DomainKeyword, nil
	case 1:
		return model.DomainRegexp, nil
	case 2:
		return model.DomainSuffix, nil
	case 3:
		return model.DomainFull, nil
	default:
		return "", fmt.Errorf("unsupported domain kind %d", kind)
	}
}

func decodeAttribute(data []byte) (string, error) {
	var key string
	var hasValue bool
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, scalar, err := cursor.next()
		if err != nil {
			return "", err
		}
		switch field {
		case 1:
			if wire != 2 {
				return "", fmt.Errorf("key has wire type %d", wire)
			}
			key = string(payload)
		case 2:
			if wire != 0 {
				return "", fmt.Errorf("boolean value has wire type %d", wire)
			}
			if hasValue {
				return "", fmt.Errorf("has multiple typed values")
			}
			hasValue = true
			if scalar == 0 {
				return "", fmt.Errorf("boolean value is not true")
			}
		case 3:
			return "", fmt.Errorf("integer value cannot be represented")
		default:
			return "", fmt.Errorf("contains unknown field %d", field)
		}
	}
	if !validAttributeKey(key) {
		return "", fmt.Errorf("invalid key %q", key)
	}
	if !hasValue {
		return "", fmt.Errorf("key %q has no typed value", key)
	}
	return "@" + key, nil
}

// DecodeGeoIP decodes a compiled geoip DAT. The returned map is keyed by
// lowercase tag name while IPTag.Name preserves the spelling from the DAT.
func DecodeGeoIP(data []byte) (map[string]IPTag, error) {
	result := make(map[string]IPTag)
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
		record, err := decodeIPTag(payload)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(record.Name)
		if _, duplicate := result[key]; duplicate {
			return nil, fmt.Errorf("duplicate geoip tag %q", record.Name)
		}
		result[key] = record
	}
	return result, nil
}

func decodeIPTag(data []byte) (IPTag, error) {
	var record IPTag
	var encodedCIDRs [][]byte
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, _, err := cursor.next()
		if err != nil {
			return IPTag{}, err
		}
		switch field {
		case 1:
			if wire != 2 {
				return IPTag{}, fmt.Errorf("geoip tag has wire type %d", wire)
			}
			record.Name = string(payload)
		case 2:
			if wire != 2 {
				return IPTag{}, fmt.Errorf("geoip cidr has wire type %d", wire)
			}
			encodedCIDRs = append(encodedCIDRs, payload)
		}
	}
	if record.Name == "" {
		return IPTag{}, fmt.Errorf("geoip entry has empty tag")
	}
	for _, encoded := range encodedCIDRs {
		prefix, err := decodeCIDR(encoded)
		if err != nil {
			return IPTag{}, fmt.Errorf("tag %q: %w", record.Name, err)
		}
		record.Prefixes = append(record.Prefixes, prefix)
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

func validDomainValue(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validAttributeKey(key string) bool {
	if !validDomainValue(key) || strings.ContainsAny(key, "@, \t") {
		return false
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
