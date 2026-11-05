// Package opencode starts the native client. The plugin owns native APIs and resource leases.
package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tarik02-org/agx/internal/adapters"
)

type Adapter struct{}

func (Adapter) Name() string { return "opencode" }

func (Adapter) ClientExecutable(configured string) (string, error) {
	executable, err := adapters.FindExecutable("opencode2", configured)
	if err == nil || configured != "" {
		return executable, err
	}
	return adapters.FindExecutable("opencode", "")
}

func (Adapter) ClientArguments(args []string, _ string) ([]string, error) {
	for _, arg := range args {
		if arg == "--session" || arg == "-s" || strings.HasPrefix(arg, "--session=") ||
			(strings.HasPrefix(arg, "-s") && !strings.HasPrefix(arg, "--")) {
			return nil, fmt.Errorf(
				"resume OpenCode sessions through the agx launcher so the plugin validates the native session first",
			)
		}
	}
	return args, nil
}

func (Adapter) BootstrapSessions(ctx context.Context, executable, directory string) error {
	// A native session listing activates this location in the normal shared service without creating a session.
	if !filepath.IsAbs(directory) {
		return fmt.Errorf("OpenCode location cannot be bootstrapped locally")
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("OpenCode location is unavailable locally; open its native service before resuming")
	}
	cmd := exec.CommandContext(ctx, executable, "session", "list", "--format", "json", "--max-count", "1")
	cmd.Dir = directory
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cannot activate the OpenCode session provider: %w", err)
	}
	return nil
}

func (Adapter) ResumeClient(session adapters.Session, privateData string) (adapters.ClientInvocation, error) {
	var data struct {
		SessionID string  `json:"sessionId"`
		Directory string  `json:"directory"`
		Server    string  `json:"server"`
		Username  *string `json:"username"`
		Password  *string `json:"password"`
	}
	if err := json.Unmarshal(
		[]byte(privateData),
		&data,
	); err != nil || data.SessionID != session.ID || data.Directory != session.Directory || data.SessionID == "" ||
		data.Directory == "" {
		return adapters.ClientInvocation{}, fmt.Errorf("invalid OpenCode resume reply")
	}
	invocation := adapters.ClientInvocation{Arguments: []string{"--session", data.SessionID}}
	server, err := url.Parse(data.Server)
	if err != nil || (server.Scheme != "http" && server.Scheme != "https") || server.Host == "" || server.User != nil ||
		server.RawQuery != "" ||
		server.Fragment != "" {
		return adapters.ClientInvocation{}, fmt.Errorf("invalid OpenCode native server route")
	}
	invocation.Arguments = append(invocation.Arguments, "--server", data.Server, data.Directory)
	invocation.Environment = map[string]string{"OPENCODE_USERNAME": "", "OPENCODE_PASSWORD": ""}
	if data.Username != nil {
		invocation.Environment["OPENCODE_USERNAME"] = *data.Username
	}
	if data.Password != nil {
		invocation.Environment["OPENCODE_PASSWORD"] = *data.Password
	}

	return invocation, nil
}
