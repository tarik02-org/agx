package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/cli/launcher"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/sessions"
)

func sessionsCommand(paths *platform.Paths, registry adapters.Registry, harnesses *harnessConfig) *cobra.Command {
	all := false
	cmd := &cobra.Command{
		Use:     "sessions [query]",
		Short:   "List harness sessions to resume, here or in every directory",
		GroupID: "harness",
		RunE: func(cmd *cobra.Command, args []string) error {
			directory, err := workingDirectory()
			if err != nil {
				return err
			}
			listing, err := listSessions(cmd.Context(), *paths, registry, *harnesses)
			if err != nil {
				return err
			}
			entries := listing.Entries
			if !all {
				entries = sessions.Within(entries, directory)
			}
			query := strings.Join(args, " ")
			entries = sessions.Search(entries, query)
			if scripting.enabled {
				errs := map[string]string{}
				for name, err := range listing.Errors {
					errs[name] = err.Error()
				}
				scope := "here"
				if all {
					scope = "all"
				}
				items := []sessionJSON{}
				for _, e := range entries {
					items = append(items, sessionJSON{
						ID:        e.ID,
						Harness:   e.Harness,
						Builtin:   e.Builtin,
						Recorded:  e.Recorded,
						Directory: e.Directory,
						Title:     jsonString(e.Title),
						Message:   jsonString(e.Message),
						StartedAt: jsonTime(e.Started),
						UpdatedAt: jsonTime(e.Updated),
					})
				}
				return printJSON(sessionsJSON{
					Scope:     scope,
					Directory: directory,
					Query:     jsonString(query),
					Sessions:  items,
					Files:     sessionFilesJSON(listing.Stats),
					Errors:    errs,
				})
			}
			reportListing(listing)
			return printSessions(entries, directory, all, query != "")
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "list sessions of every directory, not only this one and below")
	jsonFlag(cmd)
	return cmd
}

func workingDirectory() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return config.Canonical(cwd)
}

// listSessions lists every store and resumes recorded sessions with their definition while it
// still exists for the same built-in; others resume with their built-in.
func listSessions(
	ctx context.Context,
	paths platform.Paths,
	registry adapters.Registry,
	harnesses harnessConfig,
) (sessions.Listing, error) {
	listing, err := sessions.List(ctx, registry, paths.Cache, paths.State)
	if err != nil {
		return sessions.Listing{}, err
	}
	for i := range listing.Entries {
		e := &listing.Entries[i]
		if h, err := harnesses.config.Harness(e.Harness); err != nil || h.Builtin != e.Builtin {
			e.Harness = e.Builtin
		}
	}
	return listing, nil
}

func reportListing(listing sessions.Listing) {
	for _, name := range slices.Sorted(maps.Keys(listing.Errors)) {
		ui.Diagnose(ui.Warn, "cannot list %s sessions: %v", name, listing.Errors[name])
	}
	if n := listing.Stats.Malformed; n > 0 && ui.Stderr.Terminal() {
		ui.Note(fmt.Sprintf("Skipped %s.", plural(n, "unreadable session file")))
	}
}

func listingNotes(listing sessions.Listing) []string {
	notes := []string{}
	for _, name := range slices.Sorted(maps.Keys(listing.Errors)) {
		notes = append(notes, fmt.Sprintf("Cannot list %s sessions: %v", name, listing.Errors[name]))
	}
	if n := listing.Stats.Malformed; n > 0 {
		notes = append(notes, fmt.Sprintf("Skipped %s.", plural(n, "unreadable session file")))
	}
	return notes
}

