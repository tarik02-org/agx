package daemoncmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/charmbracelet/x/term"

	"github.com/tarik02-org/agx/internal/adapters/builtin"
	"github.com/tarik02-org/agx/internal/control"
	"github.com/tarik02-org/agx/internal/daemon"
	"github.com/tarik02-org/agx/internal/platform"
)

func Main() int { return Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr) }

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	paths, err := platform.DefaultPaths()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "agxd:", err)
		return 1
	}
	flags := flag.NewFlagSet("agxd", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&paths.Config, "config", paths.Config, "root configuration file")
	flags.StringVar(&paths.Cache, "cache", paths.Cache, "package and artifact cache")
	version := flags.Bool("version", false, "print version")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "agxd takes no positional arguments")
		return 2
	}
	if *version {
		if _, err := fmt.Fprintln(stdout, "agxd version", control.Version); err != nil {
			return 1
		}
		return 0
	}
	if err := paths.Derive(); err != nil {
		_, _ = fmt.Fprintln(stderr, "agxd:", err)
		return 1
	}
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, platform.StopSignals(term.IsTerminal(os.Stdin.Fd()))...)
	defer signal.Stop(signals)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := make(chan struct{})
	go func() {
		select {
		case <-signals:
		case <-ctx.Done():
			return
		}
		close(stop)
		select {
		case <-signals:
			cancel()
		case <-ctx.Done():
		}
	}()
	registry := builtin.Registry()
	if err := daemon.Serve(ctx, stop, paths, registry, registry.Builtins(control.CommandNames)); err != nil {
		_, _ = fmt.Fprintln(stderr, "agxd:", err)
		return 1
	}
	return 0
}
