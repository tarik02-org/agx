package cli

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
)

// Completion asks only a daemon that already runs, and gives up quickly so a shell never stalls.
const completionDaemonTimeout = 300 * time.Millisecond

// completer offers shell completion candidates from the configuration and, for launches, from a
// running daemon. Completion never reports errors: a configuration that does not load offers
// nothing.
type completer struct {
	paths    *platform.Paths
	builtins *config.Builtins
	loaded   bool
	config   *config.Config
}

// load reads the configuration once per completion request. Packages missing from the cache
// make a full load fail, so the fragments alone still offer their IDs then.
func (k *completer) load() *config.Config {
	if k.loaded {
		return k.config
	}
	k.loaded = true
	root, err := filepath.Abs(k.paths.Config)
	if err != nil {
		return nil
	}
	cache, err := filepath.Abs(k.paths.Cache)
	if err != nil {
		return nil
	}
	if c, err := config.Load(root, config.Options{Builtins: *k.builtins, Cache: cache}); err == nil {
		k.config = c
	} else if c, err := config.Load(root, config.Options{Builtins: *k.builtins}); err == nil {
		k.config = c
	}
	return k.config
}

type candidate struct{ value, description string }

// offer returns the candidates matching toComplete, each with a one-line description.
func offer(candidates []candidate, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	out := []cobra.Completion{}
	for _, c := range candidates {
		if strings.HasPrefix(c.value, toComplete) {
			out = append(out, cobra.CompletionWithDesc(c.value, strings.Join(strings.Fields(c.description), " ")))
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// firstArg completes only the first positional argument.
func firstArg(candidates func() []candidate) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return offer(candidates(), toComplete)
	}
}

func fixed(values ...string) cobra.CompletionFunc {
	candidates := []candidate{}
	for _, v := range values {
		value, description, _ := strings.Cut(v, "\t")
		candidates = append(candidates, candidate{value, description})
	}
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		return offer(candidates, toComplete)
	}
}

func files(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveDefault
}

func directories(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveFilterDirs
}

func yamlFiles(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	return []cobra.Completion{"yaml", "yml"}, cobra.ShellCompDirectiveFilterFileExt
}

// scope completes the keywords while they still match, and directories otherwise.
func completeScope(
	cmd *cobra.Command,
	args []string,
	toComplete string,
) ([]cobra.Completion, cobra.ShellCompDirective) {
	completions, directive := offer([]candidate{
		{"user", "everywhere"},
		{"here", "this directory"},
	}, toComplete)
	if len(completions) == 0 {
		return nil, cobra.ShellCompDirectiveFilterDirs
	}
	return completions, directive
}

func (k *completer) harnesses() []candidate {
	c := k.load()
	if c == nil {
		return nil
	}
	out := []candidate{}
	for _, name := range c.HarnessNames() {
		description := "built-in"
		if def, defined := c.Harnesses[name]; defined && def.Base != "" {
			description = "base " + def.Base
		}
		out = append(out, candidate{name, description})
	}
	return out
}

// harnessList completes a comma-separated --harness value, one name at a time.
func (k *completer) harnessList(
	cmd *cobra.Command,
	args []string,
	toComplete string,
) ([]cobra.Completion, cobra.ShellCompDirective) {
	cut := 0
	// Only a repeatable flag takes a list; resolve and explain take one harness.
	if f := cmd.Flags().Lookup("harness"); f != nil && strings.Contains(f.Value.Type(), "Slice") {
		cut = strings.LastIndex(toComplete, ",") + 1
	}
	prefix, given := toComplete[:cut], strings.Split(toComplete[:cut], ",")
	out := []candidate{}
	for _, h := range k.harnesses() {
		if !slices.Contains(given, h.value) {
			out = append(out, candidate{prefix + h.value, h.description})
		}
	}
	return offer(out, toComplete)
}

