package geosite

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"clash-rules-srs/internal/fileutil"
	"clash-rules-srs/internal/model"
)

func Merge(communityDir, outputDir string, contributions []model.Contribution) error {
	for _, contribution := range contributions {
		if contribution.Side != model.GeoSite {
			return fmt.Errorf("contribution %q targets %s, not geosite", contribution.SourceID, contribution.Side)
		}
		if filepath.Base(contribution.Tag) != contribution.Tag || contribution.Tag == "." || contribution.Tag == "" {
			return fmt.Errorf("contribution %q has unsafe geosite tag %q", contribution.SourceID, contribution.Tag)
		}
	}
	if err := copyCommunity(communityDir, outputDir); err != nil {
		return err
	}

	ordered := append([]model.Contribution(nil), contributions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Tag != ordered[j].Tag {
			return ordered[i].Tag < ordered[j].Tag
		}
		if ordered[i].SourceID != ordered[j].SourceID {
			return ordered[i].SourceID < ordered[j].SourceID
		}
		return contributionKey(ordered[i]) < contributionKey(ordered[j])
	})

	byTag := make(map[string][]model.DomainRule)
	for _, contribution := range ordered {
		byTag[contribution.Tag] = append(byTag[contribution.Tag], contribution.Domains...)
	}
	tags := make([]string, 0, len(byTag))
	for tag := range byTag {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	for _, tag := range tags {
		if err := mergeTarget(filepath.Join(outputDir, tag), byTag[tag]); err != nil {
			return fmt.Errorf("merge geosite:%s: %w", tag, err)
		}
	}
	return nil
}

func copyCommunity(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("open community directory %s: %w", source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("community path %s is not a directory", source)
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("create merged directory %s: %w", destination, err)
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		target := filepath.Join(destination, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("community symlink is not allowed: %s", path)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("community entry is not a regular file: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
		return nil
	})
}

func mergeTarget(path string, additions []model.DomainRule) error {
	lines, err := readLines(path)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(lines)+len(additions))
	merged := make([]string, 0, len(lines)+len(additions))
	for _, line := range lines {
		key, recognized := canonicalLine(line)
		if recognized {
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
		}
		merged = append(merged, line)
	}
	for _, rule := range additions {
		line, key, err := encodeRule(rule)
		if err != nil {
			return err
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, line)
	}
	var content string
	if len(merged) != 0 {
		content = strings.Join(merged, "\n") + "\n"
	}
	return fileutil.AtomicWrite(path, []byte(content), 0o644)
}

func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func canonicalLine(line string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return "", false
	}
	kind, value, found := strings.Cut(fields[0], ":")
	if !found || value == "" {
		return "", false
	}
	domainKind := model.DomainKind(kind)
	if !domainKind.Valid() {
		return "", false
	}
	return canonicalKey(domainKind, value, fields[1:]), true
}

func encodeRule(rule model.DomainRule) (string, string, error) {
	if !rule.Kind.Valid() {
		return "", "", fmt.Errorf("unsupported domain rule kind %q", rule.Kind)
	}
	if rule.Value == "" || hasUnsafeText(rule.Value) || strings.ContainsAny(rule.Value, " \t") {
		return "", "", fmt.Errorf("unsafe %s rule value %q", rule.Kind, rule.Value)
	}
	for _, attribute := range rule.Attrs {
		if attribute == "" || hasUnsafeText(attribute) || strings.ContainsAny(attribute, " \t") {
			return "", "", fmt.Errorf("unsafe %s rule attribute %q", rule.Kind, attribute)
		}
	}
	line := string(rule.Kind) + ":" + rule.Value
	if len(rule.Attrs) != 0 {
		line += " " + strings.Join(rule.Attrs, " ")
	}
	return line, canonicalKey(rule.Kind, rule.Value, rule.Attrs), nil
}

func canonicalKey(kind model.DomainKind, value string, attributes []string) string {
	return string(kind) + "\x00" + value + "\x00" + strings.Join(attributes, "\x00")
}

func contributionKey(contribution model.Contribution) string {
	parts := make([]string, 0, len(contribution.Domains))
	for _, rule := range contribution.Domains {
		parts = append(parts, canonicalKey(rule.Kind, rule.Value, rule.Attrs))
	}
	return strings.Join(parts, "\x01")
}

func hasUnsafeText(value string) bool {
	for _, r := range value {
		if r == '\u2028' || r == '\u2029' || unicode.IsControl(r) {
			return true
		}
	}
	return false
}
