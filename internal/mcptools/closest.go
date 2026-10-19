// Package mcptools describes MCP tools to people and programs: suggestions for mistyped names,
// TypeScript-like signatures and declarations from JSON Schemas, and arguments coerced by them.
package mcptools

import (
	"slices"
	"strings"
)

// Closest returns up to n of candidates nearest to name, nearest first: those containing it or
// contained in it, then those within a small edit distance. It returns none when nothing is
// close.
func Closest(name string, candidates []string, n int) []string {
	type scored struct {
		name  string
		score int
	}
	lower := strings.ToLower(name)
	found := []scored{}
	for _, c := range candidates {
		other := strings.ToLower(c)
		distance := levenshtein(lower, other)
		limit := max(2, len(lower)/3)
		switch {
		case lower == "" || other == "":
		case strings.Contains(other, lower) || strings.Contains(lower, other):
			found = append(found, scored{c, min(distance, limit)})
		case distance <= limit:
			found = append(found, scored{c, distance})
		}
	}
	slices.SortStableFunc(found, func(a, b scored) int {
		if a.score != b.score {
			return a.score - b.score
		}
		return strings.Compare(a.name, b.name)
	})
	out := []string{}
	for _, f := range found[:min(n, len(found))] {
		out = append(out, f.name)
	}
	return out
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	previous := make([]int, len(rb)+1)
	current := make([]int, len(rb)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		current[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(rb)]
}

// Suggest is the end of a message about an unknown name: the closest candidates, or all of them
// when there are few, or nothing.
func Suggest(name string, candidates []string) string {
	close := Closest(name, candidates, 3)
	switch {
	case len(close) > 0:
		return "; did you mean " + strings.Join(close, ", ") + "?"
	case len(candidates) == 0:
		return ""
	case len(candidates) <= 8:
		return "; it has " + strings.Join(candidates, ", ")
	}
	return ""
}