// definitions lists the IDs of a kind; removable keeps those a configuration fragment defines,
// as package definitions cannot be removed.
func (k *completer) definitions(kind config.Kind, removable bool) []candidate {
	c := k.load()
	if c == nil {
		return nil
	}
	out := []candidate{}
	add := func(id config.ID, source config.Source, description string) {
		if !removable || slices.Contains(c.Files, source.Path) {
			out = append(out, candidate{string(id), description})
		}
	}
	switch kind {
	case config.KindMCPServers:
		for _, id := range slices.Sorted(maps.Keys(c.MCP)) {
			add(id, c.MCP[id].Source, mcpTarget(c.MCP[id]))
		}
	case config.KindSkills:
		for _, id := range slices.Sorted(maps.Keys(c.Skills)) {
			skill := c.Skills[id]
			from := ui.ShortPath(skill.Path, "")
			switch {
			case skill.Generated():
				from = "generated"
			case skill.Package != "":
				from = c.Packages[skill.Package].Git
			}
			add(id, skill.Source, from)
		}
	case config.KindInstructions:
		for _, id := range slices.Sorted(maps.Keys(c.Instructions)) {
			i := c.Instructions[id]
			from := ui.ShortPath(i.Path, "")
			switch {
			case i.Text != "":
				from = "text"
			case i.Package != "":
				from = c.Packages[i.Package].Git
			}
			add(id, i.Source, from)
		}
	case config.KindBundles:
		for _, id := range slices.Sorted(maps.Keys(c.Bundles)) {
			add(id, c.Bundles[id].Source, "bundle")
		}
	case config.KindHarnesses:
		for _, name := range slices.Sorted(maps.Keys(c.Harnesses)) {
			description := "customized built-in"
			if base := c.Harnesses[name].Base; base != "" {
				description = "base " + base
			}
			add(config.ID(name), c.Harnesses[name].Source, description)
		}
	default:
	}
	return out
}

func mcpTarget(server config.MCPServer) string {
	if server.Transport == "stdio" {
		return strings.Join(server.Command, " ")
	}
	return server.URL
}

// resources completes use and unuse: every kind the ID may name, narrowed by --kind.
func (k *completer) resources(
	cmd *cobra.Command,
	args []string,
	toComplete string,
) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	only, _ := cmd.Flags().GetString("kind")
	out := []candidate{}
	for _, kind := range config.SelectionKinds {
		if only != "" && only != string(kind) {
			continue
		}
		for _, d := range k.definitions(kind, false) {
			out = append(out, candidate{d.value, kind.Label()})
		}
	}
	return offer(out, toComplete)
}

// secrets lists the secrets of the given kinds, or of every kind when none are given.
func (k *completer) secrets(kinds ...string) func() []candidate {
	return func() []candidate {
		c := k.load()
		if c == nil {
			return nil
		}
		out := []candidate{}
		for _, id := range slices.Sorted(maps.Keys(c.Secrets)) {
			secret := c.Secrets[id]
			if len(kinds) > 0 && !slices.Contains(kinds, secret.Kind) {
				continue
			}
			description := secret.Kind
			switch {
			case secret.Implicit:
				description += ", on demand"
			case secret.Owner != "":
				description += ", inline"
			}
			out = append(out, candidate{string(id), description})
		}
		return out
	}
}

// packages completes update, which takes several package IDs.
func (k *completer) packages(
	cmd *cobra.Command,
	args []string,
	toComplete string,
) ([]cobra.Completion, cobra.ShellCompDirective) {
	c := k.load()
	if c == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	out := []candidate{}
	for _, id := range slices.Sorted(maps.Keys(c.Packages)) {
		if slices.Contains(args, string(id)) {
			continue
		}
		p := c.Packages[id]
		source := p.Git
		if source == "" {
			source = ui.ShortPath(p.Path, "")
		}
		out = append(out, candidate{string(id), source})
	}
	return offer(out, toComplete)
}

// launches asks a daemon that already runs for its launches; completion never starts one.
func (k *completer) launches() []candidate {
	paths := *k.paths
	var err error
	if paths.Cache, err = filepath.Abs(paths.Cache); err != nil {
		return nil
	}
	if err := paths.Derive(); err != nil {
		return nil
	}
	c, err := newClient(paths)
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), completionDaemonTimeout)
	defer cancel()
	response, err := c.runtime.GetRuntime(ctx, connect.NewRequest(&v1.GetRuntimeRequest{}))
	if err != nil {
		return nil
	}
	out := []candidate{}
	for _, l := range response.Msg.Runtime.GetLaunches() {
		out = append(out, candidate{l.Id, l.Harness + " in " + ui.ShortPath(l.Directory, "")})
	}
	return out
}

