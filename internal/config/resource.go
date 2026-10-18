package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/tarik02-org/agx/internal/securefs"
)

var (
	// skillNamePattern follows the native naming rules harnesses apply to skill names.
	skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	revPattern       = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// maxSkillName is the longest skill name native harnesses accept.
const maxSkillName = 64

// SkillFile is one file of a generated skill: inline content, or a file or directory copied
// from disk.
type SkillFile struct {
	Content    string `json:"content,omitempty"`
	Executable bool   `json:"executable,omitempty"`
	// Path is the copied file or directory, absolute once merged.
	Path string `json:"path,omitempty"`
	// Directory marks a key ending in a slash, which copies a whole directory.
	Directory bool `json:"directory,omitempty"`
}

type skillFileFields struct {
	Content    string `yaml:"content"`
	Executable bool   `yaml:"executable"`
}

func (f *SkillFile) UnmarshalYAML(node *yaml.Node) error {
	*f = SkillFile{}
	switch node.Kind {
	case yaml.ScalarNode:
		if node.ShortTag() != "!!str" {
			return errors.New("skill file must be text, a path to copy, or {content, executable}")
		}
		// A one-line string written like a path copies that path; other text is file content.
		if !strings.Contains(node.Value, "\n") && isPath(node.Value) {
			f.Path = node.Value
		} else {
			f.Content = node.Value
		}
		return nil
	case yaml.MappingNode:
		var fields skillFileFields
		if err := node.Load(&fields, yaml.WithKnownFields()); err != nil {
			return err
		}
		f.Content, f.Executable = fields.Content, fields.Executable
		return nil
	default:
		return errors.New("skill file must be text, a path to copy, or {content, executable}")
	}
}

// Generated reports whether the skill is written in configuration rather than read from a directory.
func (s Skill) Generated() bool { return s.Content != "" }

// TreeHash hashes the skill directory as harnesses receive it. Executable bits of generated
// files are part of their definition, so they are hashed too.
func (s Skill) TreeHash() (string, error) {
	if !s.Generated() {
		return TreeHash(s.Path)
	}
	tmp, err := os.MkdirTemp("", "agx-skill-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	dir := filepath.Join(tmp, "skill")
	if err := s.Materialize(dir); err != nil {
		return "", err
	}
	hash, err := TreeHash(dir)
	if err != nil {
		return "", err
	}
	executable := []string{}
	for _, key := range sortedKeys(s.Files) {
		if s.Files[key].Executable {
			executable = append(executable, key)
		}
	}
	if len(executable) == 0 {
		return hash, nil
	}
	return Digest([]any{hash, executable}), nil
}

// Materialize writes the skill directory to dest, which must not exist yet.
func (s Skill) Materialize(dest string) error {
	if !s.Generated() {
		return securefs.CopyTree(s.Path, dest)
	}
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return err
	}
	document, err := s.document()
	if err != nil {
		return err
	}
	if err := writeNew(filepath.Join(dest, "SKILL.md"), []byte(document), 0o600); err != nil {
		return err
	}
	for _, key := range sortedKeys(s.Files) {
		f := s.Files[key]
		target := filepath.Join(dest, filepath.FromSlash(strings.TrimSuffix(key, "/")))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if f.Path == "" {
			mode := os.FileMode(0o600)
			if f.Executable {
				mode = 0o700
			}
			if err := writeNew(target, []byte(f.Content), mode); err != nil {
				return fmt.Errorf("files %s: %w", key, err)
			}
			continue
		}
		info, err := os.Stat(f.Path)
		if err != nil {
			return fmt.Errorf("files %s: %w", key, err)
		}
		switch {
		case info.IsDir() && !f.Directory:
			return fmt.Errorf(
				"files %s: %s is a directory; end both the key and the path with / to copy it",
				key,
				f.Path,
			)
		case !info.IsDir() && f.Directory:
			return fmt.Errorf("files %s: %s is not a directory", key, f.Path)
		}
		if err := securefs.CopyTree(f.Path, target); err != nil {
			return fmt.Errorf("files %s: %w", key, err)
		}
	}
	return nil
}

// document renders SKILL.md: name and description lead the front matter, followed by the
// extra fields in key order.
func (s Skill) document() (string, error) {
	head, err := yaml.Marshal(struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}{s.Name, s.Description})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.Write(head)
	if len(s.Frontmatter) > 0 {
		extra, err := yaml.Marshal(s.Frontmatter)
		if err != nil {
			return "", err
		}
		b.Write(extra)
	}
	b.WriteString("---\n")
	b.WriteString(s.Content)
	if !strings.HasSuffix(s.Content, "\n") {
		b.WriteString("\n")
	}
	return b.String(), nil
}

func writeNew(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// Read returns the instructions text.
func (i Instructions) Read() ([]byte, error) {
	if i.Text != "" {
		return []byte(i.Text), nil
	}
	return os.ReadFile(i.Path)
}

// isPath reports whether a selection entry or file value names a path rather than an ID or text.
// IDs cannot contain ':' or '\', so absolute Windows paths never read as IDs.
func isPath(value string) bool {
	return strings.HasPrefix(value, ".") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "~") ||
		filepath.IsAbs(value)
}

