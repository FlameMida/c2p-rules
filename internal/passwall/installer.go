package passwall

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"unicode"
)

type InstallOptions struct {
	Repo       string
	ReleaseTag string
	GeoSiteSHA string
	GeoIPSHA   string
	Fragment   []byte
}

type installerData struct {
	Repo           string
	ReleaseTag     string
	GeoSiteSHA     string
	GeoIPSHA       string
	FragmentBase64 string
}

var (
	repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	releaseTagPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	sha256Pattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

//go:embed install.sh.tmpl
var installerTemplate string

func RenderInstaller(options InstallOptions) ([]byte, error) {
	if !repositoryPattern.MatchString(options.Repo) || strings.EqualFold(options.Repo, "owner/repo") {
		return nil, fmt.Errorf("invalid or placeholder repository %q", options.Repo)
	}
	if !releaseTagPattern.MatchString(options.ReleaseTag) {
		return nil, fmt.Errorf("invalid release tag %q", options.ReleaseTag)
	}
	if !sha256Pattern.MatchString(options.GeoSiteSHA) {
		return nil, fmt.Errorf("invalid geosite SHA-256")
	}
	if !sha256Pattern.MatchString(options.GeoIPSHA) {
		return nil, fmt.Errorf("invalid geoip SHA-256")
	}
	if len(options.Fragment) == 0 {
		return nil, fmt.Errorf("managed UCI fragment is empty")
	}
	for _, r := range string(options.Fragment) {
		if r == '\x00' || r == '\r' || r == '\u2028' || r == '\u2029' || unicode.IsControl(r) && r != '\n' && r != '\t' {
			return nil, fmt.Errorf("managed UCI fragment contains a forbidden control character")
		}
	}
	parsed, err := template.New("install.sh").Option("missingkey=error").Parse(installerTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse installer template: %w", err)
	}
	data := installerData{
		Repo:           options.Repo,
		ReleaseTag:     options.ReleaseTag,
		GeoSiteSHA:     options.GeoSiteSHA,
		GeoIPSHA:       options.GeoIPSHA,
		FragmentBase64: base64.StdEncoding.EncodeToString(options.Fragment),
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, data); err != nil {
		return nil, fmt.Errorf("render installer template: %w", err)
	}
	return output.Bytes(), nil
}
