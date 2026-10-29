package sessions

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
)

var start = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func at(minutes int) time.Time { return start.Add(time.Duration(minutes) * time.Minute) }

func entry(builtin, id, dir string, started time.Time) Entry {
	return Entry{
		Session: adapters.Session{ID: id, Directory: dir, Started: started, Updated: started},
		Builtin: builtin,
		Harness: builtin,
	}
}

func finished(t time.Time) *time.Time { return &t }

func TestMatchAssignsSessionsToLaunches(t *testing.T) {
	records := []Record{
		{
			Launch:    "early",
			Harness:   "codex-cpa",
			Builtin:   "codex",
			Directory: "/work",
			Started:   at(0),
			Finished:  finished(at(30)),
		},
		// A concurrent launch in the same directory started later: it owns sessions started after it.
		{
			Launch:    "late",
			Harness:   "codex-x",
			Builtin:   "codex",
			Directory: "/work",
			Started:   at(10),
			Finished:  finished(at(30)),
		},
		{Launch: "claude", Harness: "claude-x", Builtin: "claude", Directory: "/work", Started: at(0)},
		// A launch that named its session claims no other, though it started later.
		{
			Launch:    "named",
			Harness:   "claude-y",
			Builtin:   "claude",
			Directory: "/work",
			Started:   at(1),
			Sessions:  []string{"g"},
			Exact:     true,
		},
	}
	entries := []Entry{
		entry("codex", "a", "/work", at(5)),
		entry("codex", "b", "/work", at(15)),
		entry("codex", "c", "/other", at(15)),
		entry("codex", "d", "/work", at(60)),
		entry("claude", "e", "/work", at(2)),
		entry("claude", "g", "/work", at(2)),
	}
	got := match(entries, records, nil, at(61))
	if got == nil {
		t.Fatal("match reported no change")
	}
	harness := map[string]string{}
	for _, e := range entries {
		if e.Recorded {
			harness[e.ID] = e.Harness
		}
	}
	want := map[string]string{"a": "codex-cpa", "b": "codex-x", "e": "claude-x", "g": "claude-y"}
	if len(harness) != len(want) {
		t.Fatalf("recorded = %v, want %v", harness, want)
	}
	for id, h := range want {
		if harness[id] != h {
			t.Errorf("%s resumes with %q, want %q", id, harness[id], h)
		}
	}
	sessions := map[string][]string{}
	for _, r := range got {
		sessions[r.Launch] = r.Sessions
	}
	if !slices.Equal(sessions["early"], []string{"a"}) || !slices.Equal(sessions["late"], []string{"b"}) {
		t.Errorf("records = %+v", got)
	}

	// Matched sessions stay with their record, so a second listing changes nothing.
	again := slices.Clone(entries)
	for i := range again {
		again[i].Harness, again[i].Recorded = again[i].Builtin, false
	}
	if match(again, got, nil, at(62)) != nil {
		t.Error("an unchanged listing rewrote the records")
	}
}

func TestMatchPrunesRecords(t *testing.T) {
	records := []Record{
		{
			Launch:    "gone",
			Builtin:   "codex",
			Directory: "/work",
			Started:   at(0),
			Finished:  finished(at(1)),
			Sessions:  []string{"x"},
		},
		{
			Launch:    "kept",
			Builtin:   "pi",
			Directory: "/work",
			Started:   at(0),
			Finished:  finished(at(1)),
			Sessions:  []string{"y"},
		},
		{Launch: "running", Builtin: "claude", Directory: "/work", Started: at(0)},
	}
	failed := map[string]error{"pi": fs.ErrPermission}
	got := match(nil, records, failed, at(120))
	launches := []string{}
	for _, r := range got {
		launches = append(launches, r.Launch)
	}
	// A record whose sessions are all gone is dropped; one whose store failed is kept as is; a
	// launch still inside its window may yet start a session.
	if !slices.Equal(launches, []string{"kept", "running"}) {
		t.Fatalf("kept records %v", launches)
	}
	if !slices.Equal(got[0].Sessions, []string{"y"}) {
		t.Errorf("sessions of a failed store's record changed: %v", got[0].Sessions)
	}
}

