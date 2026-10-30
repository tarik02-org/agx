package sessionstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/adapters/claude"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/sessions"
)

func writeTranscript(t *testing.T, root, folder, id, message string) string {
	t.Helper()
	dir := filepath.Join(root, "projects", folder)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{
		"type": "user", "cwd": root, "timestamp": "2026-10-29T10:00:00Z",
		"message": map[string]string{"role": "user", "content": message},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCatalogTracksChangesAndSeparatesStores(t *testing.T) {
	dir, err := platform.Canonical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, second := filepath.Join(dir, "first"), filepath.Join(dir, "second")
	cache, state := filepath.Join(dir, "cache"), filepath.Join(dir, "state")
	registry := adapters.NewRegistry(claude.Adapter{})
	catalog := NewCatalog(t.Context(), registry, cache, state)
	refresh := func(c *Catalog, root string, parsed, cached, count int) sessions.Listing {
		t.Helper()
		listing, err := c.Refresh(t.Context(), map[string]string{"claude": root})
		if err != nil || len(listing.Errors) != 0 {
			t.Fatalf("Refresh: %v, %v", err, listing.Errors)
		}
		if listing.Stats.Parsed != parsed || listing.Stats.Cached != cached || len(listing.Entries) != count {
			t.Fatalf("listing = %+v; want %d parsed, %d cached, %d sessions", listing, parsed, cached, count)
		}
		return listing
	}
	writeTranscript(t, first, "work", "a", "alpha")
	if err := NewRecords(state).Start(Record{
		Launch: "launch", Harness: "custom", Builtin: "claude", Store: first,
		Directory: first, Started: time.Now(), Sessions: []string{"a"}, Exact: true,
	}); err != nil {
		t.Fatal(err)
	}
	listing := refresh(catalog, first, 1, 0, 1)
	if listing.Entries[0].Harness != "custom" {
		t.Fatalf("recorded harness = %q", listing.Entries[0].Harness)
	}
	listing.Entries[0].Harness = "mutated"
	snapshot, ok := catalog.Cached(map[string]string{"claude": first})
	if !ok || snapshot.Entries[0].Harness != "custom" {
		t.Fatal("a caller changed the cached snapshot")
	}
	refresh(catalog, first, 0, 1, 1)
	added := writeTranscript(t, first, "work", "b", "bravo")
	refresh(catalog, first, 1, 1, 2)
	writeTranscript(t, first, "work", "a", "alpha changed")
	listing = refresh(catalog, first, 1, 1, 2)
	if !slices.ContainsFunc(
		listing.Entries,
		func(e sessions.Entry) bool { return e.ID == "a" && e.Message == "alpha changed" },
	) {
		t.Fatal("the changed message is missing")
	}
	if err := os.Remove(added); err != nil {
		t.Fatal(err)
	}
	refresh(catalog, first, 0, 1, 1)
	writeTranscript(t, second, "work", "a", "a different store")
	if got := refresh(catalog, second, 1, 0, 1).Entries[0].Harness; got != "claude" {
		t.Fatalf("another store inherited harness %q", got)
	}
	if got := refresh(catalog, first, 0, 1, 1).Entries[0].Harness; got != "custom" {
		t.Fatalf("listing another store removed the recorded harness: %q", got)
	}
	restarted := NewCatalog(t.Context(), registry, cache, state)
	refresh(restarted, first, 0, 1, 1)
	refresh(restarted, second, 0, 1, 1)
	writeTranscript(t, first, "work", "b", "indexed by another daemon")
	refresh(restarted, first, 1, 1, 2)
	refresh(catalog, first, 0, 2, 2)
}

type waitingStore struct {
	claude.Adapter
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	calls   atomic.Int32
}

func (s *waitingStore) Sessions(ctx context.Context, _ adapters.SessionFiles, root string) ([]adapters.Session, error) {
	s.calls.Add(1)
	s.once.Do(func() { close(s.entered) })
	select {
	case <-s.release:
		return []adapters.Session{{ID: "held", Directory: root}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestCatalogRefreshOutlivesCancelledWaiters(t *testing.T) {
	adapter := &waitingStore{entered: make(chan struct{}), release: make(chan struct{})}
	dir := t.TempDir()
	catalog := NewCatalog(
		t.Context(),
		adapters.NewRegistry(adapter),
		filepath.Join(dir, "cache"),
		filepath.Join(dir, "state"),
	)
	roots := map[string]string{"claude": dir}
	waiter, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := catalog.Refresh(waiter, roots); result <- err }()
	<-adapter.entered
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waiter returned %v", err)
	}
	second, end := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer end()
	if _, err := catalog.Refresh(second, roots); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second waiter returned %v", err)
	}
	if _, ok := catalog.Cached(roots); ok {
		t.Fatal("published an unfinished scan")
	}
	close(adapter.release)
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if listing, ok := catalog.Cached(roots); ok {
			if len(listing.Entries) != 1 || listing.Entries[0].ID != "held" || adapter.calls.Load() != 1 {
				t.Fatalf("shared refresh = %+v, scans = %d", listing, adapter.calls.Load())
			}
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("refresh stopped when its waiters cancelled")
		case <-ticker.C:
		}
	}
}

func TestCatalogKeepsUnreadableSessionsUntilACompleteScan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions")
	}
	for _, kind := range []string{"directory", "file"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			root := filepath.Join(dir, "store")
			writeTranscript(t, root, "healthy", "a", "alpha")
			hidden := writeTranscript(t, root, "locked", "b", "bravo")
			locked := filepath.Dir(hidden)
			catalog := NewCatalog(
				t.Context(),
				adapters.NewRegistry(claude.Adapter{}),
				filepath.Join(dir, "cache"),
				filepath.Join(dir, "state"),
			)
			roots := map[string]string{"claude": root}
			if listing, err := catalog.Refresh(t.Context(), roots); err != nil || len(listing.Entries) != 2 {
				t.Fatalf("initial listing = %+v, %v", listing, err)
			}
			if kind == "file" {
				writeTranscript(t, root, "locked", "b", "bravo changed")
				locked = hidden
			}
			if err := os.Chmod(locked, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
			if kind == "directory" {
				if _, err := os.ReadDir(locked); err == nil {
					t.Skip("directory permissions do not prevent reads")
				}
			} else if _, err := os.ReadFile(locked); err == nil {
				t.Skip("file permissions do not prevent reads")
			}
			writeTranscript(t, root, "healthy", "a", "alpha changed")
			listing, err := catalog.Refresh(t.Context(), roots)
			if err != nil || len(listing.Errors) != 1 || len(listing.Entries) != 2 {
				t.Fatalf("partial listing = %+v, %v", listing, err)
			}
			if !slices.ContainsFunc(
				listing.Entries,
				func(e sessions.Entry) bool { return e.ID == "a" && e.Message == "alpha changed" },
			) {
				t.Fatal("readable sessions were not refreshed")
			}
			if err := os.Chmod(locked, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(hidden); err != nil {
				t.Fatal(err)
			}
			listing, err = catalog.Refresh(t.Context(), roots)
			if err != nil || len(listing.Errors) != 0 || len(listing.Entries) != 1 || listing.Entries[0].ID != "a" {
				t.Fatalf("complete listing = %+v, %v", listing, err)
			}
		})
	}
}