func printSessions(entries []sessions.Entry, directory string, all, searched bool) error {
	out := ui.Stdout
	if len(entries) == 0 {
		message := "No sessions."
		switch {
		case searched:
			message = "No matching sessions."
		case !all:
			message = "No sessions in this directory; agx sessions --all lists every directory."
		}
		_, err := fmt.Fprintln(out, out.Muted(message))
		return err
	}
	now := time.Now()
	headers := []string{"HARNESS", "UPDATED", "SESSION", "DIRECTORY", "TITLE"}
	rows := [][]string{}
	fixed := []int{len(headers[0]), len(headers[1]), len(headers[2]), len(headers[3])}
	for _, e := range entries {
		row := []string{e.Harness, ui.Ago(e.Updated, now), e.ID, ui.ShortPath(e.Directory, directory)}
		for i, cell := range row {
			fixed[i] = max(fixed[i], ansi.StringWidth(cell))
		}
		rows = append(rows, append(row, cmp.Or(e.Title, e.Message)))
	}
	// On a terminal the title takes what the other columns leave; identifiers are never cut.
	if width := out.Width(); width > 0 {
		room := width
		for _, w := range fixed {
			room -= w + 2
		}
		room = max(room, 20)
		for _, row := range rows {
			row[4] = ansi.Truncate(row[4], room, "…")
		}
	}
	for _, row := range rows {
		row[0] = out.ID(row[0])
		row[1] = out.Muted(row[1])
		row[2] = out.ID(row[2])
		row[3] = out.Path(row[3])
	}
	_, err := io.WriteString(out, out.Table(headers, rows, 3))
	return err
}

// runLauncher opens the launcher and then starts or resumes the chosen harness. The launcher has
// released the terminal by the time the harness starts.
func runLauncher(ctx context.Context, paths platform.Paths, registry adapters.Registry, harnesses harnessConfig) error {
	directory, err := workingDirectory()
	if err != nil {
		return err
	}
	listing, err := listSessions(ctx, paths, registry, harnesses)
	if err != nil {
		return err
	}
	opts := launcher.Options{
		Sessions:  listing.Entries,
		Directory: directory,
		Profile:   ui.Stdout.Profile(),
		Notes:     listingNotes(listing),
	}
	if harnesses.err != nil {
		opts.Notes = append(opts.Notes, fmt.Sprintf("Only built-ins: configuration did not load: %v", harnesses.err))
	}
	// Built-ins first, then definitions, each in name order.
	names := harnesses.config.HarnessNames()
	for _, builtin := range []bool{true, false} {
		for _, name := range names {
			h, err := harnesses.config.Harness(name)
			if err != nil {
				return err
			}
			if (h.Builtin == name) == builtin {
				opts.Harnesses = append(opts.Harnesses, launcher.Harness{Name: name, Builtin: h.Builtin})
			}
		}
	}
	choice, err := launcher.Run(ctx, opts)
	if err != nil {
		return err
	}
	switch choice.Action {
	case launcher.Quit:
		return nil
	case launcher.Interrupt:
		return &ExitError{Code: 130}
	case launcher.Start:
		return runHarness(ctx, paths, registry, harnesses, choice.Harness, nil, "")
	case launcher.Resume:
		return resumeSession(ctx, paths, registry, harnesses, choice.Session, choice.Harness)
	}
	return nil
}

// resumeSession runs `agx <name> <resume arguments>` in the session's directory, so the
// directory's resources apply and the launch classifies as a resume.
func resumeSession(
	ctx context.Context,
	paths platform.Paths,
	registry adapters.Registry,
	harnesses harnessConfig,
	session sessions.Entry,
	name string,
) error {
	adapter, ok := registry.Adapter(session.Builtin)
	if !ok {
		return fmt.Errorf("no adapter for harness %s", session.Builtin)
	}
	info, err := os.Stat(session.Directory)
	if errors.Is(err, fs.ErrNotExist) || err == nil && !info.IsDir() {
		return fmt.Errorf("session directory %s no longer exists", session.Directory)
	}
	if err != nil {
		return err
	}
	if err := os.Chdir(session.Directory); err != nil {
		return err
	}
	return runHarness(ctx, paths, registry, harnesses, name, adapter.Resume(session.Session), session.ID)
}
