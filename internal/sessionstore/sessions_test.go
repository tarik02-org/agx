package sessionstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/sessions"
	"github.com/tarik02-org/agx/internal/storage"
	"github.com/tarik02-org/agx/internal/storage/statedb"
)

var start = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func at(minutes int) time.Time { return start.Add(time.Duration(minutes) * time.Minute) }

func entry(builtin, id, dir string, started time.Time) sessions.Entry {
	return sessions.Entry{
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
	entries := []sessions.Entry{
		entry("codex", "a", "/work", at(5)),
		entry("codex", "b", "/work", at(15)),
		entry("codex", "c", "/other", at(15)),
		entry("codex", "d", "/work", at(60)),
		entry("claude", "e", "/work", at(2)),
		entry("claude", "g", "/work", at(2)),
	}
	saved := seedRecords(t, records)
	canonicalEntries(t, entries)
	if err := saved.Match(t.Context(), entries, nil, at(61), map[string]string{"codex": "", "claude": ""}); err != nil {
		t.Fatal(err)
	}
	got, err := saved.load()
	if err != nil {
		t.Fatal(err)
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
	if err := saved.Match(t.Context(), again, nil, at(62), map[string]string{"codex": "", "claude": ""}); err != nil {
		t.Fatal(err)
	}
	for i := range entries {
		if again[i].Harness != entries[i].Harness || again[i].Recorded != entries[i].Recorded {
			t.Fatalf("second match changed ownership: %+v", again)
		}
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
		{Launch: "other-store", Builtin: "codex", Store: "/another-store", Started: at(0), Sessions: []string{"z"}},
	}
	failed := map[string]error{"pi": fs.ErrPermission}
	saved := seedRecords(t, records)
	if err := saved.Match(
		t.Context(),
		nil,
		failed,
		at(120),
		map[string]string{"codex": "", "pi": "", "claude": ""},
	); err != nil {
		t.Fatal(err)
	}
	got, err := saved.load()
	if err != nil {
		t.Fatal(err)
	}
	launches := []string{}
	for _, r := range got {
		launches = append(launches, r.Launch)
	}
	// A record whose sessions are all gone is dropped; one whose store failed is kept as is; a
	// launch still inside its window may yet start a session.
	if !slices.Equal(launches, []string{"kept", "other-store", "running"}) {
		t.Fatalf("kept records %v", launches)
	}
	if !slices.Equal(got[0].Sessions, []string{"y"}) {
		t.Errorf("sessions of a failed store's record changed: %v", got[0].Sessions)
	}
}

func TestSearch(t *testing.T) {
	entries := []sessions.Entry{
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
		for _, e := range sessions.Search(entries, tt.query) {
			got = append(got, e.ID)
		}
		slices.Sort(got)
		if !slices.Equal(got, tt.want) {
			t.Errorf("sessions.Search(%q) = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestWithin(t *testing.T) {
	root := filepath.Join(t.TempDir(), "work")
	entries := []sessions.Entry{
		{Session: adapters.Session{ID: "here", Directory: root}},
		{Session: adapters.Session{ID: "below", Directory: filepath.Join(root, "sub")}},
		{Session: adapters.Session{ID: "sibling", Directory: root + "-other"}},
	}
	got := []string{}
	for _, e := range sessions.Within(entries, root) {
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
	loaded, err := records.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Finished == nil || !loaded[0].Finished.Equal(at(5)) {
		t.Fatalf("records = %+v", loaded)
	}
	if err := records.Match(context.Background(), nil, nil, at(0), nil); err != nil {
		t.Fatal(err)
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
	// Concurrent listings lose no launch updates.
	wg.Go(func() {
		for range 8 {
			if err := records.Match(context.Background(), nil, nil, at(0), nil); err != nil {
				t.Error(err)
			}
		}
	})
	wg.Wait()
	loaded, err := records.load()
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
	path := filepath.Join(dir, "cache", "cache.db")
	open := func() *Index {
		t.Helper()
		index, err := OpenIndex(t.Context(), path, "claude")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = index.Close() })
		return index
	}
	index := open()
	for range 2 {
		if s, ok := index.Parse(file, parse); !ok || s.ID != "s" {
			t.Fatalf("Parse = %+v, %v", s, ok)
		}
		if err := index.Save(t.Context(), true); err != nil {
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
		if _, ok := open().Parse(file, parse); !ok {
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
	index = open()
	index.Parse(file, parse)
	if parses != 2 || index.Stats().Parsed != 1 {
		t.Fatalf("a touched file was not parsed again (%d parses, %+v)", parses, index.Stats())
	}
	if err := index.Save(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	if err := open().Save(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	if entries := open().entries; len(entries) != 0 {
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

func (s store) Sessions(context.Context, adapters.SessionFiles, string) ([]adapters.Session, error) {
	return s.sessions, nil
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	work := filepath.Join(dir, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	canonicalWork, err := platform.Canonical(work)
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
		Launch: "l", Harness: "codex-cpa", Builtin: "codex", Store: dir, Directory: canonicalWork,
		Started: now.Add(-2 * time.Hour), Finished: finished(now.Add(-30 * time.Minute)),
	}); err != nil {
		t.Fatal(err)
	}
	listing, err := NewCatalog(
		t.Context(),
		registry,
		filepath.Join(dir, "cache"),
		state,
	).Refresh(t.Context(), map[string]string{"codex": dir})
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
	records, err := NewRecords(state).load()
	if err != nil || len(records) != 1 || !slices.Equal(records[0].Sessions, []string{"a"}) {
		t.Fatalf("records = %+v, %v", records, err)
	}
}

func TestIndexDoesNotResurrectConcurrentlyPrunedFiles(t *testing.T) {
	dir := t.TempDir()
	file, database := filepath.Join(dir, "session"), filepath.Join(dir, "cache.db")
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	open := func() *Index {
		t.Helper()
		x, err := OpenIndex(t.Context(), database, "store")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = x.Close() })
		return x
	}
	parse := func(f *os.File, _ fs.FileInfo) (adapters.Session, bool, error) {
		data, err := io.ReadAll(f)
		return adapters.Session{ID: string(data)}, true, err
	}
	write("original")
	seed := open()
	seed.Parse(file, parse)
	if err := seed.Save(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	stale := open()
	write("modified")
	modified := time.Now().Add(time.Hour)
	if err := os.Chtimes(file, modified, modified); err != nil {
		t.Fatal(err)
	}
	stale.Parse(file, parse)
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := open().Save(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	if err := stale.Save(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	write("restored")
	if err := os.Chtimes(file, modified, modified); err != nil {
		t.Fatal(err)
	}
	if got, ok := open().Parse(file, parse); !ok || got.ID != "restored" {
		t.Fatalf("recreated session = %+v, %v; a stale scan resurrected its old metadata", got, ok)
	}
}

func TestCancelledRecordUpdateKeepsAssociations(t *testing.T) {
	records := NewRecords(t.TempDir())
	if err := records.Start(Record{Launch: "kept", Started: at(0)}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cancel()
	err := records.Match(ctx, nil, nil, at(10000), map[string]string{"codex": ""})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled update = %v", err)
	}
	loaded, err := records.load()
	if err != nil || len(loaded) != 1 || loaded[0].Launch != "kept" {
		t.Fatalf("records after cancellation = %+v, %v", loaded, err)
	}
}

func seedRecords(t *testing.T, records []Record) Records {
	t.Helper()
	saved := NewRecords(t.TempDir())
	for _, record := range records {
		if err := saved.Start(record); err != nil {
			t.Fatal(err)
		}
	}
	return saved
}

func canonicalEntries(t *testing.T, entries []sessions.Entry) {
	t.Helper()
	for i := range entries {
		dir, err := platform.Canonical(entries[i].Directory)
		if err != nil {
			t.Fatal(err)
		}
		entries[i].Directory = dir
	}
}

func (r Records) load() ([]Record, error) {
	ctx := context.Background()
	db, err := storage.Open(ctx, r.path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	q := statedb.New(db)
	rows, err := q.ListLaunches(ctx)
	if err != nil {
		return nil, err
	}
	associations, err := q.ListLaunchSessions(ctx)
	if err != nil {
		return nil, err
	}
	sessions := map[string][]string{}
	for _, a := range associations {
		sessions[a.Launch] = append(sessions[a.Launch], a.Session)
	}
	records := []Record{}
	for _, row := range rows {
		r := Record{
			Launch:    row.ID,
			Harness:   row.Harness,
			Builtin:   row.Builtin,
			Store:     row.Store,
			Directory: row.Directory,
			Started:   time.Unix(0, row.Started),
			Exact:     row.Exact,
			Sessions:  sessions[row.ID],
		}
		if row.Finished.Valid {
			end := time.Unix(0, row.Finished.Int64)
			r.Finished = &end
		}
		records = append(records, r)
	}
	return records, nil
}

func TestResumedSessionUsesLatestLaunchInItsStore(t *testing.T) {
	records := NewRecords(t.TempDir())
	for _, record := range []Record{
		{Launch: "old", Harness: "codex-old", Builtin: "codex", Started: at(0), Exact: true, Sessions: []string{"same"}},
		{Launch: "new", Harness: "codex-new", Builtin: "codex", Started: at(1), Exact: true, Sessions: []string{"same"}},
		{Launch: "other-builtin", Harness: "claude-work", Builtin: "claude", Started: at(2), Exact: true, Sessions: []string{"same"}},
		{Launch: "old", Harness: "codex-old", Builtin: "codex", Started: at(0), Exact: true, Sessions: []string{"same"}},
	} {
		if err := records.Start(record); err != nil {
			t.Fatal(err)
		}
	}
	entries := []sessions.Entry{entry("codex", "same", "/work", at(0)), entry("claude", "same", "/work", at(0))}
	if err := records.Match(
		t.Context(),
		entries,
		nil,
		at(2000),
		map[string]string{"codex": "", "claude": ""},
	); err != nil {
		t.Fatal(err)
	}
	if entries[0].Harness != "codex-new" || entries[1].Harness != "claude-work" {
		t.Fatalf("ownership = %+v", entries)
	}
	loaded, err := records.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("obsolete owner was retained: %+v", loaded)
	}
}

func TestIndexPersistsRepeatedChanges(t *testing.T) {
	dir := t.TempDir()
	file, path := filepath.Join(dir, "session"), filepath.Join(dir, "cache.db")
	index, err := OpenIndex(t.Context(), path, "store")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = index.Close() }()
	parse := func(f *os.File, _ fs.FileInfo) (adapters.Session, bool, error) {
		data, err := io.ReadAll(f)
		return adapters.Session{ID: string(data)}, true, err
	}
	for _, value := range []string{"first", "second", "third"} {
		if err := os.WriteFile(file, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		index.Parse(file, parse)
		if err := index.Save(t.Context(), false); err != nil {
			t.Fatal(err)
		}
	}
	reread, err := OpenIndex(t.Context(), path, "store")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reread.Close() }()
	if got, ok := reread.Parse(file, parse); !ok || got.ID != "third" || reread.Stats().Parsed != 0 {
		t.Fatalf("latest cached session = %+v, %v, %+v", got, ok, reread.Stats())
	}
}
