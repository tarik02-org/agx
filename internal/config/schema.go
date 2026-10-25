package config

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"github.com/invopop/jsonschema"
)

const schemaBase = "https://raw.githubusercontent.com/tarik02-org/agx/master/schema/"

var (
	idPattern          = `^[a-zA-Z0-9][a-zA-Z0-9._-]*(/[a-zA-Z0-9][a-zA-Z0-9._-]*)*$`
	environmentPattern = validEnvironment.String()
	// pathPattern matches selection entries read as paths: ., /, ~, or an absolute Windows path.
	pathPattern = `^([./~]|[A-Za-z]:[\\/]|\\\\)`
	// skillFileKey rejects absolute keys, . and .. segments, backslashes, and SKILL.md.
	skillFileKey = `^/|(^|/)\.{1,2}(/|$)|//|\\|(^|/)\.git(/|$)|^[Ss][Kk][Ii][Ll][Ll]\.[Mm][Dd]$`
)

// manifestExcluded lists the top-level fields package manifests cannot declare.
var manifestExcluded = []string{"imports", "scopes", "harnesses", "daemon"}

// Schema renders the JSON Schema of configuration fragments, or of package manifests.
func Schema(manifest bool, builtins Builtins) ([]byte, error) {
	r := &jsonschema.Reflector{
		FieldNameTag:               "yaml",
		RequiredFromJSONSchemaTags: true,
		ExpandedStruct:             true,
		Anonymous:                  true,
		// Argument modes come from the adapters, which a JSONSchema method cannot see.
		Mapper: func(t reflect.Type) *jsonschema.Schema {
			if t == reflect.TypeFor[Args]() {
				return &jsonschema.Schema{Ref: "#/$defs/Args"}
			}
			return nil
		},
	}
	s := r.Reflect(Document{})
	s.Definitions["Args"] = argsSchema(builtins)
	s.ID = schemaBase + "agx.schema.json"
	s.Title = "agx configuration fragment"
	if manifest {
		s.ID = schemaBase + "agx.package.schema.json"
		s.Title = "agx package manifest"
		for _, name := range manifestExcluded {
			s.Properties.Delete(name)
		}
		pruneDefinitions(s)
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// pruneDefinitions drops definitions only the removed manifest fields referenced.
func pruneDefinitions(root *jsonschema.Schema) {
	used := map[string]bool{}
	var visit func(*jsonschema.Schema)
	visit = func(s *jsonschema.Schema) {
		if s == nil {
			return
		}
		if name, ok := strings.CutPrefix(s.Ref, "#/$defs/"); ok && !used[name] {
			used[name] = true
			visit(root.Definitions[name])
		}
		children := slices.Concat(s.AllOf, s.AnyOf, s.OneOf, s.PrefixItems, []*jsonschema.Schema{
			s.Not, s.If, s.Then, s.Else, s.Items, s.Contains, s.AdditionalProperties, s.PropertyNames,
		})
		for _, child := range children {
			visit(child)
		}
		for _, child := range s.PatternProperties {
			visit(child)
		}
		if s.Properties != nil {
			for pair := s.Properties.Oldest(); pair != nil; pair = pair.Next() {
				visit(pair.Value)
			}
		}
	}
	visit(root)
	for name := range root.Definitions {
		if !used[name] {
			delete(root.Definitions, name)
		}
	}
}

func (Document) JSONSchemaExtend(s *jsonschema.Schema) {
	for _, name := range []string{"packages", "skills", "instructions", "mcp", "secrets", "bundles"} {
		property, _ := s.Properties.Get(name)
		property.PropertyNames = &jsonschema.Schema{Pattern: idPattern}
	}
	harnesses, _ := s.Properties.Get("harnesses")
	harnesses.PropertyNames = &jsonschema.Schema{Pattern: validIDSegment.String()}
}

// JSONSchema accepts a string: YAML editors validate a tagged scalar (!env, !file, !secret)
// as its string argument.
func (Value) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:        "string",
		Description: "A literal string, or !env NAME, !file PATH, or !secret ID resolved at launch.",
	}
}

func (Settings) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Description: "Native settings, in the harness's own configuration schema: Codex config.toml keys, " +
			"Claude Code settings.json keys. null removes an inherited key. !file reads a file; !secret " +
			"and !env are not allowed.",
	}
}

func (ID) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Pattern: idPattern}
}

