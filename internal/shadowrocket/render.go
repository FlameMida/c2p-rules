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
	byID := make(map[string]GroupRules, len(groups))
	seenRemarks := make(map[string]string, len(groups))
	for _, rules := range groups {
		if _, duplicate := byID[rules.Group.ID]; duplicate {
			return nil, fmt.Errorf("duplicate group id %q", rules.Group.ID)
		}
		byID[rules.Group.ID] = rules
		if err := validateRemark(rules.Group.Remarks); err != nil {
			return nil, fmt.Errorf("group %q: %w", rules.Group.ID, err)
		}
		if reservedPolicyName(rules.Group.Remarks) {
			return nil, fmt.Errorf("group %q uses reserved policy group name %q", rules.Group.ID, rules.Group.Remarks)
		}
		if previousID, duplicate := seenRemarks[rules.Group.Remarks]; duplicate {
			return nil, fmt.Errorf("groups %q and %q repeat policy group name %q", previousID, rules.Group.ID, rules.Group.Remarks)
		}
		seenRemarks[rules.Group.Remarks] = rules.Group.ID
	}
	proxyRemarks := ""
	if _, hasTLD := byID["tld_not_cn"]; hasTLD {
		proxy, ok := byID["proxy"]
		if !ok {
			return nil, fmt.Errorf("tld_not_cn requires proxy group")
		}
		proxyRemarks = proxy.Group.Remarks
	}

	var output bytes.Buffer
	fmt.Fprintf(&output, "[General]\nupdate-url = https://github.com/%s/releases/latest/download/shadowrocket.conf\n\n", repo)
	output.WriteString("[Proxy Group]\n")
	cnRemarks := ""
	for _, rules := range groups {
		if len(rules.Domains) == 0 && len(rules.CIDRs) == 0 {
			return nil, fmt.Errorf("group %q (%s) has zero rules", rules.Group.Remarks, rules.Group.ID)
		}
		candidates, selected, err := policy(rules.Group.ID, proxyRemarks)
		if err != nil {
			return nil, err
		}
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

func reservedPolicyName(value string) bool {
	switch {
	case strings.EqualFold(value, "PROXY"), strings.EqualFold(value, "DIRECT"), strings.EqualFold(value, "REJECT"):
		return true
	default:
		return false
	}
}

func policy(id, proxyRemarks string) ([]string, string, error) {
	switch id {
	case "reject":
		return []string{"REJECT", "PROXY", "DIRECT"}, "REJECT", nil
	case "cn", "private", "direct":
		return []string{"DIRECT", "PROXY"}, "DIRECT", nil
	case "tld_not_cn":
		if proxyRemarks == "" {
			return nil, "", fmt.Errorf("tld_not_cn requires proxy remarks")
		}
		return []string{proxyRemarks}, proxyRemarks, nil
	default:
		return []string{"PROXY", "DIRECT"}, "PROXY", nil
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
