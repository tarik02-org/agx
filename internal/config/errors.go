package config

import (
	"errors"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v4"
)

// SourceError is a configuration problem located in a file; Line is 0 when the problem concerns
// the whole file.
type SourceError struct {
	Path string
	Line int
	Err  error
}

func (e *SourceError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d: %v", e.Path, e.Line, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Path, e.Err)
}

func (e *SourceError) Unwrap() error { return e.Err }

func errorAt(at Source, format string, args ...any) error {
	return &SourceError{Path: at.Path, Line: at.Line, Err: fmt.Errorf(format, args...)}
}

// nodeError locates a problem decoding node; decoding fills in the file.
func nodeError(node *yaml.Node, format string, args ...any) error {
	return &SourceError{Line: node.Line, Err: fmt.Errorf(format, args...)}
}

func locate(s Source) string { return fmt.Sprintf("%s:%d", s.Path, s.Line) }

// yamlError locates an error parsing or decoding file, which data holds.
func yamlError(file string, data []byte, err error) error {
	// Decoding errors of agx's own types carry their line; LoadErrors unwraps only to its first.
	var located *SourceError
	var p *yaml.LoadError
	switch {
	case errors.As(err, &located), errors.As(err, &p) && errors.As(p, &located):
		return &SourceError{Path: file, Line: located.Line, Err: located.Err}
	case p == nil:
		return &SourceError{Path: file, Err: err}
	}
	message := "invalid YAML: " + p.Message
	if p.Mark.Index >= 0 && p.Mark.Index < len(data) && data[p.Mark.Index] == '\t' {
		message += "; indent with spaces, as YAML does not allow tabs"
	}
	if p.ContextMsg != "" && p.ContextMark.Line > 0 && p.ContextMark.Line != p.Mark.Line {
		message += fmt.Sprintf(" (%s started at line %d)", p.ContextMsg, p.ContextMark.Line)
	}
	return &SourceError{Path: file, Line: p.Mark.Line, Err: errors.New(message)}
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
