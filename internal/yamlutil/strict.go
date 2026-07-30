package yamlutil

import (
	"bytes"
	"fmt"
	"io"
	"unicode"

	"go.yaml.in/yaml/v3"
)

type StrictOptions struct {
	AllowNull bool
}

func DecodeStrict(r io.Reader, out any, options StrictOptions) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read YAML: %w", err)
	}

	var document yaml.Node
	nodeDecoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := nodeDecoder.Decode(&document); err != nil {
		return fmt.Errorf("decode YAML syntax: %w", err)
	}
	if len(document.Content) != 1 {
		return fmt.Errorf("YAML must contain one document")
	}
	if err := validateNode(document.Content[0], options); err != nil {
		return err
	}
	var extra yaml.Node
	if err := nodeDecoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("YAML must contain one document")
		}
		return fmt.Errorf("decode trailing YAML: %w", err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode YAML schema: %w", err)
	}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("YAML must contain one document")
		}
		return fmt.Errorf("decode trailing YAML: %w", err)
	}
	return nil
}

func validateNode(node *yaml.Node, options StrictOptions) error {
	if node.Anchor != "" || node.Kind == yaml.AliasNode {
		return fmt.Errorf("YAML aliases are not allowed at line %d", node.Line)
	}
	switch node.Kind {
	case yaml.MappingNode:
		seen := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return fmt.Errorf("YAML mapping key at line %d must be a string", key.Line)
			}
			if HasForbiddenControl(key.Value) {
				return fmt.Errorf("YAML key contains a forbidden control character at line %d", key.Line)
			}
			if key.Value == "<<" {
				return fmt.Errorf("YAML merge keys are not allowed at line %d", key.Line)
			}
			if _, exists := seen[key.Value]; exists {
				return fmt.Errorf("duplicate YAML key %q at line %d", key.Value, key.Line)
			}
			seen[key.Value] = struct{}{}
			if err := validateNode(node.Content[index+1], options); err != nil {
				return err
			}
		}
	case yaml.SequenceNode, yaml.DocumentNode:
		for _, child := range node.Content {
			if err := validateNode(child, options); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if node.Tag != "!!str" && !(options.AllowNull && node.Tag == "!!null") {
			return fmt.Errorf("YAML scalar at line %d must be a string", node.Line)
		}
		if HasForbiddenControl(node.Value) {
			return fmt.Errorf("YAML string contains a forbidden control character at line %d", node.Line)
		}
	}
	return nil
}

func HasForbiddenControl(value string) bool {
	for _, r := range value {
		if r == '\u2028' || r == '\u2029' || unicode.IsControl(r) {
			return true
		}
	}
	return false
}
