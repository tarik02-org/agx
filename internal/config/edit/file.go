// Package edit changes configuration fragments through their YAML node tree. Rendering keeps
// the original text of every line the change does not touch, so comments, blank lines, key
// order, quoting, flow style, and tags survive.
package edit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aymanbagabas/go-udiff"
	"github.com/aymanbagabas/go-udiff/lcs"
	"go.yaml.in/yaml/v4"

	"github.com/tarik02-org/agx/internal/config"
)

// File is one configuration fragment being edited.
type File struct {
	// Path is canonical, as configuration loading names files.
	Path string
	// Exists is false for a file the edit creates.
	Exists   bool
	original string
	// baseline is the tree as parsed, encoded with options; changes are found against it.
	baseline string
	options  []yaml.Option
	doc      *yaml.Node
	// touched holds rules and harness selections an edit changed, which Prune may drop when empty.
	touched map[*yaml.Node]bool
}

// Open reads a fragment; a missing file starts from seed, such as the default root
// configuration, or from an empty mapping.
func Open(path string, seed string) (*File, error) {
	path, err := config.Canonical(path)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return parse(path, seed, false)
	}
	if err != nil {
		return nil, err
	}
	return parse(path, string(b), true)
}

func parse(path, text string, exists bool) (*File, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{mapping()}}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: expected a mapping at the top level", path)
	}
	f := &File{Path: path, Exists: exists, original: text, doc: &doc, touched: map[*yaml.Node]bool{}}
	f.options = detectStyle(text, &doc)
	var err error
	if f.baseline, err = encode(&doc, f.options); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// Original is the file's text before the edit; empty for a new file.
func (f *File) Original() []byte {
	if !f.Exists {
		return nil
	}
	return []byte(f.original)
}

// Render returns the edited text. Lines outside the change keep their original text; when the
// change cannot be placed in the original text exactly, the whole tree is re-encoded.
func (f *File) Render() ([]byte, error) {
	after, err := encode(f.doc, f.options)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", f.Path, err)
	}
	if !f.Exists {
		return []byte(after), nil
	}
	if after == f.baseline {
		return []byte(f.original), nil
	}
	if spliced, ok := splice(f.original, f.baseline, after); ok && f.encodesTo(spliced, after) {
		return []byte(spliced), nil
	}
	return []byte(after), nil
}

// encodesTo checks that text holds the same tree as the encoding want.
func (f *File) encodesTo(text, want string) bool {
	var doc yaml.Node
	if yaml.Unmarshal([]byte(text), &doc) != nil {
		return false
	}
	got, err := encode(&doc, f.options)
	return err == nil && got == want
}

// Changed reports whether rendering would change the file.
func (f *File) Changed() (bool, error) {
	after, err := f.Render()
	if err != nil {
		return false, err
	}
	return !f.Exists || string(after) != f.original, nil
}

func encode(doc *yaml.Node, options []yaml.Option) (string, error) {
	b, err := yaml.Dump(doc, options...)
	return string(b), err
}

// detectStyle picks the indentation the file is written with, so encoded lines match the
// original ones and new lines fit beside them.
func detectStyle(text string, doc *yaml.Node) []yaml.Option {
	present := map[string]int{}
	for _, line := range strings.Split(text, "\n") {
		present[strings.TrimRight(line, " \t")]++
	}
	var best []yaml.Option
	bestScore := -1
	for _, indent := range []int{2, 4, 3} {
		for _, compact := range []bool{false, true} {
			options := []yaml.Option{
				yaml.WithV4Defaults(),
				yaml.WithIndent(indent),
				yaml.WithCompactSeqIndent(compact),
				yaml.WithLineWidth(-1),
			}
			encoded, err := encode(doc, options)
			if err != nil {
				continue
			}
			score := 0
			for _, line := range strings.Split(encoded, "\n") {
				if present[line] > 0 {
					score++
				}
			}
			if score > bestScore {
				best, bestScore = options, score
			}
		}
	}
	return best
}

