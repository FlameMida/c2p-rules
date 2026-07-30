package workspace

import "path/filepath"

type Layout struct {
	Staging     string
	Build       string
	Publish     string
	DataMerged  string
	IP          string
	Manifest    string
	GeoIPConfig string
	BaseGeoIP   string
}

func newLayout(staging string) Layout {
	build := filepath.Join(staging, "build")
	return Layout{
		Staging:     staging,
		Build:       build,
		Publish:     filepath.Join(staging, "publish"),
		DataMerged:  filepath.Join(build, "data-merged"),
		IP:          filepath.Join(build, "ip"),
		Manifest:    filepath.Join(build, "expected_tags.json"),
		GeoIPConfig: filepath.Join(build, "geoip-config.json"),
		BaseGeoIP:   filepath.Join(build, "base-geoip.dat"),
	}
}
