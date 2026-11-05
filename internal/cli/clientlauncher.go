package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/sessions"
)

func validateClientDefinition(h config.ResolvedHarness) error {
	if h.Name != h.Builtin || len(h.Args) != 0 || len(h.Environment) != 0 || len(h.Settings) != 0 {
		return fmt.Errorf(
			"native %s clients use the plugin's location configuration; custom harness arguments, environments, settings, and resource profiles are unsupported",
			h.Builtin,
		)
	}
	return nil
}

func runClientHarness(
	ctx context.Context,
	adapter adapters.ClientLauncher,
	h config.ResolvedHarness,
	args []string,
) error {
	if err := validateClientDefinition(h); err != nil {
		return err
	}
	executable, err := adapter.ClientExecutable(h.Executable)
	if err != nil {
		return err
	}
	directory, err := os.Getwd()
	if err != nil {
		return err
	}
	arguments, err := adapter.ClientArguments(args, directory)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, executable, arguments...)
	cmd.Env = guardEnviron(os.Environ())
	return supervise(cmd, nil, nil)
}

func resumeProviderSession(
	ctx context.Context,
	paths platform.Paths,
	registry adapters.Registry,
	harnesses harnessConfig,
	session sessions.Entry,
	name string,
) error {
	h, err := harnesses.config.Harness(name)
	if err != nil {
		return err
	}
	if err := validateClientDefinition(h); err != nil {
		return err
	}
	adapter, ok := registry.ClientLauncher(h.Builtin)
	if !ok || h.Builtin != session.Builtin {
		return fmt.Errorf("harness cannot resume this provider session")
	}
	executable, err := adapter.ClientExecutable(h.Executable)
	if err != nil {
		return err
	}
	client, err := daemonClient(ctx, paths, autostart(harnesses.config))
	if err != nil {
		return err
	}
	prepare := func() (*connect.Response[v1.PrepareSessionResponse], error) {
		return client.providers.PrepareSession(
			ctx,
			connect.NewRequest(
				&v1.PrepareSessionRequest{
					SourceId:        session.SourceID,
					ScopeId:         session.ScopeID,
					NativeSessionId: session.ID,
				},
			),
		)
	}
	response, err := prepare()
	if connect.CodeOf(err) == connect.CodeUnavailable {
		bootstrapCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		bootstrapErr := adapter.BootstrapSessions(bootstrapCtx, executable, session.Directory)
		cancel()
		if bootstrapErr != nil {
			return bootstrapErr
		}
		// Activation finishes before the plugin's scoped command stream is necessarily online.
		deadline := time.Now().Add(5 * time.Second)
		for {
			response, err = prepare()
			if connect.CodeOf(err) != connect.CodeUnavailable || time.Now().After(deadline) {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	if err != nil {
		return fmt.Errorf("cannot validate native session: %w", err)
	}
	invocation, err := adapter.ResumeClient(
		adapters.Session{ID: response.Msg.Session.Id, Directory: response.Msg.Session.Directory},
		response.Msg.LaunchDataJson,
	)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, executable, invocation.Arguments...)
	cmd.Dir = invocation.Directory
	cmd.Env = os.Environ()
	for name, value := range invocation.Environment {
		cmd.Env = append(cmd.Env, name+"="+value)
	}
	cmd.Env = guardEnviron(cmd.Env)
	return supervise(cmd, nil, nil)
}
