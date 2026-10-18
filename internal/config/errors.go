package config

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/tarik02-org/agx/internal/yaml"
)

var (
	// Decoding errors of nested values repeat the library's framing inside their message.
	nestedFraming = regexp.MustCompile(`yaml: construct errors: (?:line \d+: )?`)
	unknownField  = regexp.MustCompile(`field (\S+) not found in type (\S+)`)
	wrongKind     = regexp.MustCompile("cannot construct (!!\\w+)(?: `[^`]*`)? into (\\S+)(?: \\(TextUnmarshaler\\))?")
	wrongScalar   = regexp.MustCompile("cannot construct (!!\\w+) `([^`]*)` as a (!!\\w+)")
)

// yamlError rewrites a YAML parsing or decoding error of file for people: each problem is
// located by line and key path, such as mcp.github.url, and described in configuration terms
// rather than the Go types it decodes into. root is the parsed document, or nil when the file
// does not parse.
func yamlError(file string, data []byte, root *yaml.Node, err error) error {
	var many *yaml.LoadErrors
	var one *yaml.LoadError
	problems := []*yaml.LoadError{}
	switch {
	case errors.As(err, &many):
		problems = many.Errors
	case errors.As(err, &one):
		problems = append(problems, one)
	default:
		return fmt.Errorf("%s: %w", file, err)
	}
	messages := []string{}
	for _, p := range problems {
		messages = append(messages, describeProblem(file, data, root, p))
	}
	if len(messages) == 0 {
		return fmt.Errorf("%s: invalid YAML", file)
	}
	return errors.New(strings.Join(messages, "; "))
}

func describeProblem(file string, data []byte, root *yaml.Node, p *yaml.LoadError) string {
	line, column := p.Mark.Line, p.Mark.Column
	where := file
	if line > 0 {
		where += ":" + strconv.Itoa(line)
	}
	if root == nil {
		message := "invalid YAML: " + p.Message
		if p.Mark.Index >= 0 && p.Mark.Index < len(data) && data[p.Mark.Index] == '\t' {
			message += "; indent with spaces, as YAML does not allow tabs"
		}
		if p.ContextMsg != "" && p.ContextMark.Line > 0 && p.ContextMark.Line != line {
			message += fmt.Sprintf(" (%s started at line %d)", p.ContextMsg, p.ContextMark.Line)
		}
		return where + ": " + message
	}
	message := nestedFraming.ReplaceAllString(p.Message, "")
	message = unknownField.ReplaceAllStringFunc(message, func(match string) string {
		m := unknownField.FindStringSubmatch(match)
		message := "unknown key " + m[1]
		if suggestion := closestKey(m[1], yamlKeys(m[2])); suggestion != "" {
			message += " (did you mean " + suggestion + "?)"
		}
		return message
	})
	message = wrongKind.ReplaceAllStringFunc(message, func(match string) string {
		m := wrongKind.FindStringSubmatch(match)
		return "expected " + goKind(m[2]) + ", not " + tagKind(m[1])
	})
	message = wrongScalar.ReplaceAllStringFunc(message, func(match string) string {
		m := wrongScalar.FindStringSubmatch(match)
		return fmt.Sprintf("%q is not %s", m[2], tagKind(m[3]))
	})
	if path, ok := keyPath(root, line, column, strings.HasPrefix(message, "unknown key ")); ok && path != "" {
		return where + ": " + path + ": " + message
	}
	return where + ": " + message
}

// keyPath finds the node at line and column and spells its location in the document, such as
// scopes[0].add.skills. A key's own path is wanted for most problems; for an unknown key, its
// mapping's path, since the message names the key.
func keyPath(root *yaml.Node, line, column int, parentOfKey bool) (string, bool) {
	if root == nil || line == 0 {
		return "", false
	}
	var visit func(n *yaml.Node, path string) (string, bool)
	visit = func(n *yaml.Node, path string) (string, bool) {
		switch n.Kind {
		case yaml.DocumentNode:
			for _, child := range n.Content {
				if found, ok := visit(child, path); ok {
					return found, true
				}
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				key, value := n.Content[i], n.Content[i+1]
				child := key.Value
				if path != "" {
					child = path + "." + key.Value
				}
				if found, ok := visit(value, child); ok {
					return found, true
				}
				if key.Line == line && key.Column == column {
					if parentOfKey {
						return path, true
					}
					return child, true
				}
			}
		case yaml.SequenceNode:
			for i, item := range n.Content {
				if found, ok := visit(item, fmt.Sprintf("%s[%d]", path, i)); ok {
					return found, true
				}
			}
		default:
		}
		if n.Line == line && n.Column == column {
			return path, true
		}
		return "", false
	}
	return visit(root, "")
}

// goKind names what a Go type decodes from, in configuration terms.
func goKind(goType string) string {
	goType = strings.TrimPrefix(goType, "*")
	switch {
	case strings.HasPrefix(goType, "[]"):
		return "a list"
	case strings.HasPrefix(goType, "map["), strings.HasPrefix(goType, "config."):
		return "a mapping"
	case goType == "string", goType == "config.ID":
		return "a string"
	case goType == "bool":
		return "true or false"
	case strings.HasPrefix(goType, "int"), strings.HasPrefix(goType, "uint"), strings.HasPrefix(goType, "float"):
		return "a number"
	}
	return "another kind of value"
}

// tagKind names the kind of a YAML node by its resolved tag.
func tagKind(tag string) string {
	switch tag {
	case "!!str":
		return "a string"
	case "!!seq":
		return "a list"
	case "!!map":
		return "a mapping"
	case "!!int", "!!float":
		return "a number"
	case "!!bool":
		return "true or false"
	case "!!null":
		return "empty"
	}
	return tag
}

// documentTypes maps the Go type names a document decodes into, as decoding errors spell them,
// to their types.
var documentTypes = sync.OnceValue(func() map[string]reflect.Type {
	types := map[string]reflect.Type{}
	var visit func(t reflect.Type)
	visit = func(t reflect.Type) {
		switch t.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			visit(t.Elem())
		case reflect.Map:
			visit(t.Key())
			visit(t.Elem())
		case reflect.Struct:
			if _, seen := types[t.String()]; seen {
				return
			}
			types[t.String()] = t
			for field := range t.Fields() {
				visit(field.Type)
			}
		default:
		}
	}
	visit(reflect.TypeFor[Document]())
	return types
})

// yamlKeys lists the keys of the document type named goType, or nothing for a type decoded
// another way.
func yamlKeys(goType string) []string {
	t, ok := documentTypes()[strings.TrimPrefix(goType, "*")]
	if !ok {
		return nil
	}
	keys := []string{}
	for field := range t.Fields() {
		if name, _, _ := strings.Cut(field.Tag.Get("yaml"), ","); name != "" && name != "-" {
			keys = append(keys, name)
		}
	}
	return keys
}

// closestKey is the key among known that key most likely misspells or abbreviates, such as
// environment for env, or "" when none is close.
func closestKey(key string, known []string) string {
	best, bestDistance := "", 0
	for _, candidate := range known {
		distance := editDistance(key, candidate)
		abbreviates := len(key) >= 3 && strings.HasPrefix(candidate, key)
		if distance > max(1, len(candidate)/3) && !abbreviates {
			continue
		}
		if best == "" || distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	return best
}

// editDistance is the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous = current
	}
	return previous[len(b)]
}
