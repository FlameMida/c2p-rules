package passwall

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"clash-rules-srs/internal/model"
	"clash-rules-srs/internal/verify"
)

var (
	groupIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,60}$`)
	tagPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

func ValidateGroups(ctx context.Context, groups []model.Group, lookup verify.TagLookup) error {
	return verify.GroupRefs(ctx, lookup, groups)
}

func Render(groups []model.Group) ([]byte, error) {
	seenIDs := make(map[string]struct{}, len(groups))
	var output strings.Builder
	for index, group := range groups {
		if !groupIDPattern.MatchString(group.ID) {
			return nil, fmt.Errorf("group %d has invalid id %q", index, group.ID)
		}
		sectionID := "crs_" + group.ID
		if len([]byte(sectionID)) > 64 {
			return nil, fmt.Errorf("group %q section id exceeds 64 bytes", group.ID)
		}
		if _, duplicate := seenIDs[group.ID]; duplicate {
			return nil, fmt.Errorf("duplicate group id %q", group.ID)
		}
		seenIDs[group.ID] = struct{}{}
		if group.Remarks == "" {
			return nil, fmt.Errorf("group %q has empty remarks", group.ID)
		}
		if len(group.GeoSite) == 0 && len(group.GeoIP) == 0 {
			return nil, fmt.Errorf("group %q has no geosite or geoip references", group.ID)
		}
		remarks, err := uciQuote(group.Remarks)
		if err != nil {
			return nil, fmt.Errorf("group %q remarks: %w", group.ID, err)
		}
		domainList, err := renderRefs(group.ID, model.GeoSite, group.GeoSite)
		if err != nil {
			return nil, err
		}
		ipList, err := renderRefs(group.ID, model.GeoIP, group.GeoIP)
		if err != nil {
			return nil, err
		}
		if index != 0 {
			output.WriteByte('\n')
		}
		fmt.Fprintf(&output, "config shunt_rules '%s'\n", sectionID)
		fmt.Fprintf(&output, "\toption remarks %s\n", remarks)
		output.WriteString("\toption managed_by 'clash-rules-srs'\n")
		output.WriteString("\toption network 'tcp,udp'\n")
		if domainList != "" {
			quoted, err := uciQuoteMultiline(domainList)
			if err != nil {
				return nil, err
			}
			fmt.Fprintf(&output, "\toption domain_list %s\n", quoted)
		}
		if ipList != "" {
			quoted, err := uciQuoteMultiline(ipList)
			if err != nil {
				return nil, err
			}
			fmt.Fprintf(&output, "\toption ip_list %s\n", quoted)
		}
	}
	return []byte(output.String()), nil
}

func renderRefs(groupID string, side model.Side, tags []string) (string, error) {
	seen := make(map[string]struct{}, len(tags))
	references := make([]string, 0, len(tags))
	for _, tag := range tags {
		if !tagPattern.MatchString(tag) || hasForbiddenControl(tag) {
			return "", fmt.Errorf("group %q has invalid %s tag %q", groupID, side, tag)
		}
		if _, duplicate := seen[tag]; duplicate {
			return "", fmt.Errorf("group %q repeats %s:%s", groupID, side, tag)
		}
		seen[tag] = struct{}{}
		references = append(references, string(side)+":"+tag)
	}
	return strings.Join(references, "\n"), nil
}

func uciQuote(value string) (string, error) {
	if hasForbiddenControl(value) {
		return "", fmt.Errorf("UCI scalar contains a forbidden control character")
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'", nil
}

func uciQuoteMultiline(value string) (string, error) {
	for _, r := range value {
		if r != '\n' && (r == '\u2028' || r == '\u2029' || unicode.IsControl(r)) {
			return "", fmt.Errorf("UCI multiline scalar contains a forbidden control character")
		}
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'", nil
}

func hasForbiddenControl(value string) bool {
	for _, r := range value {
		if r == '\u2028' || r == '\u2029' || unicode.IsControl(r) {
			return true
		}
	}
	return false
}
