package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/tarik02-org/agx/internal/yaml"
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
	if node.Kind != yaml.ScalarNode {
		return errors.New("value must be a string, or a !env, !file, or !secret tag")
	}
	*v = Value{Source: Source{Line: node.Line, Column: node.Column}}
	switch node.Tag {
	case "!env":
		if !validEnvironment.MatchString(node.Value) {
			return fmt.Errorf("!env needs a variable name, not %q", node.Value)
		}
		// AGX_ variables carry launch grants and management credentials.
		if strings.HasPrefix(strings.ToUpper(node.Value), "AGX_") {
			return fmt.Errorf("!env %s: AGX_ variables are reserved", node.Value)
		}
		v.Env = node.Value
	case "!file":
		if node.Value == "" {
			return errors.New("!file needs a path")
		}
		v.File = node.Value
	case "!secret":
		if !validID(ID(node.Value)) {
			return fmt.Errorf("!secret needs a secret ID, not %q", node.Value)
		}
		v.Secret = ID(node.Value)
	default:
		if node.ShortTag() != "!!str" {
			return errors.New("value must be a string; quote numbers and booleans")
		}
		v.Literal = node.Value
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
		if secret.Override {
			return errors.New("inline auth cannot set override; override the MCP server instead")
		}
		a.Inline = &secret
	case node.Kind != yaml.ScalarNode:
		return errors.New("auth must be a secret kind, none, a mapping, or !secret ID")
	case node.Tag == "!secret":
		if !validID(ID(node.Value)) {
			return fmt.Errorf("!secret needs a secret ID, not %q", node.Value)
		}
		a.Secret = ID(node.Value)
	case node.ShortTag() != "!!str" || node.Value == "":
		return errors.New("auth must be a secret kind, none, a mapping, or !secret ID")
	case node.Value == "none":
		a.None = true
	default:
		a.Inline = &Secret{Kind: node.Value}
	}
	return nil
}

// SecretKey is the path of a value inside a sops file: a dotted string, or a list of segments
// when a segment contains a dot. Segments are map keys; list indexes are not supported.
type SecretKey []string

func (k *SecretKey) UnmarshalYAML(node *yaml.Node) error {
	*k = nil
	var segments []string
	switch {
	case node.Kind == yaml.ScalarNode && node.ShortTag() == "!!null":
		return nil
	case node.Kind == yaml.ScalarNode && node.ShortTag() == "!!str":
		segments = strings.Split(node.Value, ".")
	case node.Kind == yaml.SequenceNode:
		for _, item := range node.Content {
			if item.Kind != yaml.ScalarNode || item.ShortTag() != "!!str" {
				return errors.New("key segments must be strings; quote numbers")
			}
			segments = append(segments, item.Value)
		}
	default:
		return errors.New("key must be a dotted string or a list of strings")
	}
	if len(segments) == 0 {
		return errors.New("key must name at least one segment; omit it to read the whole file")
	}
	for _, segment := range segments {
		if segment == "" {
			return errors.New("key has an empty segment; write a list when a segment contains a dot")
		}
		// sops splits its --extract path at "][", which no quoting escapes.
		if strings.Contains(segment, "][") {
			return fmt.Errorf("key segment %q cannot contain ][", segment)
		}
	}
	*k = segments
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
		return "", fmt.Errorf("%s: %s: %w", locate(v.Source), target, err)
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
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("!file: %w", err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("!file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("!file %s is not a regular file", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxValueSize+1))
	if err != nil {
		return "", fmt.Errorf("!file: %w", err)
	}
	if len(data) > maxValueSize {
		return "", fmt.Errorf("!file %s exceeds 1 MiB", path)
	}
	return strings.TrimRight(string(data), "\r\n"), nil
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
		case s.Kind == "environment":
			return s.Variable
		case s.Kind == "sops":
			v = s.AgeKey
		default:
			return ""
		}
	}
	return ""
}

// taggedFields are the paths where tags may appear; "*" matches a key and "#" a list item.
var taggedFields = func() [][]string {
	out := [][]string{
		{"harnesses", "*", "environment", "*"},
		{"harnesses", "*", "args", "#"},
		{"harnesses", "*", "args", "#", "#"},
		{"harnesses", "*", "args", "#", "values", "#"},
	}
	// MCP servers are defined at top level or inline in any selection list.
	servers := [][]string{
		{"mcp", "*"},
		{"bundles", "*", "mcp", "#"},
		{"scopes", "#", "add", "mcp", "#"},
		{"scopes", "#", "remove", "mcp", "#"},
		{"harnesses", "*", "add", "mcp", "#"},
		{"harnesses", "*", "remove", "mcp", "#"},
	}
	out = append(out, []string{"secrets", "*", "age_key"})
	for _, server := range servers {
		for _, field := range [][]string{{"environment", "*"}, {"headers", "*"}, {"auth"}, {"auth", "age_key"}} {
			out = append(out, slices.Concat(server, field))
		}
	}
	return out
}()

// checkTags rejects unknown tags and tags outside taggedFields, which YAML decoding would
// otherwise silently ignore.
func checkTags(file string, node *yaml.Node, path []string) error {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.AliasNode {
		return checkTags(file, node.Alias, path)
	}
	tag := node.Tag
	if tag != "" && tag != "!" && !strings.HasPrefix(tag, "!!") && !strings.HasPrefix(tag, "tag:yaml.org,2002:") {
		switch tag {
		case "!env", "!file", "!secret":
		default:
			return fmt.Errorf("%s:%d: unknown tag %s; use !env, !file, or !secret", file, node.Line, tag)
		}
		if !slices.ContainsFunc(taggedFields, func(pattern []string) bool { return matchPath(pattern, path) }) {
			return fmt.Errorf(
				"%s:%d: %s is allowed only in harness environment and args, mcp environment, headers, and auth, and sops age_key",
				file,
				node.Line,
				tag,
			)
		}
		if node.Kind != yaml.ScalarNode {
			return fmt.Errorf("%s:%d: %s needs a scalar argument", file, node.Line, tag)
		}
		// Process arguments are visible to every local user (ps, /proc), so credentials must not
		// reach them; !file stays allowed for non-secret content such as prompts.
		if tag != "!file" && len(path) >= 3 && path[0] == "harnesses" && path[2] == "args" {
			return fmt.Errorf(
				"%s:%d: %s cannot be used in harness args, which other processes can read (ps); "+
					"set it in the harness environment instead, such as environment: {NAME: %s %s}",
				file,
				node.Line,
				tag,
				tag,
				node.Value,
			)
		}
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := checkTags(file, child, path); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			// Keys get a path no pattern matches, so tagged keys are rejected.
			if err := checkTags(file, node.Content[i], append(slices.Clone(path), "", "")); err != nil {
				return err
			}
			if err := checkTags(
				file,
				node.Content[i+1],
				append(slices.Clone(path), node.Content[i].Value),
			); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := checkTags(file, child, append(slices.Clone(path), "#")); err != nil {
				return err
			}
		}
	default:
	}
	return nil
}

func matchPath(pattern, path []string) bool {
	if len(pattern) != len(path) {
		return false
	}
	for i, segment := range pattern {
		if segment != "*" && segment != path[i] {
			return false
		}
	}
	return true
}

func sortedKeys[K ~string, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
