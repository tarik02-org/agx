package sessions

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/sahilm/fuzzy"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/platform"
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

// match assigns entries their recorded harness and returns the records with newly matched
// sessions added and stale ones pruned, or nil when nothing changed.
func match(
	entries []Entry,
	records []Record,
	failed map[string]error,
	now time.Time,
	stores map[string]string,
) []Record {
	changed := false
	canonical := map[string]string{}
	for _, root := range stores {
		canonical[root] = root
	}
	for i := range records {
		root := records[i].Store
		if root == "" {
			continue
		}
		real, ok := canonical[root]
		if !ok {
			real = root
			if path, err := platform.Canonical(root); err == nil {
				real = path
			}
			canonical[root] = real
		}
		if root != real {
			records[i].Store = real
			changed = true
		}
	}
	owner := map[[3]string]int{}
	for i, r := range records {
		for _, id := range r.Sessions {
			key := [3]string{r.Builtin, r.Store, id}
			if j, ok := owner[key]; !ok || records[j].Started.Before(r.Started) {
				owner[key] = i
			}
		}
	}
	for i := range entries {
		e := &entries[i]
		key := [3]string{e.Builtin, e.Store, e.ID}
		j, ok := owner[key]
		if !ok {
			j = -1
			for k, r := range records {
				from, to := r.window()
				if r.Exact || r.Builtin != e.Builtin || r.Store != e.Store || r.Directory != e.Directory ||
					e.Started.Before(from) ||
					e.Started.After(to) {
					continue
				}
				// Concurrent launches in one directory: the latest to start before the session wins.
				if j < 0 || records[j].Started.Before(r.Started) {
					j = k
				}
			}
			if j < 0 {
				continue
			}
			records[j].Sessions = append(records[j].Sessions, e.ID)
			owner[key] = j
			changed = true
		}
		e.Harness = records[j].Harness
		e.Recorded = true
	}
	present := map[[3]string]bool{}
	for _, e := range entries {
		present[[3]string{e.Builtin, e.Store, e.ID}] = true
	}
	kept := records[:0]
	for _, r := range records {
		root, scanned := stores[r.Builtin]
		if !scanned || root != r.Store || failed[r.Builtin] != nil {
			kept = append(kept, r)
			continue
		}
		ids := r.Sessions[:0]
		for _, id := range r.Sessions {
			if present[[3]string{r.Builtin, r.Store, id}] {
				ids = append(ids, id)
			}
		}
		if len(ids) != len(r.Sessions) {
			changed = true
		}
		r.Sessions = ids
		_, to := r.window()
		if len(ids) == 0 && now.After(to) {
			changed = true
			continue
		}
		kept = append(kept, r)
	}
	if !changed {
		return nil
	}
	return kept
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
