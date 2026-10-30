package sessions

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/storage"
)

// Catalog retains completed listings and parsed files for caller-selected stores.
// Refreshes share work and outlive individual callers, bounded by the owner's context.
type Catalog struct {
	ctx      context.Context
	registry adapters.Registry
	cache    string
	state    string
	mu       sync.Mutex
	listings map[string]*cachedListing
	workers  chan struct{}
}

type cachedListing struct {
	listing Listing
	ready   bool
	used    time.Time
	done    chan struct{}
	err     error
}

func NewCatalog(ctx context.Context, registry adapters.Registry, cache, state string) *Catalog {
	return &Catalog{
		ctx: ctx, registry: registry, cache: cache, state: state,
		listings: map[string]*cachedListing{}, workers: make(chan struct{}, 2),
	}
}

// Stores resolves paths in the calling shell, before work moves to the daemon.
func Stores(
	registry adapters.Registry,
	directory string,
	lookup func(string) string,
) (map[string]string, map[string]error) {
	roots := map[string]string{}
	errors := map[string]error{}
	for _, name := range registry.Names() {
		adapter, _ := registry.Adapter(name)
		root, err := adapter.SessionRoot(directory, lookup)
		if err == nil {
			root, err = platform.Canonical(root)
		}
		if err != nil {
			errors[name] = err
		} else {
			roots[name] = root
		}
	}
	return roots, errors
}

func cloneListing(listing Listing) Listing {
	listing.Entries = slices.Clone(listing.Entries)
	listing.Errors = maps.Clone(listing.Errors)
	return listing
}

// Cached returns a completed snapshot without touching the filesystem.
func (c *Catalog) Cached(stores map[string]string) (Listing, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.listings[config.Digest(stores)]
	if entry == nil || !entry.ready {
		return Listing{}, false
	}
	entry.used = time.Now()
	return cloneListing(entry.listing), true
}

// Refresh starts or joins a scan. Cancelling a waiter leaves the shared refresh running.
func (c *Catalog) Refresh(ctx context.Context, stores map[string]string) (Listing, error) {
	key := config.Digest(stores)
	c.mu.Lock()
	entry := c.listings[key]
	if entry == nil {
		if len(c.listings) >= 8 {
			oldest := ""
			for k, candidate := range c.listings {
				if candidate.done == nil && (oldest == "" || candidate.used.Before(c.listings[oldest].used)) {
					oldest = k
				}
			}
			delete(c.listings, oldest)
		}
		entry = &cachedListing{}
		c.listings[key] = entry
	}
	entry.used = time.Now()
	if entry.done == nil {
		entry.done = make(chan struct{})
		stores = maps.Clone(stores)
		go c.refresh(entry, stores)
	}
	done := entry.done
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return Listing{}, ctx.Err()
	case <-done:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneListing(entry.listing), entry.err
}

func (c *Catalog) refresh(entry *cachedListing, stores map[string]string) {
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	var listing Listing
	var err error
	select {
	case c.workers <- struct{}{}:
		listing, err = c.scan(ctx, stores, entry.listing)
		<-c.workers
	case <-ctx.Done():
		err = ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		entry.listing, entry.ready = listing, true
	}
	entry.err = err
	done := entry.done
	entry.done = nil
	close(done)
}

func (c *Catalog) scan(
	ctx context.Context,
	stores map[string]string,
	previous Listing,
) (Listing, error) {
	type storeResult struct {
		name     string
		sessions []adapters.Session
		stats    Stats
		err      error
	}
	names := slices.Sorted(maps.Keys(stores))
	results := make([]storeResult, len(names))
	var group errgroup.Group
	group.SetLimit(3)
	for i, name := range names {
		root := stores[name]
		group.Go(func() error {
			result := storeResult{name: name}
			index, err := OpenIndex(ctx, storage.Cache(c.cache), config.Digest([]string{name, root}))
			if err != nil {
				result.err = err
			} else {
				defer func() { _ = index.Close() }()
				adapter, _ := c.registry.Adapter(name)
				result.sessions, result.err = adapter.Sessions(ctx, index, root)
				result.err = errors.Join(result.err, index.Err())
				result.stats = index.Stats()
				if ctx.Err() == nil {
					if err := index.Save(ctx, result.err == nil); err != nil {
						result.err = err
					}
				}
			}
			results[i] = result
			return nil
		})
	}
	_ = group.Wait()
	if err := ctx.Err(); err != nil {
		return Listing{}, err
	}
	listing := Listing{Entries: []Entry{}, Errors: map[string]error{}}
	byKey := map[[2]string]int{}
	canonical := map[string]string{}
	for _, result := range results {
		listing.Stats.Parsed += result.stats.Parsed
		listing.Stats.Cached += result.stats.Cached
		listing.Stats.Malformed += result.stats.Malformed
		if result.err != nil {
			listing.Errors[result.name] = result.err
		}
		for _, session := range result.sessions {
			directory, ok := canonical[session.Directory]
			if !ok {
				directory = filepath.Clean(session.Directory)
				if real, err := platform.Canonical(session.Directory); err == nil {
					directory = real
				}
				canonical[session.Directory] = directory
			}
			session.Directory = directory
			key := [2]string{result.name, session.ID}
			if i, ok := byKey[key]; ok {
				if session.Updated.After(listing.Entries[i].Updated) {
					listing.Entries[i].Session = session
				}
				continue
			}
			byKey[key] = len(listing.Entries)
			listing.Entries = append(listing.Entries, Entry{
				Session: session, Builtin: result.name, Harness: result.name, Store: stores[result.name],
			})
		}
		if result.err != nil {
			for _, entry := range previous.Entries {
				key := [2]string{entry.Builtin, entry.ID}
				if _, found := byKey[key]; entry.Builtin == result.name && !found {
					byKey[key] = len(listing.Entries)
					listing.Entries = append(listing.Entries, entry)
				}
			}
		}
	}
	if err := NewRecords(c.state).Match(ctx, listing.Entries, listing.Errors, time.Now(), stores); err != nil {
		return Listing{}, err
	}
	slices.SortStableFunc(listing.Entries, func(a, b Entry) int {
		return cmp.Or(b.Updated.Compare(a.Updated), cmp.Compare(a.Builtin, b.Builtin), cmp.Compare(a.ID, b.ID))
	})
	return listing, nil
}
