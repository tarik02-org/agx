package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/securefs"
)

func TestFragmentsLoadInOrder(t *testing.T) {
	dir := t.TempDir()
	root := write(t, filepath.Join(dir, "agx.yaml"), `fragments:
  - first.yaml
  - agx.d/*.yaml
  - none.d/*.yaml
  - first.yaml
  - '*.yaml'
`)
	first := write(t, filepath.Join(dir, "first.yaml"), "instructions:\n  first: {text: first}\n")
	b := write(t, filepath.Join(dir, "agx.d", "20-b.yaml"), "instructions:\n  b: {text: b}\n")
	a := write(t, filepath.Join(dir, "agx.d", "10-a.yaml"), "instructions:\n  a: {text: a}\n")
	write(t, filepath.Join(dir, "agx.d", "notes.txt"), "not a fragment")
	mkdir(t, filepath.Join(dir, "agx.d", "30-dir.yaml"))

	// A fragment two entries match loads once, and a glob matching the root skips it.
	c := read(t, root)
	want := []string{canonical(t, first), canonical(t, a), canonical(t, b), canonical(t, root)}
	if !slices.Equal(c.Files, want) {
		t.Fatalf("files = %q, want %q", c.Files, want)
	}
	for path, listed := range map[string]bool{
		first:                                   true,
		filepath.Join(dir, "agx.d", "new.yaml"): true,
		filepath.Join(dir, "none.d", "x.yaml"):  true,
		filepath.Join(dir, "other", "x.yaml"):   false,
	} {
		if got := c.Lists(canonical(t, path)); got != listed {
			t.Errorf("Lists(%s) = %v, want %v", path, got, listed)
		}
	}
}

func TestMissingRootListsDropIns(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "agx.d", "10.yaml"), "instructions:\n  dropin: {text: hi}\n")
	c := read(t, filepath.Join(dir, "agx.yaml"))
	if _, ok := c.Instructions["dropin"]; !ok {
		t.Fatalf("drop-in fragment not loaded: %v", c.Files)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name:  "missing fragment",
			files: map[string]string{"agx.yaml": "fragments: [nope.yaml]\n"},
			want:  []string{"nope.yaml"},
		},
		{
			name: "error inside an listed glob match",
			files: map[string]string{
				"agx.yaml":           "fragments: [local.d/*.yaml]\n",
				"local.d/local.yaml": "bogus: true\n",
			},
			want: []string{"local.yaml", "local.yaml:1: unknown key bogus"},
		},
		{
			name:  "fragment object",
			files: map[string]string{"agx.yaml": "fragments: [{path: a.yaml}]\n"},
			want:  []string{"agx.yaml:1"},
		},
		{
			name:  "empty file",
			files: map[string]string{"agx.yaml": ""},
			want:  []string{"file is empty; write {}"},
		},
		{
			name:  "unknown field",
			files: map[string]string{"agx.yaml": "version: 1\n"},
			want:  []string{"agx.yaml:1: unknown key version"},
		},
		{
			name:  "definitions in the root",
			files: map[string]string{"agx.yaml": "fragments: []\nskills: {}\n"},
			want: []string{
				"agx.yaml:2: the root configuration holds fragments and daemon settings only; " +
					"move skills into a fragment such as agx.d/main.yaml",
			},
		},
		{
			name:  "fragments in a fragment",
			files: map[string]string{"agx.yaml": "fragments: [a.yaml]\n", "a.yaml": "fragments: [b.yaml]\n"},
			want:  []string{"a.yaml:1: fragments belongs in the root configuration"},
		},
		{
			name:  "daemon settings in a fragment",
			files: map[string]string{"agx.yaml": "fragments: [a.yaml]\n", "a.yaml": "daemon: {autostart: false}\n"},
			want:  []string{"a.yaml:1: daemon belongs in the root configuration"},
		},
		{
			name:  "duplicate key",
			files: map[string]string{"agx.yaml": "fragments: [a.yaml]\n", "a.yaml": "skills: {}\nskills: {}\n"},
			want:  []string{"skills"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				write(t, filepath.Join(dir, name), content)
			}
			_, err := config.Load(filepath.Join(dir, "agx.yaml"), config.Options{Builtins: builtins})
			wantError(t, err, tt.want...)
		})
	}
}

