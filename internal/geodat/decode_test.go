package geodat

import (
	"encoding/binary"
	"net/netip"
	"reflect"
	"strings"
	"testing"

	"clash-rules-srs/internal/model"
)

func TestInclude与属性在残差输入中已展开(t *testing.T) {
	data := siteDat(siteEntry("GOOGLE",
		domainEntry(0, "needle", attribute("cn", 2, 1)),
		domainEntry(1, `^x{1,3}\.regexp\.example$`),
		domainEntry(2, "suffix.example"),
		domainEntry(3, "full.example"),
	))
	tags, err := DecodeGeoSite(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []model.DomainRule{
		{Kind: model.DomainKeyword, Value: "needle", Attrs: []string{"@cn"}},
		{Kind: model.DomainRegexp, Value: `^x{1,3}\.regexp\.example$`},
		{Kind: model.DomainSuffix, Value: "suffix.example"},
		{Kind: model.DomainFull, Value: "full.example"},
	}
	if got := tags["google"]; got.Name != "GOOGLE" || !reflect.DeepEqual(got.Domains, want) {
		t.Fatalf("tag=%#v want=%#v", got, want)
	}
}

func Test无法回写的TypedAttribute失败(t *testing.T) {
	tests := map[string][]byte{
		"false":   attribute("cn", 2, 0),
		"int":     attribute("cn", 3, 7),
		"unknown": append(attribute("cn", 2, 1), fieldVarint(9, 1)...),
	}
	for name, attr := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeGeoSite(siteDat(siteEntry("TEST", domainEntry(2, "bad.example", attr))))
			if err == nil || !strings.Contains(err.Error(), "TEST") ||
				!strings.Contains(err.Error(), "bad.example") || !strings.Contains(err.Error(), "attribute") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestAttributes规范化为有序集合(t *testing.T) {
	tags, err := DecodeGeoSite(siteDat(siteEntry("TEST", domainEntry(2, "attrs.example",
		attribute("z", 2, 1), attribute("a", 2, 1), attribute("z", 2, 1),
	))))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"@a", "@z"}
	if got := tags["test"].Domains[0].Attrs; !reflect.DeepEqual(got, want) {
		t.Fatalf("attrs=%v want=%v", got, want)
	}
}

func TestGeoIPDecoder保持Shadowrocket前缀语义(t *testing.T) {
	want := []netip.Prefix{
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("2001:db8::/32"),
	}
	tags, err := DecodeGeoIP(ipDat(ipEntry("CN", cidrEntry(want[0]), cidrEntry(want[1]))))
	if err != nil {
		t.Fatal(err)
	}
	if got := tags["cn"].Prefixes; !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
}

func TestDecoder拒绝重复Tag与损坏Wire(t *testing.T) {
	tests := map[string]struct {
		decode func([]byte) error
		data   []byte
		want   string
	}{
		"duplicate site": {
			decode: func(data []byte) error { _, err := DecodeGeoSite(data); return err },
			data: siteDat(
				siteEntry("TEST", domainEntry(2, "one.example")),
				siteEntry("test", domainEntry(2, "two.example")),
			),
			want: "duplicate geosite tag",
		},
		"duplicate ip": {
			decode: func(data []byte) error { _, err := DecodeGeoIP(data); return err },
			data: ipDat(
				ipEntry("TEST", cidrEntry(netip.MustParsePrefix("192.0.2.0/24"))),
				ipEntry("test", cidrEntry(netip.MustParsePrefix("198.51.100.0/24"))),
			),
			want: "duplicate geoip tag",
		},
		"truncated bytes": {
			decode: func(data []byte) error { _, err := DecodeGeoSite(data); return err },
			data:   []byte{0x0a, 0x02, 0x01},
			want:   "truncated protobuf bytes",
		},
		"unsupported domain kind": {
			decode: func(data []byte) error { _, err := DecodeGeoSite(data); return err },
			data:   siteDat(siteEntry("TEST", domainEntry(4, "bad.example"))),
			want:   "unsupported domain kind 4",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.decode(test.data)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err=%v want substring %q", err, test.want)
			}
		})
	}
}

func TestDecoder消费未知普通字段(t *testing.T) {
	domain := append(domainEntry(2, "kept.example"), fieldVarint(99, 7)...)
	entry := append(siteEntry("TEST", domain), fieldBytes(100, []byte("ignored"))...)
	data := append(siteDat(entry), fieldVarint(101, 9)...)
	tags, err := DecodeGeoSite(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := tags["test"].Domains; len(got) != 1 || got[0].Value != "kept.example" {
		t.Fatalf("domains=%#v", got)
	}
}

func siteDat(entries ...[]byte) []byte {
	var result []byte
	for _, entry := range entries {
		result = append(result, fieldBytes(1, entry)...)
	}
	return result
}

func siteEntry(tag string, domains ...[]byte) []byte {
	result := fieldBytes(1, []byte(tag))
	for _, domain := range domains {
		result = append(result, fieldBytes(2, domain)...)
	}
	return result
}

func domainEntry(kind uint64, value string, attributes ...[]byte) []byte {
	result := fieldVarint(1, kind)
	result = append(result, fieldBytes(2, []byte(value))...)
	for _, attr := range attributes {
		result = append(result, fieldBytes(3, attr)...)
	}
	return result
}

func attribute(key string, typedField, typedValue uint64) []byte {
	result := fieldBytes(1, []byte(key))
	return append(result, fieldVarint(typedField, typedValue)...)
}

func ipDat(entries ...[]byte) []byte {
	var result []byte
	for _, entry := range entries {
		result = append(result, fieldBytes(1, entry)...)
	}
	return result
}

func ipEntry(tag string, cidrs ...[]byte) []byte {
	result := fieldBytes(1, []byte(tag))
	for _, cidr := range cidrs {
		result = append(result, fieldBytes(2, cidr)...)
	}
	return result
}

func cidrEntry(prefix netip.Prefix) []byte {
	result := fieldBytes(1, prefix.Addr().AsSlice())
	return append(result, fieldVarint(2, uint64(prefix.Bits()))...)
}

func fieldBytes(number uint64, value []byte) []byte {
	result := binary.AppendUvarint(nil, number<<3|2)
	result = binary.AppendUvarint(result, uint64(len(value)))
	return append(result, value...)
}

func fieldVarint(number, value uint64) []byte {
	result := binary.AppendUvarint(nil, number<<3)
	return binary.AppendUvarint(result, value)
}
