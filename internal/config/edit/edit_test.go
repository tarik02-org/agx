package edit_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/config/edit"
)

const original = `# yaml-language-server: $schema=https://example/agx.schema.json
# Personal agx configuration.
imports:
  - agx.d/*.yaml   # machine drop-ins

secrets:
  gh: {kind: keychain}   # stored with agx secret set

# config.KindMCPServers servers, alphabetical
mcp:
  github:
    url: https://api.githubcopilot.com/mcp
    auth: !secret gh
    headers:
      'X-Trace': "on"

scopes:
  # Work projects
  - directory: ~/work
    add:
      mcp: [github]   # work account
      skills:
        - ./skills/review
`

func open(t *testing.T, content string) *edit.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agx.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := edit.Open(path, "")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func render(t *testing.T, f *edit.File) string {
	t.Helper()
	f.Prune()
	b, err := f.Render()
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func same(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("rendered text differs\n--- got\n%s\n--- want\n%s", got, want)
	}
}

// rule returns the scope rule whose directory is written as directory, or the directory-less one.
func rule(t *testing.T, f *edit.File, directory string) edit.Rule {
	t.Helper()
	for _, r := range f.Rules() {
		d := edit.Get(r.Node, "directory")
		if d == nil && directory == "" || d != nil && d.Value == directory {
			return r
		}
	}
	t.Fatalf("no rule for %q", directory)
	return edit.Rule{}
}

func TestUnchangedFileRendersVerbatim(t *testing.T) {
	f := open(t, original)
	same(t, render(t, f), original)
	if changed, err := f.Changed(); err != nil || changed {
		t.Fatalf("Changed() = %v, %v", changed, err)
	}
}

func TestEditsKeepUntouchedText(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	f := open(t, original)

	docs := edit.Mapping("url", "https://docs.example/mcp")
	edit.Set(docs, "headers", edit.Mapping("X-Key", edit.Tagged("!secret", "gh")))
	edit.Set(docs, "auth", edit.Scalar("none"))
	f.Define(config.KindMCPServers, "docs", docs)
	everywhere := f.AddRule("", nil)
	edit.Append(f.List(f.Selection(everywhere, "add", true), config.KindMCPServers, true), "docs")
	same(t, render(t, f), strings.Replace(original, `      'X-Trace': "on"
`, `      'X-Trace': "on"
  docs:
    url: https://docs.example/mcp
    headers:
      X-Key: !secret gh
    auth: none
`, 1)+`  - add:
      mcp: [docs]
`)

	f.Define(config.KindMCPServers, "fs", edit.Mapping(
		"command", edit.Sequence(true, "up", "--stdio"),
		"environment", edit.Mapping("TOKEN", edit.Tagged("!env", "FS_TOKEN"), "MODE", "fast"),
	))
	work := rule(t, f, "~/work")
	edit.Append(f.List(f.Selection(work.Node, "add", true), config.KindMCPServers, true), "fs")
	got := render(t, f)
	for _, want := range []string{
		"    auth: none\n  fs:\n    command: [up, --stdio]\n    environment:\n      TOKEN: !env FS_TOKEN\n      MODE: fast\n\nscopes:\n",
		"      mcp: [github, fs] # work account\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in\n%s", want, got)
		}
	}

	edit.Append(f.List(f.Selection(work.Node, "remove", true), config.KindMCPServers, true), "docs")
	if want := "        - ./skills/review\n    remove:\n      mcp: [docs]\n  - add:\n"; !strings.Contains(
		render(t, f),
		want,
	) {
		t.Fatalf("remove list not placed in the ~/work rule:\n%s", render(t, f))
	}

	refs := f.References(config.KindMCPServers, "docs")
	wheres := []string{}
	for _, r := range refs {
		wheres = append(wheres, r.Where)
		f.Remove(r)
	}
	if strings.Join(wheres, "; ") != "scope ~/work remove; scope everywhere add" {
		t.Fatalf("references = %q", wheres)
	}
	if !f.Undefine(config.KindMCPServers, "docs") {
		t.Fatal("docs was not defined")
	}
	got = render(t, f)
	if strings.Contains(got, "docs") || strings.Contains(got, "remove:") || strings.Contains(got, "- add:") {
		t.Fatalf("emptied lists and rules were not pruned:\n%s", got)
	}
	head, _, _ := strings.Cut(original, "# config.KindMCPServers servers")
	if !strings.HasPrefix(got, head) {
		t.Fatalf("leading comments and imports changed:\n%s", got)
	}
}

func TestEmptyFlowMappingGrowsInBlockStyle(t *testing.T) {
	f := open(t, "# Team servers\nmcp: {}\n")
	f.Define(config.KindMCPServers, "viafile", edit.Mapping("url", "https://v.example/mcp"))
	same(t, render(t, f), "# Team servers\nmcp:\n  viafile:\n    url: https://v.example/mcp\n")
}

func TestNewFile(t *testing.T) {
	f, err := edit.Open(filepath.Join(t.TempDir(), "new.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	if f.Exists || f.Original() != nil {
		t.Fatal("a missing file reports that it exists")
	}
	f.Define(config.KindInstructions, "pnpm", edit.Mapping("text", "Use pnpm."))
	same(t, render(t, f), "instructions:\n  pnpm:\n    text: Use pnpm.\n")
}

func TestEntryID(t *testing.T) {
	f := open(t, `scopes:
  - add:
      skills:
        - review
        - ./skills/local-thing
        - {git: 'https://example.com/team/skills.git'}
        - {git: 'https://example.com/team/skills.git', path: skills/strict}
        - {id: deploy, description: d, content: c}
      instructions: [./docs/AGENTS.md]
      bundles: [./not-a-path-for-bundles]
`)
	add := edit.Get(f.Rules()[0].Node, "add")
	for _, tt := range []struct {
		kind    config.Kind
		index   int
		id      string
		defines bool
	}{
		{config.KindSkills, 0, "review", false},
		{config.KindSkills, 1, "local-thing", true},
		{config.KindSkills, 2, "team", true},
		{config.KindSkills, 3, "strict", true},
		{config.KindSkills, 4, "deploy", true},
		{config.KindInstructions, 0, "AGENTS", true},
		{config.KindBundles, 0, "./not-a-path-for-bundles", false},
	} {
		entry := edit.Get(add, string(tt.kind)).Content[tt.index]
		if id, defines := edit.EntryID(tt.kind, entry); id != tt.id || defines != tt.defines {
			t.Errorf("%s[%d] = %q, %v; want %q, %v", tt.kind, tt.index, id, defines, tt.id, tt.defines)
		}
	}
}

func TestWritable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agx.yaml")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := edit.Writable(path); err != nil {
		t.Fatal(err)
	}
	if err := edit.Writable(filepath.Join(dir, "missing", "new.yaml")); err != nil {
		t.Fatalf("a file in a creatable directory is not writable: %v", err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := edit.Writable("/nix/store/0000000000000000000000000000000-agx.yaml"); err == nil ||
		!strings.Contains(err.Error(), "read-only Nix store") {
		t.Errorf("Nix store path: %v", err)
	}
	if os.Geteuid() == 0 {
		t.Skip("root can write read-only files")
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := edit.Writable(path); err == nil || !strings.Contains(err.Error(), "read-only (mode 0444)") {
		t.Fatalf("read-only file: %v", err)
	}
}

func TestWriteKeepsMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agx.yaml")
	if err := os.WriteFile(path, []byte("{}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := edit.Write(path, []byte("mcp: {}\n")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "mcp: {}\n" {
		t.Fatalf("content = %q, %v", b, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v, want 0640", info.Mode().Perm())
	}
}