func TestDefinitionPrecedence(t *testing.T) {
	t.Run("conflict without override names both locations", func(t *testing.T) {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "first.yaml"), "instructions:\n  x: {text: first}\n")
		root := fragment(t, dir, "instructions:\n  x: {text: root}\n", "first.yaml")
		_, err := config.Load(root, config.Options{Builtins: builtins})
		wantError(
			t,
			err,
			"main.yaml:2",
			"conflicting definition of instructions x",
			"first.yaml:2",
			"set override: true",
		)
	})
	t.Run("override in a later fragment replaces", func(t *testing.T) {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "first.yaml"), "instructions:\n  x: {text: first}\n")
		root := fragment(t, dir, "instructions:\n  x: {text: root, override: true}\n", "first.yaml")
		if got := read(t, root).Instructions["x"].Text; got != "root" {
			t.Fatalf("text = %q, want root", got)
		}
	})
	t.Run("override in an earlier fragment does not win", func(t *testing.T) {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "first.yaml"), "instructions:\n  x: {text: first, override: true}\n")
		root := fragment(t, dir, "instructions:\n  x: {text: root}\n", "first.yaml")
		_, err := config.Load(root, config.Options{Builtins: builtins})
		wantError(t, err, "conflicting definition of instructions x")
	})

	pkg := func(t *testing.T) string {
		dir := mkdir(t, filepath.Join(t.TempDir(), "pkg"))
		write(t, filepath.Join(dir, "agx.package.yaml"), "instructions:\n  note: {text: from package}\n")
		return dir
	}
	t.Run("package cannot replace a configuration definition", func(t *testing.T) {
		root := fragment(t, t.TempDir(), "instructions:\n  team/note: {text: from root}\n")
		c := read(t, root)
		_, err := c.AddPackage("team", pkg(t))
		wantError(t, err, "package definition team/note conflicts with configuration")
	})
	t.Run("configuration override wins over a package", func(t *testing.T) {
		root := fragment(t, t.TempDir(), `instructions:
  team/note: {text: from root, override: true}
`)
		c := read(t, root)
		if _, err := c.AddPackage("team", pkg(t)); err != nil {
			t.Fatal(err)
		}
		if got := c.Instructions["team/note"].Text; got != "from root" {
			t.Fatalf("text = %q, want the root definition", got)
		}
	})
}

func TestPackageManifests(t *testing.T) {
	root := fragment(t, t.TempDir(), "{}\n")

	t.Run("inline definitions are qualified by namespace", func(t *testing.T) {
		dir := mkdir(t, filepath.Join(t.TempDir(), "pkg"))
		write(t, filepath.Join(dir, "note.md"), "N")
		write(t, filepath.Join(dir, "agx.package.yaml"), `bundles:
  b:
    instructions: [./note.md, {id: inline, text: hi}]
`)
		c := read(t, root)
		if _, err := c.AddPackage("team", dir); err != nil {
			t.Fatal(err)
		}
		for _, id := range []config.ID{"team/note", "team/inline"} {
			if _, ok := c.Instructions[id]; !ok {
				t.Errorf("missing %s in %v", id, c.Instructions)
			}
		}
		if got := c.Bundles["team/b"].Instructions; !slices.Equal(got, []config.ID{"team/note", "team/inline"}) {
			t.Errorf("bundle selects %v", got)
		}
	})
	t.Run("a package without a manifest declares its skills", func(t *testing.T) {
		dir := mkdir(t, filepath.Join(t.TempDir(), "pkg"))
		skill(t, filepath.Join(dir, "skills", "pdf"), "pdf")
		skill(t, filepath.Join(dir, ".hidden", "secret"), "secret")
		skill(t, filepath.Join(dir, "node_modules", "dep"), "dep")
		c := read(t, root)
		if _, err := c.AddPackage("repo", dir); err != nil {
			t.Fatal(err)
		}
		got := []string{}
		for id := range c.Skills {
			got = append(got, string(id))
		}
		if !slices.Equal(got, []string{"repo/pdf"}) {
			t.Fatalf("skills = %v, want [repo/pdf]", got)
		}
	})
	for _, tt := range []struct{ name, manifest, want string }{
		{"no scopes", "scopes: [{add: {}}]\n", "unknown key scopes"},
		{"no home paths", "instructions:\n  x: {path: ~/x.md}\n", "package manifests cannot use home-relative paths"},
		{"https or ssh Git", "packages:\n  dep: {git: /srv/repo.git}\n", "must use an https, ssh, or scp-style Git source"},
		{"paths stay inside", "instructions:\n  x: {path: ../x.md}\n", "path escapes package"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := mkdir(t, filepath.Join(t.TempDir(), "pkg"))
			write(t, filepath.Join(dir, "agx.package.yaml"), tt.manifest)
			_, err := read(t, root).AddPackage("team", dir)
			wantError(t, err, tt.want)
		})
	}
}