// expandHome expands a leading ~ or ~/ to the home directory of the user running agx.
func expandHome(path string) (string, error) {
	rest, ok := strings.CutPrefix(path, "~")
	if !ok {
		return path, nil
	}
	if rest != "" && rest[0] != '/' && (runtime.GOOS != "windows" || rest[0] != '\\') {
		return "", fmt.Errorf("%s: ~user paths are not supported; use ~/ or an absolute path", path)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return filepath.Join(home, rest), nil
}

// inNixStore reports whether an absolute path lies inside the read-only Nix store. Symlinks
// into the store, such as profiles, can be repointed, so only literal store paths count.
func inNixStore(path string) bool {
	return runtime.GOOS != "windows" && strings.HasPrefix(filepath.ToSlash(path), "/nix/store/")
}

// checkSkill validates the fields that apply to each kind of skill definition.
func checkSkill(id ID, s Skill) error {
	generatedOnly := map[string]bool{
		"name": s.Name != "", "description": s.Description != "", "frontmatter": len(s.Frontmatter) > 0,
		"files": len(s.Files) > 0,
	}
	switch {
	case s.Generated():
		if s.Path != "" || s.Git != "" {
			return fmt.Errorf("skill %s sets content with path or git; a skill has one source", id)
		}
		if s.Description == "" {
			return fmt.Errorf("skill %s with content needs a description", id)
		}
		for _, key := range []string{"name", "description"} {
			if _, ok := s.Frontmatter[key]; ok {
				return fmt.Errorf("skill %s: frontmatter cannot set %s; use the %s field", id, key, key)
			}
		}
		if !skillNamePattern.MatchString(s.Name) || len(s.Name) > maxSkillName {
			return fmt.Errorf(
				"skill %s: name %q must be lowercase letters, digits, and single hyphens, at most %d characters; set name",
				id,
				s.Name,
				maxSkillName,
			)
		}
		return checkSkillFiles(id, s.Files)
	case s.Path == "" && s.Git == "":
		return fmt.Errorf("skill %s needs path, git, or content", id)
	}
	for _, field := range []string{"name", "description", "frontmatter", "files"} {
		if generatedOnly[field] {
			return fmt.Errorf(
				"skill %s: %s applies only to skills with content; path and Git skills take it from their SKILL.md",
				id, field,
			)
		}
	}
	return nil
}

func checkSkillFiles(id ID, files map[string]SkillFile) error {
	keys := sortedKeys(files)
	for _, key := range keys {
		f := files[key]
		name := strings.TrimSuffix(key, "/")
		directory := name != key
		parts := strings.Split(name, "/")
		if name == "" || strings.Contains(key, `\`) || !filepath.IsLocal(filepath.FromSlash(name)) ||
			slices.ContainsFunc(parts, func(part string) bool { return part == "" || part == "." || part == ".." }) {
			return fmt.Errorf(
				"skill %s: files key %q must be a relative path inside the skill, separated by /",
				id,
				key,
			)
		}
		if slices.Contains(parts, ".git") {
			return fmt.Errorf("skill %s: files key %q cannot contain .git", id, key)
		}
		if strings.EqualFold(name, "SKILL.md") {
			return fmt.Errorf("skill %s: files cannot name SKILL.md; it is generated from content", id)
		}
		if directory != f.Directory {
			if f.Path == "" {
				return fmt.Errorf("skill %s: files %s: a key ending in / copies a directory and needs a path", id, key)
			}
			return fmt.Errorf("skill %s: files %s: end both the key and the path with / to copy a directory", id, key)
		}
		if f.Path != "" && f.Executable {
			return fmt.Errorf("skill %s: files %s: copied files keep their own executable bit", id, key)
		}
		for _, other := range keys {
			if other != key && strings.HasPrefix(strings.TrimSuffix(other, "/")+"/", name+"/") {
				return fmt.Errorf("skill %s: files %s and %s overlap", id, key, other)
			}
		}
	}
	return nil
}

// checkInstructions validates that instructions have exactly one source.
func checkInstructions(id ID, i Instructions) error {
	switch {
	case i.Text != "" && (i.Path != "" || i.Git != ""):
		return fmt.Errorf("instructions %s sets text with path or git; instructions have one source", id)
	case i.Text == "" && i.Path == "":
		if i.Git != "" {
			return fmt.Errorf("instructions %s from git needs the path of a file in the checkout", id)
		}
		return fmt.Errorf("instructions %s needs path, git with path, or text", id)
	}
	return nil
}

// checkGitSource validates the pinning fields of a Git source.
func checkGitSource(rev, ref, hash string, git bool) error {
	if rev != "" {
		if !git {
			return errors.New("rev applies only to git sources; pin a local source with hash")
		}
		if ref != "" {
			return errors.New("set ref or rev, not both")
		}
		if !revPattern.MatchString(rev) {
			return errors.New("rev must be a full 40-character lowercase commit hash; use ref for branches and tags")
		}
	}
	if ref != "" && !git {
		return errors.New("ref applies only to git sources")
	}
	if hash != "" && !ValidHash(hash) {
		return errors.New("hash must be a 64-character content hash, as recorded in lockfiles")
	}
	return nil
}
