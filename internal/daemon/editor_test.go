package daemon

import (
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
)

const editorConfig = `skills:
  alpha:
    path: ./skills/alpha # local
instructions:
  agents:
    path: ./AGENTS.md
mcp:
  docs:
    url: https://example.com/mcp
    description: Library docs.
secrets:
  token:
    kind: keychain
bundles:
  dev:
    skills: [alpha]
`

// editorServer serves an isolated configuration holding a local skill and instructions, defined
// in main.yaml beside the root, which also lists agx.d/*.yaml.
func editorServer(t *testing.T, configuration string) (*Server, string) {
	t.Helper()
	dir, err := platform.Canonical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(dir, "home"))
	writeFile(
		t,
		filepath.Join(dir, "skills", "alpha", "SKILL.md"),
		"---\nname: alpha\ndescription: Alpha skill.\n---\nBody.\n",
	)
	writeFile(t, filepath.Join(dir, "skills", "alpha", "scripts", "run.sh"), "echo\n")
	writeFile(t, filepath.Join(dir, "skills", "alpha", ".git", "HEAD"), "ref\n")
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "Be brief.\n")
	writeFile(t, filepath.Join(dir, "agx.yaml"), "fragments: [main.yaml, agx.d/*.yaml]\n")
	writeFile(t, filepath.Join(dir, "main.yaml"), configuration)
	paths := platform.Paths{
		Config: filepath.Join(dir, "agx.yaml"),
		Cache:  filepath.Join(dir, "cache"),
		State:  filepath.Join(dir, "state"),
	}
	return &Server{paths: paths, builtins: config.Builtins{}, manager: auth.New("", nil)}, dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func wantCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	if got := connect.CodeOf(err); got != want {
		t.Fatalf("%v: code %v, want %v", err, got, want)
	}
}

