package builtin_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/adapters/builtin"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/securefs"
)

const grant = "grant-token"

type fixture struct {
	config    *config.Config
	work      string
	cache     string
	fake      string
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
	write(t, filepath.Join(dir, "agx.yaml"), `skills:
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
	c, err := config.Load(filepath.Join(dir, "agx.yaml"), filepath.Join(dir, "cache"), builtin.Registry().Builtins(nil))
	if err != nil {
		t.Fatal(err)
	}
	return fixture{
		config: c,
		work:   filepath.Join(dir, "work"),
		cache:  filepath.Join(dir, "cache"),
		fake:   fake,
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

func artifacts(f fixture, s *config.Snapshot) string {
	return filepath.Join(f.cache, "artifacts", "v2-"+s.ID)
}

func checkPlan(t *testing.T, f fixture, plan adapters.Plan, s *config.Snapshot, want []string) {
	t.Helper()
	if !slices.Equal(plan.Arguments, want) {
		t.Errorf("arguments:\n got %q\nwant %q", plan.Arguments, want)
	}
	if plan.Executable != f.fake {
		t.Errorf("executable = %q, want %q", plan.Executable, f.fake)
	}
	if plan.Directory != s.Directory {
		t.Errorf("directory = %q, want %q", plan.Directory, s.Directory)
	}
	if want := map[string]string{
		"DEFINITION":              "1",
		adapters.GrantEnvironment: grant,
	}; !mapsEqual(
		plan.Environment,
		want,
	) {
		t.Errorf("environment = %v, want %v", plan.Environment, want)
	}
	if want := []string{"AGX_MANAGEMENT_TOKEN", "AGX_LIFECYCLE_TOKEN", "MCP_ONLY"}; !slices.Equal(plan.Unset, want) {
		t.Errorf("unset = %v, want %v", plan.Unset, want)
	}
	text, err := os.ReadFile(filepath.Join(artifacts(f, s), "instructions.md"))
	if err != nil || string(text) != "first\n\nsecond\n\n" {
		t.Errorf("instructions.md = %q, %v", text, err)
	}
	skill := filepath.Join(artifacts(f, s), "plugin", "skills", "review", "SKILL.md")
	if b, err := os.ReadFile(skill); err != nil || !strings.Contains(string(b), "Review.") {
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
	dir := artifacts(f, s)
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

func TestBuildPi(t *testing.T) {
	f := newFixture(t)
	plan, s, err := f.build(t, "pi", "--mode", "rpc")
	if err != nil {
		t.Fatal(err)
	}
	dir := artifacts(f, s)
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
	hash, err := f.config.Skills["review"].TreeHash()
	if err != nil {
		t.Fatal(err)
	}
	selected := "review-" + hash[:12]

	t.Setenv("CODEX_HOME", t.TempDir())
	_, _, err = f.build(t, "codex")
	if err == nil || !strings.Contains(err.Error(), "selected skill plugin is not installed; run agx install") {
		t.Fatalf("without installed plugins: %v", err)
	}

	other := "agx-" + strings.Repeat("0", 24)
	catalog := map[string][]string{"names": {other, selected}}
	if err := securefs.WriteJSON(filepath.Join(f.cache, "native", "catalog.json"), catalog); err != nil {
		t.Fatal(err)
	}
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
		`mcp_servers.agx-up={"bearer_token_env_var" = "AGX_MCP_GRANT", "url" = "http://127.0.0.1:9/launch/L/mcp/up"}`,
		"-c",
		`mcp_servers.agx-team-remote={"bearer_token_env_var" = "AGX_MCP_GRANT", ` +
			`"url" = "http://127.0.0.1:9/launch/L/mcp/team%2Fremote"}`,
		"--definition",
		"exec",
		"hi",
	})
}

func TestBuildCodexPluginsDisabled(t *testing.T) {
	f := newFixture(t)
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	write(t, filepath.Join(home, "config.toml"), "model = \"x\"\n\n[features]\nplugins = false\n")
	plan, s, err := f.build(t, "codex", "exec", "hi")
	if err != nil {
		t.Fatal(err)
	}
	checkPlan(t, f, plan, s, []string{
		"-c",
		`developer_instructions="first\n\nsecond\n\n"`,
		"-c",
		`mcp_servers.agx-up={"bearer_token_env_var" = "AGX_MCP_GRANT", "url" = "http://127.0.0.1:9/launch/L/mcp/up"}`,
		"-c",
		`mcp_servers.agx-team-remote={"bearer_token_env_var" = "AGX_MCP_GRANT", ` +
			`"url" = "http://127.0.0.1:9/launch/L/mcp/team%2Fremote"}`,
		"--definition",
		"exec",
		"hi",
	})
	want := "Codex plugins are disabled (features.plugins = false in " + filepath.Join(home, "config.toml") +
		"); agx skills for Codex need them: set features.plugins = true; launching without skills"
	if !slices.Equal(plan.Warnings, []string{want}) {
		t.Errorf("warnings = %q, want %q", plan.Warnings, want)
	}

	// Installation reports the same reason instead of running Codex.
	warnings, err := builtin.Registry().Install(t.Context(), f.config, f.cache)
	if err != nil || len(warnings) != 1 || !strings.HasPrefix(warnings[0], "Codex plugins are disabled") {
		t.Errorf("install = %q, %v", warnings, err)
	}
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

func TestTOMLAssignment(t *testing.T) {
	got, err := adapters.TOMLAssignment("key", "quote \" and \\ and\nnewline")
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]string
	value := strings.TrimPrefix(got, "key=")
	// TOML basic strings use JSON-compatible escapes for these characters.
	if err := json.Unmarshal(
		[]byte(`{"key":`+value+`}`),
		&decoded,
	); err != nil ||
		decoded["key"] != "quote \" and \\ and\nnewline" {
		t.Fatalf("TOMLAssignment = %s (%v)", got, err)
	}
}
