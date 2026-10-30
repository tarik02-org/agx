package configedit_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/adapters/builtin"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/configedit"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/storage"
)

var builtins = builtin.Registry().Builtins(nil)

func workspace(t *testing.T) (platform.Paths, func() *configedit.Session) {
	t.Helper()
	dir, err := platform.Canonical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(dir, "home"))
	t.Setenv("USERPROFILE", filepath.Join(dir, "home"))
	paths := platform.Paths{
		Config: filepath.Join(dir, "agx.yaml"),
		State:  filepath.Join(dir, "state"),
		Cache:  filepath.Join(dir, "cache"),
	}
	load := func() *configedit.Session {
		t.Helper()
		c, err := config.Load(paths.Config, config.Options{Builtins: builtins, State: paths.State, Cache: paths.Cache})
		if err != nil {
			t.Fatal(err)
		}
		s, err := configedit.New(paths, builtins, c)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	return paths, load
}

func save(t *testing.T, s *configedit.Session) {
	t.Helper()
	if changed, err := s.Commit(t.Context()); err != nil || !changed {
		t.Fatalf("commit: %t, %v", changed, err)
	}
}

func define(t *testing.T, s *configedit.Session, kind config.Kind, id string, value any) {
	t.Helper()
	if err := s.Define(kind, id, value); err != nil {
		t.Fatal(err)
	}
}

func TestLocalPersistenceAndRootIsolation(t *testing.T) {
	paths, load := workspace(t)
	s := load()
	if _, err := os.Stat(storage.State(paths.State)); !os.IsNotExist(err) {
		t.Fatalf("read created state: %v", err)
	}
	define(t, s, config.KindMCPServers, "docs", map[string]any{"command": []string{"server"}})
	if _, err := s.Enable(config.KindMCPServers, "docs", configedit.NewScope("", nil)); err != nil {
		t.Fatal(err)
	}
	save(t, s)
	c := load().Config
	if c.MCP["docs"].Command[0] != "server" || !c.MCP["docs"].Source.Local || len(c.Scopes) != 1 {
		t.Fatalf("config: %+v", c)
	}
	if _, err := os.Stat(paths.Config); !os.IsNotExist(err) {
		t.Fatalf("created YAML: %v", err)
	}
	other, err := config.Load(
		filepath.Join(filepath.Dir(paths.Config), "other.yaml"),
		config.Options{State: paths.State},
	)
	if err != nil || len(other.MCP) != 0 {
		t.Fatalf("other root: %+v %v", other, err)
	}
}

func TestOptimisticConflictAndInvalidChange(t *testing.T) {
	_, load := workspace(t)
	first, stale := load(), load()
	define(t, first, config.KindInstructions, "one", map[string]any{"text": "One."})
	define(t, stale, config.KindInstructions, "two", map[string]any{"text": "Two."})
	save(t, first)
	if _, err := stale.Commit(t.Context()); err == nil {
		t.Fatal("stale edit committed")
	}
	broken := load()
	define(t, broken, config.KindMCPServers, "bad", map[string]any{"url": 5})
	if _, err := broken.Commit(t.Context()); err == nil {
		t.Fatal("invalid definition committed")
	}
	c := load().Config
	if len(c.Instructions) != 1 || c.Local.Revision != 1 || len(c.MCP) != 0 {
		t.Fatalf("rollback failed: %+v", c.Local)
	}
}

