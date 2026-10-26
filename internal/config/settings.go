package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"
)

// Settings is a harness definition's native settings as one fragment writes them: a map in the
// harness's own configuration schema, which agx delivers per launch without validating it.
type Settings struct{ node *yaml.Node }

func (s *Settings) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return errors.New("settings must be a mapping in the harness's own configuration schema")
	}
	s.node = node
	return nil
}

// SettingSource locates where a definition sets one setting.
type SettingSource struct {
	// Path holds the keys leading to the setting.
	Path   []string `json:"path"`
	Source Source   `json:"source"`
}

func settingKey(path []string) string {
	b, _ := json.Marshal(path)
	return string(b)
}

// settings converts one fragment's settings to plain values, reading !file values relative to
// the fragment, and records where each leaf is set.
func (m *merger) settings(s Settings, name string) (map[string]any, map[string]Source, error) {
	sources := map[string]Source{}
	var convert func(node *yaml.Node, path []string) (any, error)
	convert = func(node *yaml.Node, path []string) (any, error) {
		at := Source{Path: m.path, Line: node.Line, Column: node.Column}
		fail := func(format string, args ...any) error {
			return fmt.Errorf(
				"%s: harness %s: settings %s: %s",
				locate(at),
				name,
				strings.Join(path, "."),
				fmt.Sprintf(format, args...),
			)
		}
		if node.Kind == yaml.AliasNode {
			node = node.Alias
		}
		switch node.Kind {
		case yaml.MappingNode:
			out := map[string]any{}
			for i := 0; i+1 < len(node.Content); i += 2 {
				key := node.Content[i]
				if key.Kind != yaml.ScalarNode || key.ShortTag() != "!!str" && key.ShortTag() != "!!int" {
					return nil, fail("keys must be strings")
				}
				if _, ok := out[key.Value]; ok {
					return nil, fail("key %q appears twice", key.Value)
				}
				child := append(slices.Clone(path), key.Value)
				value, err := convert(node.Content[i+1], child)
				if err != nil {
					return nil, err
				}
				if nested, ok := value.(map[string]any); !ok || len(nested) == 0 {
					sources[settingKey(child)] = Source{Path: m.path, Line: key.Line, Column: key.Column}
				}
				out[key.Value] = value
			}
			return out, nil
		case yaml.SequenceNode:
			out := []any{}
			for _, item := range node.Content {
				value, err := convert(item, path)
				if err != nil {
					return nil, err
				}
				if value == nil {
					return nil, fail("lists cannot hold null")
				}
				out = append(out, value)
			}
			return out, nil
		case yaml.ScalarNode:
		default:
			return nil, fail("unsupported YAML node")
		}
		if node.Tag == "!file" {
			file, err := m.file(node.Value)
			if err != nil {
				return nil, fail("!file %s: %v", node.Value, err)
			}
			b, err := readLimited(file)
			if err != nil {
				return nil, fail("!file %s: %v", node.Value, err)
			}
			return string(b), nil
		}
		switch node.ShortTag() {
		case "!!str":
			return node.Value, nil
		case "!!null":
			return nil, nil
		case "!!bool":
			var b bool
			if err := node.Load(&b); err != nil {
				return nil, fail("%v", err)
			}
			return b, nil
		case "!!int":
			var n int64
			if err := node.Load(&n); err != nil {
				return nil, fail("%v", err)
			}
			return n, nil
		case "!!float":
			var f float64
			if err := node.Load(&f); err != nil {
				return nil, fail("%v", err)
			}
			if math.IsInf(f, 0) || math.IsNaN(f) {
				return nil, fail("infinite and NaN numbers have no JSON form")
			}
			return f, nil
		}
		return nil, fail("unsupported tag %s", node.Tag)
	}
	value, err := convert(s.node, nil)
	if err != nil {
		return nil, nil, err
	}
	return value.(map[string]any), sources, nil
}

func readLimited(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, maxValueSize+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxValueSize {
		return nil, fmt.Errorf("larger than %d bytes", maxValueSize)
	}
	return b, nil
}

// mergeFragmentSettings adds one fragment's settings to a definition's; a leaf set by two
// fragments is a conflict.
func mergeFragmentSettings(dst, src map[string]any, path []string, conflict func([]string) error) error {
	for _, key := range slices.Sorted(maps.Keys(src)) {
		value := src[key]
		child := append(slices.Clone(path), key)
		existing, ok := dst[key]
		if !ok {
			dst[key] = value
			continue
		}
		existingMap, isMap := existing.(map[string]any)
		valueMap, valueIsMap := value.(map[string]any)
		if !isMap || !valueIsMap {
			return conflict(child)
		}
		if err := mergeFragmentSettings(existingMap, valueMap, child, conflict); err != nil {
			return err
		}
	}
	return nil
}

// layerSettings applies a derived definition's settings over its base's: maps merge key by key,
// other values replace, and null removes the inherited key.
func layerSettings(base, derived map[string]any) map[string]any {
	out := maps.Clone(base)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range derived {
		switch value := value.(type) {
		case nil:
			delete(out, key)
		case map[string]any:
			inherited, _ := out[key].(map[string]any)
			out[key] = layerSettings(inherited, value)
		default:
			out[key] = value
		}
	}
	return out
}

// withoutNulls drops the null leaves a definition kept to remove inherited keys.
func withoutNulls(m map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range m {
		switch value := value.(type) {
		case nil:
		case map[string]any:
			out[key] = withoutNulls(value)
		default:
			out[key] = value
		}
	}
	return out
}

// SettingLeaves lists the resolved settings' leaves, sorted by path: values that are not maps,
// and empty maps.
func SettingLeaves(settings map[string]any) []SettingLeaf {
	leaves := []SettingLeaf{}
	var walk func(m map[string]any, path []string)
	walk = func(m map[string]any, path []string) {
		for _, key := range slices.Sorted(maps.Keys(m)) {
			child := append(slices.Clone(path), key)
			if nested, ok := m[key].(map[string]any); ok && len(nested) > 0 {
				walk(nested, child)
				continue
			}
			leaves = append(leaves, SettingLeaf{Path: child, Value: m[key]})
		}
	}
	walk(settings, nil)
	return leaves
}

// SettingLeaf is one resolved setting.
type SettingLeaf struct {
	Path  []string
	Value any
}

// FormatSettingPath writes a path as dotted keys, quoting keys that hold dots or spaces.
func FormatSettingPath(path []string) string {
	parts := []string{}
	for _, key := range path {
		if key == "" || strings.ContainsAny(key, ". \"") {
			key = strconv.Quote(key)
		}
		parts = append(parts, key)
	}
	return strings.Join(parts, ".")
}
