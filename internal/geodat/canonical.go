package geodat

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"clash-rules-srs/internal/model"
)

// CanonicalizeGeoSite rewrites a supported compiled GeoSite DAT with stable
// tag, rule, and attribute ordering. DecodeGeoSite remains the validation and
// losslessness boundary for the compiler output accepted here.
func CanonicalizeGeoSite(data []byte) ([]byte, error) {
	if err := validateCanonicalGeoSiteSchema(data); err != nil {
		return nil, err
	}
	tags, err := DecodeGeoSite(data)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(tags))
	for name := range tags {
		names = append(names, name)
	}
	sort.Strings(names)

	var document []byte
	for _, name := range names {
		tag := tags[name]
		rules := append([]model.DomainRule(nil), tag.Domains...)
		sort.Slice(rules, func(i, j int) bool {
			return canonicalDomainKey(rules[i]) < canonicalDomainKey(rules[j])
		})
		entry := canonicalFieldBytes(1, []byte(tag.Name))
		for _, rule := range rules {
			encoded, err := encodeCanonicalDomain(rule)
			if err != nil {
				return nil, fmt.Errorf("tag %q: %w", tag.Name, err)
			}
			entry = append(entry, canonicalFieldBytes(2, encoded)...)
		}
		document = append(document, canonicalFieldBytes(1, entry)...)
	}
	return document, nil
}

func validateCanonicalGeoSiteSchema(data []byte) error {
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, _, err := cursor.next()
		if err != nil {
			return err
		}
		if field != 1 {
			return fmt.Errorf("geosite document contains unknown field %d", field)
		}
		if wire != 2 {
			return fmt.Errorf("geosite entry has wire type %d", wire)
		}
		if err := validateCanonicalSiteTagSchema(payload); err != nil {
			return err
		}
	}
	return nil
}

func validateCanonicalSiteTagSchema(data []byte) error {
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, payload, _, err := cursor.next()
		if err != nil {
			return err
		}
		switch field {
		case 1:
			if wire != 2 {
				return fmt.Errorf("geosite tag has wire type %d", wire)
			}
		case 2:
			if wire != 2 {
				return fmt.Errorf("geosite domain has wire type %d", wire)
			}
			if err := validateCanonicalDomainSchema(payload); err != nil {
				return err
			}
		default:
			return fmt.Errorf("geosite tag contains unknown field %d", field)
		}
	}
	return nil
}

func validateCanonicalDomainSchema(data []byte) error {
	cursor := wireCursor{data: data}
	for cursor.more() {
		field, wire, _, _, err := cursor.next()
		if err != nil {
			return err
		}
		switch field {
		case 1:
			if wire != 0 {
				return fmt.Errorf("domain kind has wire type %d", wire)
			}
		case 2, 3:
			if wire != 2 {
				return fmt.Errorf("domain field %d has wire type %d", field, wire)
			}
		default:
			return fmt.Errorf("geosite domain contains unknown field %d", field)
		}
	}
	return nil
}

func encodeCanonicalDomain(rule model.DomainRule) ([]byte, error) {
	kind, err := canonicalDomainKind(rule.Kind)
	if err != nil {
		return nil, err
	}
	encoded := canonicalFieldVarint(1, kind)
	encoded = append(encoded, canonicalFieldBytes(2, []byte(rule.Value))...)
	attrs := append([]string(nil), rule.Attrs...)
	sort.Strings(attrs)
	for _, attr := range attrs {
		if !strings.HasPrefix(attr, "@") || len(attr) == 1 {
			return nil, fmt.Errorf("invalid normalized attribute %q", attr)
		}
		attribute := canonicalFieldBytes(1, []byte(strings.TrimPrefix(attr, "@")))
		attribute = append(attribute, canonicalFieldVarint(2, 1)...)
		encoded = append(encoded, canonicalFieldBytes(3, attribute)...)
	}
	return encoded, nil
}

func canonicalDomainKind(kind model.DomainKind) (uint64, error) {
	switch kind {
	case model.DomainKeyword:
		return 0, nil
	case model.DomainRegexp:
		return 1, nil
	case model.DomainSuffix:
		return 2, nil
	case model.DomainFull:
		return 3, nil
	default:
		return 0, fmt.Errorf("unsupported domain kind %q", kind)
	}
}

func canonicalDomainKey(rule model.DomainRule) string {
	attrs := append([]string(nil), rule.Attrs...)
	sort.Strings(attrs)
	return string(rule.Kind) + "\x00" + rule.Value + "\x00" + strings.Join(attrs, "\x00")
}

func canonicalFieldBytes(number uint64, value []byte) []byte {
	result := binary.AppendUvarint(nil, number<<3|2)
	result = binary.AppendUvarint(result, uint64(len(value)))
	return append(result, value...)
}

func canonicalFieldVarint(number, value uint64) []byte {
	result := binary.AppendUvarint(nil, number<<3)
	return binary.AppendUvarint(result, value)
}
