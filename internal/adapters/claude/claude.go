// Package claude adapts Claude Code: skills load from a session plugin directory, instructions
// from an appended system prompt file, and MCP servers from inline --mcp-config.
package claude

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
)

const (
	modeInteractive = "interactive"
	modePrint       = "print"
	modeResume      = "resume"
)

type Adapter struct{}

func (Adapter) Name() string { return "claude" }

func (Adapter) Modes() []string {
	return []string{modeInteractive, modePrint, modeResume, config.ModePassthrough}
}

func (a Adapter) Executable(configured string) (string, error) {
	return adapters.FindExecutable(a.Name(), configured)
}

func (Adapter) Classify(args []string) string {
	if adapters.Probe(args, "-v") {
		return config.ModePassthrough
	}
	if len(args) > 0 {
		switch args[0] {
		case "auth", "plugin", "plugins", "mcp", "doctor", "install", "update", "upgrade", "setup-token", "help",
			"agents", "attach", "auto-mode", "gateway", "import", "logs", "purge", "respawn", "rm", "stop", "kill",
			"ultrareview":
			return config.ModePassthrough
		}
	}
	mode := modeInteractive
	for _, a := range args {
		if a == "--" {
			break
		}
		name, _, _ := strings.Cut(a, "=")
		switch name {
		case "-p", "--print":
			// Print output and its flags apply even when a print run resumes a session.
			return modePrint
		case "-r", "--resume", "-c", "--continue", "--from-pr", "--teleport":
			mode = modeResume
		}
	}
	return mode
}

// Directory is always the process directory: Claude has no working-directory flag.
func (Adapter) Directory(args []string, cwd string) (string, []string, error) {
	return adapters.ResolveDirectory(args, cwd, cwd, nil)
}

func (Adapter) Arguments(l adapters.Launch) ([]string, error) {
	prefix := []string{"--plugin-dir", filepath.Join(l.Artifacts, "plugin")}
	if len(l.Instructions) > 0 {
		prefix = append(prefix, "--append-system-prompt-file", l.InstructionsFile())
	}
	if servers := l.HTTPServers(); len(servers) > 0 {
		b, err := json.Marshal(map[string]any{"mcpServers": servers})
		if err != nil {
			return nil, err
		}
		prefix = append(prefix, "--mcp-config", string(b))
	}
	return prefix, nil
}

// Install does nothing: the session plugin directory needs no registration.
func (Adapter) Install(context.Context, *config.Config, string) error { return nil }
