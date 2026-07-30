package geoip

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"clash-rules-srs/internal/fileutil"
	"clash-rules-srs/internal/model"
)

type Input struct {
	Tag  string
	Path string
}

type Template struct {
	path     string
	document map[string]any
	baseURI  string
}

var tagPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func WriteInputs(directory string, contributions []model.Contribution) ([]Input, error) {
	byTag := make(map[string][]netip.Prefix)
	for _, contribution := range contributions {
		if contribution.Side != model.GeoIP {
			return nil, fmt.Errorf("contribution %q targets %s, not geoip", contribution.SourceID, contribution.Side)
		}
		if !tagPattern.MatchString(contribution.Tag) || filepath.Base(contribution.Tag) != contribution.Tag {
			return nil, fmt.Errorf("contribution %q has unsafe geoip tag %q", contribution.SourceID, contribution.Tag)
		}
		if len(contribution.Domains) != 0 {
			return nil, fmt.Errorf("geoip contribution %q contains domain rules", contribution.SourceID)
		}
		for _, prefix := range contribution.CIDRs {
			if !prefix.IsValid() {
				return nil, fmt.Errorf("geoip contribution %q contains an invalid prefix", contribution.SourceID)
			}
			byTag[contribution.Tag] = append(byTag[contribution.Tag], prefix.Masked())
		}
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("create geoip input directory %s: %w", directory, err)
	}
	tags := make([]string, 0, len(byTag))
	for tag := range byTag {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	inputs := make([]Input, 0, len(tags))
	for _, tag := range tags {
		prefixes := byTag[tag]
		sort.Slice(prefixes, func(i, j int) bool {
			left, right := prefixes[i], prefixes[j]
			if left.Addr().BitLen() != right.Addr().BitLen() {
				return left.Addr().BitLen() < right.Addr().BitLen()
			}
			if comparison := left.Addr().Compare(right.Addr()); comparison != 0 {
				return comparison < 0
			}
			return left.Bits() < right.Bits()
		})
		var builder strings.Builder
		var previous netip.Prefix
		for _, prefix := range prefixes {
			if previous.IsValid() && prefix == previous {
				continue
			}
			builder.WriteString(prefix.String())
			builder.WriteByte('\n')
			previous = prefix
		}
		path := filepath.Join(directory, tag+".txt")
		if err := fileutil.AtomicWrite(path, []byte(builder.String()), 0o644); err != nil {
			return nil, fmt.Errorf("write geoip input %s: %w", path, err)
		}
		inputs = append(inputs, Input{Tag: tag, Path: path})
	}
	return inputs, nil
}

func LoadTemplate(path string) (*Template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read geoip template %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode geoip template %s: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("geoip template %s contains trailing JSON", path)
		}
		return nil, fmt.Errorf("decode trailing geoip template %s: %w", path, err)
	}
	inputArray, err := requireArray(document, "input")
	if err != nil || len(inputArray) == 0 {
		return nil, fmt.Errorf("geoip template input must be a non-empty list")
	}
	baseEntry, err := requireEntry(inputArray[0], "input[0]", "v2rayGeoIPDat", "add")
	if err != nil {
		return nil, err
	}
	baseArgs, err := requireObject(baseEntry, "args", "input[0]")
	if err != nil {
		return nil, err
	}
	baseURI, ok := baseArgs["uri"].(string)
	if !ok || baseURI == "" {
		return nil, fmt.Errorf("geoip template input[0].args.uri must be a non-empty string")
	}
	outputArray, err := requireArray(document, "output")
	if err != nil || len(outputArray) == 0 {
		return nil, fmt.Errorf("geoip template output must be a non-empty list")
	}
	outputEntry, err := requireEntry(outputArray[0], "output[0]", "v2rayGeoIPDat", "output")
	if err != nil {
		return nil, err
	}
	if _, err := requireObject(outputEntry, "args", "output[0]"); err != nil {
		return nil, err
	}
	return &Template{path: path, document: document, baseURI: baseURI}, nil
}

func (t *Template) BaseURI() string {
	if t == nil {
		return ""
	}
	return t.baseURI
}

