// Package pi adapts Pi: skills and the appended system prompt are explicit paths, and MCP
// servers register through a generated per-launch extension.
package pi

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/securefs"
)

const (
	modeInteractive = "interactive"
	modePrint       = "print"
	modeRPC         = "rpc"
	modeResume      = "resume"
)

const extension = `export default function (pi) {
  const servers = %s;
  for (const [name, config] of Object.entries(servers)) {
    config.headers.Authorization = 'Bearer ' + process.env.%s;
    pi.registerMcpServer(name, {...config, exposure: 'direct'});
  }
}
`

type Adapter struct{}

func (Adapter) Name() string { return "pi" }

func (Adapter) Modes() []string {
	return []string{modeInteractive, modePrint, modeRPC, modeResume, config.ModePassthrough}
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
		case "install", "remove", "uninstall", "update", "list", "config", "auth", "mcp":
			return config.ModePassthrough
		}
	}
	mode := modeInteractive
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		name, value, inline := strings.Cut(a, "=")
		switch name {
		case "--list-models", "--export":
			return config.ModePassthrough
		case "-p", "--print":
			if mode != modeRPC {
				mode = modePrint
			}
		case "-c", "--continue", "-r", "--resume", "--session", "--fork":
			// Print and RPC runs keep their mode when they continue a session, as with Claude.
			if mode == modeInteractive {
				mode = modeResume
			}
		case "--mode":
			if !inline && i+1 < len(args) {
				i++
				value = args[i]
			}
			switch value {
			case "rpc":
				mode = modeRPC
			case "json":
				if mode != modeRPC {
					mode = modePrint
				}
			}
		}
	}
	return mode
}

// Directory is always the process directory: Pi has no working-directory flag.
func (Adapter) Directory(args []string, cwd string) (string, []string, error) {
	return adapters.ResolveDirectory(args, cwd, cwd, nil)
}

func (Adapter) Arguments(l adapters.Launch) ([]string, error) {
	prefix := []string{}
	for _, r := range l.Snapshot.Resources {
		if r.Kind == "skill" {
			prefix = append(
				prefix,
				"--skill",
				filepath.Join(l.Artifacts, "plugin", "skills", adapters.ArtifactName(r.ID)),
			)
		}
	}
	if len(l.Instructions) > 0 {
		prefix = append(prefix, "--append-system-prompt", l.InstructionsFile())
	}
	if servers := l.HTTPServers(); len(servers) > 0 {
		b, err := json.Marshal(servers)
		if err != nil {
			return nil, err
		}
		script := fmt.Sprintf(extension, b, adapters.GrantEnvironment)
		// Endpoint URLs belong to a launch, so this extension is never a shared artifact.
		launchDir := filepath.Join(l.Cache, "launches", l.ID)
		if err := securefs.EnsureDir(launchDir); err != nil {
			return nil, err
		}
		path := filepath.Join(launchDir, "mcp-extension.mjs")
		if err := securefs.Write(path, []byte(script), 0o600); err != nil {
			return nil, err
		}
		prefix = append(prefix, "--extension", path)
	}
	return prefix, nil
}

// Install does nothing: skills and the MCP extension are passed per launch.
func (Adapter) Install(context.Context, *config.Config, string) error { return nil }
