package daemon

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/storage"
	"github.com/tarik02-org/agx/internal/storage/statedb"
)

// revokedRetention is how long a revoked launch stays refused for attaching. A wrapper attaches
// within seconds of a daemon coming back, so this outlasts any daemon downtime that matters.
const revokedRetention = 7 * 24 * time.Hour

// savedState is what a daemon keeps for the next one. Launches are not in it: their wrappers
// attach them again (AttachLaunch).
type savedState struct {
	// Port is the listener port, which the next daemon binds again so that the endpoint URLs
	// harnesses hold stay valid.
	Port int `json:"port"`
	// Revoked maps launches revoked within revokedRetention to when, so that their wrappers
	// cannot attach them again.
	Revoked map[string]time.Time `json:"revoked,omitempty"`
}

func loadState(paths platform.Paths, now time.Time) (savedState, error) {
	state := savedState{Revoked: map[string]time.Time{}}
	db, err := storage.Open(context.Background(), storage.State(paths.State))
	if err != nil {
		return state, err
	}
	defer func() { _ = db.Close() }()
	q := statedb.New(db)
	port, err := q.GetDaemonPort(context.Background(), paths.Config)
	state.Port = int(port)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return state, err
	}
	rows, err := q.ListRevocations(context.Background(), statedb.ListRevocationsParams{
		Configuration: paths.Config, RevokedAt: now.Add(-revokedRetention).UnixNano(),
	})
	if err != nil {
		return state, err
	}
	for _, row := range rows {
		state.Revoked[row.Launch] = time.Unix(0, row.RevokedAt)
	}
	return state, nil
}

// saveState keeps the port and revocations together; s.mu must be held.
func (s *Server) saveState() {
	if err := s.writeState(); err != nil {
		fmt.Fprintf(os.Stderr, "agx daemon: cannot save daemon state: %v\n", err)
	}
}

func (s *Server) writeState() error {
	db, err := storage.Open(context.Background(), storage.State(s.paths.State))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	q := statedb.New(tx)
	ctx := context.Background()
	if err := q.PutDaemonPort(
		ctx,
		statedb.PutDaemonPortParams{Configuration: s.paths.Config, Port: int64(s.port)},
	); err != nil {
		return err
	}
	for id, at := range s.revoked {
		if time.Since(at) < revokedRetention {
			if err := q.PutRevocation(
				ctx,
				statedb.PutRevocationParams{Configuration: s.paths.Config, Launch: id, RevokedAt: at.UnixNano()},
			); err != nil {
				return err
			}
		}
	}
	if err := q.PruneRevocations(
		ctx,
		statedb.PruneRevocationsParams{
			Configuration: s.paths.Config,
			RevokedAt:     time.Now().Add(-revokedRetention).UnixNano(),
		},
	); err != nil {
		return err
	}
	return tx.Commit()
}

// shortID abbreviates a launch ID for the log, as git does commits.
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
