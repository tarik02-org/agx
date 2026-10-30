package config_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tarik02-org/agx/internal/config"
)

func TestLocalRelationalRoundTrip(t *testing.T) {
	dir := t.TempDir()
	root, state := filepath.Join(dir, "agx.yaml"), filepath.Join(dir, "state")
	local := config.EmptyLocal()
	const document = `{
  "skills":{"guide":{"name":"guide","description":"A guide","content":"Follow this guide.","frontmatter":{"allowed-tools":["Read"],"custom":{"$file":"literal-frontmatter"},"environment":{"$env":"literal-env"}},"files":{"empty.txt":{"content":""},"run.sh":{"content":"echo done","executable":true},"copy.txt":{"path":"source.txt"}}}},
  "instructions":{"policy":{"text":"Be precise.","order":-2}},
  "packages":{"kit":{"git":"https://example.com/kit.git","ref":"main","subdir":"resources"}},
  "secrets":{"credential":{"kind":"keychain"},"command":{"kind":"command","command":["get-token","--raw"]},"encrypted":{"kind":"sops","file":"secrets.yaml","key":["account","token.value"],"age_key":{"$env":"AGE_IDENTITY"}}},
  "mcp":{
   "stdio":{"command":["tool","serve"],"cwd":".","description":"Local tools","load":"on_demand","share":{"type":"directory","idle":"3m"},"environment":{"TOKEN":{"$secret":"credential"},"FILE":{"$file":"token.txt"},"NAME":{"$env":"NAME"}}},
   "http":{"url":"https://example.com/mcp","headers":{"X-Mode":"work"},"auth":{"kind":"oauth","issuer":"https://example.com","client_id":"agx","resource":"https://example.com/mcp","scopes":["read","write"]}}
  },
  "bundles":{"work":{"skills":["guide"],"instructions":[{"id":"inline-policy","text":"Inline policy."}],"mcp":["stdio","http"]}},
  "harnesses":{"work":{"base":"codex","hidden":false,"args":[{"values":["--model","example"],"modes":["interactive"]},{"values":[{"$file":"prompt.txt"}]}],"environment":{"MODE":"work"},"settings":{"model":"example","custom":{"$env":"literal-env"},"credential":{"$secret":"literal-secret"},"removed":null},"add":{"bundles":["work"]},"remove":{"mcp":["http"]}}},
  "scopes":[{"directory":".","harnesses":["work"],"add":{"bundles":["work"],"mcp":[{"id":"scoped","command":["scoped"],"environment":{"MODE":"local"}}]},"remove":{"skills":["guide"]}}]
 }`
	if err := json.Unmarshal([]byte(document), &local.Document); err != nil {
		t.Fatal(err)
	}
	settings := local.Document["harnesses"].(map[string]any)["work"].(map[string]any)["settings"].(map[string]any)
	nativeFile := filepath.Join(dir, "native.txt")
	if err := os.WriteFile(nativeFile, []byte("native setting"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings["filed"] = map[string]any{"$file": nativeFile}
	local.Lock.Packages["kit"] = config.LockedPackage{
		Source:       config.Package{Git: "https://example.com/kit.git", Ref: "main", Subdir: "resources"},
		Revision:     "commit",
		Hash:         "hash",
		Dependencies: []config.ID{"kit/dependency"},
	}
	before, err := config.Load(root, config.Options{Builtins: builtins, Local: local})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := config.WriteLocal(t.Context(), state, root, local)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := config.ReadLocal(t.Context(), state, root)
	if err != nil {
		t.Fatal(err)
	}
	if revision != 1 || persisted.Revision != 1 || len(persisted.Lock.Packages["kit"].Dependencies) != 1 ||
		persisted.Lock.Packages["kit"].Dependencies[0] != "kit/dependency" {
		t.Fatalf("persisted lock/revision = %+v", persisted)
	}
	after, err := config.Load(root, config.Options{Builtins: builtins, State: state})
	if err != nil {
		t.Fatal(err)
	}
	if config.Digest(before) != config.Digest(after) {
		a, _ := json.MarshalIndent(before, "", "  ")
		b, _ := json.MarshalIndent(after, "", "  ")
		t.Fatalf("configuration changed across persistence:\nbefore %s\nafter %s", a, b)
	}
	harness, err := after.Harness("work")
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range harness.SettingSources {
		if !setting.Source.Local || setting.Source.Line != 0 || setting.Source.Column != 0 {
			t.Fatalf("local setting %v has YAML provenance: %+v", setting.Path, setting.Source)
		}
	}
	exported, err := persisted.Export()
	if err != nil {
		t.Fatal(err)
	}
	overlay := fragment(t, t.TempDir(), string(exported))
	fromYAML, err := config.Load(overlay, config.Options{Builtins: builtins})
	if err != nil {
		t.Fatal(err)
	}
	if fromYAML.Skills["guide"].Frontmatter["custom"].(map[string]any)["$file"] != "literal-frontmatter" {
		t.Fatal("export reinterpreted frontmatter")
	}
	if fromYAML.Harnesses["work"].Settings["custom"].(map[string]any)["$env"] != "literal-env" {
		t.Fatal("export reinterpreted native JSON")
	}
	// Replacing a parent removes its old inline definitions and ordered children.
	persisted.Document["mcp"].(map[string]any)["http"] = map[string]any{
		"url":  "https://example.com/new",
		"auth": "none",
	}
	persisted.Document["bundles"].(map[string]any)["work"] = map[string]any{}
	persisted.Document["scopes"] = []any{}
	delete(persisted.Lock.Packages, "kit")
	if _, err := config.WriteLocal(t.Context(), state, root, persisted); err != nil {
		t.Fatal(err)
	}
	replacement, err := config.Load(root, config.Options{Builtins: builtins, State: state})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := replacement.MCP["scoped"]; found {
		t.Fatal("removed scope retained its inline server")
	}
	if _, found := replacement.Instructions["inline-policy"]; found {
		t.Fatal("replaced bundle retained its inline instructions")
	}
	if secret, found := replacement.Secrets["http"]; found {
		t.Fatalf("replaced auth retained its inline secret: %+v", secret)
	}
}

func TestLocalWriteFailureRollsBackDefinitionsAndRevision(t *testing.T) {
	dir := t.TempDir()
	root, state := filepath.Join(dir, "agx.yaml"), filepath.Join(dir, "state")
	local := config.EmptyLocal()
	local.Document["mcp"] = map[string]any{"server": map[string]any{"command": []string{"server"}}}
	if _, err := config.WriteLocal(t.Context(), state, root, local); err != nil {
		t.Fatal(err)
	}
	original, err := config.ReadLocal(t.Context(), state, root)
	if err != nil {
		t.Fatal(err)
	}
	invalid := original.Clone()
	invalid.Document["mcp"].(map[string]any)["server"] = map[string]any{
		"command":     []string{"changed"},
		"environment": map[string]any{"INVALID": 42},
	}
	if _, err := config.WriteLocal(t.Context(), state, root, invalid); err == nil {
		t.Fatal("invalid injected value was stored")
	}
	got, err := config.ReadLocal(t.Context(), state, root)
	if err != nil {
		t.Fatal(err)
	}
	if config.Digest(got) != config.Digest(original) {
		t.Fatal("failed write changed stored configuration or revision")
	}
	stale := original.Clone()
	stale.Revision = 0
	if _, err := config.WriteLocal(t.Context(), state, root, stale); !errors.Is(err, config.ErrConflict) {
		t.Fatalf("stale edit = %v", err)
	}
}

func TestLocalEmptyRequiredFields(t *testing.T) {
	for name, document := range map[string]string{
		"description": `{"skills":{"guide":{"content":"Follow this guide.","description":""}}}`,
		"content":     `{"skills":{"guide":{"content":"","description":""}}}`,
		"text":        `{"instructions":{"policy":{"text":""}}}`,
		"git path":    `{"instructions":{"policy":{"git":"https://example.com/kit.git","path":""}}}`,
		"local path":  `{"packages":{"kit":{"path":""}}}`,
		"sops file":   `{"secrets":{"token":{"kind":"sops","file":""}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			root, state := filepath.Join(dir, "agx.yaml"), filepath.Join(dir, "state")
			local := config.EmptyLocal()
			if err := json.Unmarshal([]byte(document), &local.Document); err != nil {
				t.Fatal(err)
			}
			before, err := config.Load(root, config.Options{Builtins: builtins, Local: local})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := config.WriteLocal(t.Context(), state, root, local); err != nil {
				t.Fatal(err)
			}
			after, err := config.Load(root, config.Options{Builtins: builtins, State: state})
			if err != nil {
				t.Fatal(err)
			}
			if config.Digest(before) != config.Digest(after) {
				t.Fatal("configuration changed across persistence")
			}
		})
	}
}

func TestLocalSettingErrorSource(t *testing.T) {
	dir := t.TempDir()
	root, state := filepath.Join(dir, "agx.yaml"), filepath.Join(dir, "state")
	local := config.EmptyLocal()
	local.Document["harnesses"] = map[string]any{"codex": map[string]any{
		"settings": map[string]any{"model": map[string]any{"$file": filepath.Join(dir, "missing")}},
	}}
	if _, err := config.WriteLocal(t.Context(), state, root, local); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(root, config.Options{Builtins: builtins, State: state})
	var source *config.SourceError
	if !errors.As(err, &source) || !source.Local || source.Line != 0 {
		t.Fatalf("local settings error has YAML provenance: %v", err)
	}
}
