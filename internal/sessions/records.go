package sessions

import (
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

// Records is the launch record file, shared by concurrent wrappers through a lock file.
type Records struct{ path string }

func NewRecords(state string) Records {
	return Records{path: filepath.Join(state, "launch-records.json")}
}

type recordFile struct {
	Version int      `json:"version"`
	Records []Record `json:"records"`
}

// Start adds a record for a launch about to spawn its harness.
func (r Records) Start(record Record) error {
	return r.update(func(records []Record) []Record { return append(records, record) })
}

// Finish closes the window of a launch.
func (r Records) Finish(launch string, at time.Time) error {
	return r.update(func(records []Record) []Record {
		for i := range records {
			if records[i].Launch == launch {
				records[i].Finished = &at
			}
		}
		return records
	})
}

// update applies change to the records under the lock; a nil result leaves the file unchanged.
func (r Records) update(change func([]Record) []Record) error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return err
	}
	lock := flock.New(r.path + ".lock")
	if err := lock.Lock(); err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	records, err := r.load()
	if err != nil {
		return err
	}
	changed := change(slices.Clone(records))
	if changed == nil {
		return nil
	}
	return securefs.WritePrivateJSON(r.path, recordFile{Version: 1, Records: changed})
}

func (r Records) load() ([]Record, error) {
	b, err := os.ReadFile(r.path)
	if errors.Is(err, fs.ErrNotExist) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	var file recordFile
	// Records only refine resumption, so a damaged file starts over instead of failing launches.
	if err := json.Unmarshal(b, &file); err == nil && file.Version == 1 {
		return file.Records, nil
	}
	return []Record{}, nil
}
