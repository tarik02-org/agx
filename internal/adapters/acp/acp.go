// Package acp adapts any agent that speaks the Agent Client Protocol (https://agentclientprotocol.com)
// on standard input and output. agx relays the protocol between the client, such as an editor,
// and the agent, and gives each session the resources of its own directory through the session
// setup requests: MCP servers as HTTP servers, instructions as the session's system prompt.
package acp

import (
	"context"
	"fmt"
	"slices"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
)

const modeACP = "acp"

type Adapter struct{}

func (Adapter) Name() string { return "acp" }

func (Adapter) Modes() []string { return []string{modeACP, config.ModePassthrough} }

// Classify serves the protocol unless args probe the agent's help or version.
func (Adapter) Classify(args []string) string {
	if adapters.Probe(args) {
		return config.ModePassthrough
	}
	return modeACP
}

// Executable requires a configured path: ACP agents have no common executable name.
func (a Adapter) Executable(configured string) (string, error) {
	if configured == "" {
		return "", fmt.Errorf(
			"%w: define a harness with base: acp and the agent's executable",
			adapters.ErrNoExecutable,
		)
	}
	return adapters.FindExecutable(a.Name(), configured)
}

// Directory is the caller's: sessions name their own.
func (Adapter) Directory(args []string, cwd string) (string, []string, error) {
	return adapters.ResolveDirectory(args, cwd, cwd, nil)
}

// Arguments adds nothing: every session takes its resources in its setup request.
func (Adapter) Arguments(adapters.Launch) ([]string, error) { return []string{}, nil }

func (a Adapter) ThreadServer(args []string) bool { return a.Classify(args) == modeACP }

// ThreadPlan describes the launch's MCP servers as ACP HTTP servers authorized by grant.
func (Adapter) ThreadPlan(l adapters.Launch, grant string) (adapters.ThreadPlan, error) {
	plan := adapters.ThreadPlan{Instructions: string(l.Instructions)}
	names := adapters.ServerNames(l.Endpoints)
	servers := []any{}
	for _, endpoint := range l.Endpoints {
		servers = append(servers, map[string]any{
			"type":    "http",
			"name":    names[endpoint.ID],
			"url":     endpoint.URL,
			"headers": []any{map[string]any{"name": "Authorization", "value": "Bearer " + grant}},
		})
	}
	if len(servers) > 0 {
		plan.Config = map[string]any{"mcpServers": servers}
	}
	skills := []string{}
	for _, r := range l.Snapshot.Resources {
		if r.Kind == "skill" && !slices.Contains(skills, string(r.ID)) {
			skills = append(skills, string(r.ID))
		}
	}
	switch len(skills) {
	case 0:
	case 1:
		plan.Warnings = append(plan.Warnings, fmt.Sprintf(
			"ACP has no way to deliver skills; skill %s in %s is not applied", skills[0], l.Snapshot.Directory))
	default:
		plan.Warnings = append(plan.Warnings, fmt.Sprintf(
			"ACP has no way to deliver skills; %d skills in %s are not applied", len(skills), l.Snapshot.Directory))
	}
	return plan, nil
}

// Relay runs the ACP relay that hands each session its own resources.
func (Adapter) Relay(
	ctx context.Context,
	host adapters.ThreadHost,
	directory string,
	client, server adapters.Stream,
) error {
	return run(ctx, host, directory, client, server)
}
