package sessions

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/storage"
	"github.com/tarik02-org/agx/internal/storage/statedb"
)

// Record remembers one managed launch, so its sessions resume with the same harness definition.
type Record struct {
	Launch    string     `json:"launch"`
	Harness   string     `json:"harness"`
	Builtin   string     `json:"builtin"`
	Store     string     `json:"store"`
	Directory string     `json:"directory"`
	Started   time.Time  `json:"started"`
	Finished  *time.Time `json:"finished,omitempty"`
	// Sessions holds the sessions known to belong to the launch: the one it resumed, and those
	// matched to it since.
	Sessions []string `json:"sessions,omitempty"`
	// Exact records list every session of the launch, which named them itself, so no session
	// matches them by time.
	Exact bool `json:"exact,omitempty"`
}

// Harnesses that cannot name sessions claim conversations started within their launch window.
const (
	skew      = 5 * time.Second
	maxLaunch = 24 * time.Hour
)

type Records struct{ path string }

func NewRecords(state string) Records { return Records{path: storage.State(state)} }

func (r Records) Start(record Record) error {
	for _, path := range []*string{&record.Store, &record.Directory} {
		if *path != "" {
			canonical, err := platform.Canonical(*path)
			if err != nil {
				return err
			}
			*path = canonical
		}
	}
	return r.transaction(context.Background(), func(q *statedb.Queries) error {
		var end sql.NullInt64
		if record.Finished != nil {
			end = sql.NullInt64{Int64: record.Finished.UnixNano(), Valid: true}
		}
		if err := q.StartLaunch(context.Background(), statedb.StartLaunchParams{
			ID: record.Launch, Harness: record.Harness, Builtin: record.Builtin, Store: record.Store,
			Directory: record.Directory, Started: record.Started.UnixNano(), Finished: end, Exact: record.Exact,
		}); err != nil {
			return err
		}
		for _, id := range record.Sessions {
			if err := q.AddLaunchSession(
				context.Background(),
				statedb.AddLaunchSessionParams{Launch: record.Launch, Session: id},
			); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r Records) Finish(launch string, at time.Time) error {
	return r.transaction(context.Background(), func(q *statedb.Queries) error {
		return q.FinishLaunch(
			context.Background(),
			statedb.FinishLaunchParams{ID: launch, Finished: sql.NullInt64{Int64: at.UnixNano(), Valid: true}},
		)
	})
}

func (r Records) transaction(ctx context.Context, change func(*statedb.Queries) error) error {
	db, err := storage.Open(ctx, r.path)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := change(statedb.New(tx)); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

// Match assigns sessions through their stored associations, or the latest eligible launch.
// Incomplete scans retain unseen associations; only changed associations are written.
func (r Records) Match(
	ctx context.Context,
	entries []Entry,
	failed map[string]error,
	now time.Time,
	stores map[string]string,
) error {
	canonical := make(map[string]string, len(stores))
	for builtin, root := range stores {
		if root != "" {
			var err error
			root, err = platform.Canonical(root)
			if err != nil {
				return err
			}
		}
		canonical[builtin] = root
	}
	stores = canonical
	for i := range entries {
		if entries[i].Store != "" {
			root, err := platform.Canonical(entries[i].Store)
			if err != nil {
				return err
			}
			entries[i].Store = root
		}
	}

	return r.transaction(ctx, func(q *statedb.Queries) error {
		owners := map[[3]string]statedb.ListStoreLaunchSessionsRow{}
		known := map[string][]statedb.ListStoreLaunchSessionsRow{}
		for builtin, root := range stores {
			rows, err := q.ListStoreLaunchSessions(
				ctx,
				statedb.ListStoreLaunchSessionsParams{Builtin: builtin, Store: root},
			)
			if err != nil {
				return err
			}
			known[builtin] = rows
			for _, row := range rows {
				key := [3]string{builtin, root, row.Session}
				if _, found := owners[key]; !found {
					owners[key] = row
				}
			}
		}
		present := map[[3]string]bool{}
		for i := range entries {
			e := &entries[i]
			key := [3]string{e.Builtin, e.Store, e.ID}
			present[key] = true
			owner, found := owners[key]
			if !found {
				match, err := q.MatchSession(ctx, statedb.MatchSessionParams{
					Builtin: e.Builtin, Store: e.Store, Directory: e.Directory, SessionStarted: e.Started.UnixNano(),
					Skew: int64(skew), MaxLaunch: int64(maxLaunch),
				})
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				if err != nil {
					return err
				}
				owner = statedb.ListStoreLaunchSessionsRow{Launch: match.ID, Harness: match.Harness, Session: e.ID}
				if err := q.AddLaunchSession(
					ctx,
					statedb.AddLaunchSessionParams{Launch: owner.Launch, Session: e.ID},
				); err != nil {
					return err
				}
				owners[key] = owner
			}
			e.Harness, e.Recorded = owner.Harness, true
		}
		for builtin, root := range stores {
			if failed[builtin] != nil {
				continue
			}
			for _, row := range known[builtin] {
				if !present[[3]string{builtin, root, row.Session}] {
					if err := q.DeleteLaunchSession(
						ctx,
						statedb.DeleteLaunchSessionParams{Launch: row.Launch, Session: row.Session},
					); err != nil {
						return err
					}
				}
			}
			if err := q.PruneLaunches(
				ctx,
				statedb.PruneLaunchesParams{
					Builtin:   builtin,
					Store:     root,
					MaxLaunch: int64(maxLaunch),
					Now:       now.UnixNano(),
					Skew:      int64(skew),
				},
			); err != nil {
				return err
			}
		}
		return nil
	})
}
