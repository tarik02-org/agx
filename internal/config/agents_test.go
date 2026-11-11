package config_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/resources"
)

func TestAgents(t *testing.T) {
	dir := t.TempDir()
	work := mkdir(t, filepath.Join(dir, "work"))
	mkdir(t, filepath.Join(dir, "other"))
	write(t, filepath.Join(dir, "agents", "reviewer.md"),
		"---\nname: unread\ndescription: Reviews a diff.\ntools: Read\n---\n\nYou review.\n")
	write(t, filepath.Join(dir, "agents", "planner.md"), "---\ndescription: Plans.\n---\nYou plan.\n")
	root := fragment(t, dir, `agents:
  reviewer:
    path: ./agents/reviewer.md
    harnesses:
      claude: {model: opus, tools: [Read]}
      codex: {model_reasoning_effort: high}
  team/scout:
    description: Finds files.
    content: You scout.
  other/scout:
    description: Finds more files.
    content: You scout further.
bundles:
  crew:
    agents: [team/scout, other/scout]
scopes:
  - directory: work
    add:
      agents: [reviewer, ./agents/planner.md]
      bundles: [crew]
harnesses:
  codex-work:
    base: codex
    remove: {agents: [planner]}
`)
	c := load(t, root)
	for _, tt := range []struct {
		harness string
		want    []resources.Agent
	}{
		{"claude", []resources.Agent{
			{ID: "other/scout", Name: "scout-" + config.Digest("other/scout")[:8], Description: "Finds more files.", Prompt: "You scout further."},
			{ID: "planner", Name: "planner", Description: "Plans.", Prompt: "You plan.\n"},
			{
				ID: "reviewer", Name: "reviewer", Description: "Reviews a diff.", Prompt: "You review.\n",
				Native: map[string]any{"model": "opus", "tools": []any{"Read"}},
			},
			{ID: "team/scout", Name: "scout-" + config.Digest("team/scout")[:8], Description: "Finds files.", Prompt: "You scout."},
		}},
		{"codex-work", []resources.Agent{
			{ID: "other/scout", Name: "scout-" + config.Digest("other/scout")[:8], Description: "Finds more files.", Prompt: "You scout further."},
			{
				ID: "reviewer", Name: "reviewer", Description: "Reviews a diff.", Prompt: "You review.\n",
				Native: map[string]any{"model_reasoning_effort": "high"},
			},
			{ID: "team/scout", Name: "scout-" + config.Digest("team/scout")[:8], Description: "Finds files.", Prompt: "You scout."},
		}},
	} {
		s, err := c.Resolve(work, tt.harness)
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.AgentsContent()
		if err != nil {
			t.Fatal(err)
		}
		if config.Digest(got) != config.Digest(tt.want) {
			t.Errorf("%s agents:\n got %+v\nwant %+v", tt.harness, got, tt.want)
		}
	}
	s, err := c.Resolve(filepath.Join(dir, "other"), "claude")
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(s.Resources, "agent"); len(got) != 0 {
		t.Errorf("agents outside the scope = %v", got)
	}

	// A subagent's prompt is part of the snapshot, and a change after resolving is caught.
	s, err = c.Resolve(work, "claude")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "agents", "planner.md"), "---\ndescription: Plans.\n---\nYou plan twice.\n")
	if _, err := s.AgentsContent(); err == nil {
		t.Error("changed subagent file went unnoticed")
	}
	changed, err := load(t, root).Resolve(work, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if changed.ID == s.ID {
		t.Error("snapshot ID ignores subagent content")
	}
	if !slices.Equal(ids(changed.Resources, "agent"), []string{"other/scout", "planner", "reviewer", "team/scout"}) {
		t.Errorf("agents = %v", ids(changed.Resources, "agent"))
	}
}

func TestAgentErrors(t *testing.T) {
	for _, tt := range []struct {
		name, yaml string
		want       []string
	}{
		{
			"unknown harness",
			"agents:\n  a: {description: d, content: c, harnesses: {cursor: {model: x}}}\n",
			[]string{"subagent a", "cursor is not a built-in harness"},
		},
		{
			"derived name",
			"agents:\n  Team_A: {description: d, content: c}\n",
			[]string{"subagent Team_A", "set name"},
		},
		{
			"file without description",
			"agents:\n  plain: {path: ./plain.md}\n",
			[]string{"subagent plain", "no description in its front matter"},
		},
		{
			"content without description",
			"agents:\n  a: {content: c}\n",
			[]string{"description"},
		},
		{
			"unknown reference",
			"scopes:\n  - add: {agents: [missing]}\n",
			[]string{`unknown subagent "missing"`},
		},
	} {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "plain.md"), "Just a prompt.\n")
		_, err := config.Load(
			fragment(t, dir, tt.yaml),
			config.Options{Builtins: builtins, Cache: filepath.Join(dir, "cache")},
		)
		t.Run(tt.name, func(t *testing.T) { wantError(t, err, tt.want...) })
	}
}
