package mcptools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// SyntaxError is an argument written wrongly, such as a pair without "=": a usage error.
type SyntaxError struct{ Message string }

func (e SyntaxError) Error() string { return e.Message }

// ArgumentError is an argument the tool's schema rejects.
type ArgumentError struct{ Message string }

func (e ArgumentError) Error() string { return e.Message }

// Object is a JSON object that keeps its members in the order they were given.
type Object struct {
	keys   []string
	values map[string]any
}

// NewObject returns an empty object.
func NewObject() *Object { return &Object{values: map[string]any{}} }

// ParseObject decodes a JSON object, keeping its members' order.
func ParseObject(text []byte) (*Object, error) {
	d := json.NewDecoder(bytes.NewReader(text))
	d.UseNumber()
	if t, err := d.Token(); err != nil || t != json.Delim('{') {
		return nil, fmt.Errorf("not a JSON object")
	}
	o := NewObject()
	for d.More() {
		t, err := d.Token()
		if err != nil {
			return nil, err
		}
		key, _ := t.(string)
		var value any
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		o.Set(key, value)
	}
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err == nil {
		return nil, fmt.Errorf("text follows the JSON object")
	}
	return o, nil
}

// Set adds or replaces a member.
func (o *Object) Set(key string, value any) {
	if _, ok := o.values[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

// Get returns a member.
func (o *Object) Get(key string) (any, bool) {
	value, ok := o.values[key]
	return value, ok
}

// Keys lists the members in order.
func (o *Object) Keys() []string { return slices.Clone(o.keys) }

func (o *Object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, key := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(o.values[key])
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// Arguments builds a tool's arguments from base, the object --args gave or nil, and key=value
// pairs, which override it. Values are coerced by the tool's input schema unless raw: enums,
// numbers, integers, booleans, null, objects and arrays as JSON literals, and a single value
// into a one-item array; a repeated key adds items to an array. "key=@path" takes the file's
// content, read by readFile, and "@@" escapes a value's leading "@". With a schema, unknown
// keys and missing required ones are rejected.
func Arguments(
	schema *Schema,
	base *Object,
	pairs []string,
	raw bool,
	readFile func(string) ([]byte, error),
) (*Object, error) {
	out := NewObject()
	if base != nil {
		for _, key := range base.keys {
			out.Set(key, base.values[key])
		}
	}
	r := renderer{root: schema}
	root := r.resolve(schema)
	given := map[string]bool{}
	for _, pair := range pairs {
		key, text, ok := strings.Cut(pair, "=")
		if !ok || key == "" {
			return nil, SyntaxError{fmt.Sprintf("argument %q must be key=value", pair)}
		}
		switch {
		case strings.HasPrefix(text, "@@"):
			text = text[1:]
		case strings.HasPrefix(text, "@"):
			b, err := readFile(text[1:])
			if err != nil {
				return nil, SyntaxError{fmt.Sprintf("argument %s: %v", key, err)}
			}
			text = string(b)
		}
		property, known := propertySchema(root, key)
		if root != nil && !known {
			return nil, ArgumentError{unknownArgument(root, key)}
		}
		var value any = text
		if !raw {
			var err error
			if value, err = r.coerce(property, key, text); err != nil {
				return nil, err
			}
		}
		if previous, exists := out.Get(key); exists && given[key] {
			items, isArray := previous.([]any)
			if !isArray || !r.accepts(property, "array") {
				return nil, ArgumentError{fmt.Sprintf("argument %s is given twice and is not an array", key)}
			}
			added, isList := value.([]any)
			if !isList {
				added = []any{value}
			}
			value = slices.Concat(items, added)
		}
		given[key] = true
		out.Set(key, value)
	}
	if root == nil {
		return out, nil
	}
	for _, key := range out.keys {
		if _, known := propertySchema(root, key); !known {
			return nil, ArgumentError{unknownArgument(root, key)}
		}
	}
	missing := []string{}
	for _, name := range root.Required {
		if _, ok := out.Get(name); !ok {
			property := renderer{root: schema, compact: true}.typeOf(root.Properties[name], 1)
			missing = append(missing, name+" ("+property+")")
		}
	}
	if len(missing) > 0 {
		return nil, ArgumentError{"missing required arguments: " + strings.Join(missing, ", ")}
	}
	return out, nil
}

// propertySchema returns the schema of argument key, and whether the schema allows it. Without
// declared properties, or with additionalProperties, any key is allowed.
func propertySchema(root *Schema, key string) (*Schema, bool) {
	if root == nil {
		return nil, true
	}
	if property, ok := root.Properties[key]; ok {
		return property, true
	}
	if root.AdditionalProperties != nil {
		return root.AdditionalProperties, !rejectsAll(root.AdditionalProperties)
	}
	return nil, len(root.Properties) == 0
}

func unknownArgument(root *Schema, key string) string {
	names := slices.Sorted(maps.Keys(root.Properties))
	message := "unknown argument " + key + Suggest(key, names)
	if len(names) > 8 && !strings.Contains(message, "did you mean") {
		message += "; agx mcp tools <server> lists its arguments"
	}
	return message
}

// accepts reports whether s allows a value of JSON type t; an unknown schema allows anything.
func (r renderer) accepts(s *Schema, t string) bool {
	types := r.types(s)
	return len(types) == 0 || slices.Contains(types, t) || t == "number" && slices.Contains(types, "integer")
}

// types lists the JSON types s allows, empty when it does not say.
func (r renderer) types(s *Schema) []string {
	s = r.resolve(s)
	if s == nil {
		return nil
	}
	out := []string{}
	switch {
	case s.Const != nil:
		out = append(out, jsonType(*s.Const))
	case len(s.Enum) > 0:
		for _, v := range s.Enum {
			out = append(out, jsonType(v))
		}
	case len(s.AnyOf) > 0 || len(s.OneOf) > 0:
		for _, member := range append(slices.Clone(s.AnyOf), s.OneOf...) {
			member := r.types(member)
			if len(member) == 0 {
				return nil
			}
			out = append(out, member...)
		}
	case len(typeKeyword(s)) > 0:
		out = append(out, typeKeyword(s)...)
	case len(s.Properties) > 0:
		out = append(out, "object")
	case s.Items != nil:
		out = append(out, "array")
	}
	return dedupe(out)
}

func jsonType(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	}
	return "number"
}

// coerce turns the text of argument key into the value its schema s asks for.
func (r renderer) coerce(s *Schema, key, text string) (any, error) {
	s = r.resolve(s)
	if s == nil {
		s = &Schema{}
	}
	if enum := r.enum(s); len(enum) > 0 {
		names := []string{}
		for _, v := range enum {
			if matches(v, text) {
				return v, nil
			}
			b, _ := json.Marshal(v)
			names = append(names, string(b))
		}
		message := fmt.Sprintf("argument %s must be one of %s, not %q", key, strings.Join(names, ", "), text)
		return nil, ArgumentError{message}
	}
	types := r.types(s)
	if len(types) == 0 {
		if value, ok := jsonLiteral(text); ok {
			return value, nil
		}
		return text, nil
	}
	has := func(t string) bool { return slices.Contains(types, t) }
	switch {
	case has("null") && text == "null":
		return nil, nil
	case has("string"):
		return text, nil
	case has("boolean") && (text == "true" || text == "false"):
		return text == "true", nil
	case has("integer") && isInteger(text):
		return json.Number(text), nil
	case has("number") && isNumber(text):
		return json.Number(text), nil
	case has("object") && strings.HasPrefix(strings.TrimSpace(text), "{"):
		value, ok := jsonLiteral(text)
		if !ok {
			return nil, ArgumentError{fmt.Sprintf("argument %s is not a valid JSON object", key)}
		}
		return value, nil
	case has("array") && strings.HasPrefix(strings.TrimSpace(text), "["):
		value, ok := jsonLiteral(text)
		if !ok {
			return nil, ArgumentError{fmt.Sprintf("argument %s is not a valid JSON array", key)}
		}
		return value, nil
	case has("array"):
		item, err := r.coerce(r.items(s), key, text)
		if err != nil {
			return nil, err
		}
		return []any{item}, nil
	}
	return nil, ArgumentError{fmt.Sprintf(
		"argument %s must be %s, not %q",
		key,
		renderer{root: r.root, compact: true}.typeOf(s, 1),
		text,
	)}
}

// enum returns the values s allows when it lists them, through anyOf members of constants.
func (r renderer) enum(s *Schema) []any {
	switch {
	case s.Const != nil:
		return []any{*s.Const}
	case len(s.Enum) > 0:
		return s.Enum
	case len(s.AnyOf) > 0 || len(s.OneOf) > 0:
		out := []any{}
		for _, member := range append(slices.Clone(s.AnyOf), s.OneOf...) {
			values := r.enum(r.resolve(member))
			if len(values) == 0 {
				return nil
			}
			out = append(out, values...)
		}
		return out
	}
	return nil
}

// items is the item schema of an array schema, through anyOf members.
func (r renderer) items(s *Schema) *Schema {
	if s.Items != nil {
		return s.Items
	}
	for _, member := range append(slices.Clone(s.AnyOf), s.OneOf...) {
		if member := r.resolve(member); member != nil && member.Items != nil {
			return member.Items
		}
	}
	return &Schema{}
}

func matches(v any, text string) bool {
	switch v := v.(type) {
	case string:
		return v == text
	case nil:
		return text == "null"
	case bool:
		return strconv.FormatBool(v) == text
	case float64:
		f, err := strconv.ParseFloat(text, 64)
		return err == nil && f == v
	}
	return false
}

func isInteger(text string) bool {
	_, err := strconv.ParseInt(text, 10, 64)
	return err == nil
}

func isNumber(text string) bool {
	_, err := strconv.ParseFloat(text, 64)
	return err == nil && !strings.ContainsAny(text, "xXpP_") && !strings.EqualFold(text, "inf") &&
		!strings.EqualFold(text, "+inf") && !strings.EqualFold(text, "-inf") && !strings.EqualFold(text, "nan")
}

// jsonLiteral parses text as a JSON object or array.
func jsonLiteral(text string) (any, bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return nil, false
	}
	d := json.NewDecoder(strings.NewReader(trimmed))
	d.UseNumber()
	var value any
	if err := d.Decode(&value); err != nil {
		return nil, false
	}
	if _, err := d.Token(); err == nil {
		return nil, false
	}
	return value, true
}