// argsSchema accepts a flat list of values, or a list of {values, modes} groups; an empty list
// is both.
func argsSchema(builtins Builtins) *jsonschema.Schema {
	one := uint64(1)
	values := &jsonschema.Schema{Type: "array", Items: &jsonschema.Schema{Ref: "#/$defs/Value"}, MinItems: &one}
	modes := []any{}
	for _, builtin := range builtins.Harnesses {
		for _, mode := range builtin {
			if !slices.Contains(modes, any(mode)) {
				modes = append(modes, mode)
			}
		}
	}
	slices.SortFunc(modes, func(a, b any) int { return strings.Compare(a.(string), b.(string)) })
	properties := jsonschema.NewProperties()
	properties.Set("values", values)
	properties.Set("modes", &jsonschema.Schema{
		Type:        "array",
		Items:       &jsonschema.Schema{Enum: modes},
		MinItems:    &one,
		Description: "Session modes the group applies to; omit for every mode except passthrough.",
	})
	group := &jsonschema.Schema{
		Description:          "Native arguments limited to session modes.",
		Type:                 "object",
		Properties:           properties,
		Required:             []string{"values"},
		AdditionalProperties: jsonschema.FalseSchema,
	}
	flat := &jsonschema.Schema{Items: &jsonschema.Schema{Type: "string"}}
	return &jsonschema.Schema{
		Type: "array",
		If:   flat,
		Then: &jsonschema.Schema{Items: &jsonschema.Schema{Ref: "#/$defs/Value"}},
		Else: &jsonschema.Schema{Items: group},
	}
}

func (Harness) JSONSchemaExtend(s *jsonschema.Schema) {
	base, _ := s.Properties.Get("base")
	base.Pattern = validIDSegment.String()
	env, _ := s.Properties.Get("environment")
	// AGX_ variables carry launch grants and management credentials.
	env.PropertyNames = &jsonschema.Schema{
		Pattern: environmentPattern,
		Not:     &jsonschema.Schema{Pattern: `^[Aa][Gg][Xx]_`},
	}
}

func (Package) JSONSchemaExtend(s *jsonschema.Schema) {
	pinning(s)
	local := forbid("ref")
	local.Required = []string{"path"}
	s.If, s.Then, s.Else = requires("git"), forbid("path"), local
}

// pinning constrains the hash field of a source.
func pinning(s *jsonschema.Schema) {
	hash, _ := s.Properties.Get("hash")
	hash.Pattern = contentHashPattern.String()
}

var gitFields = []string{"git", "ref", "hash"}

func (Skill) JSONSchemaExtend(s *jsonschema.Schema) { skillSchema(s) }

func skillSchema(s *jsonschema.Schema) {
	pinning(s)
	name, _ := s.Properties.Get("name")
	name.Pattern = skillNamePattern.String()
	name.MaxLength = ptr(uint64(maxSkillName))
	frontmatter, _ := s.Properties.Get("frontmatter")
	frontmatter.PropertyNames = &jsonschema.Schema{Not: &jsonschema.Schema{Enum: []any{"name", "description"}}}
	files, _ := s.Properties.Get("files")
	files.PropertyNames = &jsonschema.Schema{MinLength: ptr(uint64(1)), Not: &jsonschema.Schema{Pattern: skillFileKey}}
	generated := forbid(append([]string{"path"}, gitFields...)...)
	generated.Required = []string{"description"}
	written := forbid("name", "description", "frontmatter", "files")
	written.If, written.Else = requires("git"), local()
	s.If, s.Then, s.Else = requires("content"), generated, written
}

// local matches a resource read from a local path.
func local() *jsonschema.Schema {
	s := forbid("ref", "hash")
	s.Required = []string{"path"}
	return s
}

func (Instructions) JSONSchemaExtend(s *jsonschema.Schema) { instructionsSchema(s) }

func instructionsSchema(s *jsonschema.Schema) {
	pinning(s)
	file := &jsonschema.Schema{Required: []string{"path"}}
	file.If, file.Else = requires("git"), local()
	s.If, s.Then, s.Else = requires("text"), forbid(append([]string{"path"}, gitFields...)...), file
}

// JSONSchema accepts {content, executable} or {path}.
func (SkillFile) JSONSchema() *jsonschema.Schema {
	properties := jsonschema.NewProperties()
	properties.Set("content", &jsonschema.Schema{Type: "string"})
	properties.Set("executable", &jsonschema.Schema{Type: "boolean"})
	properties.Set("path", &jsonschema.Schema{
		Type:        "string",
		MinLength:   ptr(uint64(1)),
		Description: "File to copy, relative to this file; ending in / copies a directory.",
	})
	copied := forbid("executable")
	copied.Required = []string{"path"}
	return &jsonschema.Schema{
		Description:          "{content, executable} for file text, or {path} to copy a file or directory.",
		Type:                 "object",
		Properties:           properties,
		AdditionalProperties: jsonschema.FalseSchema,
		If:                   requires("content"),
		Then:                 forbid("path"),
		Else:                 copied,
	}
}

// selectionEntry turns the reflected inline definition into one choice beside a string entry.
func selectionEntry(s *jsonschema.Schema, description string, reference *jsonschema.Schema) {
	s.Properties.Delete("override")
	inline := *s
	*s = jsonschema.Schema{Description: description, If: isString, Then: reference, Else: &inline}
}

func (SkillItem) JSONSchemaExtend(s *jsonschema.Schema) {
	skillSchema(s)
	s.AllOf = append(s.AllOf, &jsonschema.Schema{If: requires("content"), Then: requires("id")})
	selectionEntry(s, "A skill ID, a skill directory path, or an inline skill definition.", pathOrID())
}

