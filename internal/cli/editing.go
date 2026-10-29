package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"go.yaml.in/yaml/v4"

	"github.com/tarik02-org/agx/internal/cli/prompt"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/config/edit"
	"github.com/tarik02-org/agx/internal/configedit"
	"github.com/tarik02-org/agx/internal/platform"
)

// interactive reports whether a person can answer prompts: stdin and stderr are terminals.
func interactive() bool { return term.IsTerminal(os.Stdin.Fd()) && ui.Stderr.Terminal() }

// editFlags are the options every configuration editing command shares.
type editFlags struct {
	scope      string
	harnesses  []string
	file       string
	defineOnly bool
	yes        bool
}

func (f *editFlags) register(cmd *cobra.Command, defines bool) {
	cmd.Flags().StringVar(&f.scope, "scope", "user",
		"where to enable: user (everywhere), here (this directory), or a directory")
	cmd.Flags().StringSliceVar(&f.harnesses, "harness", nil, "enable only for these harnesses (repeatable)")
	f.registerFile(cmd)
	if defines {
		cmd.Flags().BoolVar(&f.defineOnly, "define-only", false, "define without enabling")
	}
	f.registerYes(cmd)
}

func (f *editFlags) registerFile(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.file, "file", "",
		"fragment for new definitions and rules, relative to the root configuration's directory")
}

func (f *editFlags) registerYes(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&f.yes, "yes", "y", false, "write without confirming (implied without a terminal)")
}

// scope is where an edit enables or disables a resource: a directory, or everywhere.
type scope = configedit.Scope

func (f *editFlags) target() (scope, error) {
	harnesses := slices.Clone(f.harnesses)
	sort.Strings(harnesses)
	harnesses = slices.Compact(harnesses)
	value := f.scope
	switch value {
	case "", "user":
		return configedit.NewScope("", harnesses), nil
	case "here":
		value = "."
	}
	expanded, err := config.ExpandHome(value)
	if err != nil {
		return scope{}, &usageError{fmt.Errorf("--scope: %w", err)}
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return scope{}, err
	}
	directory, err := config.Canonical(abs)
	if err != nil {
		return scope{}, err
	}
	return configedit.NewScope(directory, harnesses), nil
}

// enableTarget is target for a rule that enables resources: its directory must exist, so a
// mistyped --scope does not write a rule that never applies. Disabling takes any directory, so
// rules of a removed one can go.
func (f *editFlags) enableTarget() (scope, error) {
	target, err := f.target()
	if err != nil || target.Directory == "" {
		return target, err
	}
	if info, err := os.Stat(target.Directory); err != nil {
		return scope{}, statError("scope directory", target.Directory, err)
	} else if !info.IsDir() {
		return scope{}, codedf(codeInvalidRequest, "scope is not a directory: %s", target.Directory)
	}
	return target, nil
}

// session gathers one command's edits across fragments and lockfiles, then validates the result,
// previews it as a diff, and writes it.
type session struct {
	*configedit.Session
	ctx         context.Context
	yes         bool
	interactive bool
	// written lists the files the commit wrote.
	written []string
	// result records what the command did, for its JSON document.
	result edits
}

// edits is the document of a command that edits configuration.
type edits struct {
	Files    []fileChange   `json:"files"`
	Defined  []definitionID `json:"defined"`
	Enabled  []definitionID `json:"enabled"`
	Disabled []definitionID `json:"disabled"`
	Removed  []definitionID `json:"removed"`
	// Scope is where resources were enabled or disabled; null when the command changed no scope.
	Scope *scope `json:"scope"`
	// NeedsLogin says whether a server mcp add defined asks for an OAuth login.
	NeedsLogin *bool `json:"needs_login,omitempty"`
}

type fileChange struct {
	Path string `json:"path"`
	Diff string `json:"diff"`
}

// definitionID names a definition by its configuration section, such as mcp or harnesses.
type definitionID struct {
	Kind config.Kind `json:"kind"`
	ID   string      `json:"id"`
}