// splice applies the change from baseline to after onto original, which encodes to baseline but
// may differ from it in blank lines and spacing. It reports false when a changed line of baseline
// has no counterpart in original.
func splice(original, baseline, after string) (string, bool) {
	if !strings.HasSuffix(original, "\n") {
		original += "\n"
	}
	a, b, c := lines(original), lines(baseline), lines(after)
	mapping := make([]int, len(b))
	for i := range mapping {
		mapping[i] = -1
	}
	ai, bi := 0, 0
	for _, d := range lcs.DiffLines(normalized(a), normalized(b)) {
		for ai < d.Start && bi < d.ReplStart {
			mapping[bi] = ai
			ai, bi = ai+1, bi+1
		}
		ai, bi = d.End, d.ReplEnd
	}
	for ai < len(a) && bi < len(b) {
		mapping[bi] = ai
		ai, bi = ai+1, bi+1
	}
	out := []string{}
	cursor := 0
	for _, d := range lcs.DiffLines(b, c) {
		start, end := 0, 0
		switch {
		case d.Start < d.End:
			if mapping[d.Start] < 0 || mapping[d.End-1] < 0 {
				return "", false
			}
			start, end = mapping[d.Start], mapping[d.End-1]+1
		case d.Start > 0:
			// Insertions follow the line before them, ahead of any blank lines.
			if mapping[d.Start-1] < 0 {
				return "", false
			}
			start = mapping[d.Start-1] + 1
			end = start
		case len(mapping) > 0 && mapping[0] >= 0:
			start, end = mapping[0], mapping[0]
		}
		if start < cursor {
			return "", false
		}
		out = append(out, a[cursor:start]...)
		inserted := c[d.ReplStart:d.ReplEnd]
		out = append(out, inserted...)
		cursor = end
		// A removal between two blank lines keeps one of them.
		if len(inserted) == 0 && len(out) > 0 && blank(out[len(out)-1]) && cursor < len(a) && blank(a[cursor]) {
			cursor++
		}
	}
	out = append(out, a[cursor:]...)
	for len(out) > 0 && blank(out[len(out)-1]) && !blank(a[len(a)-1]) {
		out = out[:len(out)-1]
	}
	return strings.Join(out, ""), true
}

// lines splits text after each newline; text must end with one.
func lines(text string) []string {
	out := strings.SplitAfter(text, "\n")
	return out[:len(out)-1]
}

func normalized(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = strings.Join(strings.Fields(line), " ")
	}
	return out
}

func blank(line string) bool { return strings.TrimSpace(line) == "" }

// Diff renders a unified diff of a file's change; empty when nothing changed.
func Diff(path string, before, after []byte) string {
	from := path
	if before == nil {
		from = "/dev/null"
	}
	return udiff.Unified(from, path, string(before), string(after))
}

// Writable explains why path cannot be edited in place, or returns nil.
func Writable(path string) error {
	real := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		real = resolved
	}
	if config.InNixStore(real) {
		where := path
		if real != path {
			where += " (" + real + ")"
		}
		return fmt.Errorf(
			"%s is in the read-only Nix store; change the configuration that generates it, or choose another file with --file",
			where,
		)
	}
	info, err := os.Stat(real)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		dir := filepath.Dir(real)
		for {
			if _, err := os.Stat(dir); err == nil || filepath.Dir(dir) == dir {
				break
			}
			dir = filepath.Dir(dir)
		}
		return writableDirectory(path, dir)
	case err != nil:
		return err
	case !info.Mode().IsRegular():
		return fmt.Errorf("%s is not a regular file", path)
	case info.Mode().Perm()&0o200 == 0:
		return fmt.Errorf("%s is read-only (mode %04o); make it writable or choose another file with --file",
			path, info.Mode().Perm())
	}
	file, err := os.OpenFile(real, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("%s is not writable: %w", path, errors.Unwrap(err))
	}
	_ = file.Close()
	return writableDirectory(path, filepath.Dir(real))
}

// writableDirectory checks that a file can be created beside path, as an atomic write needs.
func writableDirectory(path, dir string) error {
	probe, err := os.CreateTemp(dir, ".agx-probe-*")
	if err != nil {
		return fmt.Errorf("cannot write %s: directory %s is not writable", path, dir)
	}
	_ = probe.Close()
	return os.Remove(probe.Name())
}

// Write replaces path atomically, keeping the mode of an existing file; a symlinked file is
// written at its target.
func Write(path string, data []byte) error {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".agx-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	err = f.Chmod(mode)
	if err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// sortedUnique returns values sorted without duplicates.
func sortedUnique(values []string) []string {
	out := slices.Clone(values)
	slices.Sort(out)
	return slices.Compact(out)
}