func (InstructionsItem) JSONSchemaExtend(s *jsonschema.Schema) {
	instructionsSchema(s)
	s.AllOf = append(s.AllOf, &jsonschema.Schema{If: requires("text"), Then: requires("id")})
	selectionEntry(s, "An instructions ID, a file path, or an inline instructions definition.", pathOrID())
}

func (MCPItem) JSONSchemaExtend(s *jsonschema.Schema) {
	mcpSchema(s)
	s.Required = []string{"id"}
	selectionEntry(
		s,
		"An MCP server ID, or an inline MCP server definition with an id.",
		&jsonschema.Schema{Ref: "#/$defs/ID"},
	)
}

// isString decides between a string form and a mapping, which reports errors more readably than
// oneOf does.
var isString = &jsonschema.Schema{Type: "string"}

func pathOrID() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", AnyOf: []*jsonschema.Schema{{Pattern: idPattern}, {Pattern: pathPattern}}}
}

func ptr[T any](v T) *T { return &v }

func (MCPServer) JSONSchemaExtend(s *jsonschema.Schema) { mcpSchema(s) }

func mcpSchema(s *jsonschema.Schema) {
	env, _ := s.Properties.Get("environment")
	env.PropertyNames = &jsonschema.Schema{Pattern: environmentPattern}
	command, _ := s.Properties.Get("command")
	command.MinItems = ptr(uint64(1))
	command.PrefixItems = []*jsonschema.Schema{{MinLength: ptr(uint64(1))}}
	// Shared processes are for stdio; an HTTP server already serves many sessions.
	http := forbid("command", "environment", "cwd", "share")
	stdio := forbid("headers", "auth")
	stdio.Required = []string{"command"}
	s.If, s.Then, s.Else = requires("url"), http, stdio
}

// JSONSchema accepts a string for the kind shorthand, none, and !secret ID, which YAML editors
// validate as its string argument, or an inline secret mapping.
func (MCPShare) JSONSchema() *jsonschema.Schema {
	modes := []any{ShareNone, ShareDirectory, ShareRepository, ShareGlobal}
	properties := jsonschema.NewProperties()
	properties.Set("type", &jsonschema.Schema{Enum: modes})
	properties.Set("idle", &jsonschema.Schema{
		Type:        "string",
		Description: "How long the process stays after its last session, such as 30m; default 5m.",
	})
	return &jsonschema.Schema{
		Description: "stdio: none gives each harness session its own process (default); directory, " +
			"repo, or global share one process per launch directory, repository root, or " +
			"everywhere. {type, idle} also sets how long it stays after its last session.",
		If:   isString,
		Then: &jsonschema.Schema{Enum: modes},
		Else: &jsonschema.Schema{
			Type:                 "object",
			Properties:           properties,
			Required:             []string{"type"},
			AdditionalProperties: jsonschema.FalseSchema,
		},
	}
}

func (MCPAuth) JSONSchema() *jsonschema.Schema {
	inline := &jsonschema.Schema{Ref: "#/$defs/Secret", Not: requires("override")}
	return &jsonschema.Schema{
		Description: "http: bearer credential: oauth or keychain, none, an inline secret, or !secret ID, " +
			"!env NAME, or !file PATH; omitted means OAuth when the server asks for it.",
		If:   isString,
		Else: inline,
	}
}

func (Secret) JSONSchemaExtend(s *jsonschema.Schema) {
	kind, _ := s.Properties.Get("kind")
	kind.Enum = []any{"command", "keychain", "oauth", "sops"}
	kind.Description = "How the secret is obtained."
	command, _ := s.Properties.Get("command")
	command.MinItems = ptr(uint64(1))
	command.PrefixItems = []*jsonschema.Schema{{MinLength: ptr(uint64(1))}}
	s.AllOf = []*jsonschema.Schema{
		{
			If:   when("kind", &jsonschema.Schema{Const: "sops"}),
			Then: requires("file"),
			Else: forbid("file", "key", "age_key"),
		},
		{If: when("kind", &jsonschema.Schema{Const: "command"}), Then: requires("command")},
	}
}

// JSONSchema accepts a dotted key path or a list of segments.
func (SecretKey) JSONSchema() *jsonschema.Schema {
	segment := &jsonschema.Schema{Type: "string", MinLength: ptr(uint64(1))}
	return &jsonschema.Schema{
		If:   isString,
		Then: &jsonschema.Schema{Pattern: `^[^.]+(\.[^.]+)*$`},
		Else: &jsonschema.Schema{Type: "array", Items: segment, MinItems: ptr(uint64(1))},
	}
}

// requires matches objects that set the field.
func requires(field string) *jsonschema.Schema {
	return &jsonschema.Schema{Required: []string{field}}
}

// when matches objects whose field is set and matches value.
func when(field string, value *jsonschema.Schema) *jsonschema.Schema {
	s := forbid()
	s.Properties.Set(field, value)
	s.Required = []string{field}
	return s
}

// forbid matches objects that set none of the fields.
func forbid(fields ...string) *jsonschema.Schema {
	properties := jsonschema.NewProperties()
	for _, field := range fields {
		properties.Set(field, jsonschema.FalseSchema)
	}
	return &jsonschema.Schema{Properties: properties}
}
