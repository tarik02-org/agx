package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/packages"
)

// progress reports a long step on stderr, only for a person watching a terminal.
func progress(text string) {
	if ui.Stderr.Terminal() {
		ui.Note(text + "…")
	}
}

func countDefinitions(c *config.Config) definitionCounts {
	secrets := 0
	for _, secret := range c.Secrets {
		if !secret.Implicit {
			secrets++
		}
	}
	return definitionCounts{
		Skills:       len(c.Skills),
		Instructions: len(c.Instructions),
		MCP:          len(c.MCP),
		Secrets:      secrets,
		Harnesses:    len(c.Harnesses),
		Scopes:       len(c.Scopes),
		Packages:     len(c.Packages),
	}
}

// printValidation writes config validate's document. An invalid configuration gets one too,
// carrying the failure as a diagnostic, and exits 1.
func printValidation(root string, c *config.Config, loadErr error) error {
	if loadErr != nil {
		if canonical, err := config.Canonical(root); err == nil {
			root = canonical
		}
		message := loadErr.Error()
		code := codeConfigInvalid
		err := printJSON(validationJSON{
			Root:  root,
			Files: []string{},
			Diagnostics: []diagnosticJSON{
				{Severity: "error", Code: &code, Message: message, Source: errorSource(message)},
			},
		})
		if err != nil {
			return err
		}
		return &ExitError{Code: 1}
	}
	counts := countDefinitions(c)
	return printJSON(validationJSON{
		Valid:       true,
		Root:        c.Root,
		Files:       jsonList(configFiles(c)),
		Counts:      &counts,
		Diagnostics: []diagnosticJSON{},
	})
}

func printValid(c *config.Config) error {
	out := ui.Stdout
	n := countDefinitions(c)
	counts := []string{}
	for _, count := range []struct {
		n    int
		noun string
	}{
		{n.Skills, "skill"},
		{n.Instructions, "instruction"},
		{n.MCP, "MCP server"},
		{n.Secrets, "secret"},
		{n.Harnesses, "harness definition"},
		{n.Scopes, "scope"},
		{n.Packages, "package"},
	} {
		counts = append(counts, plural(count.n, count.noun))
	}
	_, err := fmt.Fprint(
		out,
		out.Status(ui.OK, "Configuration valid.")+"\n",
		out.Fields(
			[2]string{"root", out.Path(c.Root)},
			[2]string{"files", strconv.Itoa(len(configFiles(c)))},
			[2]string{"defines", strings.Join(counts, ", ")},
		),
	)
	return err
}

