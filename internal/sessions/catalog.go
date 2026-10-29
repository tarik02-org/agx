package sessions

import (
	"cmp"
	"slices"
	"strings"

	"github.com/sahilm/fuzzy"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/securefs"
)

// Entry is a session with the harness that resumes it: the recorded definition, or the built-in.
type Entry struct {
	adapters.Session
	Builtin  string `json:"builtin"`
	Harness  string `json:"harness"`
	Recorded bool   `json:"recorded"`
	Store    string `json:"-"`
}

type Listing struct {
	// Entries are newest first.
	Entries []Entry
	Stats   Stats
	// Errors holds the stores that could not be listed, by built-in harness name.
	Errors map[string]error
}

// Within keeps the entries whose directory is dir or below it.
func Within(entries []Entry, dir string) []Entry {
	kept := []Entry{}
	for _, e := range entries {
		if securefs.Within(dir, e.Directory) {
			kept = append(kept, e)
		}
	}
	return kept
}

// Search keeps the entries matching every whitespace-separated term of query, each fuzzily
// within one field: harness, title, first message, or directory. Best matches come first, newest
// among equals. Matching within a field keeps letters scattered across fields from matching.
func Search(entries []Entry, query string) []Entry {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return entries
	}
	fields := []func(Entry) string{
		func(e Entry) string { return e.Harness + " " + e.Builtin },
		func(e Entry) string { return e.Title },
		func(e Entry) string { return e.Message },
		func(e Entry) string { return e.Directory },
	}
	sources := make([]source, len(fields))
	for f, field := range fields {
		sources[f] = make(source, len(entries))
		for i, e := range entries {
			sources[f][i] = field(e)
		}
	}
	scores := make([]int, len(entries))
	hits := make([]int, len(entries))
	for _, term := range terms {
		best := map[int]int{}
		for _, src := range sources {
			for _, m := range fuzzy.FindFromNoSort(term, src) {
				if score, ok := best[m.Index]; !ok || m.Score > score {
					best[m.Index] = m.Score
				}
			}
		}
		for i, score := range best {
			scores[i] += score
			hits[i]++
		}
	}
	order := []int{}
	for i := range entries {
		if hits[i] == len(terms) {
			order = append(order, i)
		}
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(scores[b], scores[a]) })
	matched := make([]Entry, len(order))
	for i, j := range order {
		matched[i] = entries[j]
	}
	return matched
}

type source []string

func (s source) String(i int) string { return s[i] }
func (s source) Len() int            { return len(s) }