func TestInlineDefinitions(t *testing.T) {
	setup := func(t *testing.T, content string) string {
		dir := t.TempDir()
		skill(t, filepath.Join(dir, "skills", "foo"), "foo")
		skill(t, filepath.Join(dir, "skills", "My Skill"), "my-skill")
		write(t, filepath.Join(dir, "AGENTS.md"), "agents")
		mkdir(t, filepath.Join(dir, "work", "sub"))
		return fragment(t, dir, content)
	}

	t.Run("inline definitions are referenced by ID elsewhere", func(t *testing.T) {
		root := setup(t, `bundles:
  b:
    skills: [./skills/foo]
    instructions: [{id: t, text: hi}]
scopes:
  - directory: work
    add:
      skills: [foo]
      instructions: [t, ./AGENTS.md]
      mcp: [{id: docs, url: 'https://docs.example/mcp', auth: none}]
      bundles: [b]
  - directory: work/sub
    remove: {skills: [foo]}
harnesses:
  codex:
    remove: {instructions: [t]}
`)
		c := load(t, root)
		for _, kind := range []struct {
			ok   bool
			name string
		}{
			{c.Skills["foo"].Inline, "skill foo"},
			{c.Instructions["t"].Inline, "instructions t"},
			{c.Instructions["AGENTS"].Inline, "instructions AGENTS"},
			{c.MCP["docs"].Inline, "mcp docs"},
		} {
			if !kind.ok {
				t.Errorf("%s is not an inline definition", kind.name)
			}
		}
		s, err := c.Resolve(filepath.Join(filepath.Dir(root), "work", "sub"), "")
		if err != nil {
			t.Fatal(err)
		}
		if got := ids(s.Resources, "skill"); len(got) != 0 {
			t.Errorf("skills %v selected after the deeper removal", got)
		}
	})

	for _, tt := range []struct {
		name, content string
		want          []string
	}{
		{
			name: "same inline path twice names both locations",
			content: `scopes:
  - directory: work
    add: {skills: [./skills/foo]}
  - directory: work/sub
    add:
      skills: [./skills/foo]
`,
			want: []string{
				"main.yaml:6", "skill foo is also defined at", "main.yaml:3",
				"define it once, under top-level skills:, and reference its ID elsewhere",
			},
		},
		{
			name: "inline ID colliding with a top-level definition",
			content: `skills:
  foo: {path: ./skills/foo}
scopes:
  - directory: work
    add: {skills: [./skills/foo]}
`,
			want: []string{"skill foo is also defined at", "main.yaml:2"},
		},
		{
			name: "override cannot replace an inline definition",
			content: `scopes:
  - directory: work
    add: {skills: [./skills/foo]}
skills:
  foo: {path: ./skills/foo, override: true}
`,
			want: []string{"skill foo is also defined at"},
		},
		{
			name: "inline MCP servers collide across bundles and scopes",
			content: `bundles:
  b:
    mcp: [{id: m, url: 'https://a.example/mcp'}]
scopes:
  - directory: work
    add:
      mcp: [{id: m, url: 'https://b.example/mcp'}]
`,
			want: []string{"MCP server m is also defined at"},
		},
		{
			name:    "underivable path ID",
			content: "scopes:\n  - add: {skills: ['./skills/My Skill']}\n",
			want:    []string{`cannot derive an ID from "My Skill"; use a mapping with an id`},
		},
		{
			name:    "content skill needs an id",
			content: "scopes:\n  - add: {skills: [{description: d, content: c}]}\n",
			want:    []string{"skills[0]: missing id"},
		},
		{
			name:    "inline MCP needs an id",
			content: "scopes:\n  - add: {mcp: [{url: 'https://a.example/mcp'}]}\n",
			want:    []string{"mcp[0]: missing id"},
		},
		{
			name:    "MCP servers have no path form",
			content: "scopes:\n  - add: {mcp: [./server]}\n",
			want:    []string{"mcp[0]: \"./server\" is not valid here"},
		},
		{
			name:    "inline definitions cannot override",
			content: "scopes:\n  - add: {instructions: [{id: t, text: hi, override: true}]}\n",
			want:    []string{"unknown key override"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.Load(setup(t, tt.content), config.Options{Builtins: builtins})
			wantError(t, err, tt.want...)
		})
	}
}

