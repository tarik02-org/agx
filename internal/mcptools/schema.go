package mcptools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Schema is the part of a JSON Schema that describes a tool's arguments and results well
// enough to render types and coerce values. Unknown keywords are ignored.
type Schema struct {
	Type        Types              `json:"type"`
	Enum        []any              `json:"enum"`
	Const       *json.RawMessage   `json:"const"`
	Properties  Properties         `json:"properties"`
	Required    []string           `json:"required"`
	Items       *Schema            `json:"items"`
	AnyOf       []*Schema          `json:"anyOf"`
	OneOf       []*Schema          `json:"oneOf"`
	AllOf       []*Schema          `json:"allOf"`
	Ref         string             `json:"$ref"`
	Defs        map[string]*Schema `json:"$defs"`
	Definitions map[string]*Schema `json:"definitions"`
	Description string             `json:"description"`
	// Additional is additionalProperties: nil when absent, AllowAll for true, or a schema.
	Additional *Additional `json:"additionalProperties"`
}

// Additional is an additionalProperties keyword.
type Additional struct {
	Allowed bool
	Schema  *Schema
}

func (a *Additional) UnmarshalJSON(b []byte) error {
	var allowed bool
	if json.Unmarshal(b, &allowed) == nil {
		a.Allowed = allowed
		return nil
	}
	a.Allowed = true
	a.Schema = &Schema{}
	return json.Unmarshal(b, a.Schema)
}

// Types is a type keyword, given as one name or a list.
type Types []string

func (t *Types) UnmarshalJSON(b []byte) error {
	var one string
	if json.Unmarshal(b, &one) == nil {
		*t = Types{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*t = many
	return nil
}

// Property is one property of an object schema.
type Property struct {
	Name   string
	Schema *Schema
}

// Properties keeps an object schema's properties in their order, which signatures follow.
type Properties []Property

func (p *Properties) UnmarshalJSON(b []byte) error {
	d := json.NewDecoder(bytes.NewReader(b))
	if t, err := d.Token(); err != nil || t != json.Delim('{') {
		return fmt.Errorf("properties must be an object")
	}
	for d.More() {
		t, err := d.Token()
		if err != nil {
			return err
		}
		name, _ := t.(string)
		s := &Schema{}
		if err := d.Decode(s); err != nil {
			return err
		}
		*p = append(*p, Property{Name: name, Schema: s})
	}
	_, err := d.Token()
	return err
}

// ordered lists the properties, required ones first.
func (s *Schema) ordered() Properties {
	out := Properties{}
	for _, required := range []bool{true, false} {
		for _, p := range s.Properties {
			if slices.Contains(s.Required, p.Name) == required {
				out = append(out, p)
			}
		}
	}
	return out
}

// Get returns the property named name.
func (p Properties) Get(name string) (*Schema, bool) {
	for _, property := range p {
		if property.Name == name {
			return property.Schema, true
		}
	}
	return nil, false
}

// Names lists the properties in order.
func (p Properties) Names() []string {
	names := []string{}
	for _, property := range p {
		names = append(names, property.Name)
	}
	return names
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
			b, _ := json.Marshal(v)
			parts = append(parts, literal(b))
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
	types := s.Type
	if len(types) == 0 {
		switch {
		case len(s.Properties) > 0 || s.Additional != nil:
			types = Types{"object"}
		case s.Items != nil:
			types = Types{"array"}
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
		if s.Additional != nil && s.Additional.Schema != nil {
			return "Record<string, " + r.typeOf(s.Additional.Schema, depth+1) + ">"
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
	properties := s.ordered()
	var b strings.Builder
	b.WriteString("{\n")
	for i, f := range fields {
		if doc := docComment(r.resolve(properties[i].Schema).Description, inner); doc != "" {
			b.WriteString(doc)
		}
		b.WriteString(inner + f + ";\n")
	}
	b.WriteString(strings.Repeat(r.indent, depth) + "}")
	return b.String()
}

// fields renders an object's properties as name: type, required ones first and optional ones
// marked with ?. Schemas usually reach agx re-encoded with sorted keys, so their own order is
// lost anyway.
func (r renderer) fields(s *Schema, depth int) []string {
	fields := []string{}
	for _, p := range s.ordered() {
		optional := "?"
		if slices.Contains(s.Required, p.Name) {
			optional = ""
		}
		fields = append(fields, propertyName(p.Name)+optional+": "+r.typeOf(p.Schema, depth+1))
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
		} else if s.Additional != nil && s.Additional.Allowed {
			params = "...args: " + r.object(s, 0)
		}
	}
	signature := name + "(" + params + ")"
	if output != nil && (len(output.Type) > 0 || len(output.Properties) > 0 || output.Ref != "") {
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

func literal(raw []byte) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return "unknown"
	}
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
