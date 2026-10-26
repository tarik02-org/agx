package daemon

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/securefs"
)

const (
	stateVersion = 1
	// renewalWriteInterval bounds how often lease renewals alone rewrite the state file; a restored
	// lease may then end this much earlier than the wrapper last heard.
	renewalWriteInterval = 5 * time.Second
)

// StatePath is the file in which the daemon keeps its active launches across restarts.
func StatePath(paths platform.Paths) string { return filepath.Join(paths.State, "daemon-state.json") }

// savedState is what a daemon keeps for the next one: only identifiers, metadata, and token
// verifiers. Raw tokens, environment inputs, and native arguments never reach it; a restored
// launch gets those back from its wrapper (RestoreLaunch).
type savedState struct {
	Version int `json:"version"`
	// Port is the listener port, which a restarted daemon binds again so that the endpoint URLs
	// harnesses hold stay valid.
	Port int `json:"port"`
	// Config is the root configuration the launches were resolved from.
	Config   string        `json:"config"`
	Launches []savedLaunch `json:"launches"`
	// Revoked lists launches revoked within revokedRetention, whose wrappers must not re-attach
	// them; it maps a launch ID to when it was revoked.
	Revoked map[string]time.Time `json:"revoked,omitempty"`
}

// revokedRetention is how long a revoked launch stays refused for re-attaching. A wrapper renews
// within seconds of a daemon coming back, so this outlasts any daemon downtime that matters.
const revokedRetention = 7 * 24 * time.Hour

type savedLaunch struct {
	ID string `json:"id"`
	// Parent is set for a thread launch.
	Parent  string `json:"parent,omitempty"`
	Session string `json:"session,omitempty"`
	// Snapshot, Directory, and Harness rebuild the snapshot: resolving Directory for Harness
	// with the same configuration and packages yields Snapshot again.
	Snapshot  string `json:"snapshot"`
	Directory string `json:"directory"`
	Harness   string `json:"harness"`
	Mode      string `json:"mode"`
	State     string `json:"state"`
	// ThreadServer marks a harness process serving many threads, whose launch has no endpoints.
	ThreadServer bool      `json:"thread_server,omitempty"`
	LeaseSeconds int       `json:"lease_seconds"`
	Created      time.Time `json:"created"`
	Expires      time.Time `json:"expires"`
	// Grant and Lifecycle are SHA-256 verifiers of the launch's 256-bit random tokens.
	Grant     string `json:"grant_sha256"`
	Lifecycle string `json:"lifecycle_sha256"`
}

var savedStates = map[v1.Launch_State]string{
	v1.Launch_STATE_PREPARED: "prepared",
	v1.Launch_STATE_RUNNING:  "running",
}

// saveLaunch records an active launch; s.mu must be held.
func saveLaunch(l *launch) savedLaunch {
	return savedLaunch{
		ID:           l.view.Id,
		Parent:       l.view.ParentLaunchId,
		Session:      l.view.SessionId,
		Snapshot:     l.view.SnapshotId,
		Directory:    l.view.Directory,
		Harness:      l.view.Harness,
		Mode:         l.view.Mode,
		State:        savedStates[l.view.State],
		ThreadServer: l.threadServer,
		LeaseSeconds: int(l.lease / time.Second),
		Created:      l.created.UTC(),
		Expires:      l.view.ExpiresAt.AsTime().UTC(),
		Grant:        hex.EncodeToString(l.grant[:]),
		Lifecycle:    hex.EncodeToString(l.lifecycle[:]),
	}
}

// restoredLaunch is a saved launch as the next daemon holds it, before its wrapper resends what
// was not saved.
type restoredLaunch struct {
	saved     savedLaunch
	state     v1.Launch_State
	grant     [32]byte
	lifecycle [32]byte
}

func (r restoredLaunch) view() *v1.Launch {
	return &v1.Launch{
		Id:             r.saved.ID,
		SnapshotId:     r.saved.Snapshot,
		Directory:      r.saved.Directory,
		Harness:        r.saved.Harness,
		Mode:           r.saved.Mode,
		State:          r.state,
		ExpiresAt:      timestamppb.New(r.saved.Expires),
		ParentLaunchId: r.saved.Parent,
		SessionId:      r.saved.Session,
	}
}

// invalidState is the error of a state file that does not decode. Writes are atomic, so only
// an edit by hand or a damaged disk leaves one; the daemon moves it aside.
type invalidState struct {
	path string
	err  error
}

