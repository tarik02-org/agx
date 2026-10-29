package adapters

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/tarik02-org/agx/internal/config"
)

// SkillNames names the snapshot's skills inside the shared artifacts by their native names, which
// harnesses show; skills sharing a native name each get a suffix from their ID.
func SkillNames(s *config.Snapshot) map[config.ID]string {
	candidates := map[config.ID]string{}
	count := map[string]int{}
	for _, r := range s.Resources {
		if r.Kind != "skill" {
			continue
		}
		name := s.Config.Skills[r.ID].NativeName()
		candidates[r.ID] = name
		count[name]++
	}
	names := map[config.ID]string{}
	for id, name := range candidates {
		if count[name] > 1 {
			name += "-" + config.Digest(string(id))[:8]
		}
		names[id] = name
	}
	return names
}

// layout names the format of artifact directories; v3 keys them by content.
const layout = "v3"

const (
	// unusedArtifacts is how long an artifact directory no launch uses is kept, for launches
	// that select the same content again.
	unusedArtifacts = 7 * 24 * time.Hour
	// abandonedBuild is the age past which a staged artifact directory was left by an interrupted
	// build.
	abandonedBuild = time.Hour
)

// artifactsMu keeps RemoveUnusedArtifacts from removing a directory a launch is about to use.
var artifactsMu sync.RWMutex

// artifacts returns the artifact directory of the launch's skills, unless skills is false, and
// instructions, building it unless an earlier launch did, and records in the launch directory
// that the launch uses it.
func artifacts(l Launch, skills bool) (string, error) {
	s := l.Snapshot
	type skill struct {
		Name, Hash string
		id         config.ID
	}
	selected := []skill{}
	var text strings.Builder
	names := SkillNames(s)
	for _, r := range s.Resources {
		switch r.Kind {
		case "skill":
			if skills {
				selected = append(selected, skill{names[r.ID], s.SkillHashes[r.ID], r.ID})
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
	name := layout + "-" + config.Digest(struct {
		Skills       []skill
		Instructions string
	}{selected, text.String()})
	parent := filepath.Join(l.Cache, "artifacts")
	dest := filepath.Join(parent, name)
	artifactsMu.RLock()
	defer artifactsMu.RUnlock()
	if l.ID != "" {
		if err := os.MkdirAll(l.LaunchDirectory(), 0o700); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(l.LaunchDirectory(), "artifacts"), []byte(name), 0o600); err != nil {
			return "", err
		}
	}
	if _, err := os.Stat(dest); err == nil {
		now := time.Now()
		_ = os.Chtimes(dest, now, now)
		return dest, nil
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(parent, ".build-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	plugin := filepath.Join(tmp, "plugin")
	if err := os.MkdirAll(filepath.Join(plugin, ".claude-plugin"), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(
		filepath.Join(plugin, ".claude-plugin", "plugin.json"),
		[]byte(`{"name": "agx-managed", "version": "1.0.0"}`+"\n"),
		0o600,
	); err != nil {
		return "", err
	}
	for _, skill := range selected {
		hash, err := s.Config.Skills[skill.id].Materialize(filepath.Join(plugin, "skills", skill.Name))
		if err != nil {
			return "", err
		}
		// The directory's name must describe its content.
		if hash != skill.Hash {
			return "", fmt.Errorf("skill %s changed while preparing launch; retry", skill.id)
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, "instructions.md"), []byte(text.String()), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dest); err != nil {
		if _, statErr := os.Stat(dest); statErr != nil {
			return "", err
		}
	}
	return dest, nil
}

// RemoveUnusedArtifacts removes the artifact directories in cache that no launch directory
// names and that no launch used for a while, and builds an interrupted process left. Launch
// directories name the artifacts their launches use, so active holds the launches whose
// directories must name them: while one does not, as for a launch of an earlier agx, every
// artifact directory is kept. Directories in use get their use recorded at now.
func RemoveUnusedArtifacts(cache string, active []string, now time.Time) {
	if !artifactsMu.TryLock() {
		return
	}
	trash := []string{}
	defer func() {
		for _, dir := range trash {
			_ = os.RemoveAll(dir)
		}
	}()
	defer artifactsMu.Unlock()
	used := map[string]bool{}
	launches := filepath.Join(cache, "launches")
	if entries, err := os.ReadDir(launches); err == nil {
		for _, entry := range entries {
			if b, err := os.ReadFile(filepath.Join(launches, entry.Name(), "artifacts")); err == nil {
				used[string(b)] = true
			}
		}
	}
	known := true
	for _, id := range active {
		if _, err := os.Stat(filepath.Join(launches, id, "artifacts")); err != nil {
			known = false
		}
	}
	parent := filepath.Join(cache, "artifacts")
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, entry := range entries {
		dir := filepath.Join(parent, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		age := now.Sub(info.ModTime())
		switch {
		case strings.HasPrefix(entry.Name(), ".build-") || strings.HasPrefix(entry.Name(), ".remove-"):
			if age > abandonedBuild {
				trash = append(trash, dir)
			}
		case used[entry.Name()]:
			_ = os.Chtimes(dir, now, now)
		case known && age > unusedArtifacts &&
			strings.HasPrefix(entry.Name(), layout+"-"):
			// Renaming is quick, so launches wait for no removal.
			removed := filepath.Join(parent, ".remove-"+entry.Name())
			if os.Rename(dir, removed) == nil {
				trash = append(trash, removed)
			}
		}
	}
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
