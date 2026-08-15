package shadowrocket

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"clash-rules-srs/internal/model"
)

var repositoryPattern = regexp.MustCompile("^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$")

func Render(groups []GroupRules, repo string) ([]byte, error) {
	if !repositoryPattern.MatchString(repo) || strings.EqualFold(repo, "owner/repo") {
		return nil, fmt.Errorf("invalid or placeholder repository %q", repo)
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("no Shadowrocket groups")
	}

	var output bytes.Buffer
	fmt.Fprintf(&output, "[General]\nupdate-url = https://github.com/%s/releases/latest/download/shadowrocket.conf\n\n", repo)
	output.WriteString("[Proxy Group]\n")
	cnRemarks := ""
	for _, rules := range groups {
		if len(rules.Domains) == 0 && len(rules.CIDRs) == 0 {
			return nil, fmt.Errorf("group %q (%s) has zero rules", rules.Group.Remarks, rules.Group.ID)
		}
		if err := validateRemark(rules.Group.Remarks); err != nil {
			return nil, fmt.Errorf("group %q: %w", rules.Group.ID, err)
		}
		candidates, selected := policy(rules.Group.ID)
		fmt.Fprintf(&output, "%s = select,%s,policy-select-name=%s\n",
			rules.Group.Remarks, strings.Join(candidates, ","), selected)
		if rules.Group.ID == "cn" {
			if cnRemarks != "" {
				return nil, fmt.Errorf("duplicate cn group")
			}
			cnRemarks = rules.Group.Remarks
		}
	}
	if cnRemarks == "" {
		return nil, fmt.Errorf("cn group is required for GEOIP fallback")
	}

	output.WriteString("\n[Rule]\n")
	for _, rules := range groups {
		for _, line := range rules.Domains {
			if !validRuleValue(line.Value) {
				return nil, fmt.Errorf("group %q has invalid domain value %q", rules.Group.ID, line.Value)
			}
			kind, err := ruleKind(line.Kind)
			if err != nil {
				return nil, fmt.Errorf("group %q: %w", rules.Group.ID, err)
			}
			fmt.Fprintf(&output, "%s,%s,%s\n", kind, line.Value, rules.Group.Remarks)
		}
		for _, prefix := range rules.CIDRs {
			if !prefix.IsValid() {
				return nil, fmt.Errorf("group %q has invalid CIDR", rules.Group.ID)
			}
			kind := "IP-CIDR6"
			if prefix.Addr().Is4() {
				kind = "IP-CIDR"
			}
			fmt.Fprintf(&output, "%s,%s,%s,no-resolve\n", kind, prefix.Masked(), rules.Group.Remarks)
		}
	}
	fmt.Fprintf(&output, "GEOIP,CN,%s\nFINAL,PROXY\n", cnRemarks)
	return output.Bytes(), nil
}

func policy(id string) ([]string, string) {
	switch id {
	case "reject":
		return []string{"REJECT", "PROXY", "DIRECT"}, "REJECT"
	case "cn", "private", "direct":
		return []string{"DIRECT", "PROXY"}, "DIRECT"
	default:
		return []string{"PROXY", "DIRECT"}, "PROXY"
	}
}

func ruleKind(kind model.DomainKind) (string, error) {
	switch kind {
	case model.DomainSuffix:
		return "DOMAIN-SUFFIX", nil
	case model.DomainFull:
		return "DOMAIN", nil
	case model.DomainKeyword:
		return "DOMAIN-KEYWORD", nil
	default:
		return "", fmt.Errorf("unsupported domain kind %q", kind)
	}
}

func validateRemark(value string) error {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, ",=[]") {
		return fmt.Errorf("invalid remarks %q", value)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("invalid remarks %q", value)
		}
	}
	return nil
}
