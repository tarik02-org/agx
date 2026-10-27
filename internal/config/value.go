package config

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"
)

// maxValueSize bounds values read from files, matching the limit on stored secrets.
const maxValueSize = 1024 * 1024

// Value is a configuration string that is either literal or injected at launch: from the
// caller's environment (!env), a file (!file), or a secret (!secret). Exactly one field is set,
// except that an empty literal sets none. Only the tag and its argument are kept, so values
// never carry resolved content into snapshots or explanations.
type Value struct {
	Literal string `json:"literal,omitempty"`
	Env     string `json:"env,omitempty"`
	// File is absolute once merged.
	File   string `json:"file,omitempty"`
	Secret ID     `json:"secret,omitempty"`
	// Source is excluded from JSON so snapshot identity does not depend on line numbers.
	Source Source `json:"-"`
}

func (v *Value) UnmarshalYAML(node *yaml.Node) error {
	*v = Value{Source: Source{Line: node.Line, Column: node.Column}}
	consume(node)
	switch node.Tag {
	case "!env":
		if !validEnvironment.MatchString(node.Value) {
			return nodeError(node, "!env needs a variable name, not %q", node.Value)
		}
		// AGX_ variables carry launch grants and management credentials.
		if strings.HasPrefix(strings.ToUpper(node.Value), "AGX_") {
			return nodeError(node, "!env %s: AGX_ variables are reserved", node.Value)
		}
		v.Env = node.Value
	case "!file":
		if node.Value == "" {
			return nodeError(node, "!file needs a path")
		}
		v.File = node.Value
	case "!secret":
		if !validID(ID(node.Value)) {
			return nodeError(node, "!secret needs a secret ID, not %q", node.Value)
		}
		v.Secret = ID(node.Value)
	default:
		v.Literal = node.Value
	}
	return nil
}

func (s *MCPShare) UnmarshalYAML(node *yaml.Node) error {
	*s = MCPShare{Type: node.Value}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	var fields struct {
		Type string `yaml:"type"`
		Idle string `yaml:"idle"`
	}
	if err := node.Load(&fields); err != nil {
		return err
	}
	s.Type = fields.Type
	if fields.Idle != "" {
		idle, err := time.ParseDuration(fields.Idle)
		if err != nil || idle < 0 {
			return nodeError(node, "share idle must be a duration such as 30m, not %q", fields.Idle)
		}
		s.Idle = &idle
	}
	return nil
}

func (a *MCPAuth) UnmarshalYAML(node *yaml.Node) error {
	*a = MCPAuth{}
	switch {
	case node.Kind == yaml.MappingNode:
		var secret Secret
		if err := node.Load(&secret, yaml.WithKnownFields()); err != nil {
			return err
		}
		a.Inline = &secret
	case custom(node.Tag):
		return a.Value.UnmarshalYAML(node)
	case node.Value == "none":
		a.None = true
	case node.Value == "keychain" || node.Value == "oauth":
		a.Inline = &Secret{Kind: node.Value}
	default:
		return nodeError(
			node, "auth must be none, keychain, oauth, a secret mapping, or a !secret, !env, or !file value; "+
				"to use secret %s, write auth: !secret %s", node.Value, node.Value,
		)
	}
	return nil
}

// SecretKey is the path of a value inside a sops file: a dotted string, or a list of segments
// when a segment contains a dot. Segments are map keys; list indexes are not supported.
type SecretKey []string

func (k *SecretKey) UnmarshalYAML(node *yaml.Node) error {
	*k = strings.Split(node.Value, ".")
	if node.Kind == yaml.SequenceNode {
		if err := node.Load((*[]string)(k)); err != nil {
			return err
		}
	}
	for _, segment := range *k {
		// sops splits its --extract path at "][", which no quoting escapes.
		if strings.Contains(segment, "][") {
			return nodeError(node, "key segment %q cannot contain ][", segment)
		}
	}
	return nil
}

// Extract renders the key as the argument of sops decrypt --extract.
func (k SecretKey) Extract() string {
	return `["` + strings.Join(k, `"]["`) + `"]`
}

// Tagged reports whether the value is resolved at launch rather than literal.
func (v Value) Tagged() bool { return v.Env != "" || v.File != "" || v.Secret != "" }

// String renders the value as configured, with its tag; it never resolves it.
func (v Value) String() string {
	switch {
	case v.Env != "":
		return "!env " + v.Env
	case v.File != "":
		return "!file " + v.File
	case v.Secret != "":
		return "!secret " + string(v.Secret)
	}
	return v.Literal
}

