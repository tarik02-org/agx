package builtin_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/adapters/builtin"
	"github.com/tarik02-org/agx/internal/config"
)

const grant = "grant-token"

type fixture struct {
	config    *config.Config
	work      string
	cache     string
	endpoints []adapters.Endpoint
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

// newFixture configures one skill, two instructions, and two MCP servers for every harness, each
// running a fake executable.
func newFixture(t *testing.T) fixture {
	t.Helper()
	dir := t.TempDir()
	fake := filepath.Join(dir, "bin", "fake")
	if runtime.GOOS == "windows" {
		fake += ".exe"
	}
	write(t, fake, "fake")
	write(t, filepath.Join(dir, "skills", "review", "SKILL.md"), "---\nname: review\ndescription: r\n---\nReview.\n")
	if err := os.MkdirAll(filepath.Join(dir, "work"), 0o755); err != nil {
		t.Fatal(err)
	}
	executable := "'" + filepath.ToSlash(fake) + "'"
	write(t, filepath.Join(dir, "agx.yaml"), "fragments: [main.yaml]\n")
	write(t, filepath.Join(dir, "main.yaml"), `skills:
  review: {path: ./skills/review}
instructions:
  first: {text: first}
  second: {text: second}
mcp:
  up:
    command: [up]
    environment: {TOKEN: !env MCP_ONLY}
  team/remote:
    url: 'https://remote.example/mcp'
    auth: none
scopes:
  - directory: work
    add:
      skills: [review]
      instructions: [first, second]
      mcp: [up, team/remote]
harnesses:
  codex: {executable: `+executable+`}
  claude: {executable: `+executable+`}
  pi: {executable: `+executable+`}
`)
	c, err := config.Load(
		filepath.Join(dir, "agx.yaml"),
		config.Options{Builtins: builtin.Registry().Builtins(nil), Cache: filepath.Join(dir, "cache")},
	)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{
		config: c,
		work:   filepath.Join(dir, "work"),
		cache:  filepath.Join(dir, "cache"),
		endpoints: []adapters.Endpoint{
			{ID: "up", URL: "http://127.0.0.1:9/launch/L/mcp/up"},
			{ID: "team/remote", URL: "http://127.0.0.1:9/launch/L/mcp/team%2Fremote"},
		},
	}
}

// build composes the plan of harness for the fixture's work directory.
func (f fixture) build(t *testing.T, harness string, native ...string) (adapters.Plan, *config.Snapshot, error) {
	t.Helper()
	s, err := f.config.Resolve(f.work, harness)
	if err != nil {
		t.Fatal(err)
	}
	h, err := f.config.Harness(harness)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := builtin.Registry().Adapter(h.Builtin)
	plan, err := adapters.Build(
		t.Context(),
		a,
		adapters.Launch{Snapshot: s, Cache: f.cache, ID: "L", Endpoints: f.endpoints},
		h,
		[]string{"--definition"},
		map[string]string{"DEFINITION": "1"},
		grant,
		native,
	)
	return plan, s, err
}

// artifacts is the fixture's one artifact directory.
func artifacts(t *testing.T, f fixture) string {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join(f.cache, "artifacts", "v3-*"))
	if err != nil || len(dirs) != 1 {
		t.Fatalf("artifact directories = %q, %v", dirs, err)
	}
	return dirs[0]
}