// registerCompletions attaches completion to the command tree. Arguments and flag values offer
// no files unless a command says otherwise.
func (k *completer) registerCompletions(root *cobra.Command) {
	none := cobra.ShellCompDirectiveNoFileComp
	root.CompletionOptions.DefaultShellCompDirective = &none
	flags := map[string]cobra.CompletionFunc{
		"harness":    k.harnessList,
		"base":       flagValues(k.harnesses),
		"scope":      completeScope,
		"file":       yamlFiles,
		"config":     yamlFiles,
		"cache":      directories,
		"executable": files,
		"color":      fixed(string(ui.ColorAuto), string(ui.ColorAlways), string(ui.ColorNever)),
		"kind":       fixed(strings.Split(config.JoinKinds(config.SelectionKinds), ", ")...),
		"timeout":    fixed("5s", "10s", "30s", "1m"),
	}
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for name, complete := range flags {
			if cmd.NonInheritedFlags().Lookup(name) != nil {
				_ = cmd.RegisterFlagCompletionFunc(name, complete)
			}
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
	find := func(path ...string) *cobra.Command {
		cmd, _, err := root.Find(path)
		if err != nil || cmd == root {
			panic("completion: no command " + strings.Join(path, " "))
		}
		return cmd
	}
	_ = find("mcp", "add").RegisterFlagCompletionFunc("auth", fixed(
		"none\tno credential",
		"oauth\tOAuth login",
		"keychain\tsecret stored with agx secret set",
	))
	_ = find("mcp", "add").RegisterFlagCompletionFunc("load", fixed(
		"native\tin the harness's tool list",
		"on_demand\treached with agx mcp call",
	))
	_ = find("mcp", "add").RegisterFlagCompletionFunc("share", fixed(
		"none\ta process per harness session",
		"directory\tone process per launch directory",
		"repo\tone process per repository root",
		"global\tone process for every launch",
	))
	set := func(complete cobra.CompletionFunc, path ...string) {
		find(path...).ValidArgsFunction = complete
	}
	set(firstArg(func() []candidate { return k.definitions(config.KindMCPServers, true) }), "mcp", "remove")
	set(firstArg(func() []candidate { return k.definitions(config.KindMCPServers, false) }), "mcp", "tools")
	set(k.toolServers, "mcp", "call")
	set(func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		candidates := []candidate{}
		for _, c := range k.definitions(config.KindMCPServers, false) {
			if !slices.Contains(args, c.value) {
				candidates = append(candidates, c)
			}
		}
		return offer(candidates, toComplete)
	}, "mcp", "types")
	set(firstArg(func() []candidate { return k.definitions(config.KindSkills, true) }), "skill", "remove")
	set(firstArg(func() []candidate { return k.definitions(config.KindInstructions, true) }), "instructions", "remove")
	set(firstArg(func() []candidate { return k.definitions(config.KindHarnesses, true) }), "harness", "remove")
	set(k.resources, "use")
	set(k.resources, "unuse")
	set(firstArg(k.secrets("oauth")), "secret", "login")
	set(firstArg(k.secrets("oauth", "keychain")), "secret", "logout")
	set(firstArg(k.secrets("oauth")), "secret", "rotate")
	set(firstArg(k.secrets("keychain")), "secret", "set")
	set(firstArg(k.secrets()), "secret", "status")
	set(k.packages, "update")
	set(firstArg(k.launches), "launch", "show")
	set(firstArg(k.launches), "launch", "revoke")
	set(firstArgThen(directories), "explain")
	set(firstArgThen(directories), "doctor")
	set(firstArgThen(files), "skill", "add")
	set(firstArgThen(files), "instructions", "add")
	set(afterDash(files), "mcp", "add")
	set(afterDash(files), "harness", "add")
	set(k.run, "run")
}

// toolServers completes the <server>. of mcp call; tools come from the server, which completion
// does not reach.
func (k *completer) toolServers(
	cmd *cobra.Command,
	args []string,
	toComplete string,
) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	candidates := []candidate{}
	for _, c := range k.definitions(config.KindMCPServers, false) {
		candidates = append(candidates, candidate{c.value + ".", c.description})
	}
	out, directive := offer(candidates, toComplete)
	return out, directive | cobra.ShellCompDirectiveNoSpace
}

// run completes the harness name, then leaves native arguments to the shell.
func (k *completer) run(
	cmd *cobra.Command,
	args []string,
	toComplete string,
) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveDefault
	}
	return offer(k.harnesses(), toComplete)
}

func flagValues(candidates func() []candidate) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		return offer(candidates(), toComplete)
	}
}

// firstArgThen completes the first argument with complete and offers nothing after it.
func firstArgThen(complete cobra.CompletionFunc) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return complete(cmd, args, toComplete)
	}
}

// afterDash completes the native command after --, and nothing before it.
func afterDash(complete cobra.CompletionFunc) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if cmd.ArgsLenAtDash() < 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return complete(cmd, args, toComplete)
	}
}
