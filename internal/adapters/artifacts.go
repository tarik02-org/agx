package adapters

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/pelletier/go-toml/v2"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/securefs"
)

// TOMLAssignment renders key=value for a TOML command-line override: value is a string or a
// map of string fields, rendered as an inline table.
func TOMLAssignment(key string, value any) (string, error) {
	// Marshal a one-field document to get correct TOML escaping and inline JSON-like tables.
	if str, ok := value.(string); ok {
		b, err := toml.Marshal(map[string]string{"v": str})
		if err != nil {
			return "", err
		}
		return key + "=" + strings.TrimSpace(strings.TrimPrefix(string(b), "v =")), nil
	}
	fields := value.(map[string]any)
	keys := []string{}
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, k := range keys {
		parts = append(parts, strconv.Quote(k)+" = "+strconv.Quote(fields[k].(string)))
	}
	return key + "={" + strings.Join(parts, ", ") + "}", nil
}

// ArtifactName is the directory name of a resource inside the shared artifacts.
func ArtifactName(id config.ID) string { return "resource-" + config.Digest(string(id))[:16] }

func artifacts(s *config.Snapshot, cache string) (string, error) {
	parent := filepath.Join(cache, "artifacts")
	if err := securefs.EnsureDir(parent); err != nil {
		return "", err
	}
	dest := filepath.Join(parent, "v1-"+s.ID)
	if _, err := os.Stat(dest); err == nil {
		return dest, nil
	}
	tmp, err := os.MkdirTemp(parent, ".build-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	plugin := filepath.Join(tmp, "plugin")
	if err := os.MkdirAll(plugin, 0o700); err != nil {
		return "", err
	}
	if err := securefs.WriteJSON(
		filepath.Join(plugin, ".claude-plugin", "plugin.json"),
		map[string]string{"name": "agx-managed", "version": "1.0.0"},
	); err != nil {
		return "", err
	}
	var text strings.Builder
	for _, r := range s.Resources {
		switch r.Kind {
		case "skill":
			if err := s.Config.Skills[r.ID].Materialize(
				filepath.Join(plugin, "skills", ArtifactName(r.ID)),
			); err != nil {
				return "", err
			}
		case "instructions":
			b, err := s.Config.Instructions[r.ID].Read()
			if err != nil {
				return "", err
			}
			text.Write(b)
			text.WriteString("\n\n")
		}
	}
	if err := securefs.Write(filepath.Join(tmp, "instructions.md"), []byte(text.String()), 0o600); err != nil {
		return "", err
	}
	// Detect edits during compilation instead of caching content under the wrong snapshot ID.
	check, err := s.Config.Resolve(s.Directory, s.Harness)
	if err != nil {
		return "", err
	}
	if check.ID != s.ID {
		return "", fmt.Errorf("resources changed while preparing launch; retry")
	}
	if err := os.Rename(tmp, dest); err != nil {
		if _, statErr := os.Stat(dest); statErr != nil {
			return "", err
		}
	}
	return dest, nil
}

// maxServerName keeps harness tool names such as mcp__<server>__<tool> within model tool-name limits.
const maxServerName = 32

// ServerNames gives each MCP server a readable native server name. The prefix keeps them apart from
// servers configured natively; a hash suffix separates IDs that sanitize to the same name.
func ServerNames(endpoints []Endpoint) map[config.ID]string {
	candidates := map[config.ID]string{}
	count := map[string]int{}
	for _, endpoint := range endpoints {
		var name strings.Builder
		name.WriteString("agx-")
		for _, r := range string(endpoint.ID) {
			if r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
				name.WriteRune(r)
			} else {
				name.WriteRune('-')
			}
		}
		candidates[endpoint.ID] = name.String()
		count[name.String()]++
	}
	names := map[config.ID]string{}
	for id, name := range candidates {
		if count[name] > 1 || len(name) > maxServerName {
			suffix := "-" + config.Digest(string(id))[:8]
			name = name[:min(len(name), maxServerName-len(suffix))] + suffix
		}
		names[id] = name
	}
	return names
}
