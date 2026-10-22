package config_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tarik02-org/agx/internal/config"
)

func TestHarnessSettings(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "prompt.md"), "Be brief.")
	write(t, filepath.Join(dir, "more.yaml"), `harnesses:
  codex-cpa:
    settings:
      model_providers:
        cpa: {wire_api: responses}
`)
	root := write(t, filepath.Join(dir, "agx.yaml"), `imports: [{path: more.yaml}]
harnesses:
  codex:
    settings:
      model: o3
      sandbox_workspace_write: {network_access: true}
  codex-cpa:
    base: codex
    settings:
      model: null
      model_provider: cpa
      model_reasoning_effort: high
      notify: [say, done]
      instructions: !file prompt.md
      model_providers:
        cpa: {name: CPA, base_url: "http://127.0.0.1:8317/v1", max_retries: 3}
`)
	c, err := config.Load(root, filepath.Join(dir, "cache"), builtins)
	if err != nil {
		t.Fatal(err)
	}
	h, err := c.Harness("codex-cpa")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"model_provider":          "cpa",
		"model_reasoning_effort":  "high",
		"notify":                  []any{"say", "done"},
		"instructions":            "Be brief.",
		"sandbox_workspace_write": map[string]any{"network_access": true},
		"model_providers": map[string]any{"cpa": map[string]any{
			"name": "CPA", "base_url": "http://127.0.0.1:8317/v1", "max_retries": int64(3), "wire_api": "responses",
		}},
	}
	if !reflect.DeepEqual(h.Settings, want) {
		t.Fatalf("settings = %#v", h.Settings)
	}
	sources := map[string]string{}
	for _, s := range h.SettingSources {
		sources[config.FormatSettingPath(s.Path)] = filepath.Base(s.Source.Path)
	}
	if sources["sandbox_workspace_write.network_access"] != "agx.yaml" ||
		sources["model_providers.cpa.wire_api"] != "more.yaml" || len(sources) != 9 {
		t.Fatalf("sources = %v", sources)
	}
}

func TestHarnessSettingsErrors(t *testing.T) {
	for name, tt := range map[string]struct{ config, want string }{
		"secret": {
			"harnesses:\n  codex:\n    settings:\n      token: !secret t\n",
			"agx.yaml:4: !secret is not allowed in harness settings",
		},
		"env": {
			"harnesses:\n  claude:\n    settings:\n      env: {KEY: !env KEY}\n",
			"agx.yaml:4: !env is not allowed in harness settings",
		},
		"pi": {
			"harnesses:\n  pi-work:\n    base: pi\n    settings: {defaultModel: x}\n",
			"harness pi-work: pi takes no settings; pass its options in args instead",
		},
		"reserved codex server": {
			"harnesses:\n  codex:\n    settings:\n      mcp_servers: {agx-docs: {url: x}}\n",
			"settings mcp_servers.agx-docs: agx-* servers are agx's own",
		},
		"claude settings flag": {
			"harnesses:\n  claude:\n    args: [--settings, x.json]\n    settings: {model: opus}\n",
			"settings and --settings in args both set",
		},
		"not a map": {
			"harnesses:\n  codex:\n    settings: [a]\n",
			"settings must be a mapping",
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := write(t, filepath.Join(t.TempDir(), "agx.yaml"), tt.config)
			_, err := config.Load(root, filepath.Join(t.TempDir(), "cache"), builtins)
			wantError(t, err, tt.want)
		})
	}
	t.Run("fragment conflict", func(t *testing.T) {
		dir := t.TempDir()
		write(
			t,
			filepath.Join(dir, "a.yaml"),
			"harnesses:\n  codex:\n    settings: {model_providers: {cpa: {name: A}}}\n",
		)
		root := write(t, filepath.Join(dir, "agx.yaml"),
			"imports: [{path: a.yaml}]\nharnesses:\n  codex:\n    settings: {model_providers: {cpa: {name: B}}}\n")
		_, err := config.Load(root, filepath.Join(dir, "cache"), builtins)
		wantError(t, err, "settings model_providers.cpa.name already set at", "a.yaml:3")
	})
}