func TestListDefinitions(t *testing.T) {
	s, dir := editorServer(t, editorConfig)
	writeFile(t, filepath.Join(dir, "agx.d", "10-fixed.yaml"), "mcp:\n  fixed:\n    command: [fixed]\n")
	if os.Geteuid() != 0 {
		if err := os.Chmod(filepath.Join(dir, "agx.d", "10-fixed.yaml"), 0o444); err != nil {
			t.Fatal(err)
		}
	}
	response, err := s.ListDefinitions(t.Context(), connect.NewRequest(&v1.ListDefinitionsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]*v1.Definition{}
	order := []string{}
	for _, d := range response.Msg.Definitions {
		byKey[d.Kind+"/"+d.Id] = d
		order = append(order, d.Kind+"/"+d.Id)
	}
	want := []string{"skills/alpha", "instructions/agents", "mcp/docs", "mcp/fixed", "secrets/token", "bundles/dev"}
	if !slices.Equal(order, want) {
		t.Fatalf("definitions = %v, want %v", order, want)
	}
	alpha := byKey["skills/alpha"]
	if alpha.Description != "Alpha skill." ||
		alpha.Path != filepath.Join(dir, "skills", "alpha") || !alpha.ReadOnly || alpha.Package != "" ||
		alpha.Source.Path != filepath.Join(dir, "main.yaml") || alpha.Source.Line != 2 {
		t.Fatalf("alpha = %+v", alpha)
	}
	if docs := byKey["mcp/docs"]; docs.Description != "Library docs." ||
		!docs.ReadOnly {
		t.Fatalf("docs = %+v", docs)
	}
	if agents := byKey["instructions/agents"]; agents.Path != filepath.Join(dir, "AGENTS.md") {
		t.Fatalf("agents = %+v", agents)
	}
	if fixed := byKey["mcp/fixed"]; os.Geteuid() != 0 && !fixed.ReadOnly {
		t.Fatalf("a definition in a read-only file is editable: %+v", fixed)
	}

	skill, err := s.GetSkill(t.Context(), connect.NewRequest(&v1.GetSkillRequest{SkillId: "alpha"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(skill.Msg.SkillMd, "Body.") ||
		!slices.Equal(skill.Msg.Files, []string{"SKILL.md", "scripts/run.sh"}) {
		t.Fatalf("skill = %+v", skill.Msg)
	}
	_, err = s.GetSkill(t.Context(), connect.NewRequest(&v1.GetSkillRequest{SkillId: "missing"}))
	wantCode(t, err, connect.CodeNotFound)
}

func TestReadFileRestrictions(t *testing.T) {
	s, dir := editorServer(
		t,
		strings.Replace(editorConfig, "skills:\n", "skills:\n  cached:\n    path: ./cache/kit/skill\n", 1),
	)
	writeFile(t, filepath.Join(dir, "cache", "kit", "skill", "SKILL.md"), "---\nname: cached\n---\n")
	writeFile(t, filepath.Join(dir, "secret.txt"), "private\n")
	writeFile(t, filepath.Join(dir, "skills", "alpha", "blob.bin"), "\xff\xfe\x00")
	read := func(path string) (*v1.ReadFileResponse, error) {
		response, err := s.ReadFile(t.Context(), connect.NewRequest(&v1.ReadFileRequest{Path: path}))
		if err != nil {
			return nil, err
		}
		return response.Msg, nil
	}
	_, err := read(filepath.Join(dir, "main.yaml"))
	wantCode(t, err, connect.CodeInvalidArgument)
	if got, err := read(
		filepath.Join(dir, "skills", "alpha", "scripts", "run.sh"),
	); err != nil || got.Content != "echo\n" ||
		got.ReadOnly {
		t.Fatalf("skill file = %+v, %v", got, err)
	}
	if got, err := read(filepath.Join(dir, "AGENTS.md")); err != nil || got.Content != "Be brief.\n" {
		t.Fatalf("instructions = %+v, %v", got, err)
	}
	if got, err := read(
		filepath.Join(dir, "skills", "alpha", "blob.bin"),
	); err != nil || !got.Binary ||
		got.Content != "" {
		t.Fatalf("binary = %+v, %v", got, err)
	}
	if got, err := read(filepath.Join(dir, "cache", "kit", "skill", "SKILL.md")); err != nil || !got.ReadOnly {
		t.Fatalf("cached skill file = %+v, %v", got, err)
	}
	for _, path := range []string{
		filepath.Join(dir, "secret.txt"),
		filepath.Join(dir, "skills", "alpha", "..", "..", "secret.txt"),
		"skills/alpha/SKILL.md",
	} {
		_, err := read(path)
		wantCode(t, err, connect.CodeInvalidArgument)
	}
	_, err = read(filepath.Join(dir, "skills", "alpha", "missing.md"))
	wantCode(t, err, connect.CodeNotFound)
}

func define(kind, id string, value any) *v1.ConfigChange {
	parsed, err := structpb.NewValue(value)
	if err != nil {
		panic(err)
	}
	return &v1.ConfigChange{
		Change: &v1.ConfigChange_Define{Define: &v1.DefineChange{Kind: kind, Id: id, Value: parsed}},
	}
}

func enable(kind config.Kind, id string) *v1.ConfigChange {
	return &v1.ConfigChange{Change: &v1.ConfigChange_Enable{Enable: &v1.SelectionChange{Kind: string(kind), Id: id}}}
}

func TestUpdateConfiguration(t *testing.T) {
	s, dir := editorServer(t, editorConfig)
	changes := []*v1.ConfigChange{
		define("mcp", "search", map[string]any{"command": []any{"search"}}),
		enable("mcp", "search"),
	}
	update := func(revision int64, changes []*v1.ConfigChange) (*v1.UpdateConfigurationResponse, error) {
		response, err := s.UpdateConfiguration(
			t.Context(),
			connect.NewRequest(&v1.UpdateConfigurationRequest{Changes: changes, ExpectedRevision: revision}),
		)
		if err != nil {
			return nil, err
		}
		return response.Msg, nil
	}
	got, err := update(0, changes)
	if err != nil || got.Revision != 1 {
		t.Fatalf("update: %+v %v", got, err)
	}
	_, err = update(0, []*v1.ConfigChange{define("instructions", "stale", map[string]any{"text": "stale"})})
	wantCode(t, err, connect.CodeAborted)
	_, err = update(
		1,
		[]*v1.ConfigChange{
			define("instructions", "valid", map[string]any{"text": "valid"}),
			define("mcp", "bad", map[string]any{"url": 5}),
		},
	)
	wantCode(t, err, connect.CodeFailedPrecondition)
	c, err := s.reload()
	if err != nil || len(c.Instructions) != 1 || len(c.MCP) != 2 || c.Local.Revision != 1 {
		t.Fatalf("rollback: %+v %v", c, err)
	}
	_, err = update(1, []*v1.ConfigChange{define("mcp", "docs", map[string]any{"command": []any{"changed"}})})
	wantCode(t, err, connect.CodeFailedPrecondition)
	if data, _ := os.ReadFile(filepath.Join(dir, "main.yaml")); string(data) != editorConfig {
		t.Fatal("changed YAML overlay")
	}
	definitions, err := s.ListDefinitions(t.Context(), connect.NewRequest(&v1.ListDefinitionsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range definitions.Msg.Definitions {
		if d.Id == "search" {
			found = true
			if d.ReadOnly || !d.Source.Local || d.Value.GetStructValue().Fields["command"] == nil {
				t.Fatalf("local definition: %+v", d)
			}
		}
	}
	if !found {
		t.Fatal("local definition not listed")
	}
	// A fresh daemon reads the same definitions and revision.
	replacement := &Server{paths: s.paths, builtins: s.builtins, manager: auth.New("", nil)}
	restored, err := replacement.GetConfiguration(t.Context(), connect.NewRequest(&v1.GetConfigurationRequest{}))
	if err != nil || restored.Msg.Revision != 1 {
		t.Fatalf("restart: %+v %v", restored, err)
	}
}

func TestWriteResourceFile(t *testing.T) {
	s, dir := editorServer(t, editorConfig)
	path := filepath.Join(dir, "AGENTS.md")
	_, err := s.WriteResourceFile(
		t.Context(),
		connect.NewRequest(&v1.WriteResourceFileRequest{Path: path, Content: "Updated."}),
	)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "Updated." {
		t.Fatalf("resource: %s %v", data, err)
	}
	_, err = s.WriteResourceFile(
		t.Context(),
		connect.NewRequest(&v1.WriteResourceFileRequest{Path: filepath.Join(dir, "main.yaml"), Content: "{}"}),
	)
	wantCode(t, err, connect.CodeInvalidArgument)
}

// skillRepository commits a repository holding the skills alpha and beta.
func skillRepository(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := filepath.Join(t.TempDir(), "kit.git")
	for _, name := range []string{"alpha", "beta"} {
		writeFile(
			t,
			filepath.Join(dir, "skills", name, "SKILL.md"),
			"---\nname: "+name+"\ndescription: "+name+" skill\n---\n",
		)
	}
	writeFile(t, filepath.Join(dir, "skills", "alpha", "reference.md"), "Reference.\n")
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "."},
		{"-c", "user.name=agx", "-c", "user.email=agx@example.com", "commit", "-q", "-m", "skills"},
	} {
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestPreviewSource(t *testing.T) {
	repo := skillRepository(t)
	s, dir := editorServer(t, editorConfig)
	main := filepath.Join(dir, "main.yaml")
	url := "file://" + filepath.ToSlash(repo)
	preview, err := s.PreviewSource(t.Context(), connect.NewRequest(&v1.PreviewSourceRequest{Source: url}))
	if err != nil {
		t.Fatal(err)
	}
	msg := preview.Msg
	if msg.Package.Id != "kit" || msg.Package.Git != url || len(msg.Skills) != 2 ||
		msg.Skills[0].Name != "alpha" || msg.Skills[0].Description != "alpha skill" ||
		!slices.Equal(msg.Skills[0].Files, []string{"SKILL.md", "reference.md"}) {
		t.Fatalf("preview = %+v", msg)
	}
	if b, _ := os.ReadFile(main); string(b) != editorConfig {
		t.Fatal("previewing changed the configuration")
	}
	if _, err := os.Stat(filepath.Join(dir, "agx.lock.json")); !os.IsNotExist(err) {
		t.Fatalf("previewing wrote a lockfile: %v", err)
	}

	readPreview := func(skill, path string) (*v1.ReadPreviewFileResponse, error) {
		response, err := s.ReadPreviewFile(t.Context(), connect.NewRequest(&v1.ReadPreviewFileRequest{
			PreviewId: msg.PreviewId, Skill: skill, Path: path,
		}))
		if err != nil {
			return nil, err
		}
		return response.Msg, nil
	}
	if got, err := readPreview("alpha", "reference.md"); err != nil || got.Content != "Reference.\n" {
		t.Fatalf("reference = %+v, %v", got, err)
	}
	for _, path := range []string{"../beta/SKILL.md", "/etc/passwd", ""} {
		_, err := readPreview("alpha", path)
		wantCode(t, err, connect.CodeInvalidArgument)
	}
	_, err = readPreview("gamma", "SKILL.md")
	wantCode(t, err, connect.CodeNotFound)
	_, err = s.ReadPreviewFile(t.Context(), connect.NewRequest(&v1.ReadPreviewFileRequest{PreviewId: "expired"}))
	wantCode(t, err, connect.CodeNotFound)

	// Adding the previewed package with one of its skills declares, locks, and enables it.
	changes := []*v1.ConfigChange{
		{Change: &v1.ConfigChange_AddPackage{AddPackage: msg.Package}},
		enable("skills", "kit/alpha"),
	}
	if _, err := s.UpdateConfiguration(
		t.Context(),
		connect.NewRequest(&v1.UpdateConfigurationRequest{Changes: changes}),
	); err != nil {
		t.Fatal(err)
	}
	c, err := s.reload()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Skills["kit/alpha"]; !ok {
		t.Fatalf("the added package's skills are not loaded: %v", c.Missing)
	}
}

func TestSkillInterface(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nicon")
	for _, tc := range []struct {
		name, metadata      string
		wantColor, wantIcon string
		outside             bool
	}{
		{
			name:      "declared",
			metadata:  "interface:\n  brand_color: \"#3B82F6\"\n  icon_small: ./assets/small.png\n",
			wantColor: "#3B82F6",
			wantIcon:  "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		},
		{name: "not a color", metadata: "interface:\n  brand_color: blue\n"},
		{name: "not an image", metadata: "interface:\n  icon_small: ./assets/small.txt\n"},
		{name: "outside the skill", metadata: "interface:\n  icon_small: ../outside.png\n", outside: true},
		{name: "no interface", metadata: "policy: {}\n"},
		{name: "not YAML", metadata: "interface: [\n"},
		{name: "no file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "skill")
			writeFile(t, filepath.Join(dir, "assets", "small.png"), string(png))
			writeFile(t, filepath.Join(dir, "assets", "small.txt"), "text")
			if tc.outside {
				writeFile(t, filepath.Join(parent, "outside.png"), string(png))
			}
			if tc.metadata != "" {
				writeFile(t, filepath.Join(dir, "agents", "openai.yaml"), tc.metadata)
			}
			color, icon := skillInterface(dir)
			if color != tc.wantColor || icon != tc.wantIcon {
				t.Fatalf("skillInterface = %q, %q; want %q, %q", color, icon, tc.wantColor, tc.wantIcon)
			}
		})
	}
}

func TestInstallFailureRedactsCredentials(t *testing.T) {
	got := installFailure(errors.New("clone https://user:ghp_secret@github.com/acme/skills: denied"))
	if strings.Contains(got, "ghp_secret") || !strings.Contains(got, "https://***@github.com/acme/skills") {
		t.Fatalf("installFailure = %q", got)
	}
}