func TestGeneratedSkillValidation(t *testing.T) {
	for _, tt := range []struct{ name, skill, want string }{
		{"needs a description", "{content: body}", "skills.d: missing description"},
		{"invalid name", "{name: Deploy_X, description: d, content: body}", `skills.d.name: "Deploy_X" is not valid here`},
		{"name from an invalid ID", "{description: d, content: body}", "must be lowercase letters"},
		{"frontmatter: key \"name\" is not allowed", "{description: d, content: body, frontmatter: {name: x}}", "frontmatter: key \"name\" is not allowed"},
		{"files escape", "{description: d, content: body, files: {../x: {content: y}}}", "files: key \"../x\" is not allowed"},
		{"files name SKILL.md", "{description: d, content: body, files: {skill.md: {content: y}}}", "files: key \"skill.md\" is not allowed"},
		{"files contain .git", "{description: d, content: body, files: {.git/config: {content: y}}}", "files: key \".git/config\" is not allowed"},
		{"files overlap", "{description: d, content: body, files: {a: {content: x}, a/b: {content: y}}}", "files a and a/b overlap"},
		{"file as a string", "{description: d, content: body, files: {a: ./x}}", "files.a: expected a mapping, not a string"},
		{"file with content and path", "{description: d, content: body, files: {a: {content: x, path: ./x}}}", "files.a.path: not allowed here"},
		{"name on a path skill", "{path: ./s, name: foo}", "skills.d.name: not allowed here"},
		{"rev", "{git: 'https://example.com/r.git', rev: abc123}", "unknown key rev"},
		{"Git path escapes", "{git: 'https://example.com/r.git', path: ../x}", "must stay inside the Git checkout"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id := "d"
			if tt.name == "name from an invalid ID" {
				id = "My.Skill"
			}
			root := fragment(t, t.TempDir(), "skills:\n  "+id+": "+tt.skill+"\n")
			_, err := config.Load(root, config.Options{Builtins: builtins})
			wantError(t, err, tt.want)
		})
	}
}