func TestSearch(t *testing.T) {
	entries := []Entry{
		{
			Session: adapters.Session{ID: "1", Title: "Fix flaky login test", Directory: "/work/app"},
			Builtin: "codex",
			Harness: "codex",
		},
		{
			Session: adapters.Session{ID: "2", Message: "Write release notes", Directory: "/work/lib"},
			Builtin: "pi",
			Harness: "pi",
		},
		{
			Session: adapters.Session{ID: "3", Title: "login page", Directory: "/work/flaky"},
			Builtin: "claude",
			Harness: "claude-x",
		},
	}
	for _, tt := range []struct {
		query string
		want  []string
	}{
		{"", []string{"1", "2", "3"}},
		{"flaky login", []string{"1", "3"}},
		{"release", []string{"2"}},
		{"claude-x", []string{"3"}},
		{"zzzqqq", []string{}},
	} {
		got := []string{}
		for _, e := range Search(entries, tt.query) {
			got = append(got, e.ID)
		}
		slices.Sort(got)
		if !slices.Equal(got, tt.want) {
			t.Errorf("Search(%q) = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestWithin(t *testing.T) {
	root := filepath.Join(t.TempDir(), "work")
	entries := []Entry{
		{Session: adapters.Session{ID: "here", Directory: root}},
		{Session: adapters.Session{ID: "below", Directory: filepath.Join(root, "sub")}},
		{Session: adapters.Session{ID: "sibling", Directory: root + "-other"}},
	}
	got := []string{}
	for _, e := range Within(entries, root) {
		got = append(got, e.ID)
	}
	if !slices.Equal(got, []string{"here", "below"}) {
		t.Fatalf("Within = %v", got)
	}
}

func TestRecordsFile(t *testing.T) {
	state := t.TempDir()
	records := NewRecords(state)
	if err := records.Start(Record{Launch: "l", Harness: "codex", Builtin: "codex", Started: at(0)}); err != nil {
		t.Fatal(err)
	}
	if err := records.Finish("l", at(5)); err != nil {
		t.Fatal(err)
	}
	// A thread that never bound finishes without a record.
	if err := records.Finish("unbound", at(5)); err != nil {
		t.Fatal(err)
	}
	loaded, folded, err := records.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Finished == nil || !loaded[0].Finished.Equal(at(5)) || folded {
		t.Fatalf("records = %+v, folded %v", loaded, folded)
	}
	if err := records.update(func([]Record) []Record { return nil }); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(records.path)
	if err != nil {
		t.Fatal(err)
	}
	if lines := bytes.Count(b, []byte("\n")); lines != 1 {
		t.Fatalf("a listing left %d lines for one record:\n%s", lines, b)
	}
	if err := os.WriteFile(records.path, append([]byte("{damaged\n"), b...), 0o600); err != nil {
		t.Fatal(err)
	}
	if loaded, _, err := records.load(); err != nil || len(loaded) != 1 {
		t.Fatalf("a damaged line must be skipped: %v, %v", loaded, err)
	}
}

func TestRecordsKeepConcurrentLaunches(t *testing.T) {
	records := NewRecords(t.TempDir())
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			launch := fmt.Sprint(i)
			if err := records.Start(Record{Launch: launch, Builtin: "codex", Started: at(i)}); err != nil {
				t.Error(err)
			}
			if err := records.Finish(launch, at(i+1)); err != nil {
				t.Error(err)
			}
		})
	}
	// A listing folding the log meanwhile loses none of them.
	wg.Go(func() {
		for range 8 {
			if err := records.update(func([]Record) []Record { return nil }); err != nil {
				t.Error(err)
			}
		}
	})
	wg.Wait()
	loaded, _, err := records.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 32 {
		t.Fatalf("%d of 32 records kept", len(loaded))
	}
	for _, r := range loaded {
		if r.Finished == nil {
			t.Errorf("launch %s lost its finish", r.Launch)
		}
	}
}