// shellQuote quotes a word for a command a person copies into a shell: POSIX single quotes, or
// double quotes on Windows, whose paths cannot contain one.
func shellQuote(word string) string {
	safe := func(r rune) bool {
		return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			strings.ContainsRune("_-./:@%+,=", r) || runtime.GOOS == "windows" && r == '\\'
	}
	if word != "" && !strings.ContainsFunc(word, func(r rune) bool { return !safe(r) }) {
		return word
	}
	if runtime.GOOS == "windows" {
		return `"` + word + `"`
	}
	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	if strings.HasSuffix(noun, "s") {
		return fmt.Sprintf("%d %ses", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func snapshotFields(out *ui.Printer, snapshot *config.Snapshot) string {
	return out.Fields(
		[2]string{"directory", out.Path(snapshot.Directory)},
		[2]string{"snapshot", out.ID(snapshot.ID)},
		[2]string{"harness", out.ID(snapshot.Harness)},
		[2]string{"environment", strings.Join(snapshot.RequiredEnvironment, " ")},
	)
}

func printResolve(snapshot *config.Snapshot) error {
	out := ui.Stdout
	var b strings.Builder
	b.WriteString(snapshotFields(out, snapshot))
	b.WriteString("\n")
	if len(snapshot.Resources) == 0 {
		b.WriteString(out.Muted("No resources selected.") + "\n")
	} else {
		if out.Styled() {
			b.WriteString(out.Heading("Resources") + "\n")
		}
		rows := [][]string{}
		for _, r := range snapshot.Resources {
			rows = append(rows, []string{r.Kind, out.ID(string(r.ID)) + mcpMark(out, snapshot.Config, r)})
		}
		b.WriteString(out.Table([]string{"KIND", "ID"}, rows))
	}
	_, err := io.WriteString(out, b.String())
	return err
}

// printExplain keeps the unstyled form line-oriented ("skill x (selected)", indented steps,
// "inject <target> = <value>"); styled output draws the same lines as a tree.
func printExplain(snapshot *config.Snapshot) error {
	out := ui.Stdout
	var b strings.Builder
	b.WriteString(snapshotFields(out, snapshot))
	if out.Styled() {
		b.WriteString("\n" + out.Heading("Resources") + "\n")
	} else {
		b.WriteString("\n")
	}
	if len(snapshot.Traces) == 0 {
		b.WriteString(out.Muted("No resources match this directory.") + "\n")
	}
	changeWidth := 0
	for _, t := range snapshot.Traces {
		for _, step := range t.Steps {
			changeWidth = max(changeWidth, len(step.Change))
		}
	}
	resources := out.Tree()
	for _, t := range snapshot.Traces {
		state, level := "removed", ui.Error
		if t.Selected {
			state, level = "selected", ui.OK
		}
		root := fmt.Sprintf("%s %s (%s)", t.Resource.Kind, t.Resource.ID, state)
		if out.Styled() {
			root = fmt.Sprintf("%s %s  %s", t.Resource.Kind, out.ID(string(t.Resource.ID)), out.Badge(level, state))
		}
		root += mcpMark(out, snapshot.Config, t.Resource)
		if t.Order != 0 {
			root += " " + out.Muted("order ") + strconv.Itoa(t.Order)
		}
		node := out.Node(root)
		for _, step := range t.Steps {
			node.Child(explainStep(out, step, changeWidth))
		}
		resources.Child(node)
	}
	if len(snapshot.Traces) > 0 {
		b.WriteString(resources.String() + "\n")
	}
	if len(snapshot.Injections) > 0 {
		if out.Styled() {
			b.WriteString("\n" + out.Heading("Injected values") + "\n")
		} else {
			b.WriteString("\n")
		}
		// Tagged values are shown as configured; they resolve only at launch.
		injections := out.Tree()
		for _, i := range snapshot.Injections {
			location := out.Path(fmt.Sprintf("%s:%d", i.Source.Path, i.Source.Line))
			if out.Styled() {
				injections.Child(out.Node(out.Label(i.Target)+" = "+out.Value(i.Text), location))
			} else {
				injections.Child(out.Node("inject "+i.Target+" = "+i.Text, location))
			}
		}
		b.WriteString(injections.String() + "\n")
	}
	_, err := io.WriteString(out, b.String())
	return err
}

// mcpMark notes after its ID that an MCP server is on demand or shared.
func mcpMark(out *ui.Printer, c *config.Config, r config.Resource) string {
	if r.Kind != "mcp" {
		return ""
	}
	notes := []string{}
	server := c.MCP[r.ID]
	if server.Load == config.LoadOnDemand {
		notes = append(notes, "on demand")
	}
	switch server.Share.Type {
	case config.ShareDirectory:
		notes = append(notes, "shared per directory")
	case config.ShareRepository:
		notes = append(notes, "shared per repository")
	case config.ShareGlobal:
		notes = append(notes, "shared")
	}
	if len(notes) == 0 {
		return ""
	}
	return " " + out.Muted("("+strings.Join(notes, ", ")+")")
}

// printExplainCompact puts each resource on one line with the steps that decided it, shortening
// paths against the configuration directory, the explained directory, and the home directory.
func printExplainCompact(snapshot *config.Snapshot) error {
	out := ui.Stdout
	short := shortener(snapshot)
	var b strings.Builder
	header := out.Path(short.path(snapshot.Directory))
	id := snapshot.ID
	if len(id) > 12 {
		id = id[:12]
	}
	header += "  " + out.Muted("snapshot ") + out.ID(id)
	if snapshot.Harness != "" {
		header += "  " + out.Muted("harness ") + out.ID(snapshot.Harness)
	}
	if len(snapshot.RequiredEnvironment) > 0 {
		header += "  " + out.Muted("env ") + strings.Join(snapshot.RequiredEnvironment, " ")
	}
	b.WriteString(header + "\n\n")
	if len(snapshot.Traces) == 0 {
		b.WriteString(out.Muted("No resources match this directory.") + "\n")
	}
	kindWidth, idWidth := 0, 0
	for _, t := range snapshot.Traces {
		kindWidth = max(kindWidth, len(t.Resource.Kind))
		idWidth = max(idWidth, len(t.Resource.ID))
	}
	for _, t := range snapshot.Traces {
		mark := out.Badge(ui.Error, "✗")
		if t.Selected {
			mark = out.Badge(ui.OK, "✓")
		}
		line := mark + " " + out.Muted(
			pad(t.Resource.Kind, kindWidth),
		) + "  " + out.ID(
			pad(string(t.Resource.ID), idWidth),
		)
		steps := []string{}
		if mark := mcpMark(out, snapshot.Config, t.Resource); mark != "" {
			steps = append(steps, strings.TrimSpace(mark))
		}
		if t.Order != 0 {
			steps = append(steps, out.Muted("order ")+strconv.Itoa(t.Order))
		}
		for _, step := range t.Steps {
			steps = append(steps, compactStep(out, short, snapshot.Directory, step))
		}
		b.WriteString(line + "  " + strings.Join(steps, out.Muted(", ")) + "\n")
	}
	if len(snapshot.Injections) > 0 {
		b.WriteString("\n")
		for _, i := range snapshot.Injections {
			b.WriteString(
				out.Label(i.Target) + " = " + out.Value(i.Text) + "  " +
					out.Muted(short.location(i.Source.Path, i.Source.Line)) + "\n",
			)
		}
	}
	_, err := io.WriteString(out, b.String())
	return err
}

func compactStep(out *ui.Printer, short pathShortener, directory string, step config.Step) string {
	parts := []string{}
	if step.Change == "remove" {
		parts = append(parts, out.Badge(ui.Error, "removed"))
	}
	if len(step.BundleChain) > 0 {
		chain := []string{}
		for _, id := range step.BundleChain {
			chain = append(chain, string(id))
		}
		parts = append(parts, out.Muted("via ")+out.ID(strings.Join(chain, " → ")))
	}
	if step.Harness != "" {
		parts = append(parts, out.Muted("harness ")+out.ID(step.Harness))
	} else if step.Directory != directory {
		parts = append(parts, out.Muted("from ")+scopeLabel(out, short.path(step.Directory)))
	}
	parts = append(parts, out.Muted(short.location(step.Source.Path, step.Source.Line)))
	return strings.Join(parts, " ")
}

func pad(text string, width int) string {
	return text + strings.Repeat(" ", max(0, width-len(text)))
}

type pathShortener struct{ configDir, home string }

func shortener(snapshot *config.Snapshot) pathShortener {
	s := pathShortener{}
	if snapshot.Config != nil {
		s.configDir = filepath.Dir(snapshot.Config.Root)
	}
	if home, err := os.UserHomeDir(); err == nil {
		s.home = home
	}
	return s
}

// location names configuration files relative to the root's directory, where most live.
func (s pathShortener) location(path string, line int) string {
	if s.configDir != "" {
		if rel, err := filepath.Rel(s.configDir, path); err == nil && !strings.HasPrefix(rel, "..") {
			path = rel
		} else {
			path = s.path(path)
		}
	}
	return fmt.Sprintf("%s:%d", path, line)
}

func (s pathShortener) path(path string) string {
	if s.home != "" {
		if rel, err := filepath.Rel(s.home, path); err == nil && !strings.HasPrefix(rel, "..") {
			if rel == "." {
				return "~"
			}
			return filepath.Join("~", rel)
		}
	}
	return path
}

// scopeLabel names a rule's directory, or "everywhere" for a rule without one.
func scopeLabel(out *ui.Printer, directory string) string {
	if directory == "" {
		return out.Muted("everywhere")
	}
	return out.Path(directory)
}

func explainStep(out *ui.Printer, step config.Step, changeWidth int) string {
	location := fmt.Sprintf("%s:%d", step.Source.Path, step.Source.Line)
	if !out.Styled() {
		origin := "scope=" + scopeLabel(out, step.Directory)
		if step.Harness != "" {
			origin = "harness=" + step.Harness
		}
		return fmt.Sprintf("%s %s %s bundles=%v", step.Change, location, origin, step.BundleChain)
	}
	level := ui.OK
	if step.Change == "remove" {
		level = ui.Error
	}
	change := out.Badge(level, step.Change+strings.Repeat(" ", changeWidth-len(step.Change)))
	origin := out.Muted("scope ") + scopeLabel(out, step.Directory)
	if step.Harness != "" {
		origin = out.Muted("harness ") + out.ID(step.Harness)
	}
	line := change + "  " + out.Path(location) + "  " + origin
	if len(step.BundleChain) > 0 {
		chain := []string{}
		for _, id := range step.BundleChain {
			chain = append(chain, out.ID(string(id)))
		}
		line += "  " + out.Muted("via ") + strings.Join(chain, out.Muted(" → "))
	}
	return line
}

func printInstalled(result packages.Result, update bool) error {
	out := ui.Stdout
	verb := "Installed"
	if update {
		verb = "Updated"
	}
	var b strings.Builder
	b.WriteString(out.Status(ui.OK, fmt.Sprintf("%s %s.", verb, plural(len(result.PackageIDs), "package"))) + "\n")
	ids := []string{}
	for _, id := range result.PackageIDs {
		ids = append(ids, out.ID(string(id)))
	}
	b.WriteString(out.List(ids...))
	if len(result.Lockfiles) > 0 {
		b.WriteString(out.Label("lockfiles") + "\n")
		paths := []string{}
		for _, path := range result.Lockfiles {
			paths = append(paths, out.Path(path))
		}
		b.WriteString(out.List(paths...))
	}
	_, err := io.WriteString(out, b.String())
	return err
}

// enumName turns STATE_NEEDS_LOGIN into needs-login: one grep-friendly word per table cell.
func enumName(name string) string {
	_, name, _ = strings.Cut(name, "_")
	return strings.ReplaceAll(strings.ToLower(name), "_", "-")
}

func launchState(out *ui.Printer, state v1.Launch_State) string {
	level := map[v1.Launch_State]ui.Level{
		v1.Launch_STATE_RUNNING: ui.OK,
		v1.Launch_STATE_REVOKED: ui.Error,
		v1.Launch_STATE_EXPIRED: ui.Warn,
	}[state]
	return out.Badge(level, enumName(state.String()))
}

func timestamp(t *timestamppb.Timestamp) string {
	if t == nil {
		return ""
	}
	return t.AsTime().Local().Format(time.RFC3339)
}

func printLaunches(launches []*v1.Launch) error {
	out := ui.Stdout
	if len(launches) == 0 {
		_, err := fmt.Fprintln(out, out.Muted("No launches."))
		return err
	}
	rows := [][]string{}
	children := map[string][]*v1.Launch{}
	for _, l := range launches {
		if l.ParentLaunchId != "" {
			children[l.ParentLaunchId] = append(children[l.ParentLaunchId], l)
		}
	}
	listed := map[string]bool{}
	var add func(l *v1.Launch, depth int)
	add = func(l *v1.Launch, depth int) {
		listed[l.Id] = true
		id, mode := out.ID(l.Id), l.Mode
		if l.ParentLaunchId != "" {
			// Thread launches follow the process launch that serves them.
			id, mode = strings.Repeat("  ", depth)+id, "thread"
		}
		rows = append(rows, []string{id, out.ID(l.Harness), mode, launchState(out, l.State), out.Path(l.Directory)})
		for _, child := range children[l.Id] {
			add(child, depth+1)
		}
	}
	for _, l := range launches {
		if l.ParentLaunchId == "" {
			add(l, 0)
		}
	}
	// A thread launch whose parent was pruned stands alone.
	for _, l := range launches {
		if !listed[l.Id] {
			add(l, 0)
		}
	}
	_, err := io.WriteString(out, out.Table([]string{"LAUNCH", "HARNESS", "MODE", "STATE", "DIRECTORY"}, rows, 4))
	return err
}

func printLaunch(l *v1.Launch, servers []*v1.McpRuntime) error {
	out := ui.Stdout
	var b strings.Builder
	b.WriteString(out.Fields(
		[2]string{"launch", out.ID(l.Id)},
		[2]string{"harness", out.ID(l.Harness)},
		[2]string{"mode", l.Mode},
		[2]string{"state", launchState(out, l.State)},
		[2]string{"directory", out.Path(l.Directory)},
		[2]string{"snapshot", out.ID(l.SnapshotId)},
		[2]string{"expires", timestamp(l.ExpiresAt)},
		[2]string{"parent", optional(out.ID, l.ParentLaunchId)},
		[2]string{"session", optional(out.ID, l.SessionId)},
	))
	rows := [][]string{}
	diagnostics := []*v1.Diagnostic{}
	for _, c := range servers {
		if c.LaunchId != l.Id {
			continue
		}
		level := map[v1.McpRuntime_State]ui.Level{
			v1.McpRuntime_STATE_READY:      ui.OK,
			v1.McpRuntime_STATE_NEEDS_AUTH: ui.Warn,
			v1.McpRuntime_STATE_FAILED:     ui.Error,
		}[c.State]
		rows = append(rows, []string{out.ID(c.McpId), out.Badge(level, enumName(c.State.String()))})
		diagnostics = append(diagnostics, c.Diagnostics...)
	}
	if len(rows) > 0 {
		b.WriteString("\n")
		if out.Styled() {
			b.WriteString(out.Heading("MCP") + "\n")
		}
		b.WriteString(out.Table([]string{"MCP", "STATE"}, rows))
	}
	b.WriteString(diagnosticLines(out, diagnostics))
	_, err := io.WriteString(out, b.String())
	return err
}

// optional styles value, leaving an empty one empty so Fields omits it.
func optional(style func(string) string, value string) string {
	if value == "" {
		return ""
	}
	return style(value)
}

func diagnosticLines(out *ui.Printer, diagnostics []*v1.Diagnostic) string {
	var b strings.Builder
	for _, d := range diagnostics {
		level, word := ui.Warn, "warning"
		if d.Severity == v1.Diagnostic_SEVERITY_ERROR {
			level, word = ui.Error, "error"
		}
		line := out.Badge(level, word) + " " + d.Summary
		if d.Code != "" {
			line += " " + out.Muted("("+d.Code+")")
		}
		if d.Source != nil && d.Source.Path != "" {
			line += " " + out.Path(fmt.Sprintf("%s:%d", d.Source.Path, d.Source.Line))
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func printDaemonStatus(view *v1.RuntimeView) error {
	out := ui.Stdout
	running := 0
	for _, l := range view.Launches {
		if l.State == v1.Launch_STATE_RUNNING {
			running++
		}
	}
	var b strings.Builder
	b.WriteString(out.Fields(
		[2]string{"daemon", out.Badge(ui.OK, "running")},
		[2]string{"version", view.Version},
		[2]string{"instance", out.ID(view.DaemonInstanceId)},
		[2]string{"launches", fmt.Sprintf("%d (%d running)", len(view.Launches), running)},
		[2]string{"mcp", strconv.Itoa(len(view.McpServers))},
	))
	b.WriteString(diagnosticLines(out, view.Diagnostics))
	_, err := io.WriteString(out, b.String())
	return err
}

func printSecretStatuses(statuses []*v1.SecretStatus) error {
	out := ui.Stdout
	if len(statuses) == 0 {
		_, err := fmt.Fprintln(out, out.Muted("No secrets configured."))
		return err
	}
	rows := [][]string{}
	diagnostics := []*v1.Diagnostic{}
	for _, s := range statuses {
		level := map[v1.SecretStatus_State]ui.Level{
			v1.SecretStatus_STATE_READY:       ui.OK,
			v1.SecretStatus_STATE_PENDING:     ui.Warn,
			v1.SecretStatus_STATE_NEEDS_LOGIN: ui.Warn,
			v1.SecretStatus_STATE_UNAVAILABLE: ui.Error,
		}[s.State]
		expires := timestamp(s.ExpiresAt)
		if expires == "" {
			expires = "-"
		}
		kind := s.Kind
		switch {
		case s.OnDemand:
			kind += " (on demand)"
		case s.McpId != "":
			kind += " (inline)"
		}
		rows = append(
			rows,
			[]string{out.ID(s.SecretId), kind, out.Badge(level, enumName(s.State.String())), expires},
		)
		diagnostics = append(diagnostics, s.Diagnostics...)
	}
	_, err := io.WriteString(
		out,
		out.Table([]string{"SECRET", "KIND", "STATE", "EXPIRES"}, rows)+diagnosticLines(out, diagnostics),
	)
	return err
}
