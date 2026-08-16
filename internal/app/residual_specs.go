package app

import (
	"fmt"

	"clash-rules-srs/internal/geosite"
	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/targets"
)

func residualSpecsForGroups(groups []model.Group) (geosite.ResidualSpec, geosite.ResidualSpec, error) {
	proxyIndex := groupIndex(groups, "proxy")
	directIndex := groupIndex(groups, "direct")
	if proxyIndex < 0 || directIndex < 0 {
		return geosite.ResidualSpec{}, geosite.ResidualSpec{}, fmt.Errorf("residual groups require proxy and direct")
	}

	proxy := geosite.ResidualSpec{
		OutputTag:           targets.ProxyResidualTag,
		BasisTags:           []string{"gfw", "proxy"},
		EarlierActiveTags:   earlierGeoSiteTags(groups, proxyIndex),
		LaterEquivalentTags: []string{"tld-not-cn"},
	}
	direct := geosite.ResidualSpec{
		OutputTag:         targets.DirectResidualTag,
		BasisTags:         []string{"direct"},
		EarlierActiveTags: earlierGeoSiteTags(groups, directIndex),
	}
	return proxy, direct, nil
}

func groupIndex(groups []model.Group, id string) int {
	for index, group := range groups {
		if group.ID == id {
			return index
		}
	}
	return -1
}

func earlierGeoSiteTags(groups []model.Group, end int) []string {
	seen := make(map[string]struct{})
	var tags []string
	for _, group := range groups[:end] {
		for _, tag := range group.GeoSite {
			if _, exists := seen[tag]; exists {
				continue
			}
			seen[tag] = struct{}{}
			tags = append(tags, tag)
		}
	}
	return tags
}
