package config_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tarik02-org/agx/internal/config"
)

// resolveFixture is a configuration with nested scopes, a directory-less rule, harness rules, and
// instructions with explicit orders.
func resolveFixture(t *testing.T) (root, work string) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"review", "lint", "deploy"} {
		skill(t, filepath.Join(dir, "skills", name), name)
	}
	work = mkdir(t, filepath.Join(dir, "work"))
	mkdir(t, filepath.Join(work, "sub", "deep"))
	root = fragment(t, dir, `skills:
  review: {path: ./skills/review}
  lint: {path: ./skills/lint}
  deploy: {path: ./skills/deploy}
instructions:
  first: {text: first}
  a: {text: a}
  b: {text: b}
  deeper: {text: deeper}
  h: {text: h}
  opening: {text: opening, order: -5}
  closing: {text: closing, order: 10}
bundles:
  base:
    skills: [review, lint]
scopes:
  - directory: work/sub
    add:
      instructions: [opening, deeper]
    remove:
      bundles: [base]
  - directory: work
    add:
      bundles: [base]
      instructions: [a, b, closing]
  - add:
      instructions: [first]
  - directory: work/sub/deep
    add: {skills: [lint]}
  - directory: work
    harnesses: [codex]
    add: {skills: [deploy]}
harnesses:
  codex:
    add: {instructions: [h]}
  codex-cpa:
    base: codex
    remove: {skills: [deploy]}
`)
	return root, work
}

func TestResolveScopes(t *testing.T) {
	root, work := resolveFixture(t)
	c := load(t, root)
	for _, tt := range []struct {
		name, dir, harness string
		skills             []string
	}{
		{"outside every directory rule", filepath.Dir(root), "", []string{}},
		{"bundle added", work, "", []string{"lint", "review"}},
		{"bundle removed deeper", filepath.Join(work, "sub"), "", []string{}},
		{"leaf added again deeper still", filepath.Join(work, "sub", "deep"), "", []string{"lint"}},
		{"harness-filtered rule applies to its harness", work, "codex", []string{"deploy", "lint", "review"}},
		{"harness-filtered rule skipped for others", work, "claude", []string{"lint", "review"}},
		{"definitions match their base's filter, then apply their own removals", work, "codex-cpa", []string{"lint", "review"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, err := c.Resolve(tt.dir, tt.harness)
			if err != nil {
				t.Fatal(err)
			}
			if got := ids(s.Resources, "skill"); !slices.Equal(got, tt.skills) {
				t.Fatalf("skills = %v, want %v", got, tt.skills)
			}
		})
	}
}

func TestResolveTraces(t *testing.T) {
	root, work := resolveFixture(t)
	s, err := load(t, root).Resolve(filepath.Join(work, "sub"), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, trace := range s.Traces {
		if trace.Resource != (config.Resource{Kind: "skill", ID: "review"}) {
			continue
		}
		if trace.Selected {
			t.Fatal("review is selected after its bundle was removed")
		}
		changes := []string{}
		for _, step := range trace.Steps {
			changes = append(changes, step.Change)
			if !slices.Equal(step.BundleChain, []config.ID{"base"}) {
				t.Errorf("step %+v lacks its bundle chain", step)
			}
		}
		if !slices.Equal(changes, []string{"add", "remove"}) {
			t.Fatalf("changes = %v, want [add remove]", changes)
		}
		return
	}
	t.Fatal("no trace for review")
}

func TestInstructionOrder(t *testing.T) {
	root, work := resolveFixture(t)
	c := load(t, root)
	sub := filepath.Join(work, "sub")
	for _, tt := range []struct {
		harness string
		want    []string
	}{
		// Order sorts first; then everywhere rules, shallower scopes, list order, and harness rules last.
		{"", []string{"opening", "first", "a", "b", "deeper", "closing"}},
		{"codex", []string{"opening", "first", "a", "b", "deeper", "h", "closing"}},
	} {
		s, err := c.Resolve(sub, tt.harness)
		if err != nil {
			t.Fatal(err)
		}
		if got := ids(s.Resources, "instructions"); !slices.Equal(got, tt.want) {
			t.Errorf("harness %q: instructions = %v, want %v", tt.harness, got, tt.want)
		}
	}
}

func TestResolveCanonicalizesSymlinks(t *testing.T) {
	root, work := resolveFixture(t)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(work, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	c := load(t, root)
	direct, err := c.Resolve(work, "")
	if err != nil {
		t.Fatal(err)
	}
	linked, err := c.Resolve(alias, "")
	if err != nil {
		t.Fatal(err)
	}
	if linked.ID != direct.ID || linked.Directory != direct.Directory {
		t.Fatalf("symlinked directory resolved as %s (%s), want %s (%s)",
			linked.Directory, linked.ID, direct.Directory, direct.ID)
	}
}

func TestSnapshotIdentity(t *testing.T) {
	dir := t.TempDir()
	work := mkdir(t, filepath.Join(dir, "work"))
	notes := write(t, filepath.Join(dir, "notes.md"), "one")
	prompt := write(t, filepath.Join(dir, "prompt.txt"), "secret prompt")
	root := fragment(t, dir, `instructions:
  notes: {path: ./notes.md}
scopes:
  - add: {instructions: [notes]}
harnesses:
  codex:
    environment:
      PROMPT: !file ./prompt.txt
`)
	id := func() string {
		t.Helper()
		s, err := load(t, root).Resolve(work, "codex")
		if err != nil {
			t.Fatal(err)
		}
		return s.ID
	}
	first := id()
	write(t, prompt, "another prompt")
	if id() != first {
		t.Error("!file content changed the snapshot ID; it resolves at launch")
	}
	write(t, notes, "two")
	if id() == first {
		t.Error("instructions content did not change the snapshot ID")
	}
	second := id()
	content, err := os.ReadFile(root)
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, "# a comment moves every line\n"+string(content))
	if id() != second {
		t.Error("source line numbers changed the snapshot ID")
	}
}
