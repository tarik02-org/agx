package cli

import (
	"context"
	"os"
	"os/exec"
	"slices"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
)

func runClientHarness(
	ctx context.Context,
	paths platform.Paths,
	adapter adapters.ClientLauncher,
	harnesses harnessConfig,
	h config.ResolvedHarness,
	args []string,
) error {
	executable, err := adapter.ClientExecutable(h.Executable)
	if err != nil {
		return err
	}
	arguments := args
	environment := map[string]string{}
	if len(h.Arguments(adapter.Classify(args))) > 0 || len(h.Environment) > 0 {
		client, err := daemonClient(ctx, paths, autostart(harnesses.config))
		if err != nil {
			return err
		}
		inputs := map[string]string{}
		for _, injection := range h.Injections() {
			if name := harnesses.config.Variable(injection.Value); name != "" {
				if value, ok := os.LookupEnv(name); ok {
					inputs[name] = value
				}
			}
		}
		resolved, err := client.config.ResolveClientCommand(ctx, connect.NewRequest(&v1.ResolveClientCommandRequest{
			Harness: h.Name, NativeArguments: args, EnvironmentInputs: inputs,
		}))
		if err != nil {
			return err
		}
		arguments = slices.Concat(resolved.Msg.Arguments, args)
		environment = resolved.Msg.Environment
	}
	cmd := exec.CommandContext(ctx, executable, arguments...)
	cmd.Env = environ(environment, nil)
	return supervise(cmd, nil, nil)
}
