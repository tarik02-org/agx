package configedit_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/yaml"

	"github.com/tarik02-org/agx/internal/adapters/builtin"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/config/edit"
	"github.com/tarik02-org/agx/internal/configedit"
	"github.com/tarik02-org/agx/internal/platform"
)

var builtins = builtin.Registry().Builtins([]string{"agx", "help", "install", "run"})

// workspace is an isolated configuration directory with a skill and instructions on disk.
type workspace struct {
	dir   string
	paths platform.Paths
}

func newWorkspace(t *testing.T, configuration string) *workspace {
	t.Helper()
	dir, err := config.Canonical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(dir, "home"))
	w := &workspace{
		dir:   dir,
		paths: platform.Paths{Config: filepath.Join(dir, "agx.yaml"), Cache: filepath.Join(dir, "cache")},
	}
	for _, name := range []string{"alpha", "beta"} {
		w.write(t, filepath.Join("skills", name, "SKILL.md"), "---\nname: "+name+"\ndescription: "+name+" skill\n---\n")
	}
	w.write(t, "AGENTS.md", "Be brief.\n")
	w.write(t, "agx.yaml", configuration)
	return w
}

func (w *workspace) write(t *testing.T, rel, content string) string {
	t.Helper()
	path := filepath.Join(w.dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func (w *workspace) read(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(w.dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (w *workspace) session(t *testing.T) *configedit.Session {
	t.Helper()
	c, err := config.Load(w.paths.Config, w.paths.Cache, builtins)
	if err != nil {
		t.Fatal(err)
	}
	s, err := configedit.New(w.paths, builtins, c)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func node(t *testing.T, text string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Content[0]
}

func plan(t *testing.T, s *configedit.Session) *configedit.Plan {
	t.Helper()
	p, err := s.Plan()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func after(t *testing.T, p *configedit.Plan, path string) string {
	t.Helper()
	for _, f := range p.Files {
		if f.Path == path {
			return string(f.After)
		}
	}
	t.Fatalf("the plan does not write %s: %+v", path, p.Files)
	return ""
}

func wantCode(t *testing.T, err error, want configedit.Code) {
	t.Helper()
	if code, ok := configedit.CodeOf(err); !ok || code != want {
		t.Fatalf("error = %v, want code %s", err, want)
	}
}

const base = `# The configuration.
skills:
  alpha:
    path: ./skills/alpha # first
instructions:
  agents:
    path: ./AGENTS.md
imports:
  - glob: agx.d/*.yaml
`

func TestDefineCreatesAndReplaces(t *testing.T) {
	w := newWorkspace(t, base)
	fragment := w.write(t, "agx.d/10-mcp.yaml", "mcp:\n  fs:\n    command: [server-fs] # local\n")
	s := w.session(t)
	if err := s.Define(edit.MCP, "docs", node(t, "url: https://example.com/mcp\n"), s.Root); err != nil {
		t.Fatal(err)
	}
	if err := s.Define(edit.Skills, "alpha", node(t, "path: ./skills/beta\n"), ""); err != nil {
		t.Fatal(err)
	}
	// An existing definition is replaced where it is, whatever file the change names.
	if err := s.Define(edit.MCP, "fs", node(t, "command: [server-fs, --root, /]\n"), s.Root); err != nil {
		t.Fatal(err)
	}
	p := plan(t, s)
	root := after(t, p, s.Root)
	want := `# The configuration.
skills:
  alpha:
    path: ./skills/beta
instructions:
  agents:
    path: ./AGENTS.md
imports:
  - glob: agx.d/*.yaml
mcp:
  docs:
    url: https://example.com/mcp
`
	if root != want {
		t.Fatalf("root =\n%s\nwant\n%s", root, want)
	}
	if got := after(t, p, fragment); !strings.Contains(got, "--root") || strings.Contains(root, "fs:") {
		t.Fatalf("fragment =\n%s", got)
	}
	if _, ok := p.Config.MCP["docs"]; !ok {
		t.Fatal("the planned configuration lacks the new server")
	}
}

func TestDefineRefusesReadOnlyFiles(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes read-only files")
	}
	w := newWorkspace(t, base)
	fragment := w.write(t, "agx.d/10-mcp.yaml", "mcp:\n  fs:\n    command: [server-fs]\n")
	if err := os.Chmod(fragment, 0o444); err != nil {
		t.Fatal(err)
	}
	s := w.session(t)
	wantCode(t, s.Define(edit.MCP, "fs", node(t, "command: [other]\n"), ""), configedit.ReadOnly)
	if err := s.Define(edit.MCP, "new", node(t, "command: [new]\n"), fragment); err != nil {
		t.Fatal(err)
	}
	_, err := s.Plan()
	wantCode(t, err, configedit.ReadOnly)
	wantCode(t, s.Define("unknown", "x", node(t, "a: b\n"), ""), configedit.InvalidRequest)
}

func TestUndefineRemovesReferences(t *testing.T) {
	w := newWorkspace(t, base+`bundles:
  dev:
    skills: [alpha]
    instructions: [agents]
scopes:
  - add:
      skills: [alpha]
      bundles: [dev]
`)
	s := w.session(t)
	if err := s.Undefine(edit.Skills, "alpha"); err != nil {
		t.Fatal(err)
	}
	got := after(t, plan(t, s), s.Root)
	if strings.Contains(got, "alpha") {
		t.Fatalf("references remain:\n%s", got)
	}
	if !strings.Contains(got, "instructions: [agents]") || !strings.Contains(got, "bundles: [dev]") {
		t.Fatalf("unrelated entries went:\n%s", got)
	}
	wantCode(t, s.Undefine(edit.MCP, "missing"), configedit.NotFound)
}

func TestEnableDisable(t *testing.T) {
	w := newWorkspace(t, base)
	s := w.session(t)
	work := filepath.Join(w.dir, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enable(edit.Skills, "alpha", configedit.NewScope("", nil), s.Root); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enable(
		edit.Instructions,
		"agents",
		configedit.NewScope(work, []string{"codex"}),
		s.Root,
	); err != nil {
		t.Fatal(err)
	}
	got := after(t, plan(t, s), s.Root)
	want := base + `scopes:
  - add:
      skills: [alpha]
  - directory: ` + work + `
    harnesses: [codex]
    add:
      instructions: [agents]
`
	if got != want {
		t.Fatalf("enabled =\n%s\nwant\n%s", got, want)
	}
	if err := os.WriteFile(w.paths.Config, []byte(got), 0o644); err != nil {
		t.Fatal(err)
	}

	// Disabling what a matching rule selects drops the entry, and the rule it empties.
	s = w.session(t)
	if _, err := s.Disable(
		edit.Instructions,
		"agents",
		configedit.NewScope(work, []string{"codex"}),
		s.Root,
	); err != nil {
		t.Fatal(err)
	}
	// Disabling in a directory what an outer rule selects writes a remove entry.
	if _, err := s.Disable(edit.Skills, "alpha", configedit.NewScope(work, nil), s.Root); err != nil {
		t.Fatal(err)
	}
	got = after(t, plan(t, s), s.Root)
	want = base + `scopes:
  - add:
      skills: [alpha]
  - directory: ` + work + `
    remove:
      skills: [alpha]
`
	if got != want {
		t.Fatalf("disabled =\n%s\nwant\n%s", got, want)
	}
}

func TestWriteFile(t *testing.T) {
	// A skill whose directory is in the cache is not local, though a fragment defines it.
	w := newWorkspace(
		t,
		strings.Replace(base, "skills:\n", "skills:\n  cached:\n    path: ./cache/packages/kit/skill\n", 1),
	)
	w.write(t, "cache/packages/kit/skill/SKILL.md", "---\nname: cached\n---\n")
	s := w.session(t)
	skillFile := filepath.Join(w.dir, "skills", "alpha", "SKILL.md")
	if err := s.WriteFile(skillFile, []byte("rewritten\n")); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteFile(filepath.Join(w.dir, "skills", "alpha", "notes", "new.md"), []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteFile(filepath.Join(w.dir, "AGENTS.md"), []byte("Be thorough.\n")); err != nil {
		t.Fatal(err)
	}
	wantCode(t, s.WriteFile(filepath.Join(w.dir, "skills", "beta", "SKILL.md"), nil), configedit.InvalidRequest)
	wantCode(
		t,
		s.WriteFile(filepath.Join(w.dir, "skills", "alpha", "..", "beta", "SKILL.md"), nil),
		configedit.InvalidRequest,
	)
	wantCode(t, s.WriteFile(filepath.Join(w.dir, "skills", "alpha"), nil), configedit.InvalidRequest)
	wantCode(t, s.WriteFile("skills/alpha/SKILL.md", nil), configedit.InvalidRequest)
	wantCode(t, s.WriteFile(w.paths.Config, nil), configedit.InvalidRequest)
	wantCode(
		t,
		s.WriteFile(filepath.Join(w.dir, "cache", "packages", "kit", "skill", "SKILL.md"), nil),
		configedit.ReadOnly,
	)
	// A symlink out of the skill does not reach the file it names.
	link := filepath.Join(w.dir, "skills", "alpha", "link.md")
	if err := os.Symlink(filepath.Join(w.dir, "skills", "beta", "SKILL.md"), link); err == nil {
		wantCode(t, s.WriteFile(link, nil), configedit.InvalidRequest)
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
	}
	p := plan(t, s)
	if len(p.Files) != 3 {
		t.Fatalf("plan = %+v", p.Files)
	}
	if _, err := p.Write(); err != nil {
		t.Fatal(err)
	}
	if got := w.read(t, "skills/alpha/notes/new.md"); got != "new\n" {
		t.Fatalf("new file = %q", got)
	}
	if got := w.read(t, "AGENTS.md"); got != "Be thorough.\n" {
		t.Fatalf("instructions = %q", got)
	}
}

func TestPlanHashesAndApplyConflicts(t *testing.T) {
	w := newWorkspace(t, base)
	s := w.session(t)
	fragment := filepath.Join(w.dir, "agx.d", "20-new.yaml")
	if err := s.Define(edit.MCP, "docs", node(t, "url: https://example.com/mcp\n"), fragment); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enable(edit.MCP, "docs", configedit.NewScope("", nil), s.Root); err != nil {
		t.Fatal(err)
	}
	p := plan(t, s)
	sum := sha256.Sum256([]byte(base))
	expected := map[string]string{}
	for _, f := range p.Files {
		expected[f.Path] = f.BeforeSHA256()
		switch f.Path {
		case s.Root:
			if f.BeforeSHA256() != hex.EncodeToString(sum[:]) || !strings.Contains(f.Diff(), "+      mcp: [docs]") {
				t.Fatalf("root: %s\n%s", f.BeforeSHA256(), f.Diff())
			}
		case fragment:
			if f.BeforeSHA256() != "" || !strings.HasPrefix(f.Diff(), "--- /dev/null") {
				t.Fatalf("new fragment: %q\n%s", f.BeforeSHA256(), f.Diff())
			}
		default:
			t.Fatalf("unexpected file %s", f.Path)
		}
	}
	if err := p.Check(expected); err != nil {
		t.Fatal(err)
	}
	expected[s.Root] = strings.Repeat("0", 64)
	wantCode(t, p.Check(expected), configedit.Conflict)
	delete(expected, s.Root)
	wantCode(t, p.Check(expected), configedit.Conflict)

	// A file changed after planning fails the write before anything is written.
	w.write(t, "agx.yaml", base+"# edited\n")
	_, err := p.Write()
	wantCode(t, err, configedit.Conflict)
	if _, err := os.Stat(fragment); !os.IsNotExist(err) {
		t.Fatalf("the new fragment was written: %v", err)
	}
}

func TestInvalidChangeWritesNothing(t *testing.T) {
	w := newWorkspace(t, base)
	s := w.session(t)
	if _, err := s.Enable(edit.Skills, "missing", configedit.NewScope("", nil), s.Root); err != nil {
		t.Fatal(err)
	}
	_, err := s.Plan()
	wantCode(t, err, configedit.ConfigInvalid)
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("the error does not name the load problem: %v", err)
	}
	if got := w.read(t, "agx.yaml"); got != base {
		t.Fatalf("configuration changed:\n%s", got)
	}
}

func TestParseSource(t *testing.T) {
	for arg, want := range map[string]configedit.Source{
		"anthropics/skills@v1/document-skills": {
			Git: "https://github.com/anthropics/skills.git", Ref: "v1", Subdir: "document-skills", Name: "anthropics",
		},
		"https://example.com/kit.git": {Git: "https://example.com/kit.git", Name: "kit"},
	} {
		got, err := configedit.ParseSource(arg, configedit.GitHub, false)
		if err != nil || got != want {
			t.Errorf("ParseSource(%q) = %+v, %v; want %+v", arg, got, err, want)
		}
	}
	_, err := configedit.ParseSource("./skills", configedit.GitHub, false)
	wantCode(t, err, configedit.InvalidRequest)
	if got, err := configedit.ParseSource("./skills", configedit.GitHub, true); err != nil || got.Local != "./skills" {
		t.Errorf("a local path = %+v, %v", got, err)
	}
}