func newSession(ctx context.Context, paths platform.Paths, builtins config.Builtins, yes bool) (*session, error) {
	c, err := config.Load(paths.Config, config.Options{Builtins: builtins, Cache: paths.Cache})
	if err != nil {
		return nil, codedf(codeConfigInvalid, "configuration is invalid; fix it before editing: %w", err)
	}
	edits, err := configedit.New(paths, builtins, c)
	if err != nil {
		return nil, err
	}
	// JSON mode implies --yes and never prompts.
	tty := interactive() && !scripting.enabled
	s := &session{
		Session:     edits,
		ctx:         ctx,
		yes:         yes || !tty,
		interactive: tty,
	}
	if tty {
		edits.Choose = s.chooseFile
	}
	return s, nil
}

// useFile makes --file, relative to the root configuration's directory, the fragment new
// definitions and rules go to.
func (s *session) useFile(flag string) error {
	if flag == "" {
		return nil
	}
	expanded, err := config.ExpandHome(flag)
	if err != nil {
		return &usageError{fmt.Errorf("--file: %w", err)}
	}
	if !filepath.IsAbs(expanded) {
		expanded = filepath.Join(s.Dir, expanded)
	}
	s.File = expanded
	return nil
}

// chooseFile asks which writable fragment new definitions go to, or for a new one in agx.d.
func (s *session) chooseFile(candidates []string) (string, error) {
	labels := []string{}
	for _, path := range candidates {
		labels = append(labels, ui.ShortPath(path, ""))
	}
	i, err := prompt.Choose(s.ctx, "Write to:", append(labels, "new file…"))
	if err != nil {
		return "", promptError(err)
	}
	if i < len(candidates) {
		return candidates[i], nil
	}
	name, err := prompt.Input(s.ctx, "File name in agx.d:", "main", func(name string) error {
		if name == "" || strings.ContainsAny(name, `/\`) {
			return errors.New("a file name without directories")
		}
		return nil
	})
	if err != nil {
		return "", promptError(err)
	}
	return filepath.Join(s.Dir, "agx.d", strings.TrimSuffix(name, ".yaml")+".yaml"), nil
}

// report prints the session's JSON document in JSON mode; a person saw the diff.
func (s *session) report() error {
	return emit(s.result, func() error { return nil })
}

// definitionFile is where new definitions go: --file, or the fragment Target chooses.
func (s *session) definitionFile(flag string) (string, error) {
	if err := s.useFile(flag); err != nil {
		return "", err
	}
	return s.Target()
}

// enable selects id in the first rule matching target, or in a new rule in the target fragment.
func (s *session) enable(kind config.Kind, id string, target scope) (bool, error) {
	s.result.Enabled = append(s.result.Enabled, definitionID{Kind: kind, ID: id})
	s.result.Scope = &target
	return s.Enable(kind, id, target)
}

// disable takes id out of the target scope.
func (s *session) disable(kind config.Kind, id string, target scope) (bool, error) {
	s.result.Disabled = append(s.result.Disabled, definitionID{Kind: kind, ID: id})
	s.result.Scope = &target
	return s.Disable(kind, id, target)
}

var errDeclined = errors.New("nothing was written")

// commit validates the edited configuration in memory, shows the change, and writes it. It
// returns false when there was nothing to write.
func (s *session) commit() (bool, error) {
	plan, err := s.Plan()
	if err != nil {
		return false, err
	}
	if len(plan.Files) == 0 {
		hint("Nothing to change.")
		return false, nil
	}
	if !scripting.enabled {
		var preview strings.Builder
		for _, p := range plan.Files {
			if p.Kind != configedit.Lockfile {
				preview.WriteString(styleDiff(ui.Stdout, p.Diff()))
			}
		}
		if _, err := io.WriteString(ui.Stdout, preview.String()); err != nil {
			return false, err
		}
	}
	if !s.yes {
		ok, err := prompt.Confirm(s.ctx, "Write the change?", true)
		if err != nil {
			return false, promptError(err)
		}
		if !ok {
			return false, errDeclined
		}
	}
	written, err := plan.Write()
	s.written = append(s.written, written...)
	if err != nil {
		return false, err
	}
	for _, p := range plan.Files {
		s.result.Files = append(s.result.Files, fileChange{Path: p.Path, Diff: p.Diff()})
	}
	for _, path := range s.written {
		_, _ = fmt.Fprintln(ui.Stderr, ui.Stderr.Status(ui.OK, "Wrote "+ui.Stderr.Path(path)+"."))
	}
	s.Config = plan.Config
	return true, nil
}

// styleDiff colors a unified diff for a styled stream.
func styleDiff(out *ui.Printer, diff string) string {
	if !out.Styled() {
		return diff
	}
	var b strings.Builder
	for _, line := range strings.SplitAfter(diff, "\n") {
		text := strings.TrimSuffix(line, "\n")
		switch {
		case text == "":
		case strings.HasPrefix(text, "---") || strings.HasPrefix(text, "+++"):
			text = out.Heading(text)
		case strings.HasPrefix(text, "@@"):
			text = out.Muted(text)
		case strings.HasPrefix(text, "+"):
			text = out.Badge(ui.OK, text)
		case strings.HasPrefix(text, "-"):
			text = out.Badge(ui.Error, text)
		}
		b.WriteString(text)
		if strings.HasSuffix(line, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// promptError maps an aborted form to exit status 130.
func promptError(err error) error {
	switch {
	case errors.Is(err, prompt.ErrInterrupted), errors.Is(err, context.Canceled):
		return &ExitError{Code: 130}
	}
	return err
}

// ask prompts for a missing argument on a terminal; without one it is a usage error.
func (s *session) ask(label, placeholder string, validate func(string) error) (string, error) {
	if !s.interactive {
		return "", &usageError{fmt.Errorf("missing %s", strings.ToLower(strings.TrimSuffix(label, ":")))}
	}
	value, err := prompt.Input(s.ctx, label, placeholder, func(value string) error {
		if value == "" {
			return errors.New("required")
		}
		if validate != nil {
			return validate(value)
		}
		return nil
	})
	return value, promptError(err)
}

// define adds a top-level definition to file, refusing an ID that is already defined.
func (s *session) define(kind config.Kind, id, file string, value *yaml.Node) error {
	if at, exists := configedit.Lookup(s.Config, kind, id); exists {
		return codedf(
			codeConflict,
			"%s %s is already defined at %s:%d",
			kind.Label(),
			id,
			at.Source.Path,
			at.Source.Line,
		)
	}
	f, err := s.Open(file)
	if err != nil {
		return err
	}
	f.Define(kind, id, value)
	s.result.Defined = append(s.result.Defined, definitionID{Kind: kind, ID: id})
	return nil
}

// enableAfterDefine enables a new definition unless --define-only; scope edits go to a matching
// rule, otherwise in the target fragment.
func (s *session) enableAfterDefine(kind config.Kind, id string, flags editFlags) error {
	if flags.defineOnly {
		return nil
	}
	target, err := flags.enableTarget()
	if err != nil {
		return err
	}
	_, err = s.enable(kind, id, target)
	return err
}

// removeDefinition deletes a fragment's definition of id, with its references when force is set.
func (s *session) removeDefinition(kind config.Kind, id string, force bool) error {
	if err := s.RemoveDefinition(kind, id, force); err != nil {
		return err
	}
	s.result.Removed = append(s.result.Removed, definitionID{Kind: kind, ID: id})
	return nil
}

// value reads a --header or --env value: !secret ID, !env NAME, and !file PATH become tags.
func value(text string) *yaml.Node {
	for _, tag := range []string{"!secret", "!env", "!file"} {
		if rest, ok := strings.CutPrefix(text, tag+" "); ok {
			return edit.Tagged(tag, strings.TrimSpace(rest))
		}
	}
	return edit.Scalar(text)
}

// pairs parses repeated KEY=VALUE flags into a mapping in the order given.
func pairs(flag string, values []string) (*yaml.Node, error) {
	m := edit.Mapping()
	for _, pair := range values {
		key, text, ok := strings.Cut(pair, "=")
		if !ok || key == "" {
			return nil, &usageError{fmt.Errorf("--%s %q: expected KEY=VALUE", flag, pair)}
		}
		edit.Set(m, key, value(text))
	}
	return m, nil
}
