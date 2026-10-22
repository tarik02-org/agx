package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
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
imports:
  - glob: agx.d/*.yaml
`

// editorServer serves an isolated configuration holding a local skill and instructions.
func editorServer(t *testing.T, configuration string) (*Server, string) {
	t.Helper()
	dir, err := config.Canonical(t.TempDir())
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
	writeFile(t, filepath.Join(dir, "agx.yaml"), configuration)
	paths := platform.Paths{
		Config: filepath.Join(dir, "agx.yaml"),
		Cache:  filepath.Join(dir, "cache"),
		State:  filepath.Join(dir, "state"),
	}
	return &Server{paths: paths, builtins: config.Builtins{}}, dir
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

// reason returns the ErrorDetail reason and code of an RPC error.
func reason(t *testing.T, err error) (connect.Code, v1.ErrorDetail_Reason) {
	t.Helper()
	var rpc *connect.Error
	if !errors.As(err, &rpc) {
		t.Fatalf("not an RPC error: %v", err)
	}
	for _, d := range rpc.Details() {
		if value, err := d.Value(); err == nil {
			if detail, ok := value.(*v1.ErrorDetail); ok {
				return rpc.Code(), detail.Reason
			}
		}
	}
	t.Fatalf("no reason: %v", err)
	return 0, 0
}

func wantReason(t *testing.T, err error, want v1.ErrorDetail_Reason) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error, want %v", want)
	}
	if _, got := reason(t, err); got != want {
		t.Fatalf("%v: reason %v, want %v", err, got, want)
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
	if alpha.Yaml != "path: ./skills/alpha # local\n" || alpha.Description != "Alpha skill." ||
		alpha.Path != filepath.Join(dir, "skills", "alpha") || alpha.ReadOnly || alpha.Package != "" ||
		alpha.Source.Path != s.paths.Config || alpha.Source.Line != 2 {
		t.Fatalf("alpha = %+v", alpha)
	}
	if docs := byKey["mcp/docs"]; docs.Description != "Library docs." ||
		!strings.Contains(docs.Yaml, "url: https://example.com/mcp") {
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
	wantReason(t, err, v1.ErrorDetail_REASON_NOT_FOUND)
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
	if got, err := read(
		s.paths.Config,
	); err != nil || got.Content != strings.Replace(editorConfig, "skills:\n", "skills:\n  cached:\n    path: ./cache/kit/skill\n", 1) ||
		got.ReadOnly {
		t.Fatalf("fragment = %+v, %v", got, err)
	}
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
		wantReason(t, err, v1.ErrorDetail_REASON_INVALID_REQUEST)
	}
	_, err := read(filepath.Join(dir, "skills", "alpha", "missing.md"))
	wantReason(t, err, v1.ErrorDetail_REASON_NOT_FOUND)
}

func define(kind, id, yaml string) *v1.ConfigChange {
	return &v1.ConfigChange{Change: &v1.ConfigChange_Define{Define: &v1.DefineChange{Kind: kind, Id: id, Yaml: yaml}}}
}

func enable(kind, id string) *v1.ConfigChange {
	return &v1.ConfigChange{Change: &v1.ConfigChange_Enable{Enable: &v1.SelectionChange{Kind: kind, Id: id}}}
}

func TestPlanAndApplyChanges(t *testing.T) {
	s, dir := editorServer(t, editorConfig)
	changes := []*v1.ConfigChange{
		define("mcp", "search", "url: https://example.com/search\n"),
		enable("mcp", "search"),
		{Change: &v1.ConfigChange_WriteFile{WriteFile: &v1.FileWrite{
			Path:    filepath.Join(dir, "skills", "alpha", "SKILL.md"),
			Content: "---\nname: alpha\ndescription: Edited.\n---\n",
		}}},
	}
	planned, err := s.PlanChanges(t.Context(), connect.NewRequest(&v1.PlanChangesRequest{Changes: changes}))
	if err != nil {
		t.Fatal(err)
	}
	files := planned.Msg.Files
	sum := sha256.Sum256([]byte(editorConfig))
	if len(files) != 2 || files[0].Path != s.paths.Config || files[0].BeforeSha256 != hex.EncodeToString(sum[:]) ||
		!strings.Contains(files[0].Diff, "+  search:") || !strings.Contains(files[0].Diff, "+      mcp: [search]") ||
		!strings.Contains(files[1].Diff, "+description: Edited.") {
		t.Fatalf("plan = %+v", files)
	}
	if b, _ := os.ReadFile(s.paths.Config); string(b) != editorConfig {
		t.Fatal("planning wrote the configuration")
	}

	// A file changed since the plan fails the apply.
	stale := []*v1.PlannedFile{{Path: files[0].Path, BeforeSha256: strings.Repeat("0", 64)}, files[1]}
	_, err = s.ApplyChanges(t.Context(), connect.NewRequest(&v1.ApplyChangesRequest{Changes: changes, Expected: stale}))
	wantReason(t, err, v1.ErrorDetail_REASON_CONFLICT)

	applied, err := s.ApplyChanges(
		t.Context(),
		connect.NewRequest(&v1.ApplyChangesRequest{Changes: changes, Expected: files}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied.Msg.Written) != 2 {
		t.Fatalf("written = %v", applied.Msg.Written)
	}
	c, err := s.reload()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.MCP["search"]; !ok {
		t.Fatal("the daemon did not reload the applied configuration")
	}

	// Replaying the applied plan finds the files changed.
	_, err = s.ApplyChanges(t.Context(), connect.NewRequest(&v1.ApplyChangesRequest{Changes: changes, Expected: files}))
	wantReason(t, err, v1.ErrorDetail_REASON_CONFLICT)

	undefine := &v1.ConfigChange{
		Change: &v1.ConfigChange_Undefine{Undefine: &v1.UndefineChange{Kind: "skills", Id: "alpha"}},
	}
	planned, err = s.PlanChanges(
		t.Context(),
		connect.NewRequest(&v1.PlanChangesRequest{Changes: []*v1.ConfigChange{undefine}}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(planned.Msg.Files) != 1 || !strings.Contains(planned.Msg.Files[0].Diff, "-    skills: [alpha]") {
		t.Fatalf("undefine plan = %+v", planned.Msg.Files)
	}

	for name, test := range map[string]struct {
		change *v1.ConfigChange
		want   v1.ErrorDetail_Reason
	}{
		"invalid":     {define("mcp", "bad", "url: 5\ncommand: [x]\n"), v1.ErrorDetail_REASON_CONFIG_INVALID},
		"unknown tag": {define("mcp", "bad", "url: !shell whoami\n"), v1.ErrorDetail_REASON_INVALID_REQUEST},
		"sequence":    {define("mcp", "bad", "[a, b]\n"), v1.ErrorDetail_REASON_INVALID_REQUEST},
		"unknown":     {enable("skills", "missing"), v1.ErrorDetail_REASON_CONFIG_INVALID},
		"outside": {&v1.ConfigChange{Change: &v1.ConfigChange_WriteFile{WriteFile: &v1.FileWrite{
			Path: filepath.Join(dir, "elsewhere.md"),
		}}}, v1.ErrorDetail_REASON_INVALID_REQUEST},
		"empty": {&v1.ConfigChange{}, v1.ErrorDetail_REASON_INVALID_REQUEST},
	} {
		_, err := s.PlanChanges(
			t.Context(),
			connect.NewRequest(&v1.PlanChangesRequest{Changes: []*v1.ConfigChange{test.change}}),
		)
		if err == nil {
			t.Fatalf("%s: planned", name)
		}
		if _, got := reason(t, err); got != test.want {
			t.Errorf("%s: %v: reason %v, want %v", name, err, got, test.want)
		}
	}
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
	s, _ := editorServer(t, editorConfig)
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
	if b, _ := os.ReadFile(s.paths.Config); string(b) != editorConfig {
		t.Fatal("previewing changed the configuration")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(s.paths.Config), "agx.lock.json")); !os.IsNotExist(err) {
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
		wantReason(t, err, v1.ErrorDetail_REASON_INVALID_REQUEST)
	}
	_, err = readPreview("gamma", "SKILL.md")
	wantReason(t, err, v1.ErrorDetail_REASON_NOT_FOUND)
	_, err = s.ReadPreviewFile(t.Context(), connect.NewRequest(&v1.ReadPreviewFileRequest{PreviewId: "expired"}))
	wantReason(t, err, v1.ErrorDetail_REASON_NOT_FOUND)

	// Adding the previewed package with one of its skills declares, locks, and enables it.
	changes := []*v1.ConfigChange{
		{Change: &v1.ConfigChange_AddPackage{AddPackage: msg.Package}},
		enable("skills", "kit/alpha"),
	}
	planned, err := s.PlanChanges(t.Context(), connect.NewRequest(&v1.PlanChangesRequest{Changes: changes}))
	if err != nil {
		t.Fatal(err)
	}
	if len(planned.Msg.Files) != 2 || !strings.HasSuffix(planned.Msg.Files[1].Path, "agx.lock.json") ||
		planned.Msg.Files[1].BeforeSha256 != "" || !strings.Contains(planned.Msg.Files[0].Diff, "+      skills: [kit/alpha]") {
		t.Fatalf("plan = %+v", planned.Msg.Files)
	}
	if _, err := s.ApplyChanges(t.Context(), connect.NewRequest(&v1.ApplyChangesRequest{
		Changes: changes, Expected: planned.Msg.Files,
	})); err != nil {
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
