package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v4"
)

// Invocation modes that adapters classify native arguments into.
const (
	ModeInteractive = "interactive"
	ModeExec        = "exec"
	ModeResume      = "resume"
	ModeAppServer   = "app-server"
	ModePrint       = "print"
	ModeRPC         = "rpc"
	ModePassthrough = "passthrough"
)

// Builtins maps each built-in harness to the modes its adapter recognizes.
var Builtins = map[string][]string{
	"codex":  {ModeInteractive, ModeExec, ModeResume, ModeAppServer, ModePassthrough},
	"claude": {ModeInteractive, ModePrint, ModeResume, ModePassthrough},
	"pi":     {ModeInteractive, ModePrint, ModeRPC, ModePassthrough},
}

// reservedHarnessNames holds command names a definition must not shadow. The CLI derives them
// from its command tree, which this package cannot import.
var reservedHarnessNames = map[string]bool{}

// ReserveHarnessNames records command names that harness definitions cannot use.
func ReserveHarnessNames(names ...string) {
	for _, name := range names {
		reservedHarnessNames[name] = true
	}
}

// ArgGroup is a list of native arguments scoped to modes; no modes means every session mode.
type ArgGroup struct {
	Values []Value  `json:"values"`
	Modes  []string `json:"modes,omitempty"`
}

type argGroupFields struct {
	Values []Value  `yaml:"values"`
	Modes  []string `yaml:"modes"`
}

func (g *ArgGroup) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		return node.Load(&g.Values, yaml.WithKnownFields())
	case yaml.MappingNode:
		var plain argGroupFields
		if err := node.Load(&plain, yaml.WithKnownFields()); err != nil {
			return err
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "modes" && len(plain.Modes) == 0 {
				return errors.New("modes must not be empty; omit it to apply in every session mode")
			}
		}
		g.Values, g.Modes = plain.Values, plain.Modes
		return nil
	default:
		return errors.New("argument group must be a list of strings or {values, modes}")
	}
}

// Args is a harness's argument groups: a flat list of strings is one group for every session mode.
type Args []ArgGroup

func (a *Args) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.SequenceNode {
		return errors.New("args must be a list of strings or a list of argument groups")
	}
	scalars := 0
	for _, child := range node.Content {
		if child.Kind == yaml.ScalarNode {
			scalars++
		}
	}
	switch {
	case len(node.Content) == 0:
		*a = nil
		return nil
	case scalars == len(node.Content):
		var group ArgGroup
		if err := node.Load(&group.Values, yaml.WithKnownFields()); err != nil {
			return err
		}
		*a = Args{group}
		return nil
	case scalars > 0:
		return errors.New(
			"args mixes strings with argument groups; write a flat list of strings or a list of groups",
		)
	}
	var groups []ArgGroup
	if err := node.Load(&groups, yaml.WithKnownFields()); err != nil {
		return err
	}
	*a = groups
	return nil
}

// Harness is one fragment's declaration of a harness.
type Harness struct {
	Base       string           `yaml:"base"       jsonschema_description:"Harness this definition extends."`
	Executable string           `yaml:"executable" jsonschema_description:"Absolute path of the native executable; may start with ~/."`
	Args       Args             `yaml:"args"       jsonschema_description:"Native arguments inserted before the caller's: a list of strings for every session mode, or a list of groups."`
	Env        map[string]Value `yaml:"env"        jsonschema_description:"Environment variables for the harness."`
	Add        SelectionSpec    `yaml:"add"        jsonschema_description:"Resources selected for this harness."`
	Remove     SelectionSpec    `yaml:"remove"     jsonschema_description:"Resources deselected for this harness."`
	Override   bool             `yaml:"override"   jsonschema_description:"Replace the definition from earlier fragments."`
}

