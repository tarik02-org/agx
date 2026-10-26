package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"

	validator "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"go.yaml.in/yaml/v4"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// positions locates each value of a file by its path of keys and list indexes.
type positions map[string]Source

func positionKey(keys []string) string { return strings.Join(keys, "\x00") }

// at locates the value at keys, or its nearest ancestor that exists.
func (p positions) at(keys ...string) Source {
	for i := len(keys); i > 0; i-- {
		if s, ok := p[positionKey(keys[:i])]; ok {
			return s
		}
	}
	return p[""]
}

// decode parses a configuration fragment, or a package manifest, validates it against the
// schema, and decodes it.
func decode(path string, data []byte, manifest bool, builtins Builtins) (Document, positions, error) {
	var d Document
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var root yaml.Node
	if err := dec.Decode(&root); errors.Is(err, io.EOF) {
		return d, nil, &SourceError{
			Path: path, Err: errors.New("file is empty; write {} for a fragment that declares nothing"),
		}
	} else if err != nil {
		return d, nil, yamlError(path, data, err)
	}
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return d, nil, &SourceError{Path: path, Err: errors.New("expected one YAML document")}
	}
	at := positions{"": {Path: path, Line: 1, Column: 1}}
	value, err := toJSON(path, &root, nil, at)
	if err != nil {
		return d, nil, err
	}
	s, err := compiledSchema(manifest, builtins)
	if err != nil {
		return d, nil, err
	}
	if err := s.compiled.Validate(value); err != nil {
		return d, nil, s.explain(at, err)
	}
	if err := root.Load(&d, yaml.WithKnownFields()); err != nil {
		forgetTags(&root)
		return d, nil, yamlError(path, data, err)
	}
	if err := checkTags(&root); err != nil {
		return d, nil, yamlError(path, data, err)
	}
	return d, at, nil
}

// toJSON converts a YAML tree to the values JSON decoding produces, recording where each is.
// Tagged scalars become their argument, which is what the schema describes.
func toJSON(file string, n *yaml.Node, keys []string, at positions) (any, error) {
	if custom(n.Tag) {
		switch {
		case n.Tag != "!env" && n.Tag != "!file" && n.Tag != "!secret":
			return nil, errorAt(Source{Path: file, Line: n.Line}, "unknown tag %s; use !env, !file, or !secret", n.Tag)
		case n.Kind != yaml.ScalarNode:
			return nil, errorAt(Source{Path: file, Line: n.Line}, "%s needs a scalar argument", n.Tag)
		}
		return n.Value, nil
	}
	switch n.Kind {
	case yaml.DocumentNode:
		return toJSON(file, n.Content[0], keys, at)
	case yaml.AliasNode:
		return toJSON(file, n.Alias, keys, at)
	case yaml.MappingNode:
		out := map[string]any{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i]
			child := append(slices.Clone(keys), key.Value)
			at[positionKey(child)] = Source{Path: file, Line: key.Line, Column: key.Column}
			value, err := toJSON(file, n.Content[i+1], child, at)
			if err != nil {
				return nil, err
			}
			out[key.Value] = value
		}
		return out, nil
	case yaml.SequenceNode:
		out := []any{}
		for i, item := range n.Content {
			child := append(slices.Clone(keys), strconv.Itoa(i))
			at[positionKey(child)] = Source{Path: file, Line: item.Line, Column: item.Column}
			value, err := toJSON(file, item, child, at)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, nil
	default:
	}
	switch n.ShortTag() {
	case "!!null":
		return nil, nil
	case "!!bool":
		var b bool
		if n.Load(&b) == nil {
			return b, nil
		}
	case "!!int":
		var i int64
		if n.Load(&i) == nil {
			return i, nil
		}
	case "!!float":
		var f float64
		if n.Load(&f) == nil {
			return f, nil
		}
	}
	return n.Value, nil
}

// custom reports whether a tag is an application tag rather than a YAML core one.
func custom(tag string) bool {
	return tag != "" && tag != "!" && !strings.HasPrefix(tag, "!!") && !strings.HasPrefix(tag, "tag:yaml.org,2002:")
}

// consumed holds the tagged nodes decoding interpreted, so a tag decoding ignored, such as one
// on a plain string field, is reported rather than dropped.
var consumed sync.Map

func consume(n *yaml.Node) { consumed.Store(n, true) }

// checkTags rejects tagged nodes that decoding did not consume, forgetting the ones it did.
func checkTags(root *yaml.Node) error {
	var misplaced error
	walk(root, func(n *yaml.Node) {
		if _, ok := consumed.LoadAndDelete(n); !ok && custom(n.Tag) && misplaced == nil {
			misplaced = nodeError(
				n,
				"%s is allowed only in harness environment and args, mcp environment, headers, and auth, and sops age_key",
				n.Tag,
			)
		}
	})
	return misplaced
}

func forgetTags(root *yaml.Node) { walk(root, func(n *yaml.Node) { consumed.Delete(n) }) }

