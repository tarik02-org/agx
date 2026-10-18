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
	version, _ := s.Properties.Get("version")
	version.Const = 1
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
	s.OneOf = []*jsonschema.Schema{requires("path"), requires("git")}
}

func (MCPServer) JSONSchemaExtend(s *jsonschema.Schema) {
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
