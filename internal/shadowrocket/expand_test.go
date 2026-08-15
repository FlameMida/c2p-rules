package shadowrocket_test

import (
	"context"
	"encoding/binary"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/shadowrocket"
)

func TestInclude被展开进Google组(t *testing.T) {
	site, ip := writeDatPair(t,
		geoSiteDat(siteEntry("GOOGLE",
			domainEntry(2, "google.example"),
			domainEntry(2, "youtube-from-include.example"),
		)),
		geoIPDat(),
	)
	groups := []model.Group{{ID: "google", Remarks: "Google 服务", GeoSite: []string{"google"}}}
	got, err := shadowrocket.Expand(context.Background(), site, ip, groups)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !hasDomain(got[0].Domains, model.DomainSuffix, "youtube-from-include.example") {
		t.Fatalf("expanded=%+v", got)
	}
}

func TestMergeBaseGeoIP含底包(t *testing.T) {
	site, ip := writeDatPair(t,
		geoSiteDat(siteEntry("CN", domainEntry(2, "cn.example"))),
		geoIPDat(ipEntry("CN",
			cidrEntry(netip.MustParsePrefix("192.0.2.0/24")),
			cidrEntry(netip.MustParsePrefix("2001:db8::/32")),
		)),
	)
	groups := []model.Group{{ID: "cn", Remarks: "中国大陆", GeoSite: []string{"cn"}, GeoIP: []string{"cn"}}}
	got, err := shadowrocket.Expand(context.Background(), site, ip, groups)
	if err != nil {
		t.Fatal(err)
	}
	if want := []netip.Prefix{
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("2001:db8::/32"),
	}; len(got[0].CIDRs) != len(want) || got[0].CIDRs[0] != want[0] || got[0].CIDRs[1] != want[1] {
		t.Fatalf("cidrs=%v want=%v", got[0].CIDRs, want)
	}
}

func TestRegexp被跳过(t *testing.T) {
	withAttribute := append(domainEntry(2, "kept.example"), fieldBytes(3, fieldBytes(1, []byte("cn")))...)
	site, ip := writeDatPair(t,
		geoSiteDat(siteEntry("TEST",
			withAttribute,
			domainEntry(1, "^x{1,3}\\.skipped\\.example$"),
			domainEntry(2, "kept.example"),
		)),
		geoIPDat(),
	)
	groups := []model.Group{{ID: "test", Remarks: "测试组", GeoSite: []string{"test"}}}
	got, err := shadowrocket.Expand(context.Background(), site, ip, groups)
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Domains) != 1 || got[0].Domains[0].Value != "kept.example" {
		t.Fatalf("domains=%+v", got[0].Domains)
	}
}

func TestExpand拒绝缺失Tag与空组(t *testing.T) {
	tests := map[string]struct {
		site []byte
		want string
	}{
		"missing":     {geoSiteDat(), "geosite:not-exist"},
		"regexp only": {geoSiteDat(siteEntry("TEST", domainEntry(1, "only-regexp"))), "测试组"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			site, ip := writeDatPair(t, test.site, geoIPDat())
			tag := "test"
			if name == "missing" {
				tag = "not-exist"
			}
			_, err := shadowrocket.Expand(context.Background(), site, ip, []model.Group{{
				ID: "test", Remarks: "测试组", GeoSite: []string{tag},
			}})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestExpand拒绝损坏Dat(t *testing.T) {
	invalidPrefix := append(fieldBytes(1, netip.MustParseAddr("192.0.2.1").AsSlice()), fieldVarint(2, 33)...)
	tests := map[string]struct {
		site   []byte
		ip     []byte
		groups []model.Group
		want   string
	}{
		"known field wrong wire": {
			site: fieldVarint(1, 1), ip: geoIPDat(), want: "geosite entry has wire type",
		},
		"truncated bytes": {
			site: []byte{0x0a, 0x02, 0x01}, ip: geoIPDat(), want: "truncated protobuf bytes",
		},
		"truncated varint": {
			site: []byte{0x80}, ip: geoIPDat(), want: "invalid protobuf field key",
		},
		"invalid domain enum": {
			site: geoSiteDat(siteEntry("TEST", domainEntry(4, "bad.example"))), ip: geoIPDat(), want: "unsupported domain kind 4",
		},
		"invalid ip": {
			site: geoSiteDat(), ip: geoIPDat(ipEntry("TEST", fieldBytes(1, []byte{1, 2, 3}))), want: "invalid cidr address",
		},
		"invalid prefix": {
			site: geoSiteDat(), ip: geoIPDat(ipEntry("TEST", invalidPrefix)), want: "invalid /33",
		},
		"duplicate geosite tag": {
			site: geoSiteDat(
				siteEntry("TEST", domainEntry(2, "one.example")),
				siteEntry("test", domainEntry(2, "two.example")),
			),
			ip: geoIPDat(), want: "duplicate geosite tag",
		},
		"duplicate geoip tag": {
			site: geoSiteDat(),
			ip: geoIPDat(
				ipEntry("TEST", cidrEntry(netip.MustParsePrefix("192.0.2.0/24"))),
				ipEntry("test", cidrEntry(netip.MustParsePrefix("198.51.100.0/24"))),
			),
			want: "duplicate geoip tag",
		},
		"missing geoip tag": {
			site: geoSiteDat(), ip: geoIPDat(),
			groups: []model.Group{{ID: "test", Remarks: "测试组", GeoIP: []string{"not-exist"}}},
			want:   "geoip:not-exist",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			site, ip := writeDatPair(t, test.site, test.ip)
			_, err := shadowrocket.Expand(context.Background(), site, ip, test.groups)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err=%v want substring %q", err, test.want)
			}
		})
	}
}

func TestExpand消费未知普通字段(t *testing.T) {
	domain := append(domainEntry(2, "kept.example"), fieldVarint(99, 7)...)
	entry := append(siteEntry("TEST", domain), fieldBytes(100, []byte("ignored"))...)
	siteData := append(geoSiteDat(entry), fieldVarint(101, 9)...)
	site, ip := writeDatPair(t, siteData, geoIPDat())
	got, err := shadowrocket.Expand(context.Background(), site, ip, []model.Group{{
		ID: "test", Remarks: "测试组", GeoSite: []string{"test"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Domains) != 1 || got[0].Domains[0].Value != "kept.example" {
		t.Fatalf("expanded=%+v", got)
	}
}

func writeDatPair(t *testing.T, siteData, ipData []byte) (string, string) {
	t.Helper()
	root := t.TempDir()
	site := filepath.Join(root, "geosite.dat")
	ip := filepath.Join(root, "geoip.dat")
	if err := os.WriteFile(site, siteData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ip, ipData, 0o600); err != nil {
		t.Fatal(err)
	}
	return site, ip
}

func geoSiteDat(entries ...[]byte) []byte {
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

func domainEntry(kind uint64, value string) []byte {
	result := fieldVarint(1, kind)
	return append(result, fieldBytes(2, []byte(value))...)
}

func geoIPDat(entries ...[]byte) []byte {
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
	address := prefix.Addr().AsSlice()
	result := fieldBytes(1, address)
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

func hasDomain(lines []shadowrocket.DomainLine, kind model.DomainKind, value string) bool {
	for _, line := range lines {
		if line.Kind == kind && line.Value == value {
			return true
		}
	}
	return false
}
