package targets

import "clash-rules-srs/internal/model"

const (
	ProxyResidualTag  = "crs-proxy-residual"
	DirectResidualTag = "crs-direct-residual"
)

var generatedGeoSiteTags = []string{DirectResidualTag, ProxyResidualTag}

func GeneratedGeoSiteTags() []string {
	return append([]string(nil), generatedGeoSiteTags...)
}

func IsGenerated(side model.Side, tag string) bool {
	if side != model.GeoSite {
		return false
	}
	for _, generated := range generatedGeoSiteTags {
		if tag == generated {
			return true
		}
	}
	return false
}
