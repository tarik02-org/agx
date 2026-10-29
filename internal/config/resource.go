package config

import (
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
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
	commitPattern    = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// maxSkillName is the longest skill name native harnesses accept.
const maxSkillName = 64

// SkillFile is one file of a generated skill: inline content, or a file or directory copied
// from disk.
type SkillFile struct {
	Content    string `yaml:"content"    json:"content,omitempty"`
	Executable bool   `yaml:"executable" json:"executable,omitempty"`
	// Path is the copied file or directory, absolute once merged.
	Path string `yaml:"path" json:"path,omitempty"`
	// Directory marks a key ending in a slash, which copies a whole directory.
	Directory bool `yaml:"-" json:"directory,omitempty"`
}

// Generated reports whether the skill is written in configuration rather than read from a directory.
func (s Skill) Generated() bool { return s.Content != "" }

// ContentHash hashes the skill directory as harnesses receive it, or for a skill inside an
// installed package, its place in the package's content. Executable bits of generated files are
// part of their definition, so they are hashed too.
func (s Skill) ContentHash() (string, error) {
	if s.installed != "" {
		return s.installed, nil
	}
	return s.hashTree(nil)
}

// hashTree hashes the skill directory. Unless create is nil, it also writes the directory as it
// reads it: create makes each directory, returning nil, and opens each file for its content.
func (s Skill) hashTree(create func(rel string, mode fs.FileMode, dir bool) (io.WriteCloser, error)) (string, error) {
	h := newTreeHash()
	if err := s.tree(func(rel string, size int64, mode fs.FileMode, content io.Reader) error {
		if create == nil {
			return h.add(rel, size, content)
		}
		w, err := create(rel, mode, content == nil)
		if err != nil {
			return err
		}
		if content == nil {
			return h.add(rel, size, nil)
		}
		err = h.add(rel, size, io.TeeReader(content, w))
		if closeErr := w.Close(); err == nil {
			err = closeErr
		}
		return err
	}); err != nil {
		return "", err
	}
	executable := []string{}
	for _, key := range sortedKeys(s.Files) {
		if s.Files[key].Executable {
			executable = append(executable, key)
		}
	}
	if len(executable) == 0 {
		return h.sum(), nil
	}
	return Digest([]any{h.sum(), executable}), nil
}

// NativeName is the name harnesses show for the skill: the configured name of a generated skill,
// otherwise the name in its SKILL.md front matter. A missing or invalid name falls back to the
// directory name made valid, since the name also names the skill's directory.
func (s Skill) NativeName() string {
	if s.Generated() {
		return s.Name
	}
	if b, err := os.ReadFile(filepath.Join(s.Path, "SKILL.md")); err == nil {
		if name := frontmatterName(string(b)); skillNamePattern.MatchString(name) && len(name) <= maxSkillName {
			return name
		}
	}
	var name strings.Builder
	for _, r := range strings.ToLower(filepath.Base(s.Path)) {
		switch {
		case r < 128 && (r >= 'a' && r <= 'z' || r >= '0' && r <= '9'):
			name.WriteRune(r)
		case name.Len() > 0 && !strings.HasSuffix(name.String(), "-"):
			name.WriteRune('-')
		}
	}
	result := strings.TrimSuffix(name.String()[:min(name.Len(), maxSkillName)], "-")
	if result == "" {
		return "skill"
	}
	return result
}

// frontmatterName reads name from a leading YAML front matter block, or returns "".
func frontmatterName(document string) string {
	document = strings.TrimPrefix(document, "\ufeff")
	document = strings.ReplaceAll(document, "\r\n", "\n")
	rest, ok := strings.CutPrefix(document, "---\n")
	if !ok {
		return ""
	}
	head, _, ok := strings.Cut(rest, "\n---")
	if !ok {
		return ""
	}
	var fields struct {
		Name string `yaml:"name"`
	}
	if yaml.Unmarshal([]byte(head), &fields) != nil {
		return ""
	}
	return strings.TrimSpace(fields.Name)
}

// Materialize writes the skill directory to dest, which must not exist yet, and returns the
// content hash of what it wrote. A skill inside an installed package reports its ContentHash,
// since installation hashed that content already.
func (s Skill) Materialize(dest string) (string, error) {
	hash, err := s.hashTree(func(rel string, mode fs.FileMode, dir bool) (io.WriteCloser, error) {
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if dir {
			return nil, os.MkdirAll(target, 0o700)
		}
		return os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	})
	if err != nil || s.installed == "" {
		return hash, err
	}
	return s.installed, nil
}

// tree visits the skill directory as Materialize writes it and in the order TreeHash walks a
// directory, without reading file content the visitor does not read: rel is slash-separated,
// mode holds the file's permissions, and content is nil for directories.
func (s Skill) tree(visit func(rel string, size int64, mode fs.FileMode, content io.Reader) error) error {
	walk := func(src, prefix string) error {
		return securefs.Walk(src, func(rel string, info fs.FileInfo, content io.Reader) error {
			return visit(path.Join(prefix, rel), info.Size(), 0o600|info.Mode().Perm()&0o111, content)
		})
	}
	if !s.Generated() {
		return walk(s.Path, ".")
	}
	document, err := s.document()
	if err != nil {
		return err
	}
	// keys maps each path to the files key it holds; SKILL.md and parent directories hold none.
	keys := map[string]string{"SKILL.md": ""}
	for key := range s.Files {
		rel := strings.TrimSuffix(key, "/")
		keys[rel] = key
		for dir := path.Dir(rel); dir != "."; dir = path.Dir(dir) {
			if _, ok := keys[dir]; !ok {
				keys[dir] = ""
			}
		}
	}
	if err := visit(".", 0, 0, nil); err != nil {
		return err
	}
	// Comparing segments orders the paths as a walk of the written directory meets them.
	rels := slices.SortedFunc(maps.Keys(keys), func(a, b string) int {
		return slices.Compare(strings.Split(a, "/"), strings.Split(b, "/"))
	})
	for _, rel := range rels {
		key := keys[rel]
		f := s.Files[key]
		switch {
		case rel == "SKILL.md":
			err = visit(rel, int64(len(document)), 0o600, strings.NewReader(document))
		case key == "":
			err = visit(rel, 0, 0, nil)
		case f.Path == "":
			mode := fs.FileMode(0o600)
			if f.Executable {
				mode = 0o700
			}
			err = visit(rel, int64(len(f.Content)), mode, strings.NewReader(f.Content))
		default:
			var info fs.FileInfo
			if info, err = os.Stat(f.Path); err != nil {
				break
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
			err = walk(f.Path, rel)
		}
		if err != nil && key != "" {
			return fmt.Errorf("files %s: %w", key, err)
		}
		if err != nil {
			return err
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

// Read returns the instructions text.
func (i Instructions) Read() ([]byte, error) {
	if i.Text != "" {
		return []byte(i.Text), nil
	}
	return os.ReadFile(i.Path)
}

// IsPath reports whether a selection entry names a path rather than an ID.
// IDs cannot contain ':' or '\', so absolute Windows paths never read as IDs.
func IsPath(value string) bool {
	return strings.HasPrefix(value, ".") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "~") ||
		filepath.IsAbs(value)
}

// ExpandHome expands a leading ~ or ~/ to the home directory of the user running agx.
func ExpandHome(path string) (string, error) {
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

// InNixStore reports whether an absolute path lies inside the read-only Nix store. Symlinks
// into the store, such as profiles, can be repointed, so only literal store paths count.
func InNixStore(path string) bool {
	return runtime.GOOS != "windows" && strings.HasPrefix(filepath.ToSlash(path), "/nix/store/")
}

// checkSkillFiles checks what the schema cannot: a directory key copies a directory path, and
// no key lies inside another.
func checkSkillFiles(id ID, files map[string]SkillFile) error {
	keys := sortedKeys(files)
	for _, key := range keys {
		f := files[key]
		name := strings.TrimSuffix(key, "/")
		if strings.HasSuffix(key, "/") != f.Directory {
			if f.Path == "" {
				return fmt.Errorf("skill %s: files %s: a key ending in / copies a directory and needs a path", id, key)
			}
			return fmt.Errorf("skill %s: files %s: end both the key and the path with / to copy a directory", id, key)
		}
		for _, other := range keys {
			if other != key && strings.HasPrefix(strings.TrimSuffix(other, "/")+"/", name+"/") {
				return fmt.Errorf("skill %s: files %s and %s overlap", id, key, other)
			}
		}
	}
	return nil
}

// IsCommit reports whether a Git ref is a full commit hash, which pins its source.
func IsCommit(ref string) bool { return commitPattern.MatchString(ref) }