// Resolve produces the value for a launch. env looks up the caller's environment, and secret
// resolves a secret ID. Errors name target and the configuration location, never the value.
func (v Value) Resolve(
	target string,
	env func(string) (string, bool),
	secret func(ID) (string, error),
) (string, error) {
	fail := func(err error) (string, error) {
		return "", errorAt(v.Source, "%s: %w", target, err)
	}
	value := v.Literal
	switch {
	case v.Env != "":
		found, ok := env(v.Env)
		if !ok || found == "" {
			return fail(fmt.Errorf("environment variable %s is not set", v.Env))
		}
		value = found
	case v.File != "":
		found, err := readValueFile(v.File)
		if err != nil {
			return fail(err)
		}
		value = found
	case v.Secret != "":
		found, err := secret(v.Secret)
		if err != nil {
			return fail(err)
		}
		value = found
	}
	if strings.ContainsRune(value, 0) {
		return fail(fmt.Errorf("%s resolves to a value containing a NUL byte", v))
	}
	return value, nil
}

// readValueFile drops trailing newlines, as shell command substitution does, so a token saved
// by an editor does not end in a line break.
func readValueFile(path string) (string, error) {
	data, err := readLimited(path)
	if err != nil {
		return "", fmt.Errorf("!file: %w", err)
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}

// readLimited reads a regular file of at most maxValueSize bytes.
func readLimited(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxValueSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxValueSize {
		return nil, fmt.Errorf("%s exceeds 1 MiB", path)
	}
	return data, nil
}

// Injection describes where a launch injects a tagged value, for explanations and checks.
type Injection struct {
	// Target names what the value configures, such as "mcp gh environment TOKEN".
	Target string `json:"target"`
	// Text is the value as configured, such as "!secret gh-token".
	Text   string `json:"value"`
	Source Source `json:"source"`
	Value  Value  `json:"-"`
}

func injection(target string, v Value) Injection {
	return Injection{Target: target, Text: v.String(), Source: v.Source, Value: v}
}

// Injections lists an MCP server's environment and headers in a stable order.
func (c MCPServer) Injections(id ID) []Injection {
	out := []Injection{}
	for _, field := range []struct {
		name   string
		values map[string]Value
	}{{"environment", c.Environment}, {"header", c.Headers}} {
		for _, key := range sortedKeys(field.values) {
			out = append(out, injection(fmt.Sprintf("mcp %s %s %s", id, field.name, key), field.values[key]))
		}
	}
	return out
}

// Secrets lists every secret the MCP server uses, in its auth field or injected with !secret.
func (c MCPServer) Secrets() []ID {
	ids := []ID{}
	if c.Auth.Secret != "" {
		ids = append(ids, c.Auth.Secret)
	}
	for _, i := range c.Injections("") {
		if i.Value.Secret != "" && !slices.Contains(ids, i.Value.Secret) {
			ids = append(ids, i.Value.Secret)
		}
	}
	return ids
}

// Injections lists the harness environment and every argument group, whatever its modes.
func (h ResolvedHarness) Injections() []Injection {
	out := []Injection{}
	for _, key := range sortedKeys(h.Environment) {
		out = append(out, injection(fmt.Sprintf("harness %s environment %s", h.Name, key), h.Environment[key]))
	}
	for _, g := range h.Args {
		for _, v := range g.Values {
			out = append(out, injection("harness "+h.Name+" args", v))
		}
	}
	return out
}

// Resolve produces the argument groups for mode and the environment at launch.
func (h ResolvedHarness) Resolve(
	mode string,
	env func(string) (string, bool),
	secret func(ID) (string, error),
) ([]string, map[string]string, error) {
	args := []string{}
	for _, v := range h.Arguments(mode) {
		value, err := v.Resolve("harness "+h.Name+" args", env, secret)
		if err != nil {
			return nil, nil, err
		}
		args = append(args, value)
	}
	environment := map[string]string{}
	for _, key := range sortedKeys(h.Environment) {
		value, err := h.Environment[key].Resolve(fmt.Sprintf("harness %s environment %s", h.Name, key), env, secret)
		if err != nil {
			return nil, nil, err
		}
		environment[key] = value
	}
	return args, environment, nil
}

// Variable returns the caller environment variable a value needs at launch, or "". A sops
// secret needs the variable its age key does.
func (c *Config) Variable(v Value) string {
	return Variable(c.Secrets, v)
}

// Variable returns the caller environment variable a value needs at launch, given the secrets
// its !secret references resolve against, or "".
func Variable(secrets map[ID]Secret, v Value) string {
	// Validation rejects age_key cycles; the bound keeps an unvalidated configuration finite.
	for range len(secrets) + 1 {
		if v.Env != "" {
			return v.Env
		}
		s, ok := secrets[v.Secret]
		switch {
		case !ok || v.Secret == "":
			return ""
		case s.Kind == "sops":
			v = s.AgeKey
		default:
			return ""
		}
	}
	return ""
}

func sortedKeys[K ~string, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