func TestOverlayPrecedenceAndOwnership(t *testing.T) {
	paths, load := workspace(t)
	s := load()
	define(t, s, config.KindMCPServers, "docs", map[string]any{"command": []string{"local"}, "description": "local"})
	define(t, s, config.KindHarnesses, "work", map[string]any{"base": "codex", "args": []string{"local"}})
	save(t, s)
	if err := os.MkdirAll(filepath.Join(filepath.Dir(paths.Config), "agx.d"), 0o700); err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(filepath.Dir(paths.Config), "agx.d", "overlay.yaml")
	data := "mcp:\n  docs:\n    command: [yaml]\nharnesses:\n  work:\n    base: claude\n"
	if err := os.WriteFile(overlay, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	s = load()
	if s.Config.MCP["docs"].Command[0] != "yaml" || s.Config.MCP["docs"].Description != "" {
		t.Fatalf("partial overlay: %+v", s.Config.MCP["docs"])
	}
	h, err := s.Config.Harness("work")
	if err != nil || h.Builtin != "claude" || len(h.Args) != 0 {
		t.Fatalf("harness overlay: %+v %v", h, err)
	}
	if err := s.Define(
		config.KindMCPServers,
		"docs",
		map[string]any{"command": []string{"changed"}},
	); err == nil ||
		!strings.Contains(err.Error(), overlay) {
		t.Fatalf("ownership: %v", err)
	}
	if err := os.Remove(overlay); err != nil {
		t.Fatal(err)
	}
	if load().Config.MCP["docs"].Command[0] != "local" {
		t.Fatal("overlay deleted underlying local definition")
	}
}

func TestTaggedValuesAndRevisionInputs(t *testing.T) {
	paths, load := workspace(t)
	s := load()
	define(
		t,
		s,
		config.KindMCPServers,
		"server",
		map[string]any{
			"command": []string{"server"},
			"environment": map[string]any{
				"TOKEN":  map[string]any{"$env": "TOKEN"},
				"FILE":   map[string]any{"$file": "credential.txt"},
				"SECRET": map[string]any{"$secret": "credential"},
			},
		},
	)
	define(t, s, config.KindSecrets, "credential", map[string]any{"kind": "keychain"})
	save(t, s)
	inputs := &config.Inputs{}
	c, err := config.Load(
		paths.Config,
		config.Options{Builtins: builtins, State: paths.State, Cache: paths.Cache, Inputs: inputs},
	)
	if err != nil {
		t.Fatal(err)
	}
	if c.MCP["server"].Environment["TOKEN"].Env != "TOKEN" {
		t.Fatalf("tag: %+v", c.MCP["server"].Environment)
	}
	if value := c.MCP["server"].Environment["FILE"]; value.File != filepath.Join(
		filepath.Dir(c.Root),
		"credential.txt",
	) ||
		!value.Source.Local {
		t.Fatalf("file tag: %+v", value)
	}
	if value := c.MCP["server"].Environment["SECRET"]; value.Secret != "credential" {
		t.Fatalf("secret tag: %+v", value)
	}
	db, err := storage.Open(t.Context(), storage.State(paths.State))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(
		t.Context(),
		"INSERT INTO launches VALUES ('irrelevant','codex','codex','store','directory',0,NULL,1)",
	)
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if inputs.Changed() {
		t.Fatal("unrelated state invalidated configuration")
	}
	s = load()
	define(t, s, config.KindInstructions, "added", map[string]any{"text": "Added."})
	save(t, s)
	if !inputs.Changed() {
		t.Fatal("configuration revision was not watched")
	}
	exported, err := s.Local.Export()
	if err != nil || !strings.Contains(string(exported), "!env TOKEN") {
		t.Fatalf("export: %s %v", exported, err)
	}
}

func TestResourceWriteRestrictions(t *testing.T) {
	paths, load := workspace(t)
	file := filepath.Join(filepath.Dir(paths.Config), "AGENTS.md")
	if err := os.WriteFile(file, []byte("Before."), 0o600); err != nil {
		t.Fatal(err)
	}
	s := load()
	define(t, s, config.KindInstructions, "agents", map[string]any{"path": file})
	save(t, s)
	if err := configedit.WriteFile(load().Config, paths, file, []byte("After.")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != "After." {
		t.Fatalf("file: %s %v", data, err)
	}
	if err := configedit.WriteFile(load().Config, paths, paths.Config, []byte("{}")); err == nil {
		t.Fatal("wrote configuration file")
	}
}

func TestCancelledCommit(t *testing.T) {
	paths, load := workspace(t)
	s := load()
	define(t, s, config.KindInstructions, "cancelled", map[string]any{"text": "Do not save."})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Commit(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("commit: %v", err)
	}
	if _, err := os.Stat(storage.State(paths.State)); !os.IsNotExist(err) {
		t.Fatalf("cancelled write created database: %v", err)
	}
}

func TestConcurrentLocalWriters(t *testing.T) {
	_, load := workspace(t)
	first, second := load(), load()
	define(t, first, config.KindInstructions, "first", map[string]any{"text": "First."})
	define(t, second, config.KindInstructions, "second", map[string]any{"text": "Second."})
	results := make(chan error, 2)
	start := make(chan struct{})
	for _, s := range []*configedit.Session{first, second} {
		go func() { <-start; _, err := s.Commit(t.Context()); results <- err }()
	}
	close(start)
	one, two := <-results, <-results
	if (one == nil) == (two == nil) || one != nil && !errors.Is(one, config.ErrConflict) ||
		two != nil && !errors.Is(two, config.ErrConflict) {
		t.Fatalf("writers: %v, %v", one, two)
	}
	if c := load().Config; len(c.Instructions) != 1 || c.Local.Revision != 1 {
		t.Fatalf("lost atomicity: %+v", c.Local)
	}
}

func TestLocalInlineDefinitionOrigin(t *testing.T) {
	_, load := workspace(t)
	s := load()
	define(
		t,
		s,
		config.KindBundles,
		"bundle",
		map[string]any{"instructions": []any{map[string]any{"id": "inline", "text": "Inline."}}},
	)
	save(t, s)
	if source := load().Config.Instructions["inline"].Source; !source.Local || source.Line != 0 {
		t.Fatalf("inline origin: %+v", source)
	}
}

func TestInputsNoticeRepairedDatabase(t *testing.T) {
	paths, _ := workspace(t)
	if err := os.MkdirAll(paths.State, 0o700); err != nil {
		t.Fatal(err)
	}
	database := storage.State(paths.State)
	if err := os.WriteFile(database, []byte(strings.Repeat("invalid", 1000)), 0o600); err != nil {
		t.Fatal(err)
	}
	inputs := &config.Inputs{}
	_, err := config.Load(paths.Config, config.Options{State: paths.State, Inputs: inputs})
	if err == nil {
		t.Fatal("corrupt state loaded")
	}
	if err := os.Remove(database); err != nil {
		t.Fatal(err)
	}
	if !inputs.Changed() {
		t.Fatal("repair did not invalidate cached load failure")
	}
	if _, err := config.Load(paths.Config, config.Options{State: paths.State}); err != nil {
		t.Fatal(err)
	}
}

func TestScopeCannotSilentlyOverrideYAML(t *testing.T) {
	paths, load := workspace(t)
	dir := filepath.Join(filepath.Dir(paths.Config), "agx.d")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(dir, "rules.yaml")
	data := "mcp:\n  docs:\n    command: [docs]\nbundles:\n  dev:\n    mcp: [docs]\nscopes:\n  - add:\n      bundles: [dev]\n"
	if err := os.WriteFile(overlay, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	s := load()
	if _, err := s.Disable(
		config.KindMCPServers,
		"docs",
		configedit.NewScope("", nil),
	); err == nil ||
		!strings.Contains(err.Error(), overlay) {
		t.Fatalf("shadowed edit: %v", err)
	}
	if _, err := s.Disable(
		config.KindMCPServers,
		"docs",
		configedit.NewScope(filepath.Dir(paths.Config), nil),
	); err != nil {
		t.Fatalf("deeper rule: %v", err)
	}
	save(t, s)
}

func TestParseSource(t *testing.T) {
	for arg, want := range map[string]configedit.Source{
		"anthropics/skills@v1/document-skills": {Git: "https://github.com/anthropics/skills.git", Ref: "v1", Subdir: "document-skills", Name: "anthropics"},
		"https://example.com/kit.git":          {Git: "https://example.com/kit.git", Name: "kit"},
	} {
		got, err := configedit.ParseSource(arg, configedit.GitHub, false)
		if err != nil || got != want {
			t.Fatalf("source %s: %+v %v", arg, got, err)
		}
	}
	if _, err := configedit.ParseSource("./skills", configedit.GitHub, false); err == nil {
		t.Fatal("preview accepted a local path")
	}
	if got, err := configedit.ParseSource("./skills", configedit.GitHub, true); err != nil || got.Local != "./skills" {
		t.Fatalf("local path: %+v %v", got, err)
	}
}

func TestResourceFilesStayWithinDeclaredContent(t *testing.T) {
	paths, load := workspace(t)
	root := filepath.Dir(paths.Config)
	skill := filepath.Join(root, "skill")
	if err := os.MkdirAll(skill, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(skill, "SKILL.md"),
		[]byte("---\nname: skill\ndescription: d\n---\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	cached := filepath.Join(paths.Cache, "private.md")
	if err := os.MkdirAll(paths.Cache, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cached, []byte("cached"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := load()
	define(t, s, config.KindSkills, "skill", map[string]any{"path": skill})
	define(t, s, config.KindInstructions, "cached", map[string]any{"path": cached})
	save(t, s)
	c := load().Config
	created := filepath.Join(skill, "new.md")
	if err := configedit.WriteFile(c, paths, created, []byte("New.")); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(created); err != nil || string(data) != "New." {
		t.Fatalf("new content: %s %v", data, err)
	}
	for _, path := range []string{cached, storage.State(paths.State), paths.Config} {
		if err := configedit.WriteFile(c, paths, path, []byte("Do not write.")); err == nil {
			t.Fatalf("wrote protected file %s", path)
		}
	}
	outside := filepath.Join(root, "outside.md")
	if err := os.WriteFile(outside, []byte("Outside."), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(skill, "escape.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if err := configedit.WriteFile(c, paths, link, []byte("Escape.")); err == nil {
		t.Fatal("wrote outside through symlink")
	}
}

func TestReferencedSecretNeedsDependentEdit(t *testing.T) {
	_, load := workspace(t)
	s := load()
	define(t, s, config.KindSecrets, "credential", map[string]any{"kind": "keychain"})
	define(
		t,
		s,
		config.KindMCPServers,
		"server",
		map[string]any{
			"command":     []string{"server"},
			"environment": map[string]any{"TOKEN": map[string]any{"$secret": "credential"}},
		},
	)
	save(t, s)
	s = load()
	if err := s.Undefine(config.KindSecrets, "credential"); err == nil {
		t.Fatal("deleted a referenced secret")
	}
	if changed, err := s.Commit(t.Context()); err != nil || changed {
		t.Fatalf("rejected deletion changed config: %t %v", changed, err)
	}
	define(t, s, config.KindMCPServers, "server", map[string]any{"command": []string{"server"}})
	if err := s.Undefine(config.KindSecrets, "credential"); err != nil {
		t.Fatal(err)
	}
	save(t, s)
	if len(load().Config.Secrets) != 0 {
		t.Fatal("secret remains after dependent edit")
	}
}