func checkPlan(t *testing.T, f fixture, plan adapters.Plan, s *config.Snapshot, want []string) {
	t.Helper()
	if !slices.Equal(plan.Arguments, want) {
		t.Errorf("arguments:\n got %q\nwant %q", plan.Arguments, want)
	}
	if plan.Directory != s.Directory {
		t.Errorf("directory = %q, want %q", plan.Directory, s.Directory)
	}
	if want := map[string]string{
		"DEFINITION":               "1",
		adapters.GrantEnvironment:  grant,
		adapters.LaunchEnvironment: "L",
	}; !mapsEqual(
		plan.Environment,
		want,
	) {
		t.Errorf("environment = %v, want %v", plan.Environment, want)
	}
	if want := []string{"AGX_MANAGEMENT_TOKEN", "AGX_LIFECYCLE_TOKEN", "MCP_ONLY"}; !slices.Equal(plan.Unset, want) {
		t.Errorf("unset = %v, want %v", plan.Unset, want)
	}
	text, err := os.ReadFile(filepath.Join(artifacts(t, f), "instructions.md"))
	if err != nil || string(text) != "first\n\nsecond\n\n" {
		t.Errorf("instructions.md = %q, %v", text, err)
	}
	// Codex gets skills from its native installation.
	skill := filepath.Join(artifacts(t, f), "plugin", "skills", "review", "SKILL.md")
	if b, err := os.ReadFile(skill); s.Harness == "codex" && !os.IsNotExist(err) {
		t.Errorf("materialized skill for codex: %v", err)
	} else if s.Harness != "codex" && (err != nil || !strings.Contains(string(b), "Review.")) {
		t.Errorf("materialized skill = %q, %v", b, err)
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestBuildClaude(t *testing.T) {
	f := newFixture(t)
	plan, s, err := f.build(t, "claude", "-p", "hi")
	if err != nil {
		t.Fatal(err)
	}
	dir := artifacts(t, f)
	mcp := `{"mcpServers":{` +
		`"agx-team-remote":{"headers":{"Authorization":"Bearer ${AGX_MCP_GRANT}"},"type":"http","url":"http://127.0.0.1:9/launch/L/mcp/team%2Fremote"},` +
		`"agx-up":{"headers":{"Authorization":"Bearer ${AGX_MCP_GRANT}"},"type":"http","url":"http://127.0.0.1:9/launch/L/mcp/up"}}}`
	checkPlan(t, f, plan, s, []string{
		"--plugin-dir", filepath.Join(dir, "plugin"),
		"--append-system-prompt-file", filepath.Join(dir, "instructions.md"),
		"--mcp-config", mcp,
		"--definition", "-p", "hi",
	})
	if _, err := os.Stat(filepath.Join(dir, "plugin", ".claude-plugin", "plugin.json")); err != nil {
		t.Errorf("plugin manifest: %v", err)
	}
}

func TestBuildClaudeStreamInput(t *testing.T) {
	f := newFixture(t)
	for _, tt := range []struct {
		native []string
		prompt string
	}{
		{[]string{"-p", "hi"}, ""},
		{[]string{"--input-format", "stream-json", "--output-format", "stream-json"}, "first\n\nsecond"},
		{[]string{"--input-format=stream-json", "--resume=abc"}, "first\n\nsecond"},
	} {
		plan, _, err := f.build(t, "claude", tt.native...)
		if err != nil {
			t.Fatal(err)
		}
		if plan.InitializeSystemPrompt != tt.prompt {
			t.Errorf("%q: initialize prompt = %q, want %q", tt.native, plan.InitializeSystemPrompt, tt.prompt)
		}
		if !slices.Contains(plan.Arguments, "--append-system-prompt-file") {
			t.Errorf("%q: arguments lack the instructions file: %q", tt.native, plan.Arguments)
		}
	}
}

func TestBuildPi(t *testing.T) {
	f := newFixture(t)
	plan, s, err := f.build(t, "pi", "--mode", "rpc")
	if err != nil {
		t.Fatal(err)
	}
	dir := artifacts(t, f)
	extension := filepath.Join(f.cache, "launches", "L", "mcp-extension.mjs")
	checkPlan(t, f, plan, s, []string{
		"--skill", filepath.Join(dir, "plugin", "skills", "review"),
		"--append-system-prompt", filepath.Join(dir, "instructions.md"),
		"--extension", extension,
		"--definition", "--mode", "rpc",
	})
	b, err := os.ReadFile(extension)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"agx-up":{`, "/launch/L/mcp/up", "process.env.AGX_MCP_GRANT", "pi.registerMcpServer"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("extension lacks %q:\n%s", want, b)
		}
	}
}

func TestBuildCodex(t *testing.T) {
	f := newFixture(t)
	hash, err := f.config.Skills["review"].ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	selected := "review-" + hash[:12]

	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	// The fake executable cannot run, so the installation the launch starts fails.
	_, _, err = f.build(t, "codex")
	if err == nil ||
		!strings.Contains(err.Error(), "installing native codex plugins: native plugin installation failed") {
		t.Fatalf("without installed plugins: %v", err)
	}

	other := "agx-" + strings.Repeat("0", 24)
	installPlugins(t, home, other, selected)
	plan, s, err := f.build(t, "codex", "exec", "hi")
	if err != nil {
		t.Fatal(err)
	}
	checkPlan(t, f, plan, s, []string{
		"-c",
		`developer_instructions="first\n\nsecond\n\n"`,
		"-c",
		"plugins." + other + "@agx-managed.enabled=false",
		"-c",
		"plugins." + selected + "@agx-managed.enabled=true",
		"-c",
		`mcp_servers.agx-up={bearer_token_env_var = 'AGX_MCP_GRANT', url = 'http://127.0.0.1:9/launch/L/mcp/up'}`,
		"-c",
		`mcp_servers.agx-team-remote={bearer_token_env_var = 'AGX_MCP_GRANT', ` +
			`url = 'http://127.0.0.1:9/launch/L/mcp/team%2Fremote'}`,
		"--definition",
		"exec",
		"hi",
	})

	// A selected plugin removed with codex plugin remove is installed again.
	write(t, filepath.Join(home, "config.toml"), "[plugins.\""+other+"@agx-managed\"]\nenabled = true\n")
	if _, _, err := f.build(t, "codex", "exec", "hi"); err == nil ||
		!strings.Contains(err.Error(), "installing native codex plugins") {
		t.Errorf("after codex plugin remove: %v", err)
	}
}

func TestBuildCodexAppServer(t *testing.T) {
	f := newFixture(t)
	hash, err := f.config.Skills["review"].ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	selected := "review-" + hash[:12] + "@agx-managed"
	other := "agx-" + strings.Repeat("0", 24)
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	installPlugins(t, home, other, "review-"+hash[:12])
	// The process carries no snapshot resources; its threads get theirs.
	plan, s, err := f.build(t, "codex", "app-server")
	if err != nil {
		t.Fatal(err)
	}
	checkPlan(t, f, plan, s, []string{"--definition", "app-server"})
	// Other transports keep the per-process resources.
	if plan, _, err := f.build(t, "codex", "app-server", "--listen", "ws://127.0.0.1:1"); err != nil ||
		!slices.Contains(plan.Arguments, `developer_instructions="first\n\nsecond\n\n"`) {
		t.Errorf("websocket app-server arguments = %q, %v", plan.Arguments, err)
	}

	a, _ := builtin.Registry().Adapter("codex")
	threads := a.(adapters.Threads)
	for _, tt := range []struct {
		args []string
		want bool
	}{
		{[]string{"app-server"}, true},
		{[]string{"-c", "x=1", "app-server", "--stdio"}, true},
		{[]string{"app-server", "--listen", "stdio://"}, true},
		{[]string{"app-server", "--listen=ws://127.0.0.1:1"}, false},
		{[]string{"app-server", "--listen", "unix://"}, false},
		{[]string{"app-server", "generate-ts"}, false},
		{[]string{"exec", "hi"}, false},
	} {
		if got := threads.ThreadServer(tt.args); got != tt.want {
			t.Errorf("ThreadServer(%q) = %v", tt.args, got)
		}
	}
	thread, err := adapters.BuildThread(
		t.Context(),
		a,
		threads,
		adapters.Launch{Snapshot: s, Cache: f.cache, ID: "T", Endpoints: f.endpoints[:1]},
		"thread-grant",
	)
	if err != nil {
		t.Fatal(err)
	}
	config, err := json.Marshal(thread.Config)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"mcp_servers":{"agx-up":{"http_headers":{"Authorization":"Bearer thread-grant"},` +
		`"url":"http://127.0.0.1:9/launch/L/mcp/up"}},"plugins":{"` + other + `@agx-managed":{"enabled":false},"` +
		selected + `":{"enabled":true}},"shell_environment_policy.set.AGX_LAUNCH_ID":"T",` +
		`"shell_environment_policy.set.AGX_MCP_GRANT":"thread-grant"}`
	if string(config) != want {
		t.Errorf("thread config:\n got %s\nwant %s", config, want)
	}
	if thread.Instructions != "first\n\nsecond\n\n" ||
		!slices.Equal(thread.DisabledPlugins, []string{other + "@agx-managed"}) {
		t.Errorf("thread plan = %+v", thread)
	}
}

func TestBuildCodexPluginsDisabled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake Codex is a shell script")
	}
	f := newFixture(t)
	// Codex reports plugins off, and runs no other command.
	write(t, filepath.Join(filepath.Dir(f.work), "bin", "fake"), "#!/bin/sh\n"+
		`[ "$*" = "features list" ] && printf 'plugins  stable  false\n' || exit 1`+"\n")
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	plan, s, err := f.build(t, "codex", "exec", "hi")
	if err != nil {
		t.Fatal(err)
	}
	checkPlan(t, f, plan, s, []string{
		"-c",
		`developer_instructions="first\n\nsecond\n\n"`,
		"-c",
		`mcp_servers.agx-up={bearer_token_env_var = 'AGX_MCP_GRANT', url = 'http://127.0.0.1:9/launch/L/mcp/up'}`,
		"-c",
		`mcp_servers.agx-team-remote={bearer_token_env_var = 'AGX_MCP_GRANT', ` +
			`url = 'http://127.0.0.1:9/launch/L/mcp/team%2Fremote'}`,
		"--definition",
		"exec",
		"hi",
	})
	// The launch reports why plugins are off.
	want := "Codex plugins are disabled (features.plugins = false in " + filepath.Join(home, "config.toml") +
		" or the harness arguments); agx skills for Codex need them: set features.plugins = true, or pass -c features.plugins=true " +
		"in the harness definition's args; launching without skills"
	if !slices.Equal(plan.Warnings, []string{want}) {
		t.Errorf("warnings = %q, want %q", plan.Warnings, want)
	}

	// Installation reports the same reason.
	warnings, err := builtin.Registry().Install(t.Context(), f.config, f.cache, nil)
	if err != nil || len(warnings) != 1 || !strings.HasPrefix(warnings[0], "Codex plugins are disabled") {
		t.Errorf("install = %q, %v", warnings, err)
	}
}

// installPlugins installs plugins in home as Codex does.
func installPlugins(t *testing.T, home string, names ...string) {
	t.Helper()
	config := ""
	for _, name := range names {
		config += "[plugins.\"" + name + "@agx-managed\"]\nenabled = true\n"
		write(t, filepath.Join(home, "plugins", "cache", "agx-managed", name, "1.0.0", "README"), "")
	}
	write(t, filepath.Join(home, "config.toml"), config)
}

func TestSkillNames(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a", "SKILL.md"), "---\nname: lint\ndescription: a\n---\nA.\n")
	write(t, filepath.Join(dir, "b", "SKILL.md"), "---\r\nname: lint\r\ndescription: b\r\n---\r\nB.\r\n")
	write(t, filepath.Join(dir, "My Notes", "SKILL.md"), "no front matter\n")
	skills := map[config.ID]config.Skill{
		"team/a":    {Path: filepath.Join(dir, "a")},
		"team/b":    {Path: filepath.Join(dir, "b")},
		"notes":     {Path: filepath.Join(dir, "My Notes")},
		"generated": {Name: "deploy", Description: "d", Content: "Deploy."},
	}
	s := &config.Snapshot{Config: &config.Config{Skills: skills}}
	for _, id := range []config.ID{"team/a", "team/b", "notes", "generated"} {
		s.Resources = append(s.Resources, config.Resource{Kind: "skill", ID: id})
	}
	names := adapters.SkillNames(s)
	if names["notes"] != "my-notes" || names["generated"] != "deploy" {
		t.Errorf("names = %v", names)
	}
	if names["team/a"] == names["team/b"] || !strings.HasPrefix(names["team/a"], "lint-") ||
		!strings.HasPrefix(names["team/b"], "lint-") {
		t.Errorf("colliding names not separated: %q, %q", names["team/a"], names["team/b"])
	}
}

func TestServerNames(t *testing.T) {
	names := adapters.ServerNames([]adapters.Endpoint{
		{ID: "team/github"},
		{ID: "team.github"},
		{ID: "a-very-long-server-identifier-that-exceeds"},
		{ID: "plain"},
	})
	if names["plain"] != "agx-plain" {
		t.Errorf("plain = %q", names["plain"])
	}
	if names["team/github"] == names["team.github"] || !strings.HasPrefix(names["team/github"], "agx-team-github-") {
		t.Errorf("colliding names not separated: %q, %q", names["team/github"], names["team.github"])
	}
	for id, name := range names {
		if len(name) > 32 {
			t.Errorf("%s: %q is longer than 32 characters", id, name)
		}
	}
}

func TestArtifactsFollowContent(t *testing.T) {
	f := newFixture(t)
	pluginDir := func(directory string) string {
		t.Helper()
		s, err := f.config.Resolve(directory, "claude")
		if err != nil {
			t.Fatal(err)
		}
		h, err := f.config.Harness("claude")
		if err != nil {
			t.Fatal(err)
		}
		a, _ := builtin.Registry().Adapter("claude")
		plan, err := adapters.Build(
			t.Context(),
			a,
			adapters.Launch{Snapshot: s, Cache: f.cache, ID: "L"},
			h,
			nil,
			nil,
			"",
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		return plan.Arguments[slices.Index(plan.Arguments, "--plugin-dir")+1]
	}
	first := pluginDir(f.work)
	nested := filepath.Join(f.work, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	// Another directory selecting the same content shares the directory.
	if got := pluginDir(nested); got != first {
		t.Errorf("nested directory artifacts = %s, want %s", got, first)
	}
	// The launch directory names the artifacts its launch uses.
	if b, err := os.ReadFile(filepath.Join(f.cache, "launches", "L", "artifacts")); err != nil ||
		string(b) != filepath.Base(filepath.Dir(first)) {
		t.Errorf("launch's artifacts = %q, %v", b, err)
	}
	// A skill edited after resolution is not cached under the hash resolution found.
	s, err := f.config.Resolve(nested, "pi")
	if err != nil {
		t.Fatal(err)
	}
	write(
		t,
		filepath.Join(filepath.Dir(f.work), "skills", "review", "SKILL.md"),
		"---\nname: review\ndescription: r\n---\nChanged.\n",
	)
	h, _ := f.config.Harness("pi")
	a, _ := builtin.Registry().Adapter("pi")
	if _, err := adapters.Build(
		t.Context(),
		a,
		adapters.Launch{Snapshot: s, Cache: t.TempDir()},
		h,
		nil,
		nil,
		"",
		nil,
	); err == nil ||
		!strings.Contains(err.Error(), "changed while preparing launch") {
		t.Errorf("building artifacts of an edited skill: %v", err)
	}
	changed := pluginDir(f.work)
	if changed == first {
		t.Fatal("a changed skill kept its artifacts")
	}
	if b, err := os.ReadFile(filepath.Join(changed, "skills", "review", "SKILL.md")); err != nil ||
		!strings.Contains(string(b), "Changed.") {
		t.Errorf("changed skill = %q, %v", b, err)
	}
}

func TestRemoveUnusedArtifacts(t *testing.T) {
	cache := t.TempDir()
	now := time.Now()
	old := now.Add(-8 * 24 * time.Hour)
	dir := func(name string, at time.Time) string {
		t.Helper()
		path := filepath.Join(cache, "artifacts", name)
		write(t, filepath.Join(path, "instructions.md"), "")
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
		return path
	}
	used := dir("v3-used", old)
	unused := dir("v3-unused", old)
	recent := dir("v3-recent", now.Add(-time.Hour))
	legacy := dir("v2-legacy", old)
	build := dir(".build-1", now.Add(-2*time.Hour))
	building := dir(".build-2", now)
	write(t, filepath.Join(cache, "launches", "A", "artifacts"), "v3-used")
	exists := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}

	// A launch whose directory names no artifacts keeps them all.
	adapters.RemoveUnusedArtifacts(cache, []string{"A", "B"}, now)
	if !exists(unused) || !exists(legacy) {
		t.Fatal("removed artifacts while a launch's are unknown")
	}
	adapters.RemoveUnusedArtifacts(cache, []string{"A"}, now)
	for path, want := range map[string]bool{
		used: true, unused: false, recent: true, legacy: false, build: false, building: true,
	} {
		if exists(path) != want {
			t.Errorf("%s exists = %v, want %v", filepath.Base(path), !want, want)
		}
	}
	// Use is recorded, so the directory outlives the launch by the grace period.
	if info, err := os.Stat(used); err != nil || info.ModTime().Before(now.Add(-time.Minute)) {
		t.Errorf("used artifacts = %v, %v", info, err)
	}
}
