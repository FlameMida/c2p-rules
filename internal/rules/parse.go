package rules

import (
	"bufio"
	"fmt"
	"io"
	"net/netip"
	"regexp"
	"strings"
	"unicode"

	"clash-rules-srs/internal/model"
	"go.yaml.in/yaml/v3"
)

var classicalDomainKinds = map[string]string{
	"DOMAIN-SUFFIX":  "domain",
	"DOMAIN":         "full",
	"DOMAIN-KEYWORD": "keyword",
	"DOMAIN-REGEX":   "regexp",
}

func Parse(r io.Reader, format model.Format, behavior model.Behavior) (model.Buckets, error) {
	return parse(r, format, behavior, false)
}

func parse(r io.Reader, format model.Format, behavior model.Behavior, strictYAML bool) (model.Buckets, error) {
	if format != model.YAML && format != model.Text {
		return model.Buckets{}, fmt.Errorf("unsupported rule format %q", format)
	}
	if behavior != model.Domain && behavior != model.IPCIDR && behavior != model.Classical {
		return model.Buckets{}, fmt.Errorf("unsupported rule behavior %q", behavior)
	}

	var (
		items []string
		err   error
	)
	if format == model.YAML {
		items, err = readYAMLPayload(r, strictYAML)
	} else {
		items, err = readTextRules(r)
	}
	if err != nil {
		return model.Buckets{}, err
	}

	var buckets model.Buckets
	for index, item := range items {
		switch behavior {
		case model.Domain:
			rule, err := parseDomainValue(item)
			if err != nil {
				return model.Buckets{}, fmt.Errorf("rule %d: %w", index+1, err)
			}
			buckets.Domains = append(buckets.Domains, rule)
		case model.IPCIDR:
			prefix, err := parsePrefix(item)
			if err != nil {
				return model.Buckets{}, fmt.Errorf("rule %d: %w", index+1, err)
			}
			buckets.CIDRs = append(buckets.CIDRs, prefix)
		case model.Classical:
			if err := parseClassical(item, &buckets); err != nil {
				return model.Buckets{}, fmt.Errorf("rule %d: %w", index+1, err)
			}
		}
	}
	return buckets, nil
}

func readYAMLPayload(r io.Reader, strict bool) ([]string, error) {
	decoder := yaml.NewDecoder(r)
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode rule YAML: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("rule YAML root must be a mapping")
	}
	root := document.Content[0]
	if strict {
		if err := validateCustomYAML(root); err != nil {
			return nil, err
		}
	}
	var payload *yaml.Node
	for index := 0; index < len(root.Content); index += 2 {
		key := root.Content[index]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return nil, fmt.Errorf("rule YAML mapping keys must be strings")
		}
		if key.Value == "payload" {
			if payload != nil {
				return nil, fmt.Errorf("rule YAML contains duplicate payload keys")
			}
			payload = root.Content[index+1]
		}
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("rule YAML must contain one document")
		}
		return nil, fmt.Errorf("decode trailing rule YAML: %w", err)
	}
	if payload == nil || payload.Kind == yaml.ScalarNode && payload.Tag == "!!null" {
		return nil, nil
	}
	if payload.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("rule YAML payload must be a list")
	}
	items := make([]string, 0, len(payload.Content))
	for index, item := range payload.Content {
		if item.Kind != yaml.ScalarNode || item.Tag != "!!str" {
			return nil, fmt.Errorf("rule YAML payload item %d must be a string", index+1)
		}
		if hasControl(item.Value) {
			return nil, fmt.Errorf("rule YAML payload item %d contains a control character", index+1)
		}
		items = append(items, item.Value)
	}
	return items, nil
}

func validateCustomYAML(root *yaml.Node) error {
	seen := make(map[string]struct{}, len(root.Content)/2)
	for index := 0; index < len(root.Content); index += 2 {
		key := root.Content[index]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return fmt.Errorf("custom rule YAML mapping keys must be strings")
		}
		if hasControl(key.Value) {
			return fmt.Errorf("custom rule YAML key contains a control character")
		}
		if key.Value == "<<" {
			return fmt.Errorf("custom rule YAML merge keys are not allowed")
		}
		if _, exists := seen[key.Value]; exists {
			return fmt.Errorf("custom rule YAML contains duplicate key %q", key.Value)
		}
		seen[key.Value] = struct{}{}
		if key.Value != "payload" {
			return fmt.Errorf("custom rule YAML contains unknown field %q", key.Value)
		}
		if err := validateCustomYAMLValue(root.Content[index+1]); err != nil {
			return err
		}
	}
	return nil
}

