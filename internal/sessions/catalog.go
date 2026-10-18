package sessions

import (
	"cmp"
	"context"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/sahilm/fuzzy"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/securefs"
)

// Entry is a session with the harness that resumes it: the recorded definition, or the built-in.
type Entry struct {
	adapters.Session
	Builtin  string `json:"builtin"`
	Harness  string `json:"harness"`
	Recorded bool   `json:"recorded"`
}

type Listing struct {
	// Entries are newest first.
	Entries []Entry
	Stats   Stats
	// Errors holds the stores that could not be listed, by built-in harness name.
	Errors map[string]error
}

// List reads every harness store, matches sessions to launch records, and prunes records whose
// sessions are gone. The index lives under cache; records under state.
func List(ctx context.Context, registry adapters.Registry, cache, state string) (Listing, error) {
	index := OpenIndex(filepath.Join(cache, "sessions", "index.json"))
	listing := Listing{Errors: map[string]error{}}
	byKey := map[[2]string]int{}
	canonical := map[string]string{}
	for _, name := range registry.Names() {
		a, _ := registry.Adapter(name)
		found, err := a.Sessions(ctx, index)
		if err != nil {
			listing.Errors[name] = err
			continue
		}
		for _, s := range found {
			dir, ok := canonical[s.Directory]
			if !ok {
				// A deleted directory canonicalizes through its nearest existing parent.
				dir = filepath.Clean(s.Directory)
				if c, err := config.Canonical(s.Directory); err == nil {
					dir = c
				}
				canonical[s.Directory] = dir
			}
			s.Directory = dir
			key := [2]string{name, s.ID}
			// The same ID in two files, as after a copy or an interrupted rewrite: the newer wins.
			if i, ok := byKey[key]; ok {
				if s.Updated.After(listing.Entries[i].Updated) {
					listing.Entries[i].Session = s
				}
				continue
			}
			byKey[key] = len(listing.Entries)
			listing.Entries = append(listing.Entries, Entry{Session: s, Builtin: name, Harness: name})
		}
	}
	if err := ctx.Err(); err != nil {
		return Listing{}, err
	}
	if err := NewRecords(state).update(func(records []Record) []Record {
		return match(listing.Entries, records, listing.Errors, time.Now())
	}); err != nil {
		return Listing{}, err
	}
	listing.Stats = index.Stats()
	if err := index.Save(len(listing.Errors) == 0); err != nil {
		return Listing{}, err
	}
	slices.SortStableFunc(listing.Entries, func(a, b Entry) int {
		return cmp.Or(b.Updated.Compare(a.Updated), cmp.Compare(a.Builtin, b.Builtin), cmp.Compare(a.ID, b.ID))
	})
	return listing, nil
}

// match assigns entries their recorded harness and returns the records with newly matched
// sessions added and stale ones pruned, or nil when nothing changed.
func match(entries []Entry, records []Record, failed map[string]error, now time.Time) []Record {
	changed := false
	owner := map[[2]string]int{}
	for i, r := range records {
		for _, id := range r.Sessions {
			key := [2]string{r.Builtin, id}
			if j, ok := owner[key]; !ok || records[j].Started.Before(r.Started) {
				owner[key] = i
			}
		}
	}
	for i := range entries {
		e := &entries[i]
		key := [2]string{e.Builtin, e.ID}
		j, ok := owner[key]
		if !ok {
			j = -1
			for k, r := range records {
				from, to := r.window()
				if r.Builtin != e.Builtin || r.Directory != e.Directory || e.Started.Before(from) ||
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
	present := map[[2]string]bool{}
	for _, e := range entries {
		present[[2]string{e.Builtin, e.ID}] = true
	}
	kept := records[:0]
	for _, r := range records {
		if failed[r.Builtin] != nil {
			kept = append(kept, r)
			continue
		}
		ids := r.Sessions[:0]
		for _, id := range r.Sessions {
			if present[[2]string{r.Builtin, id}] {
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
