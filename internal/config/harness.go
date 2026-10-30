package config

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v4"
)

// ModePassthrough is the mode of invocations that run without a managed launch, such as
// help/version probes and native management commands; argument groups skip it unless named.
const ModePassthrough = "passthrough"

// Builtins describes what agx provides that configuration resolves against but cannot import:
// the built-in harnesses with the modes their adapters classify invocations into, and the
// command names harness definitions must not shadow.
type Builtins struct {
	Harnesses map[string][]string
	Commands  []string
	// Settings checks the native settings of definitions based on a built-in, given their
	// resolved settings and literal arguments; a built-in without an entry takes no settings.
	Settings map[string]func(settings map[string]any, args []string) error
}

// Bare returns a configuration without fragments, in which only the built-in harnesses resolve.
func Bare(builtins Builtins) *Config {
	return &Config{Harnesses: map[string]HarnessDefinition{}, builtins: builtins}
}

// ArgGroup is a list of native arguments scoped to modes; no modes means every session mode.
type ArgGroup struct {
	Values []Value  `yaml:"values" json:"values"`
	Modes  []string `yaml:"modes"  json:"modes,omitempty"`
}

// Args is a harness's argument groups: a flat list of strings is one group for every session mode.
type Args []ArgGroup

func (a *Args) UnmarshalYAML(node *yaml.Node) error {
	*a = nil
	if len(node.Content) == 0 {
		return nil
	}
	if node.Content[0].Kind == yaml.ScalarNode {
		var group ArgGroup
		if err := node.Load(&group.Values); err != nil {
			return err
		}
		*a = Args{group}
	} else if err := node.Load((*[]ArgGroup)(a), yaml.WithKnownFields()); err != nil {
		return err
	}
	for _, g := range *a {
		for _, v := range g.Values {
			// Process arguments are visible to every local user (ps, /proc), so credentials must
			// not reach them; !file stays allowed for non-secret content such as prompts.
			if v.Env != "" || v.Secret != "" {
				tag, name := "!env", v.Env
				if v.Secret != "" {
					tag, name = "!secret", string(v.Secret)
				}
				return &SourceError{Line: v.Source.Line, Err: fmt.Errorf(
					"%s cannot be used in harness args, which other processes can read (ps); "+
						"set it in the harness environment instead, such as environment: {NAME: %s %s}",
					tag, tag, name,
				)}
			}
		}
	}
	return nil
}

// Harness is one fragment's declaration of a harness.
type Harness struct {
	Base        string           `yaml:"base"        jsonschema_description:"Harness this definition extends."`
	Executable  string           `yaml:"executable"  jsonschema_description:"Absolute path of the native executable; may start with ~/."`
	Args        Args             `yaml:"args"        jsonschema_description:"Native arguments inserted before the caller's: a list of strings for every session mode, or a list of {values, modes} groups."`
	Environment map[string]Value `yaml:"environment" jsonschema_description:"Environment variables for the harness."`
	Add         SelectionSpec    `yaml:"add"         jsonschema_description:"Resources selected for this harness."`
	Remove      SelectionSpec    `yaml:"remove"      jsonschema_description:"Resources deselected for this harness."`
	Hidden      *bool            `yaml:"hidden"      jsonschema_description:"Leave this harness out of the launcher's new-session list; it still runs by name. Not inherited."`
	Settings    Settings         `yaml:"settings"    jsonschema_description:"Native settings in the harness's own schema: Codex config.toml keys, Claude Code settings.json keys."`
	Override    bool             `yaml:"override"    jsonschema_description:"Replace the definition from earlier fragments."`
}

// HarnessDefinition merges every fragment declaring one harness name.
type HarnessDefinition struct {
	Base        string             `json:"base,omitempty"`
	Executable  string             `json:"executable,omitempty"`
	Args        []ArgGroup         `json:"args,omitempty"`
	Environment map[string]Value   `json:"environment,omitempty"`
	Selections  []HarnessSelection `json:"selections,omitempty"`
	Hidden      bool               `json:"hidden,omitempty"`
	// Settings are the definition's own native settings; null leaves remove inherited keys.
	Settings map[string]any `json:"settings,omitempty"`
	// SettingSources locates each leaf of Settings, keyed by settingKey of its path.
	SettingSources map[string]Source `json:"-"`
	Source         Source            `json:"source"`
	// sources locates set-once fields (base, executable, environment.<name>) for conflict errors.
	sources map[string]Source
}

// HarnessSelection is one fragment's add/remove contribution to a harness.
type HarnessSelection struct {
	Add    Selection `json:"add"`
	Remove Selection `json:"remove"`
	Source Source    `json:"source"`
}

