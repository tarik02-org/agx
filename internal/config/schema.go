package config

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/invopop/jsonschema"
)

const schemaBase = "https://raw.githubusercontent.com/tarik02-org/agx/master/schema/"

var (
	idPattern          = `^[a-zA-Z0-9][a-zA-Z0-9._-]*(/[a-zA-Z0-9][a-zA-Z0-9._-]*)*$`
	environmentPattern = `^(?![Ww][Ee][Ff][Tt]_)[a-zA-Z_][a-zA-Z0-9_]*$`
	// pathPattern matches selection entries read as paths: ., /, ~, or an absolute Windows path.
	pathPattern = `^([./~]|[A-Za-z]:[\\/]|\\\\)`
	// skillFileKey rejects absolute keys, . and .. segments, backslashes, and SKILL.md.
	skillFileKey = `^/|(^|/)\.{1,2}(/|$)|//|\\|(^|/)\.git(/|$)|^[Ss][Kk][Ii][Ll][Ll]\.[Mm][Dd]$`
)

// manifestExcluded lists the top-level fields package manifests cannot declare.
var manifestExcluded = []string{"imports", "lockfile", "scopes", "harnesses", "daemon"}

// Schema renders the JSON Schema of configuration fragments, or of package manifests.
func Schema(manifest bool) ([]byte, error) {
	r := &jsonschema.Reflector{
		FieldNameTag:               "yaml",
		RequiredFromJSONSchemaTags: true,
		ExpandedStruct:             true,
		Anonymous:                  true,
	}
	s := r.Reflect(Document{})
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

func (ID) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Pattern: idPattern}
}

// JSONSchema accepts a flat list of values, or a list of groups; an empty list is both.
func (Args) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		AnyOf: []*jsonschema.Schema{
			{Type: "array", Items: &jsonschema.Schema{Ref: "#/$defs/Value"}},
			{Type: "array", Items: ArgGroup{}.JSONSchema()},
		},
	}
}