func walk(n *yaml.Node, visit func(*yaml.Node)) {
	visit(n)
	for _, child := range n.Content {
		walk(child, visit)
	}
}

// schema is a compiled JSON Schema with the document it was compiled from.
type schema struct {
	compiled *validator.Schema
	document any
}

var schemas sync.Map

// compiledSchema compiles the schema of fragments or manifests once per set of harness modes.
func compiledSchema(manifest bool, builtins Builtins) (*schema, error) {
	key := fmt.Sprint(manifest, Digest(builtins.Harnesses))
	if s, ok := schemas.Load(key); ok {
		return s.(*schema), nil
	}
	b, err := Schema(manifest, builtins)
	if err != nil {
		return nil, err
	}
	document, err := validator.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	c := validator.NewCompiler()
	if err := c.AddResource("agx.json", document); err != nil {
		return nil, err
	}
	compiled, err := c.Compile("agx.json")
	if err != nil {
		return nil, err
	}
	s := &schema{compiled: compiled, document: document}
	schemas.Store(key, s)
	return s, nil
}

var printer = message.NewPrinter(language.English)

// explain turns a schema violation into a located error naming the offending key path.
func (s *schema) explain(at positions, err error) error {
	var v *validator.ValidationError
	if !errors.As(err, &v) {
		return err
	}
	parent := v
	for len(v.Causes) > 0 {
		if _, ok := v.ErrorKind.(*kind.PropertyNames); ok {
			break
		}
		parent, v = v, v.Causes[0]
	}
	keys := v.InstanceLocation
	if _, ok := v.ErrorKind.(*kind.PropertyNames); ok {
		// The validator reuses the location of a propertyNames error while it goes on, so the
		// mapping is the property, of the parent's object, whose schema holds propertyNames.
		pointer := strings.TrimSuffix(v.SchemaURL, "/propertyNames")
		name := strings.NewReplacer("~1", "/", "~0", "~").Replace(pointer[strings.LastIndex(pointer, "/")+1:])
		keys = append(slices.Clone(parent.InstanceLocation), name)
	}
	where := at.at(keys...)
	var problem string
	switch k := v.ErrorKind.(type) {
	case *kind.AdditionalProperties:
		key := k.Properties[0]
		where = at.at(append(slices.Clone(keys), key)...)
		problem = "unknown key " + key
		if suggestion := closestKey(key, s.properties(v.SchemaURL)); suggestion != "" {
			problem += " (did you mean " + suggestion + "?)"
		}
	case *kind.PropertyNames:
		where = at.at(append(slices.Clone(keys), k.Property)...)
		problem = fmt.Sprintf("key %q is not allowed", k.Property)
	case *kind.FalseSchema:
		problem = "not allowed here"
	case *kind.Required:
		problem = "missing " + strings.Join(k.Missing, ", ")
	case *kind.Type:
		problem = fmt.Sprintf("expected %s, not %s", jsonKind(k.Want[0]), jsonKind(k.Got))
	case *kind.Pattern:
		problem = fmt.Sprintf("%q is not valid here", k.Got)
	case *kind.Enum:
		want := []string{}
		for _, w := range k.Want {
			want = append(want, fmt.Sprint(w))
		}
		problem = fmt.Sprintf("%v must be one of %s", k.Got, strings.Join(want, ", "))
	case *kind.MinItems, *kind.MinLength:
		problem = "must not be empty"
	case *kind.MaxLength:
		problem = fmt.Sprintf("must be at most %d characters", k.Want)
	case *kind.Minimum:
		problem = fmt.Sprintf("must be at least %v", k.Want)
	default:
		problem = v.ErrorKind.LocalizedString(printer)
	}
	if path := keyPath(keys); path != "" {
		problem = path + ": " + problem
	}
	return &SourceError{Path: where.Path, Line: where.Line, Err: errors.New(problem)}
}

// properties lists the keys the schema object at url declares.
func (s *schema) properties(url string) []string {
	_, pointer, _ := strings.Cut(url, "#")
	node := s.document
	for _, token := range strings.Split(pointer, "/")[1:] {
		m, _ := node.(map[string]any)
		node = m[strings.NewReplacer("~1", "/", "~0", "~").Replace(token)]
	}
	m, _ := node.(map[string]any)
	properties, _ := m["properties"].(map[string]any)
	return slices.Sorted(maps.Keys(properties))
}

// keyPath spells a location in the document, such as scopes[0].add.skills.
func keyPath(keys []string) string {
	var b strings.Builder
	for _, key := range keys {
		if _, err := strconv.Atoi(key); err == nil {
			b.WriteString("[" + key + "]")
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(key)
	}
	return b.String()
}

// jsonKind names a JSON type in configuration terms.
func jsonKind(t string) string {
	switch t {
	case "object":
		return "a mapping"
	case "array":
		return "a list"
	case "string":
		return "a string"
	case "boolean":
		return "true or false"
	case "integer", "number":
		return "a number"
	case "null":
		return "empty"
	}
	return t
}