// ResolvedHarness layers a definition over its base chain.
type ResolvedHarness struct {
	Name    string `json:"name"`
	Builtin string `json:"builtin"`
	// Chain lists the built-in first and Name last.
	Chain       []string         `json:"chain"`
	Executable  string           `json:"executable"`
	Args        []ArgGroup       `json:"args"`
	Environment map[string]Value `json:"environment"`
	// Hidden keeps the harness out of the launcher's new-session list. Definitions based on it do
	// not inherit it, so hiding a built-in leaves its variants offered.
	Hidden bool `json:"hidden"`
	// Settings are the native settings layered along the chain, without nulls.
	Settings map[string]any `json:"settings"`
	// SettingSources locates each resolved leaf in the definition that set it.
	SettingSources []SettingSource `json:"setting_sources"`
}

// Arguments returns the argument groups that apply to mode, in order.
func (h ResolvedHarness) Arguments(mode string) []Value {
	out := []Value{}
	for _, g := range h.Args {
		if len(g.Modes) == 0 && mode != ModePassthrough || slices.Contains(g.Modes, mode) {
			out = append(out, g.Values...)
		}
	}
	return out
}

// Harness resolves a built-in or defined harness through its base chain.
func (c *Config) Harness(name string) (ResolvedHarness, error) {
	chain := []string{}
	for current := name; ; {
		if slices.Contains(chain, current) {
			return ResolvedHarness{}, fmt.Errorf(
				"harness base cycle: %s -> %s",
				strings.Join(chain, " -> "),
				current,
			)
		}
		chain = append(chain, current)
		if _, builtin := c.builtins.Harnesses[current]; builtin {
			break
		}
		def, defined := c.Harnesses[current]
		switch {
		case !defined && current == name:
			return ResolvedHarness{}, fmt.Errorf("unknown harness %q", name)
		case !defined:
			return ResolvedHarness{}, fmt.Errorf("harness %s: unknown base %q", chain[len(chain)-2], current)
		case def.Base == "":
			return ResolvedHarness{}, fmt.Errorf("harness %s needs a base harness", current)
		}
		current = def.Base
	}
	slices.Reverse(chain)
	h := ResolvedHarness{
		Name:        name,
		Builtin:     chain[0],
		Chain:       chain,
		Environment: map[string]Value{},
		Hidden:      c.Harnesses[name].Hidden,
	}
	settings := map[string]any{}
	for _, n := range chain {
		def := c.Harnesses[n]
		if def.Executable != "" {
			h.Executable = def.Executable
		}
		h.Args = append(h.Args, def.Args...)
		for key, value := range def.Environment {
			h.Environment[key] = value
		}
		settings = layerSettings(settings, def.Settings)
	}
	h.Settings = withoutNulls(settings)
	for _, leaf := range SettingLeaves(h.Settings) {
		// A resolved leaf comes whole from the last definition that set its path.
		for i := len(chain) - 1; i >= 0; i-- {
			if at, ok := c.Harnesses[chain[i]].SettingSources[settingKey(leaf.Path)]; ok {
				h.SettingSources = append(h.SettingSources, SettingSource{Path: leaf.Path, Source: at})
				break
			}
		}
	}
	return h, nil
}