func (ArgGroup) JSONSchema() *jsonschema.Schema {
	one := uint64(1)
	values := &jsonschema.Schema{Type: "array", Items: &jsonschema.Schema{Ref: "#/$defs/Value"}, MinItems: &one}
	modes := []any{}
	for _, builtin := range Builtins {
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
	return &jsonschema.Schema{
		Description: "Native arguments: a list, or {values, modes} to limit them to session modes.",
		OneOf: []*jsonschema.Schema{
			values,
			{
				Type:                 "object",
				Properties:           properties,
				Required:             []string{"values"},
				AdditionalProperties: jsonschema.FalseSchema,
			},
		},
	}
}

func (Harness) JSONSchemaExtend(s *jsonschema.Schema) {
	base, _ := s.Properties.Get("base")
	base.Pattern = validIDSegment.String()
	env, _ := s.Properties.Get("env")
	env.PropertyNames = &jsonschema.Schema{Pattern: environmentPattern}
}

func (Import) JSONSchemaExtend(s *jsonschema.Schema) {
	s.OneOf = []*jsonschema.Schema{requires("path"), requires("glob")}
	s.If = requires("glob")
	s.Then = forbid()
	s.Then.Properties.Set("optional", &jsonschema.Schema{Const: false})
}

func (Package) JSONSchemaExtend(s *jsonschema.Schema) {
	pinning(s)
	s.OneOf = []*jsonschema.Schema{requires("path"), requires("git")}
	s.AllOf = append(s.AllOf, &jsonschema.Schema{If: requires("rev"), Then: requires("git")})
}

// pinning constrains the rev and hash fields of a source and keeps ref and rev apart.
func pinning(s *jsonschema.Schema) {
	rev, _ := s.Properties.Get("rev")
	rev.Pattern = revPattern.String()
	hash, _ := s.Properties.Get("hash")
	hash.Pattern = contentHashPattern.String()
	s.AllOf = append(s.AllOf, &jsonschema.Schema{Not: &jsonschema.Schema{Required: []string{"ref", "rev"}}})
}

var gitFields = []string{"git", "ref", "rev", "hash"}

func (Skill) JSONSchemaExtend(s *jsonschema.Schema) { skillSchema(s) }

func skillSchema(s *jsonschema.Schema) {
	pinning(s)
	name, _ := s.Properties.Get("name")
	name.Pattern = skillNamePattern.String()
	name.MaxLength = ptr(uint64(maxSkillName))
	frontmatter, _ := s.Properties.Get("frontmatter")
	frontmatter.PropertyNames = &jsonschema.Schema{Not: &jsonschema.Schema{Enum: []any{"name", "description"}}}
	files, _ := s.Properties.Get("files")
	files.PropertyNames = &jsonschema.Schema{Not: &jsonschema.Schema{Pattern: skillFileKey}}
	generatedOnly := []string{"name", "description", "frontmatter", "files"}
	generated := forbid(append([]string{"path"}, gitFields...)...)
	generated.Required = []string{"content", "description"}
	git := forbid(append([]string{"content"}, generatedOnly...)...)
	git.Required = []string{"git"}
	local := forbid(append(append([]string{"content"}, generatedOnly...), gitFields...)...)
	local.Required = []string{"path"}
	s.OneOf = []*jsonschema.Schema{generated, git, local}
}

func (Instructions) JSONSchemaExtend(s *jsonschema.Schema) { instructionsSchema(s) }

func instructionsSchema(s *jsonschema.Schema) {
	pinning(s)
	text := forbid(append([]string{"path"}, gitFields...)...)
	text.Required = []string{"text"}
	git := forbid("text")
	git.Required = []string{"git", "path"}
	local := forbid(append([]string{"text"}, gitFields...)...)
	local.Required = []string{"path"}
	s.OneOf = []*jsonschema.Schema{text, git, local}
}

// JSONSchema accepts text, a path to copy, or {content, executable}.
func (SkillFile) JSONSchema() *jsonschema.Schema {
	properties := jsonschema.NewProperties()
	properties.Set("content", &jsonschema.Schema{Type: "string"})
	properties.Set("executable", &jsonschema.Schema{Type: "boolean"})
	return &jsonschema.Schema{
		Description: "File text, a path to copy (ending in / for a directory), or {content, executable}.",
		OneOf: []*jsonschema.Schema{
			{Type: "string"},
			{
				Type:                 "object",
				Properties:           properties,
				Required:             []string{"content"},
				AdditionalProperties: jsonschema.FalseSchema,
			},
		},
	}
}

// selectionEntry turns the reflected inline definition into one choice beside a string entry.
func selectionEntry(s *jsonschema.Schema, description string, reference *jsonschema.Schema) {
	s.Properties.Delete("override")
	inline := *s
	*s = jsonschema.Schema{Description: description, OneOf: []*jsonschema.Schema{reference, &inline}}
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

func pathOrID() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", AnyOf: []*jsonschema.Schema{{Pattern: idPattern}, {Pattern: pathPattern}}}
}

func ptr[T any](v T) *T { return &v }

func (MCPServer) JSONSchemaExtend(s *jsonschema.Schema) { mcpSchema(s) }

func mcpSchema(s *jsonschema.Schema) {
	env, _ := s.Properties.Get("environment")
	env.PropertyNames = &jsonschema.Schema{Pattern: `^[a-zA-Z_][a-zA-Z0-9_]*$`}
	stdio := forbid("url", "headers", "auth")
	stdio.Properties.Set("transport", &jsonschema.Schema{Const: "stdio"})
	http := forbid("command", "environment", "cwd")
	http.Properties.Set("transport", &jsonschema.Schema{Const: "http"})
	s.OneOf = []*jsonschema.Schema{requires("url"), requires("command")}
	s.AllOf = []*jsonschema.Schema{
		{If: requires("command"), Then: stdio},
		{If: requires("url"), Then: http},
	}
}

// JSONSchema accepts a string for the kind shorthand, none, and !secret ID, which YAML editors
// validate as its string argument, or an inline secret mapping.
func (MCPAuth) JSONSchema() *jsonschema.Schema {
	inline := &jsonschema.Schema{Ref: "#/$defs/Secret", Not: requires("override")}
	return &jsonschema.Schema{
		Description: "http: bearer credential: oauth or keychain, none, an inline secret, or !secret ID; " +
			"omitted means OAuth when the server asks for it.",
		OneOf: []*jsonschema.Schema{
			{Type: "string", Pattern: idPattern},
			inline,
		},
	}
}

func (Secret) JSONSchemaExtend(s *jsonschema.Schema) {
	kind, _ := s.Properties.Get("kind")
	kind.Enum = []any{"environment", "daemon_environment", "command", "keychain", "oauth"}
	kind.Description = "How the secret is obtained."
	s.AllOf = []*jsonschema.Schema{
		{
			If:   when("kind", &jsonschema.Schema{Enum: []any{"environment", "daemon_environment"}}),
			Then: requires("variable"),
		},
		{If: when("kind", &jsonschema.Schema{Const: "command"}), Then: requires("command")},
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
