package mcptools

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// Schema is a JSON Schema describing a tool's arguments or result.
type Schema = jsonschema.Schema

// typeKeyword lists the types s's type keyword names, given as one name or a list.
func typeKeyword(s *Schema) []string {
	if s.Type != "" {
		return []string{s.Type}
	}
	return s.Types
}

// rejectsAll reports whether s is the false schema, which jsonschema-go decodes as {"not": {}}.
func rejectsAll(s *Schema) bool {
	return s.Not != nil
}

// ordered lists s's property names, required ones first. Schemas usually reach agx re-encoded
// with sorted keys, so sorting each group keeps the order they arrive in.
func ordered(s *Schema) []string {
	out := []string{}
	for _, required := range []bool{true, false} {
		for _, name := range slices.Sorted(maps.Keys(s.Properties)) {
			if slices.Contains(s.Required, name) == required {
				out = append(out, name)
			}
		}
	}
	return out
}

// ParseSchema decodes a JSON Schema; an empty text is a schema that accepts anything.
func ParseSchema(text string) (*Schema, error) {
	s := &Schema{}
	if strings.TrimSpace(text) == "" {
		return s, nil
	}
	if err := json.Unmarshal([]byte(text), s); err != nil {
		return nil, fmt.Errorf("invalid JSON Schema: %w", err)
	}
	return s, nil
}

// renderer turns schemas into TypeScript types, resolving local references against root.
type renderer struct {
	root *Schema
	// compact abbreviates nested objects beyond the first level, for one-line signatures.
	compact bool
	// indent is the indentation of multi-line object types; empty renders them on one line.
	indent string
}

// maxDepth stops rendering references that recurse.
const maxDepth = 8

// resolve follows a local $ref.
func (r renderer) resolve(s *Schema) *Schema {
	for range maxDepth {
		if s == nil || s.Ref == "" {
			return s
		}
		var target *Schema
		switch {
		case strings.HasPrefix(s.Ref, "#/$defs/"):
			target = r.root.Defs[strings.TrimPrefix(s.Ref, "#/$defs/")]
		case strings.HasPrefix(s.Ref, "#/definitions/"):
			target = r.root.Definitions[strings.TrimPrefix(s.Ref, "#/definitions/")]
		case s.Ref == "#":
			target = r.root
		}
		if target == nil {
			return &Schema{}
		}
		s = target
	}
	return &Schema{}
}

// typeOf renders s as a TypeScript type.
func (r renderer) typeOf(s *Schema, depth int) string {
	s = r.resolve(s)
	if s == nil || depth > maxDepth {
		return "unknown"
	}
	switch {
	case s.Const != nil:
		return literal(*s.Const)
	case len(s.Enum) > 0:
		parts := []string{}
		for _, v := range s.Enum {
			parts = append(parts, literal(v))
		}
		return union(parts)
	case len(s.AnyOf) > 0 || len(s.OneOf) > 0:
		parts := []string{}
		for _, member := range append(slices.Clone(s.AnyOf), s.OneOf...) {
			parts = append(parts, r.typeOf(member, depth+1))
		}
		return union(parts)
	case len(s.AllOf) > 0:
		parts := []string{}
		for _, member := range s.AllOf {
			parts = append(parts, wrap(r.typeOf(member, depth+1)))
		}
		return strings.Join(dedupe(parts), " & ")
	}
	types := typeKeyword(s)
	if len(types) == 0 {
		switch {
		case len(s.Properties) > 0 || s.AdditionalProperties != nil:
			types = []string{"object"}
		case s.Items != nil:
			types = []string{"array"}
		default:
			return "unknown"
		}
	}
	parts := []string{}
	for _, t := range types {
		switch t {
		case "string", "boolean", "null":
			parts = append(parts, t)
		case "number", "integer":
			parts = append(parts, "number")
		case "array":
			if s.Items == nil {
				parts = append(parts, "unknown[]")
			} else {
				parts = append(parts, wrap(r.typeOf(s.Items, depth+1))+"[]")
			}
		case "object":
			parts = append(parts, r.object(s, depth))
		default:
			parts = append(parts, "unknown")
		}
	}
	return union(parts)
}