func TestPinnedSources(t *testing.T) {
	rev := strings.Repeat("a", 40)
	hash := strings.Repeat("b", 64)
	for _, tt := range []struct {
		name string
		p    config.Package
		want bool
	}{
		{"git with ref", config.Package{Git: "https://example.com/r.git", Ref: "main"}, false},
		{"git with a commit ref", config.Package{Git: "https://example.com/r.git", Ref: rev}, true},
		{"git with a short commit ref", config.Package{Git: "https://example.com/r.git", Ref: rev[:12]}, false},
		{"local path", config.Package{Path: "/srv/pkg"}, false},
		{"local path with hash", config.Package{Path: "/srv/pkg", Hash: hash}, true},
		{"nix store path", config.Package{Path: "/nix/store/abc-pkg"}, runtime.GOOS != "windows"},
		{"nix store lookalike", config.Package{Path: "/nix/storefront/pkg"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Pinned(); got != tt.want {
				t.Fatalf("Pinned() = %v, want %v", got, tt.want)
			}
		})
	}

	for _, tt := range []struct{ name, pkg, want string }{
		{"ref on a path", "{path: ./x, ref: " + rev + "}", "packages.p.ref: not allowed here"},
		{"malformed hash", "{path: ./x, hash: abc}", "packages.p.hash: \"abc\" is not valid here"},
		{"path and git", "{path: ./x, git: 'https://example.com/r.git'}", "packages.p.path: not allowed here"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := fragment(t, t.TempDir(), "packages:\n  p: "+tt.pkg+"\n")
			_, err := config.Load(root, config.Options{Builtins: builtins})
			wantError(t, err, tt.want)
		})
	}

	t.Run("a hash-pinned package loads from the cache without a lockfile", func(t *testing.T) {
		dir := t.TempDir()
		src := mkdir(t, filepath.Join(dir, "src"))
		write(t, filepath.Join(src, "agx.package.yaml"), "instructions:\n  note: {path: note.md}\n")
		write(t, filepath.Join(src, "note.md"), "pinned note")
		hash, err := config.TreeHash(src)
		if err != nil {
			t.Fatal(err)
		}
		root := fragment(t, dir, "packages:\n  p: {path: ./src, hash: "+hash+"}\n")
		cache := filepath.Join(dir, "cache")

		missing, err := config.Load(root, config.Options{Builtins: builtins, Cache: cache})
		if err != nil {
			t.Fatal(err)
		}
		if got := missing.MissingWarnings(missing.AllMissing()); len(got) != 1 ||
			got[0] != "package p is not installed; run agx install" {
			t.Fatalf("warnings = %q", got)
		}

		if err := securefs.CopyTree(src, filepath.Join(cache, "packages", hash)); err != nil {
			t.Fatal(err)
		}
		c, err := config.Load(root, config.Options{Builtins: builtins, Cache: cache})
		if err != nil {
			t.Fatal(err)
		}
		if b, err := c.Instructions["p/note"].Read(); err != nil || string(b) != "pinned note" {
			t.Fatalf("p/note = %q, %v", b, err)
		}
	})

	t.Run("an unpinned package needs a lock", func(t *testing.T) {
		dir := t.TempDir()
		mkdir(t, filepath.Join(dir, "src"))
		root := fragment(t, dir, "packages:\n  p: {path: ./src}\n")
		c, err := config.Load(root, config.Options{Builtins: builtins, Cache: filepath.Join(dir, "cache")})
		if err != nil {
			t.Fatal(err)
		}
		if got := c.AllMissing(); len(got) != 1 || got[0] != "p" {
			t.Fatalf("missing = %v", got)
		}
		write(t, filepath.Join(dir, "agx.lock.json"), "{not json")
		_, err = config.Load(root, config.Options{Builtins: builtins, Cache: filepath.Join(dir, "cache")})
		wantError(t, err, "agx.lock.json", "run agx install")
	})
}

func TestMissingPackagesAreSkipped(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "a"))
	mkdir(t, filepath.Join(dir, "b"))
	write(t, filepath.Join(dir, "note.md"), "kept")
	root := fragment(t, dir, `packages:
  a: {path: ./a}
  b: {path: ./b}
skills:
  remote: {git: https://example.invalid/skills.git, path: remote}
instructions:
  note: {path: note.md}
bundles:
  team: {skills: [a/review], mcp: [b/search]}
scopes:
  - add:
      skills: [a/review, a/lint, remote]
      instructions: [note]
      bundles: [team, b/defaults]
`)
	c, err := config.Load(root, config.Options{Builtins: builtins, Cache: filepath.Join(dir, "cache")})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.AllMissing(); !slices.Equal(got, []config.ID{"a", "b", "skill:remote"}) {
		t.Fatalf("missing = %v", got)
	}
	if _, ok := c.Skills["remote"]; ok {
		t.Fatal("a skill whose Git source is missing stayed defined")
	}
	snapshot, err := c.Resolve(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Resources) != 1 || snapshot.Resources[0].ID != "note" {
		t.Fatalf("resources = %v", snapshot.Resources)
	}
	if got := c.MissingWarnings(snapshot.Missing); !slices.Equal(got, []string{
		"package a is not installed; run agx install",
		"package b is not installed; run agx install",
		"skill remote: its Git source is not installed; run agx install",
	}) {
		t.Fatalf("warnings = %q", got)
	}

	// A reference no missing package explains is still an error.
	bad := fragment(t, dir, `packages:
  a: {path: ./a}
scopes:
  - add:
      skills: [c/review]
`)
	_, err = config.Load(bad, config.Options{Builtins: builtins, Cache: filepath.Join(dir, "cache")})
	wantError(t, err, `unknown skill "c/review"`)
}