func WriteConfig(template *Template, inputs []Input, baseDatPath, publishDir, outputPath string) error {
	if template == nil {
		return fmt.Errorf("geoip template is nil")
	}
	baseInfo, err := os.Stat(baseDatPath)
	if err != nil {
		return fmt.Errorf("open base geoip dat %s: %w", baseDatPath, err)
	}
	if !baseInfo.Mode().IsRegular() {
		return fmt.Errorf("base geoip dat %s is not a regular file", baseDatPath)
	}
	baseAbsolute, err := filepath.Abs(baseDatPath)
	if err != nil {
		return fmt.Errorf("resolve base geoip dat %s: %w", baseDatPath, err)
	}
	publishAbsolute, err := filepath.Abs(publishDir)
	if err != nil {
		return fmt.Errorf("resolve publish directory %s: %w", publishDir, err)
	}

	document := cloneObject(template.document)

	inputArray, err := requireArray(document, "input")
	if err != nil || len(inputArray) == 0 {
		return fmt.Errorf("geoip template input must be a non-empty list")
	}
	baseEntry, err := requireEntry(inputArray[0], "input[0]", "v2rayGeoIPDat", "add")
	if err != nil {
		return err
	}
	baseArgs, err := requireObject(baseEntry, "args", "input[0]")
	if err != nil {
		return err
	}
	baseArgs["uri"] = baseAbsolute

	orderedInputs := append([]Input(nil), inputs...)
	sort.Slice(orderedInputs, func(i, j int) bool { return orderedInputs[i].Tag < orderedInputs[j].Tag })
	for _, input := range orderedInputs {
		if !tagPattern.MatchString(input.Tag) {
			return fmt.Errorf("invalid geoip input tag %q", input.Tag)
		}
		absolute, err := filepath.Abs(input.Path)
		if err != nil {
			return fmt.Errorf("resolve geoip input %s: %w", input.Path, err)
		}
		info, err := os.Stat(absolute)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("geoip input %s is not a regular file", input.Path)
		}
		inputArray = append(inputArray, map[string]any{
			"type":   "text",
			"action": "add",
			"args": map[string]any{
				"name": input.Tag,
				"uri":  absolute,
			},
		})
	}
	document["input"] = inputArray

	outputArray, err := requireArray(document, "output")
	if err != nil || len(outputArray) == 0 {
		return fmt.Errorf("geoip template output must be a non-empty list")
	}
	outputEntry, err := requireEntry(outputArray[0], "output[0]", "v2rayGeoIPDat", "output")
	if err != nil {
		return err
	}
	outputArgs, err := requireObject(outputEntry, "args", "output[0]")
	if err != nil {
		return err
	}
	outputArgs["outputDir"] = publishAbsolute
	outputArgs["outputName"] = "geoip.dat"

	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode geoip config: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := fileutil.AtomicWrite(outputPath, encoded, 0o644); err != nil {
		return fmt.Errorf("write geoip config %s: %w", outputPath, err)
	}
	return nil
}

func cloneObject(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = cloneValue(item)
	}
	return cloned
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneObject(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneValue(item)
		}
		return cloned
	default:
		return value
	}
}

func requireArray(document map[string]any, key string) ([]any, error) {
	value, exists := document[key]
	if !exists {
		return nil, fmt.Errorf("missing %s", key)
	}
	array, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a list", key)
	}
	return array, nil
}

func requireEntry(value any, location, wantType, wantAction string) (map[string]any, error) {
	entry, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("geoip template %s must be an object", location)
	}
	if entry["type"] != wantType || entry["action"] != wantAction {
		return nil, fmt.Errorf("geoip template %s must be type %s action %s", location, wantType, wantAction)
	}
	return entry, nil
}

func requireObject(entry map[string]any, key, location string) (map[string]any, error) {
	value, exists := entry[key]
	if !exists {
		return nil, fmt.Errorf("geoip template %s.%s is required", location, key)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("geoip template %s.%s must be an object", location, key)
	}
	return object, nil
}
