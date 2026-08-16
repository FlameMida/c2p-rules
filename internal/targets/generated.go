package targets

import "strings"

const (
	ProxyResidualTag  = "proxy"
	DirectResidualTag = "direct"
)

var residualGeoSiteTags = []string{DirectResidualTag, ProxyResidualTag}
var retiredGeoSiteTags = []string{"crs-direct-residual", "crs-proxy-residual", "gfw", "tld-not-cn"}

func ResidualGeoSiteTags() []string {
	return append([]string(nil), residualGeoSiteTags...)
}

func RetiredGeoSiteTags() []string {
	return append([]string(nil), retiredGeoSiteTags...)
}

func IsIntermediateGeoSiteTag(tag string) bool {
	for _, intermediate := range []string{"gfw", "tld-not-cn"} {
		if strings.EqualFold(tag, intermediate) {
			return true
		}
	}
	return false
}