func TestRecordsFoldTheLegacyFile(t *testing.T) {
	state := t.TempDir()
	legacy := filepath.Join(state, "launch-records.json")
	if err := os.WriteFile(legacy, []byte(`{"version":1,"records":[
		{"launch":"old","harness":"codex-x","builtin":"codex","directory":"/work","started":"2026-10-01T12:00:00Z"}
	]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	records := NewRecords(state)
	if err := records.Finish("old", at(5)); err != nil {
		t.Fatal(err)
	}
	if err := records.Start(Record{Launch: "new", Builtin: "codex", Started: at(10)}); err != nil {
		t.Fatal(err)
	}
	if err := records.update(func([]Record) []Record { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the legacy file was not removed: %v", err)
	}
	loaded, folded, err := records.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || loaded[0].Launch != "old" || loaded[0].Harness != "codex-x" ||
		loaded[0].Finished == nil || !loaded[0].Finished.Equal(at(5)) || loaded[1].Launch != "new" || !folded {
		t.Fatalf("records = %+v, folded %v", loaded, folded)
	}
}

func TestIndexReusesUnchangedFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(file, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	parses := 0
	parse := func(f *os.File, info fs.FileInfo) (adapters.Session, bool, error) {
		parses++
		return adapters.Session{ID: "s", Updated: info.ModTime()}, true, nil
	}
	path := filepath.Join(dir, "cache", "index.json")
	for range 2 {
		index := OpenIndex(path)
		if s, ok := index.Parse(file, parse); !ok || s.ID != "s" {
			t.Fatalf("Parse = %+v, %v", s, ok)
		}
		if err := index.Save(true); err != nil {
			t.Fatal(err)
		}
	}
	if parses != 1 {
		t.Fatalf("parsed %d times, want the second listing to reuse the cache", parses)
	}
	if runtime.GOOS != "windows" {
		// An unchanged file is not even opened.
		if err := os.Chmod(file, 0); err != nil {
			t.Fatal(err)
		}
		if _, ok := OpenIndex(path).Parse(file, parse); !ok {
			t.Fatal("an unchanged file was opened")
		}
		if err := os.Chmod(file, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(file, later, later); err != nil {
		t.Fatal(err)
	}
	index := OpenIndex(path)
	index.Parse(file, parse)
	if parses != 2 || index.Stats().Parsed != 1 {
		t.Fatalf("a touched file was not parsed again (%d parses, %+v)", parses, index.Stats())
	}
	if err := index.Save(true); err != nil {
		t.Fatal(err)
	}
	if err := OpenIndex(path).Save(true); err != nil {
		t.Fatal(err)
	}
	if entries := OpenIndex(path).entries; len(entries) != 0 {
		t.Fatalf("pruning kept files not seen by the listing: %v", entries)
	}
}

// store is an adapter whose sessions are fixed.
type store struct {
	adapters.Adapter
	name     string
	sessions []adapters.Session
}

func (s store) Name() string { return s.name }

func (s store) Sessions(context.Context, adapters.SessionFiles) ([]adapters.Session, error) {
	return s.sessions, nil
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	work := filepath.Join(dir, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	canonicalWork, err := config.Canonical(work)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	registry := adapters.NewRegistry(
		store{name: "codex", sessions: []adapters.Session{
			{ID: "a", Directory: work, Started: now.Add(-time.Hour), Updated: now.Add(-time.Hour)},
			// The same ID twice, as after a copy: the newer file wins.
			{ID: "a", Directory: work, Title: "newer", Started: now.Add(-time.Hour), Updated: now},
			{ID: "b", Directory: filepath.Join(dir, "deleted", "sub"), Started: now, Updated: now.Add(-time.Minute)},
		}},
	)
	state := filepath.Join(dir, "state")
	if err := NewRecords(state).Start(Record{
		Launch: "l", Harness: "codex-cpa", Builtin: "codex", Directory: canonicalWork,
		Started: now.Add(-2 * time.Hour), Finished: finished(now.Add(-30 * time.Minute)),
	}); err != nil {
		t.Fatal(err)
	}
	listing, err := List(context.Background(), registry, filepath.Join(dir, "cache"), state)
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Entries) != 2 {
		t.Fatalf("entries = %+v", listing.Entries)
	}
	newest, other := listing.Entries[0], listing.Entries[1]
	if newest.ID != "a" || newest.Title != "newer" || newest.Directory != canonicalWork ||
		newest.Harness != "codex-cpa" || !newest.Recorded {
		t.Errorf("newest = %+v", newest)
	}
	if other.ID != "b" || other.Harness != "codex" || other.Recorded {
		t.Errorf("other = %+v", other)
	}
	records, _, err := NewRecords(state).load()
	if err != nil || len(records) != 1 || !slices.Equal(records[0].Sessions, []string{"a"}) {
		t.Fatalf("records = %+v, %v", records, err)
	}
}