// HarnessDefinition merges every fragment declaring one harness name.
type HarnessDefinition struct {
	Base       string             `json:"base,omitempty"`
	Executable string             `json:"executable,omitempty"`
	Args       []ArgGroup         `json:"args,omitempty"`
	Env        map[string]Value   `json:"env,omitempty"`
	Selections []HarnessSelection `json:"selections,omitempty"`
	Source     Source             `json:"source"`
	// sources locates set-once fields (base, executable, env.<name>) for conflict errors.
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
	Name    string
	Builtin string
	// Chain lists the built-in first and Name last.
	Chain      []string
	Executable string
	Args       []ArgGroup
	Env        map[string]Value
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
		if _, builtin := Builtins[current]; builtin {
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
	h := ResolvedHarness{Name: name, Builtin: chain[0], Chain: chain, Env: map[string]Value{}}
	for _, n := range chain {
		def := c.Harnesses[n]
		if def.Executable != "" {
			h.Executable = def.Executable
		}
		h.Args = append(h.Args, def.Args...)
		for key, value := range def.Env {
			h.Env[key] = value
		}
	}
	return h, nil
}

// HarnessNames lists built-in and defined harnesses in lexical order.
func (c *Config) HarnessNames() []string {
	names := []string{}
	for name := range Builtins {
		names = append(names, name)
	}
	for name := range c.Harnesses {
		if _, builtin := Builtins[name]; !builtin {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (m *merger) harnesses(d Document) error {
	c, node, path := m.c, m.node, m.path
	names := []string{}
	for name := range d.Harnesses {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		h := d.Harnesses[name]
		if !validIDSegment.MatchString(name) {
			return fmt.Errorf("%s: invalid harness name %q", path, name)
		}
		at := func(keys ...string) Source {
			return position(node, path, append([]string{"harnesses", name}, keys...)...)
		}
		if h.Executable != "" {
			executable, err := expandHome(h.Executable)
			if err != nil {
				return fmt.Errorf("%s: harness %s: executable %w", locate(at("executable")), name, err)
			}
			if !filepath.IsAbs(executable) {
				return fmt.Errorf("%s: harness %s: executable must be absolute", locate(at("executable")), name)
			}
			h.Executable = filepath.Clean(executable)
		}
		def, exists := c.Harnesses[name]
		if h.Override || !exists {
			def = HarnessDefinition{Source: at(), Env: map[string]Value{}, sources: map[string]Source{}}
		}
		set := func(field string, at Source, apply func()) error {
			if previous, ok := def.sources[field]; ok {
				return fmt.Errorf(
					"%s: harness %s: %s already set at %s; set override: true to replace the definition",
					locate(at),
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
		for _, key := range sortedKeys(h.Env) {
			value, err := m.place(h.Env[key])
			if err != nil {
				return err
			}
			if err := set("env."+key, at("env", key), func() { def.Env[key] = value }); err != nil {
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
		_, builtin := Builtins[name]
		if builtin && def.Base != "" {
			return fmt.Errorf("%s: built-in harness %s cannot set base", locate(def.sources["base"]), name)
		}
		if !builtin && reservedHarnessNames[name] {
			return fmt.Errorf("%s: harness %s would shadow the agx %s command", locate(def.Source), name, name)
		}
		h, err := c.Harness(name)
		if err != nil {
			return fmt.Errorf("%s: %w", locate(def.Source), err)
		}
		modes := Builtins[h.Builtin]
		for _, g := range def.Args {
			if len(g.Values) == 0 {
				return fmt.Errorf("%s: harness %s: argument group needs values", locate(def.Source), name)
			}
			for _, mode := range g.Modes {
				if !slices.Contains(modes, mode) {
					return fmt.Errorf(
						"%s: harness %s: %s has no mode %q (modes: %s)",
						locate(def.Source),
						name,
						h.Builtin,
						mode,
						strings.Join(modes, ", "),
					)
				}
			}
		}
		for key := range def.Env {
			// AGX_ variables carry launch grants and management credentials.
			if !validEnvironment.MatchString(key) || strings.HasPrefix(strings.ToUpper(key), "AGX_") {
				return fmt.Errorf(
					"%s: harness %s: invalid environment name %q",
					locate(def.sources["env."+key]),
					name,
					key,
				)
			}
		}
	}
	for _, s := range c.Scopes {
		for _, name := range s.Harnesses {
			if _, err := c.Harness(name); err != nil {
				return fmt.Errorf("%s: scope: %w", locate(s.Source), err)
			}
		}
	}
	return nil
}

func (s Selection) empty() bool {
	return len(s.Skills) == 0 && len(s.Instructions) == 0 && len(s.MCP) == 0 && len(s.Bundles) == 0
}

func locate(s Source) string { return fmt.Sprintf("%s:%d", s.Path, s.Line) }

// position locates the mapping key at path, falling back to the nearest ancestor found.
func position(node *yaml.Node, file string, path ...string) Source {
	out := Source{Path: file, Line: 1, Column: 1}
	if node == nil || len(node.Content) == 0 {
		return out
	}
	n := node.Content[0]
	for _, key := range path {
		if n.Kind != yaml.MappingNode {
			return out
		}
		found := false
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == key {
				out.Line, out.Column = n.Content[i].Line, n.Content[i].Column
				n = n.Content[i+1]
				found = true
				break
			}
		}
		if !found {
			return out
		}
	}
	return out
}