func TestHomeExpansion(t *testing.T) {
	home := isolateHome(t)
	skill(t, filepath.Join(home, "skills", "s"), "s")
	write(t, filepath.Join(home, "frag.yaml"), "instructions:\n  frag: {text: frag}\n")
	mkdir(t, filepath.Join(home, "work"))
	root := fragment(t, t.TempDir(), `packages:
  p: {path: ~/pkg}
skills:
  s: {path: ~/skills/s}
mcp:
  up:
    command: [~/bin/up, --stdio]
    cwd: ~/srv
scopes:
  - directory: ~/work
    add: {skills: [s]}
harnesses:
  codex:
    executable: ~/bin/codex
    environment:
      PROMPT: !file ~/prompt.txt
`, "~/frag.yaml")
	c := read(t, root)
	in := func(parts ...string) string { return filepath.Join(append([]string{home}, parts...)...) }
	for _, tt := range []struct{ name, got, want string }{
		{"skill path", c.Skills["s"].Path, in("skills", "s")},
		{"package path", c.Packages["p"].Path, in("pkg")},
		{"mcp command", c.MCP["up"].Command[0], in("bin", "up")},
		{"mcp cwd", c.MCP["up"].Cwd, in("srv")},
		{"harness executable", c.Harnesses["codex"].Executable, in("bin", "codex")},
		{"!file", c.Harnesses["codex"].Environment["PROMPT"].File, in("prompt.txt")},
		{"scope directory", c.Scopes[0].Directory, canonical(t, in("work"))},
	} {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
	if _, ok := c.Instructions["frag"]; !ok {
		t.Error("~ fragment not loaded")
	}
	if got := c.MCP["up"].Command[1]; got != "--stdio" {
		t.Errorf("arguments after the executable changed: %q", got)
	}

	for _, content := range []string{
		"scopes:\n  - directory: ~root/work\n",
		"harnesses:\n  codex: {executable: ~bin/codex}\n",
	} {
		root := fragment(t, t.TempDir(), content)
		_, err := config.Load(root, config.Options{Builtins: builtins})
		wantError(t, err, "~user paths are not supported")
	}
	root = write(t, filepath.Join(t.TempDir(), "agx.yaml"), "fragments: [~other/x.yaml]\n")
	_, err := config.Load(root, config.Options{Builtins: builtins})
	wantError(t, err, "~user paths are not supported")
}

func TestUnreadableScopeDirectory(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs directory permissions that bind the current user")
	}
	dir := t.TempDir()
	locked := mkdir(t, filepath.Join(dir, "locked"))
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	// TempDir cleanup must list the directory to remove it.
	t.Cleanup(func() {
		if err := os.Chmod(locked, 0o755); err != nil {
			t.Error(err)
		}
	})
	root := fragment(t, dir, "scopes:\n  - directory: locked/work\n")
	c := read(t, root)
	if got, want := c.Scopes[0].Directory, filepath.Join(canonical(t, dir), "locked", "work"); got != want {
		t.Errorf("scope directory = %q, want %q", got, want)
	}
}