func (r renderer) object(s *Schema, depth int) string {
	if len(s.Properties) == 0 {
		if s.AdditionalProperties != nil {
			return "Record<string, " + r.typeOf(s.AdditionalProperties, depth+1) + ">"
		}
		return "Record<string, unknown>"
	}
	if r.compact && depth > 1 {
		return "{…}"
	}
	fields := r.fields(s, depth)
	if r.indent == "" {
		return "{ " + strings.Join(fields, "; ") + " }"
	}
	inner := strings.Repeat(r.indent, depth+1)
	names := ordered(s)
	var b strings.Builder
	b.WriteString("{\n")
	for i, f := range fields {
		if property := r.resolve(s.Properties[names[i]]); property != nil {
			b.WriteString(docComment(property.Description, inner))
		}
		b.WriteString(inner + f + ";\n")
	}
	b.WriteString(strings.Repeat(r.indent, depth) + "}")
	return b.String()
}

// fields renders an object's properties as name: type, required ones first and optional ones
// marked with ?.
func (r renderer) fields(s *Schema, depth int) []string {
	fields := []string{}
	for _, name := range ordered(s) {
		optional := "?"
		if slices.Contains(s.Required, name) {
			optional = ""
		}
		fields = append(fields, propertyName(name)+optional+": "+r.typeOf(s.Properties[name], depth+1))
	}
	return fields
}

// Signature renders a tool compactly and TypeScript-like: its name, its arguments as
// parameters, and its structured result when it declares one.
func Signature(name string, input, output *Schema) string {
	r := renderer{root: input, compact: true}
	params := ""
	if s := r.resolve(input); s != nil {
		if len(s.Properties) > 0 {
			params = strings.Join(r.fields(s, 0), ", ")
		} else if s.AdditionalProperties != nil && !rejectsAll(s.AdditionalProperties) {
			params = "...args: " + r.object(s, 0)
		}
	}
	signature := name + "(" + params + ")"
	if output != nil && (len(typeKeyword(output)) > 0 || len(output.Properties) > 0 || output.Ref != "") {
		o := renderer{root: output, compact: true}
		signature += ": " + o.typeOf(output, 0)
	}
	return signature
}

// FirstLine is the first line of a description, shortened to about limit characters.
func FirstLine(text string, limit int) string {
	text = strings.TrimSpace(text)
	if i := strings.IndexAny(text, "\r\n"); i >= 0 {
		text = strings.TrimSpace(text[:i])
	}
	runes := []rune(text)
	if len(runes) > limit {
		return strings.TrimSpace(string(runes[:limit-1])) + "…"
	}
	return text
}

func literal(v any) string {
	switch v := v.(type) {
	case string:
		return strconv.Quote(v)
	case nil:
		return "null"
	case bool, float64:
		b, _ := json.Marshal(v)
		return string(b)
	}
	return "unknown"
}

func union(parts []string) string {
	parts = dedupe(parts)
	if slices.Contains(parts, "unknown") {
		return "unknown"
	}
	return strings.Join(parts, " | ")
}

func dedupe(parts []string) []string {
	out := []string{}
	for _, p := range parts {
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// wrap parenthesizes a union or intersection inside an array or intersection.
func wrap(t string) string {
	if strings.Contains(t, " | ") || strings.Contains(t, " & ") {
		return "(" + t + ")"
	}
	return t
}

// propertyName quotes a property name that is not an identifier.
func propertyName(name string) string {
	if identifier(name) {
		return name
	}
	return strconv.Quote(name)
}

func identifier(name string) bool {
	if name == "" {
		return false
	}
	for i, c := range name {
		letter := c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		if !letter && (i == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// docComment renders text as a JSDoc comment at indent, or nothing for empty text.
func docComment(text, indent string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = strings.ReplaceAll(text, "*/", "*\\/")
	lines := strings.Split(text, "\n")
	if len(lines) == 1 {
		return indent + "/** " + lines[0] + " */\n"
	}
	var b strings.Builder
	b.WriteString(indent + "/**\n")
	for _, line := range lines {
		b.WriteString(strings.TrimRight(indent+" * "+strings.TrimRight(line, "\r"), " ") + "\n")
	}
	b.WriteString(indent + " */\n")
	return b.String()
}
