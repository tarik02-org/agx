package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/tarik02-org/agx/internal/yaml"

	"github.com/tarik02-org/agx/internal/cli/prompt"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/config/edit"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/securefs"
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
	cmd.Flags().StringVar(&f.file, "file", "", "fragment to write definitions to (default: the root configuration)")
	if defines {
		cmd.Flags().BoolVar(&f.defineOnly, "define-only", false, "define without enabling")
	}
	f.registerYes(cmd)
}

func (f *editFlags) registerYes(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&f.yes, "yes", "y", false, "write without confirming (implied without a terminal)")
}

// scope is where an edit enables or disables a resource: a directory, or everywhere.
type scope struct {
	// directory is canonical, or empty for everywhere.
	directory string
	// written is the directory as a new rule spells it: ~/ under the home directory.
	written   string
	harnesses []string
}

func (s scope) String() string {
	label := "everywhere"
	if s.directory != "" {
		label = s.written
	}
	if len(s.harnesses) > 0 {
		label += " for " + strings.Join(s.harnesses, ", ")
	}
	return label
}

func (f *editFlags) target() (scope, error) {
	harnesses := slices.Clone(f.harnesses)
	sort.Strings(harnesses)
	harnesses = slices.Compact(harnesses)
	value := f.scope
	switch value {
	case "", "user":
		return scope{harnesses: harnesses}, nil
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
	return scope{directory: directory, written: homeRelative(directory), harnesses: harnesses}, nil
}

// homeRelative writes a path under the home directory as ~/rest, so configuration stays portable.
func homeRelative(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if canonical, err := config.Canonical(home); err == nil {
		home = canonical
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + filepath.ToSlash(rest)
	}
	return path
}

// session gathers one command's edits across fragments and lockfiles, then validates the result,
// previews it as a diff, and writes it.
type session struct {
	ctx         context.Context
	paths       platform.Paths
	builtins    config.Builtins
	config      *config.Config
	root        string
	files       map[string]*edit.File
	order       []string
	locks       map[string][]byte
	yes         bool
	interactive bool
	// written lists the files the commit wrote.
	written []string
	// result records what the command did, for its JSON document.
	result editJSON
}

func (s scope) json() *scopeJSON {
	out := &scopeJSON{Harnesses: slices.Clone(s.harnesses)}
	if out.Harnesses == nil {
		out.Harnesses = []string{}
	}
	if s.directory != "" {
		directory := s.directory
		out.Directory = &directory
	}
	return out
}

func newSession(ctx context.Context, paths platform.Paths, builtins config.Builtins, yes bool) (*session, error) {
	c, err := config.Load(paths.Config, paths.Cache, builtins)
	if err != nil {
		return nil, codedf(codeConfigInvalid, "configuration is invalid; fix it before editing: %w", err)
	}
	root, err := config.Canonical(paths.Config)
	if err != nil {
		return nil, err
	}
	// JSON mode implies --yes and never prompts.
	tty := interactive() && !scripting.enabled
	return &session{
		ctx:         ctx,
		paths:       paths,
		builtins:    builtins,
		config:      c,
		root:        root,
		files:       map[string]*edit.File{},
		locks:       map[string][]byte{},
		yes:         yes || !tty,
		interactive: tty,
		result: editJSON{
			Files:    []fileChangeJSON{},
			Defined:  []definitionJSON{},
			Enabled:  []definitionJSON{},
			Disabled: []definitionJSON{},
			Removed:  []definitionJSON{},
		},
	}, nil
}

// report prints the session's JSON document in JSON mode.
func (s *session) report() error {
	if !scripting.enabled {
		return nil
	}
	return printJSON(s.result)
}

// file opens a fragment once per session; a missing root starts from the default configuration.
func (s *session) file(path string) (*edit.File, error) {
	path, err := config.Canonical(path)
	if err != nil {
		return nil, err
	}
	if f, ok := s.files[path]; ok {
		return f, nil
	}
	seed := ""
	if path == s.root {
		seed = config.DefaultConfig
	}
	f, err := edit.Open(path, seed)
	if err != nil {
		return nil, err
	}
	s.files[path] = f
	s.order = append(s.order, path)
	return f, nil
}

// definitionFile is where new definitions go: --file, or the root configuration.
func (s *session) definitionFile(flag string) (string, error) {
	if flag == "" {
		return s.root, nil
	}
	expanded, err := config.ExpandHome(flag)
	if err != nil {
		return "", &usageError{fmt.Errorf("--file: %w", err)}
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", err
	}
	return config.Canonical(abs)
}

// fragments lists the configuration's fragments in load order, then files opened for the edit.
func (s *session) fragments() []string {
	out := slices.Clone(s.config.Files)
	for _, path := range s.order {
		if !slices.Contains(out, path) {
			out = append(out, path)
		}
	}
	return out
}

// matchingRules visits the rules with the target's directory and harness filter in load order;
// visit returns true to stop. Rules in files that cannot be written are skipped.
func (s *session) matchingRules(target scope, visit func(*edit.File, edit.Rule) (bool, error)) error {
	for _, path := range s.fragments() {
		if edit.Writable(path) != nil {
			continue
		}
		f, err := s.file(path)
		if err != nil {
			return err
		}
		for _, r := range f.Rules() {
			if !r.Matches(target.directory, target.harnesses) {
				continue
			}
			if stop, err := visit(f, r); stop || err != nil {
				return err
			}
		}
	}
	return nil
}

// enable selects id in the first rule matching target, or in a new rule in fallback, and drops it
// from that rule's remove list. It returns whether anything changed.
func (s *session) enable(kind, id string, target scope, fallback string) (bool, error) {
	s.result.Enabled = append(s.result.Enabled, definitionJSON{Kind: kind, ID: id})
	s.result.Scope = target.json()
	changed := false
	found := false
	err := s.matchingRules(target, func(f *edit.File, r edit.Rule) (bool, error) {
		found = true
		changed = s.include(f, r.Node, kind, id, target)
		return true, nil
	})
	if err != nil || found {
		return changed, err
	}
	f, err := s.file(fallback)
	if err != nil {
		return false, err
	}
	return s.include(f, f.AddRule(target.written, target.harnesses), kind, id, target), nil
}

// include drops id from the rule's remove list and appends it to its add list. When the remove
// entry was all that deselected id, as after unuse, the add entry would change nothing and is
// left out, so use undoes unuse to the original text.
func (s *session) include(f *edit.File, rule *yaml.Node, kind, id string, target scope) bool {
	unremoved := false
	if remove := f.List(edit.Get(rule, "remove"), kind, false); remove != nil {
		if i := edit.Find(kind, remove, id); i >= 0 {
			edit.RemoveAt(remove, i)
			f.Touch(rule)
			unremoved = true
		}
	}
	var without [][]config.Resource
	if unremoved {
		without = s.selection(target)
	}
	add := f.List(f.Selection(rule, "add", true), kind, true)
	if edit.Find(kind, add, id) >= 0 {
		return unremoved
	}
	edit.Append(add, id)
	if without != nil && slices.EqualFunc(without, s.selection(target), slices.Equal) {
		edit.RemoveAt(add, len(add.Content)-1)
		// Prune then drops the add list this created.
		f.Touch(rule)
	}
	return true
}

// selection resolves the edited configuration for each harness of target, in the target's
// directory or, for everywhere, the file system root. It returns nil when it cannot resolve.
func (s *session) selection(target scope) [][]config.Resource {
	overlay := config.Overlay{}
	for path, f := range s.files {
		b, err := f.Render()
		if err != nil {
			return nil
		}
		overlay[path] = b
	}
	for path, b := range s.locks {
		overlay[path] = b
	}
	c, err := config.LoadWith(s.paths.Config, s.paths.Cache, s.builtins, overlay)
	if err != nil {
		return nil
	}
	directory := target.directory
	if directory == "" {
		directory = filepath.VolumeName(s.root) + string(filepath.Separator)
	}
	harnesses := target.harnesses
	if len(harnesses) == 0 {
		harnesses = []string{""}
	}
	out := [][]config.Resource{}
	for _, harness := range harnesses {
		snapshot, err := c.Resolve(directory, harness)
		if err != nil {
			return nil
		}
		out = append(out, snapshot.Resources)
	}
	return out
}

// disable takes id out of the target scope: from the add list of a matching rule that selects it,
// otherwise by appending it to the remove list of the first matching rule or a new rule in
// fallback, since another rule selects it.
func (s *session) disable(kind, id string, target scope, fallback string) (bool, error) {
	s.result.Disabled = append(s.result.Disabled, definitionJSON{Kind: kind, ID: id})
	s.result.Scope = target.json()
	changed := false
	err := s.matchingRules(target, func(f *edit.File, r edit.Rule) (bool, error) {
		add := f.List(edit.Get(r.Node, "add"), kind, false)
		i := edit.Find(kind, add, id)
		if i < 0 {
			return false, nil
		}
		if _, defines := edit.EntryID(kind, add.Content[i]); defines {
			return true, codedf(
				codeConflict,
				"%s %s is defined inline in a rule of %s; remove it with agx %s remove %s",
				kindLabel(kind), id, f.Path, kindCommand(kind), id,
			)
		}
		edit.RemoveAt(add, i)
		f.Touch(r.Node)
		changed = true
		return true, nil
	})
	if err != nil || changed {
		return changed, err
	}
	var rule *yaml.Node
	var file *edit.File
	err = s.matchingRules(target, func(f *edit.File, r edit.Rule) (bool, error) {
		file, rule = f, r.Node
		return true, nil
	})
	if err != nil {
		return false, err
	}
	if rule == nil {
		if file, err = s.file(fallback); err != nil {
			return false, err
		}
		rule = file.AddRule(target.written, target.harnesses)
	}
	remove := file.List(file.Selection(rule, "remove", true), kind, true)
	if edit.Find(kind, remove, id) >= 0 {
		return false, nil
	}
	edit.Append(remove, id)
	return true, nil
}

// references lists the entries naming id in every fragment except the one at defining.
func (s *session) references(kind, id string) ([]string, error) {
	out := []string{}
	for _, path := range s.fragments() {
		f, err := s.file(path)
		if err != nil {
			return nil, err
		}
		for _, r := range f.References(kind, id) {
			out = append(out, fmt.Sprintf("%s: %s", ui.ShortPath(path, ""), r.Where))
		}
	}
	return out, nil
}

// removeReferences drops every entry naming id, refusing when a file holding one cannot be written.
func (s *session) removeReferences(kind, id string) error {
	for _, path := range s.fragments() {
		f, err := s.file(path)
		if err != nil {
			return err
		}
		refs := f.References(kind, id)
		if len(refs) == 0 {
			continue
		}
		if err := edit.Writable(path); err != nil {
			return codedf(codeReadOnly, "cannot remove the references to %s: %w", id, err)
		}
		for _, r := range refs {
			f.Remove(r)
		}
	}
	return nil
}

var errDeclined = errors.New("nothing was written")

// commit validates the edited configuration in memory, shows the change, and writes it. It
// returns false when there was nothing to write.
func (s *session) commit() (bool, error) {
	type pending struct {
		path          string
		before, after []byte
	}
	changed := []pending{}
	overlay := config.Overlay{}
	for _, path := range s.order {
		f := s.files[path]
		f.Prune()
		after, err := f.Render()
		if err != nil {
			return false, err
		}
		if f.Exists && bytes.Equal(after, f.Original()) {
			continue
		}
		changed = append(changed, pending{path: path, before: f.Original(), after: after})
		overlay[path] = after
	}
	if len(changed) == 0 {
		hint("Nothing to change.")
		return false, nil
	}
	for path, b := range s.locks {
		overlay[path] = b
	}
	for _, p := range changed {
		if err := edit.Writable(p.path); err != nil {
			return false, coded(codeReadOnly, err)
		}
	}
	c, err := config.LoadWith(s.paths.Config, s.paths.Cache, s.builtins, overlay)
	if err != nil {
		return false, codedf(
			codeConfigInvalid,
			"the change would make the configuration invalid, so nothing was written: %w",
			err,
		)
	}
	for _, p := range changed {
		if slices.Contains(c.Files, p.path) {
			continue
		}
		// A fragment outside the configuration is checked on its own.
		if _, err := config.LoadWith(p.path, s.paths.Cache, s.builtins, overlay); err != nil {
			return false, codedf(
				codeConfigInvalid,
				"the change would make %s invalid, so nothing was written: %w",
				p.path,
				err,
			)
		}
		ui.Diagnose(ui.Warn, "%s is not imported by %s; its definitions apply once it is", p.path, s.root)
	}
	if !scripting.enabled {
		var preview strings.Builder
		for _, p := range changed {
			preview.WriteString(styleDiff(ui.Stdout, edit.Diff(p.path, p.before, p.after)))
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
	for _, p := range changed {
		if err := edit.Write(p.path, p.after); err != nil {
			return false, err
		}
		s.written = append(s.written, p.path)
		s.result.Files = append(
			s.result.Files,
			fileChangeJSON{Path: p.path, Diff: edit.Diff(p.path, p.before, p.after), Written: true},
		)
	}
	for _, path := range slices.Sorted(maps.Keys(s.locks)) {
		current, err := os.ReadFile(path)
		if err == nil && bytes.Equal(current, s.locks[path]) {
			continue
		}
		if err := securefs.Write(path, s.locks[path], 0o600); err != nil {
			return false, err
		}
		s.written = append(s.written, path)
		s.result.Files = append(
			s.result.Files,
			fileChangeJSON{Path: path, Diff: edit.Diff(path, current, s.locks[path]), Written: true},
		)
	}
	if !scripting.enabled {
		for _, path := range s.written {
			_, _ = fmt.Fprintln(ui.Stderr, ui.Stderr.Status(ui.OK, "Wrote "+ui.Stderr.Path(path)+"."))
		}
	}
	s.config = c
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

// promptError turns Ctrl-C into exit status 130 and Esc into a cancellation.
func promptError(err error) error {
	switch {
	case errors.Is(err, prompt.ErrInterrupted):
		return &ExitError{Code: 130}
	case errors.Is(err, prompt.ErrCancelled):
		return errDeclined
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

func kindLabel(kind string) string {
	switch kind {
	case edit.Skills:
		return "skill"
	case edit.MCP:
		return "MCP server"
	case edit.Bundles:
		return "bundle"
	case "harnesses":
		return "harness"
	}
	return "instructions"
}

func kindCommand(kind string) string {
	switch kind {
	case edit.Skills:
		return "skill"
	case edit.MCP:
		return "mcp"
	case "harnesses":
		return "harness"
	}
	return kind
}

// definitionSource locates a resource definition, and the package that declares it if any.
func definitionSource(c *config.Config, kind, id string) (config.Source, bool, bool) {
	var source config.Source
	inline, exists := false, false
	switch kind {
	case edit.Skills:
		var d config.Skill
		d, exists = c.Skills[config.ID(id)]
		source, inline = d.Source, d.Inline
	case edit.Instructions:
		var d config.Instructions
		d, exists = c.Instructions[config.ID(id)]
		source, inline = d.Source, d.Inline
	case edit.MCP:
		var d config.MCPServer
		d, exists = c.MCP[config.ID(id)]
		source, inline = d.Source, d.Inline
	case edit.Bundles:
		var d config.Bundle
		d, exists = c.Bundles[config.ID(id)]
		source = d.Source
	}
	return source, inline, exists
}

// define adds a top-level definition to file, refusing an ID that is already defined.
func (s *session) define(kind, id, file string, value *yaml.Node) error {
	if source, _, exists := definitionSource(s.config, kind, id); exists {
		return codedf(
			codeConflict,
			"%s %s is already defined at %s:%d",
			kindLabel(kind),
			id,
			source.Path,
			source.Line,
		)
	}
	f, err := s.file(file)
	if err != nil {
		return err
	}
	f.Define(kind, id, value)
	s.result.Defined = append(s.result.Defined, definitionJSON{Kind: kind, ID: id})
	return nil
}

// enableAfterDefine enables a new definition unless --define-only; scope edits go to a matching
// rule, otherwise beside the definition.
func (s *session) enableAfterDefine(kind, id, file string, flags editFlags) error {
	if flags.defineOnly {
		return nil
	}
	target, err := flags.target()
	if err != nil {
		return err
	}
	_, err = s.enable(kind, id, target, file)
	return err
}

// removeDefinition deletes a fragment's definition of id, with its references when force is set.
func (s *session) removeDefinition(kind, id string, force bool) error {
	source, inline, exists := definitionSource(s.config, kind, id)
	if !exists {
		return codedf(codeNotFound, "unknown %s %s", kindLabel(kind), id)
	}
	if !slices.Contains(s.config.Files, source.Path) {
		for prefix := config.ID(id); strings.Contains(string(prefix), "/"); {
			prefix = prefix[:strings.LastIndex(string(prefix), "/")]
			if kind == edit.Skills && s.skillPackage(prefix) {
				return codedf(
					codeReadOnly,
					"skill %s comes from package %s; remove the package with agx skill remove %s, "+
						"or disable the skill with agx unuse %s",
					id, prefix, prefix, id,
				)
			}
		}
		return codedf(
			codeReadOnly,
			"%s %s is defined by a package (%s); remove that package instead, or disable it with agx unuse %s",
			kindLabel(kind),
			id,
			source.Path,
			id,
		)
	}
	f, err := s.file(source.Path)
	if err != nil {
		return err
	}
	if inline {
		removed := false
		for _, r := range f.References(kind, id) {
			if r.Defines {
				f.Remove(r)
				removed = true
				break
			}
		}
		if !removed {
			return fmt.Errorf("cannot find the inline definition of %s in %s", id, source.Path)
		}
	} else if !f.Undefine(kind, id) {
		return fmt.Errorf("cannot find the definition of %s in %s", id, source.Path)
	}
	s.result.Removed = append(s.result.Removed, definitionJSON{Kind: kind, ID: id})
	refs, err := s.references(kind, id)
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		return nil
	}
	if !force {
		return codedf(codeConflict, "%s %s is still referenced; pass --force to remove the references too:\n  %s",
			kindLabel(kind), id, strings.Join(refs, "\n  "))
	}
	return s.removeReferences(kind, id)
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