func TestDaemonAutostart(t *testing.T) {
	dir := t.TempDir()
	root := write(t, filepath.Join(dir, "agx.yaml"), "daemon: {autostart: false, lease_seconds: 30}\n")
	if c := read(t, root); c.Daemon.Autostart == nil || *c.Daemon.Autostart || c.Daemon.LeaseSeconds != 30 ||
		c.Daemon.RenewSeconds != 20 {
		t.Fatalf("daemon = %+v, want autostart false, lease 30, renewal 20", c.Daemon)
	}
	if c := read(t, write(t, filepath.Join(dir, "default.yaml"), "{}\n")); c.Daemon.Autostart != nil {
		t.Fatalf("autostart = %v, want unset", *c.Daemon.Autostart)
	}
}

func TestFormatVersions(t *testing.T) {
	dir := t.TempDir()
	pin := filepath.Join(dir, "pin.json")
	hash := strings.Repeat("a", 64)
	if err := config.WritePin(pin, hash); err != nil {
		t.Fatal(err)
	}
	if got, err := config.ReadPin(pin); err != nil || got != hash {
		t.Fatalf("ReadPin = %q, %v", got, err)
	}
	for _, tt := range []struct{ content, want string }{
		{`{"version":2,"hash":"` + hash + `"}`, "uses format version 2, but this agx reads only version 1; upgrade agx"},
		{`{"hash":"` + hash + `"}`, "invalid pin record"},
	} {
		write(t, pin, tt.content)
		_, err := config.ReadPin(pin)
		wantError(t, err, tt.want)
	}
	lock := write(t, filepath.Join(dir, "agx.lock.json"), `{"version":2,"packages":{},"sources":{}}`)
	_, err := config.ReadLock(lock, nil)
	wantError(t, err, "uses format version 2, but this agx reads only version 1")
}

func TestMCPLoad(t *testing.T) {
	dir := t.TempDir()
	root := fragment(t, dir, `mcp:
  plain: {command: [plain]}
  tracker:
    url: https://tracker.example/mcp
    description: Issues of the platform team
    load: on_demand
`)
	c := read(t, root)
	if got := c.MCP["plain"].Load; got != config.LoadNative {
		t.Fatalf("default load = %q", got)
	}
	if tracker := c.MCP["tracker"]; tracker.Load != config.LoadOnDemand ||
		tracker.Description != "Issues of the platform team" {
		t.Fatalf("tracker = %+v", tracker)
	}
	bad := fragment(t, t.TempDir(), "mcp:\n  x: {command: [x], load: lazy}\n")
	if _, err := config.Load(bad, config.Options{Builtins: builtins}); err == nil ||
		!strings.Contains(err.Error(), "mcp.x.load: lazy must be one of native, on_demand") {
		t.Fatalf("invalid load: %v", err)
	}
}

func TestMCPShare(t *testing.T) {
	root := fragment(t, t.TempDir(), `mcp:
  plain: {command: [plain]}
  search: {command: [search], share: repo}
  index: {command: [index], share: {type: directory, idle: 30m}}
`)
	c := read(t, root)
	if got := c.MCP["plain"].ShareMode(); got != config.ShareNone {
		t.Fatalf("default share = %q", got)
	}
	if got := c.MCP["search"].Share.Type; got != config.ShareRepository {
		t.Fatalf("share = %q", got)
	}
	if index := c.MCP["index"].Share; index.Type != config.ShareDirectory || index.Idle == nil ||
		*index.Idle != 30*time.Minute {
		t.Fatalf("index share = %+v", index)
	}
	for content, want := range map[string]string{
		"mcp:\n  x: {command: [x], share: {type: global, idle: soon}}\n": "share idle must be a duration",
		"mcp:\n  x: {command: [x], share: {idle: 1m}}\n":                 "mcp.x.share: missing type",
		"mcp:\n  x: {command: [x], share: {type: none, idle: 1m}}\n":     "share idle needs a share type other than none",
		"mcp:\n  x: {command: [x], share: project}\n":                    "project must be one of none, directory, repo, global",
		"mcp:\n  x: {url: https://x.example/mcp, share: global}\n":       "mcp.x.share: not allowed here",
	} {
		bad := fragment(t, t.TempDir(), content)
		if _, err := config.Load(
			bad,
			config.Options{Builtins: builtins},
		); err == nil ||
			!strings.Contains(err.Error(), want) {
			t.Fatalf("%q: %v", content, err)
		}
	}
}
