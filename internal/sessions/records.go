package sessions

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/gofrs/flock"

	"github.com/tarik02-org/agx/internal/securefs"
)

// Record remembers one managed launch, so its sessions resume with the same harness definition.
type Record struct {
	Launch    string     `json:"launch"`
	Harness   string     `json:"harness"`
	Builtin   string     `json:"builtin"`
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

// Harnesses stamp a session when it begins, so a launch that cannot name its sessions claims those
// of its harness and directory that started while it ran. skew absorbs the gap between the
// wrapper's clock reading and the harness's.
const (
	skew = 5 * time.Second
	// maxLaunch closes the window of a launch whose wrapper died before recording its end.
	maxLaunch = 24 * time.Hour
)

func (r Record) window() (time.Time, time.Time) {
	end := r.Started.Add(maxLaunch)
	if r.Finished != nil {
		end = *r.Finished
	}
	return r.Started.Add(-skew), end.Add(skew)
}

// Records is the launch record log. Each launch appends a line when it starts and when it
// finishes, so a write costs the same however many records there are; a listing folds the lines
// into one per record and rewrites the log, pruned. Losing the last line in a crash only costs a
// resumption its definition, so appends are not synced.
type Records struct{ path string }

func NewRecords(state string) Records {
	return Records{path: filepath.Join(state, "launch-records.jsonl")}
}

// finishLine is the line Finish appends: a record line without a start.
type finishLine struct {
	Launch   string    `json:"launch"`
	Finished time.Time `json:"finished"`
}

// Start adds a record for a launch about to spawn its harness.
func (r Records) Start(record Record) error { return r.append(record) }

// Finish closes the window of a launch.
func (r Records) Finish(launch string, at time.Time) error {
	return r.append(finishLine{Launch: launch, Finished: at})
}

func (r Records) append(line any) error {
	b, err := json.Marshal(line)
	if err != nil {
		return err
	}
	unlock, err := r.lock()
	if err != nil {
		return err
	}
	defer unlock()
	return securefs.AppendPrivate(r.path, append(b, '\n'))
}

func (r Records) lock() (func(), error) {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return nil, err
	}
	lock := flock.New(r.path + ".lock")
	if err := lock.Lock(); err != nil {
		return nil, err
	}
	return func() { _ = lock.Unlock() }, nil
}

// update applies change to the records under the lock and rewrites the log with one line per
// record. A nil result leaves the records unchanged, and the log is rewritten only if it was not
// already folded.
func (r Records) update(change func([]Record) []Record) error {
	unlock, err := r.lock()
	if err != nil {
		return err
	}
	defer unlock()
	records, folded, err := r.load()
	if err != nil {
		return err
	}
	changed := change(slices.Clone(records))
	if changed == nil {
		if folded {
			return nil
		}
		changed = records
	}
	var b []byte
	for _, record := range changed {
		line, err := json.Marshal(record)
		if err != nil {
			return err
		}
		b = append(append(b, line...), '\n')
	}
	if err := securefs.WritePrivate(r.path, b); err != nil {
		return err
	}
	return nil
}

// load folds the log into records in the order of their first line; folded reports
// whether the log already holds exactly one line per record.
func (r Records) load() (records []Record, folded bool, err error) {
	records = []Record{}
	byLaunch := map[string]int{}
	add := func(record Record) {
		i, ok := byLaunch[record.Launch]
		if !ok {
			if record.Started.IsZero() {
				// The finish of a launch whose start was lost, or of a thread that never bound.
				return
			}
			byLaunch[record.Launch] = len(records)
			records = append(records, record)
			return
		}
		if record.Finished != nil {
			records[i].Finished = record.Finished
		}
		for _, id := range record.Sessions {
			if !slices.Contains(records[i].Sessions, id) {
				records[i].Sessions = append(records[i].Sessions, id)
			}
		}
	}
	// Records only refine resumption, so damaged content is skipped instead of failing launches.
	b, err := os.ReadFile(r.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, false, err
	}
	lines := 0
	for line := range bytes.Lines(b) {
		lines++
		var record Record
		if json.Unmarshal(line, &record) == nil && record.Launch != "" {
			add(record)
		}
	}
	return records, lines == len(records), nil
}