// HarnessNames lists built-in and defined harnesses in lexical order.
func (c *Config) HarnessNames() []string {
	names := []string{}
	for name := range c.builtins.Harnesses {
		names = append(names, name)
	}
	for name := range c.Harnesses {
		if _, builtin := c.builtins.Harnesses[name]; !builtin {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (m *merger) harnesses(d Document) error {
	c := m.c
	for _, name := range sortedKeys(d.Harnesses) {
		h := d.Harnesses[name]
		at := func(keys ...string) Source { return m.at.at(append([]string{"harnesses", name}, keys...)...) }
		if h.Executable != "" {
			executable, err := ExpandHome(h.Executable)
			if err != nil {
				return errorAt(at("executable"), "harness %s: executable %w", name, err)
			}
			if !filepath.IsAbs(executable) {
				return errorAt(at("executable"), "harness %s: executable must be absolute", name)
			}
			h.Executable = filepath.Clean(executable)
		}
		def, exists := c.Harnesses[name]
		if h.Override || !exists || def.Source.Local && !at().Local {
			def = HarnessDefinition{
				Source:         at(),
				Environment:    map[string]Value{},
				SettingSources: map[string]Source{},
				sources:        map[string]Source{},
			}
		}
		set := func(field string, at Source, apply func()) error {
			if previous, ok := def.sources[field]; ok {
				return errorAt(
					at, "harness %s: %s already set at %s; set override: true to replace the definition",
					name,
					field,
					locate(previous),
				)
			}
			def.sources[field] = at
			apply()
			return nil
		}
		if h.Base != "" {
			if err := set("base", at("base"), func() { def.Base = h.Base }); err != nil {
				return err
			}
		}
		if h.Executable != "" {
			if err := set("executable", at("executable"), func() { def.Executable = h.Executable }); err != nil {
				return err
			}
		}
		if h.Hidden != nil {
			if err := set("hidden", at("hidden"), func() { def.Hidden = *h.Hidden }); err != nil {
				return err
			}
		}
		for _, key := range sortedKeys(h.Environment) {
			value, err := m.place(h.Environment[key])
			if err != nil {
				return err
			}
			if err := set(
				"environment."+key,
				at("environment", key),
				func() { def.Environment[key] = value },
			); err != nil {
				return err
			}
		}
		for _, g := range h.Args {
			values := make([]Value, len(g.Values))
			for i, v := range g.Values {
				var err error
				if values[i], err = m.place(v); err != nil {
					return err
				}
			}
			def.Args = append(def.Args, ArgGroup{Values: values, Modes: g.Modes})
		}
		if h.Settings.node != nil {
			settings, sources, err := m.settings(h.Settings, name)
			if err != nil {
				return err
			}
			if def.Settings == nil {
				def.Settings = map[string]any{}
			}
			conflict := func(path []string) error {
				previous := def.SettingSources[settingKey(path)]
				for key, at := range def.SettingSources {
					// The earlier fragment may have set a leaf below this path instead.
					if previous == (Source{}) && strings.HasPrefix(key, strings.TrimSuffix(settingKey(path), "]")+",") {
						previous = at
					}
				}
				return errorAt(
					at(
						"settings",
					),
					"harness %s: settings %s already set at %s; set override: true to replace the definition",
					name,
					FormatSettingPath(path),
					locate(previous),
				)
			}
			if err := mergeFragmentSettings(def.Settings, settings, nil, conflict); err != nil {
				return err
			}
			maps.Copy(def.SettingSources, sources)
		}
		add, err := m.selection(h.Add)
		if err != nil {
			return err
		}
		remove, err := m.selection(h.Remove)
		if err != nil {
			return err
		}
		if !add.empty() || !remove.empty() {
			def.Selections = append(def.Selections, HarnessSelection{Add: add, Remove: remove, Source: at()})
		}
		c.Harnesses[name] = def
	}
	return nil
}

// validateHarnesses checks definitions after every fragment merged, as bases may be declared later.
func (c *Config) validateHarnesses() error {
	names := []string{}
	for name := range c.Harnesses {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		def := c.Harnesses[name]
		_, builtin := c.builtins.Harnesses[name]
		if builtin && def.Base != "" {
			return errorAt(def.sources["base"], "built-in harness %s cannot set base", name)
		}
		if !builtin && slices.Contains(c.builtins.Commands, name) {
			return errorAt(def.Source, "harness %s would shadow the agx %s command", name, name)
		}
		h, err := c.Harness(name)
		if err != nil {
			return errorAt(def.Source, "%w", err)
		}
		modes := c.builtins.Harnesses[h.Builtin]
		if err := c.validateSettings(name, def, h); err != nil {
			return err
		}
		for _, g := range def.Args {
			for _, mode := range g.Modes {
				if !slices.Contains(modes, mode) {
					return errorAt(
						def.Source, "harness %s: %s has no mode %q (modes: %s)",
						name,
						h.Builtin,
						mode,
						strings.Join(modes, ", "),
					)
				}
			}
		}
	}
	for _, s := range c.Scopes {
		for _, name := range s.Harnesses {
			if _, err := c.Harness(name); err != nil {
				return errorAt(s.Source, "scope: %w", err)
			}
		}
	}
	return nil
}

func (s Selection) empty() bool {
	return len(s.Skills) == 0 && len(s.Instructions) == 0 && len(s.MCP) == 0 && len(s.Bundles) == 0
}

// validateSettings checks a definition's resolved settings against its built-in harness.
func (c *Config) validateSettings(name string, def HarnessDefinition, h ResolvedHarness) error {
	if len(h.Settings) == 0 {
		return nil
	}
	at := def.Source
	if len(h.SettingSources) > 0 {
		at = h.SettingSources[0].Source
	}
	check, ok := c.builtins.Settings[h.Builtin]
	if !ok {
		return errorAt(
			at, "harness %s: %s takes no settings; pass its options in args instead",
			name,
			h.Builtin,
		)
	}
	args := []string{}
	for _, g := range h.Args {
		for _, v := range g.Values {
			if !v.Tagged() {
				args = append(args, v.Literal)
			}
		}
	}
	if err := check(h.Settings, args); err != nil {
		return errorAt(at, "harness %s: %w", name, err)
	}
	return nil
}