func validateCustomYAMLValue(node *yaml.Node) error {
	if node.Anchor != "" || node.Kind == yaml.AliasNode {
		return fmt.Errorf("custom rule YAML aliases are not allowed")
	}
	switch node.Kind {
	case yaml.MappingNode, yaml.SequenceNode:
		for _, child := range node.Content {
			if err := validateCustomYAMLValue(child); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if node.Tag != "!!str" && node.Tag != "!!null" {
			return fmt.Errorf("custom rule YAML scalar must be a string")
		}
		if hasControl(node.Value) {
			return fmt.Errorf("custom rule YAML string contains a control character")
		}
	}
	return nil
}

func readTextRules(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var items []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if hasControl(line) {
			return nil, fmt.Errorf("text rule contains a control character")
		}
		items = append(items, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read text rules: %w", err)
	}
	return items, nil
}

func parseDomainValue(value string) (model.DomainRule, error) {
	value = cleanValue(value)
	if value == "" {
		return model.DomainRule{}, fmt.Errorf("domain value is empty")
	}
	if strings.HasPrefix(value, "+.") || strings.HasPrefix(value, "*.") {
		value = value[2:]
		if err := validatePlainDomainValue(value); err != nil {
			return model.DomainRule{}, err
		}
		return model.DomainRule{Kind: "domain", Value: value}, nil
	}
	if strings.ContainsAny(value, "*?") {
		expression := globToRegexp(value)
		if _, err := regexp.Compile(expression); err != nil {
			return model.DomainRule{}, fmt.Errorf("invalid domain glob %q: %w", value, err)
		}
		return model.DomainRule{Kind: "regexp", Value: expression}, nil
	}
	if err := validatePlainDomainValue(value); err != nil {
		return model.DomainRule{}, err
	}
	return model.DomainRule{Kind: "full", Value: value}, nil
}

func parseClassical(value string, buckets *model.Buckets) error {
	value = strings.TrimSpace(value)
	typeName, remainder, found := strings.Cut(value, ",")
	if !found || strings.TrimSpace(typeName) == "" || strings.TrimSpace(remainder) == "" {
		return fmt.Errorf("classical rule must be TYPE,value: %q", value)
	}
	typeName = strings.TrimSpace(typeName)
	if kind, ok := classicalDomainKinds[typeName]; ok {
		domainValue := strings.TrimSpace(remainder)
		if kind == "regexp" {
			if _, err := regexp.Compile(domainValue); err != nil {
				return fmt.Errorf("invalid domain regexp %q: %w", domainValue, err)
			}
		} else if err := validatePlainDomainValue(domainValue); err != nil {
			return err
		}
		buckets.Domains = append(buckets.Domains, model.DomainRule{Kind: kind, Value: domainValue})
		return nil
	}
	if typeName == "IP-CIDR" || typeName == "IP-CIDR6" {
		parts := strings.Split(remainder, ",")
		if len(parts) > 2 || len(parts) == 2 && strings.TrimSpace(parts[1]) != "no-resolve" {
			return fmt.Errorf("%s only accepts an optional no-resolve field", typeName)
		}
		prefix, err := parsePrefix(parts[0])
		if err != nil {
			return err
		}
		if typeName == "IP-CIDR" && !prefix.Addr().Is4() || typeName == "IP-CIDR6" && !prefix.Addr().Is6() {
			return fmt.Errorf("%s has the wrong address family: %s", typeName, prefix)
		}
		buckets.CIDRs = append(buckets.CIDRs, prefix)
		return nil
	}
	buckets.Skipped = append(buckets.Skipped, value)
	return nil
}

func parsePrefix(value string) (netip.Prefix, error) {
	value = cleanValue(value)
	prefix, err := netip.ParsePrefix(value)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("invalid CIDR %q: %w", value, err)
	}
	return prefix.Masked(), nil
}

func validatePlainDomainValue(value string) error {
	if value == "" || strings.ContainsAny(value, ", \t\r\n") || hasControl(value) {
		return fmt.Errorf("invalid domain value %q", value)
	}
	return nil
}

func cleanValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && (value[0] == '\'' && value[len(value)-1] == '\'' || value[0] == '"' && value[len(value)-1] == '"') {
		value = value[1 : len(value)-1]
	}
	return strings.TrimSpace(value)
}

func globToRegexp(glob string) string {
	var builder strings.Builder
	builder.WriteByte('^')
	for _, r := range glob {
		switch r {
		case '*':
			builder.WriteString(`[^.]*`)
		case '?':
			builder.WriteString(`[^.]`)
		default:
			builder.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	builder.WriteByte('$')
	return builder.String()
}

func hasControl(value string) bool {
	for _, r := range value {
		if r == '\u2028' || r == '\u2029' || unicode.IsControl(r) {
			return true
		}
	}
	return false
}
