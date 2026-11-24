package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ValidateSettings rejects --settings in a definition's arguments: the definition's settings
// already take Claude Code's single --settings.
func (Adapter) ValidateSettings(_ map[string]any, args []string) error {
	for _, arg := range args {
		if arg == "--settings" || strings.HasPrefix(arg, "--settings=") {
			return errors.New("settings and --settings in args both set Claude Code's --settings; keep one")
		}
	}
	return nil
}

// SettingsArguments writes settings to a file of the launch and passes it as --settings, which
// Claude Code applies above the user's and the project's settings for that session only. A file
// keeps the command line short and out of ps.
func (Adapter) SettingsArguments(settings map[string]any, dir string) ([]string, error) {
	if dir == "" {
		return nil, nil
	}
	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		return nil, err
	}
	return []string{"--settings", path}, nil
}

// MergeNativeSettings takes every --settings out of native and layers it over settings, in
// order: Claude Code keeps only the last --settings, so an Agent SDK host's own would replace
// the definition's. Each is a JSON object or a path to a file holding one, as Claude Code
// accepts.
func (Adapter) MergeNativeSettings(
	settings map[string]any,
	native []string,
	directory string,
) (map[string]any, []string, error) {
	rest := make([]string, 0, len(native))
	for i := 0; i < len(native); i++ {
		arg := native[i]
		if arg == "--" {
			rest = append(rest, native[i:]...)
			break
		}
		value, ok := strings.CutPrefix(arg, "--settings=")
		if arg == "--settings" && i+1 < len(native) {
			i++
			value, ok = native[i], true
		}
		if !ok {
			rest = append(rest, arg)
			continue
		}
		layer, err := readSettings(value, directory)
		if err != nil {
			return nil, nil, fmt.Errorf("--settings %s: %w", value, err)
		}
		settings = mergeSettings(settings, layer).(map[string]any)
	}
	return settings, rest, nil
}

func readSettings(value, directory string) (map[string]any, error) {
	b := []byte(value)
	if !strings.HasPrefix(strings.TrimSpace(value), "{") {
		path := value
		if !filepath.IsAbs(path) {
			path = filepath.Join(directory, path)
		}
		var err error
		if b, err = os.ReadFile(path); err != nil {
			return nil, err
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	// Numbers stay as written rather than round through float64.
	decoder.UseNumber()
	var settings map[string]any
	if err := decoder.Decode(&settings); err != nil {
		return nil, err
	}
	if settings == nil {
		return nil, errors.New("not a JSON object")
	}
	return settings, nil
}

// mergeSettings layers over onto base the way Claude Code merges its settings sources: objects
// key by key, lists concatenated without duplicates, anything else replaced. Neither argument
// is modified.
func mergeSettings(base, over any) any {
	switch over := over.(type) {
	case map[string]any:
		baseMap, ok := base.(map[string]any)
		if !ok {
			return over
		}
		merged := make(map[string]any, len(baseMap)+len(over))
		for key, value := range baseMap {
			merged[key] = value
		}
		for key, value := range over {
			if existing, ok := merged[key]; ok {
				merged[key] = mergeSettings(existing, value)
			} else {
				merged[key] = value
			}
		}
		return merged
	case []any:
		baseList, ok := base.([]any)
		if !ok {
			return over
		}
		merged := slices.Clone(baseList)
		for _, value := range over {
			if !slices.ContainsFunc(merged, func(v any) bool { return sameSetting(v, value) }) {
				merged = append(merged, value)
			}
		}
		return merged
	default:
		return over
	}
}

// sameSetting compares values from YAML and from JSON, which decode numbers to different types.
func sameSetting(a, b any) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(ja, jb)
}