func (e invalidState) Error() string {
	return fmt.Sprintf("invalid daemon state %s: %v", e.path, e.err)
}

func (e invalidState) Unwrap() error { return e.err }

func decodeState(path string, b []byte) (savedState, error) {
	var saved savedState
	if err := json.Unmarshal(b, &saved); err != nil {
		return savedState{}, invalidState{path: path, err: err}
	}
	if saved.Version != stateVersion {
		return savedState{}, invalidState{path: path, err: fmt.Errorf("unknown format version %d", saved.Version)}
	}
	return saved, nil
}

// loadState reads the state file for configuration root config at now. It keeps the launches
// still active, parents before their thread launches, and drops expired ones, those of another
// configuration, and thread launches whose parent is gone. A missing file holds nothing; a file
// that does not decode is an invalidState error.
func loadState(path, config string, now time.Time) (int, []restoredLaunch, map[string]time.Time, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil, nil, nil
	}
	if err != nil {
		return 0, nil, nil, err
	}
	saved, err := decodeState(path, b)
	if err != nil {
		return 0, nil, nil, err
	}
	revoked := map[string]time.Time{}
	for id, at := range saved.Revoked {
		if now.Sub(at) < revokedRetention {
			revoked[id] = at
		}
	}
	if saved.Config != config {
		if len(saved.Launches) > 0 {
			slog.Warn(
				"not restoring launches saved for another configuration",
				"launches", len(saved.Launches),
				"saved", saved.Config,
				"config", config,
			)
		}
		return saved.Port, nil, revoked, nil
	}
	kept := map[string]bool{}
	restored := []restoredLaunch{}
	// Parents come first in the file, so one pass sees every parent before its threads.
	for _, l := range saved.Launches {
		state := v1.Launch_STATE_UNSPECIFIED
		for value, name := range savedStates {
			if name == l.State {
				state = value
			}
		}
		r := restoredLaunch{saved: l, state: state}
		grant, grantErr := hex.DecodeString(l.Grant)
		lifecycle, lifecycleErr := hex.DecodeString(l.Lifecycle)
		reason := ""
		switch {
		case !now.Before(l.Expires):
			reason = "its lease expired at " + l.Expires.Format(time.RFC3339)
		case l.Parent != "" && !kept[l.Parent]:
			reason = "its parent launch is not restored"
		case state == v1.Launch_STATE_UNSPECIFIED, l.ID == "", l.LeaseSeconds <= 0,
			grantErr != nil, lifecycleErr != nil, len(grant) != len(r.grant), len(lifecycle) != len(r.lifecycle):
			reason = "its saved entry is invalid"
		}
		if reason != "" {
			// Its wrapper can still re-attach it while the harness runs.
			slog.Warn("not restoring launch", "launch", shortID(l.ID), "harness", l.Harness, "reason", reason)
			continue
		}
		copy(r.grant[:], grant)
		copy(r.lifecycle[:], lifecycle)
		kept[l.ID] = true
		restored = append(restored, r)
	}
	return saved.Port, restored, revoked, nil
}

// shortID abbreviates a launch ID for the log, as git does commits.
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// stateStore writes the state file. Writes carry the generation of the state they hold, so a
// slower writer never replaces a newer state; renewals alone are throttled.
type stateStore struct {
	path     string
	mu       sync.Mutex
	written  uint64
	at       time.Time
	pending  bool
	frozen   bool
	interval time.Duration
}

// write stores state of generation at now and reports whether it reached the file. A renewal
// waits until interval passed since the last write; due then reports it.
func (s *stateStore) write(generation uint64, state savedState, renewal bool, now time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.frozen || generation <= s.written {
		return false, nil
	}
	if renewal && now.Sub(s.at) < s.interval {
		s.pending = true
		return false, nil
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return false, err
	}
	if err := securefs.Write(s.path, append(b, '\n'), 0o600); err != nil {
		return false, err
	}
	s.written, s.at, s.pending = generation, now, false
	return true, nil
}

// due reports whether a throttled renewal waits to be written at now.
func (s *stateStore) due(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending && !s.frozen && now.Sub(s.at) >= s.interval
}

// freeze stops all later writes, once a stopping daemon wrote its final state.
func (s *stateStore) freeze() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frozen = true
}
