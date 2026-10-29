package configedit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"maps"
	"os"
	"slices"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/config/edit"
	"github.com/tarik02-org/agx/internal/securefs"
)

// FileKind says how a planned file is written.
type FileKind int

const (
	// Fragment is a configuration fragment.
	Fragment FileKind = iota
	// Content is a file of a local skill or instructions definition.
	Content
	// Lockfile is a lockfile, written private to the user.
	Lockfile
)

// PlannedFile is one file a plan writes.
type PlannedFile struct {
	Path string
	Kind FileKind
	// Before is the current content; nil for a file the plan creates.
	Before []byte
	After  []byte
}

// Diff is a unified diff from the current content.
func (f PlannedFile) Diff() string { return edit.Diff(f.Path, f.Before, f.After) }

// BeforeSHA256 is the hex SHA-256 of the current content, or empty for a new file.
func (f PlannedFile) BeforeSHA256() string { return digest(f.Before) }

func digest(b []byte) string {
	if b == nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Plan is a validated set of file changes.
type Plan struct {
	Files []PlannedFile
	// Config is the configuration the plan produces.
	Config *config.Config
}

// Plan renders the session's edits, refuses files that cannot be written, and validates the
// configuration they produce. A plan without files has nothing to change.
func (s *Session) Plan() (*Plan, error) {
	plan := &Plan{Config: s.Config}
	overlay := config.Overlay{}
	for _, path := range s.order {
		f := s.files[path]
		f.Prune()
		after, err := f.Render()
		if err != nil {
			return nil, err
		}
		if f.Exists && bytes.Equal(after, f.Original()) {
			continue
		}
		plan.Files = append(plan.Files, PlannedFile{Path: path, Kind: Fragment, Before: f.Original(), After: after})
		overlay[path] = after
	}
	for _, path := range s.writeOrder {
		before, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			before = nil
		} else if err != nil {
			return nil, err
		}
		if before != nil && bytes.Equal(before, s.writes[path]) {
			continue
		}
		plan.Files = append(plan.Files, PlannedFile{Path: path, Kind: Content, Before: before, After: s.writes[path]})
	}
	if len(plan.Files) == 0 {
		return plan, nil
	}
	for path, b := range s.Locks {
		overlay[path] = b
	}
	for _, p := range plan.Files {
		if err := edit.Writable(p.Path); err != nil {
			return nil, coded(ReadOnly, err)
		}
	}
	c, err := config.Load(s.Paths.Config, config.Options{Builtins: s.Builtins, Cache: s.Paths.Cache, Overlay: overlay})
	if err != nil {
		return nil, errorf(
			ConfigInvalid,
			"the change would make the configuration invalid, so nothing was written: %w",
			err,
		)
	}
	for _, path := range slices.Sorted(maps.Keys(s.Locks)) {
		current, err := os.ReadFile(path)
		if err == nil && bytes.Equal(current, s.Locks[path]) {
			continue
		}
		if err != nil {
			current = nil
		}
		plan.Files = append(plan.Files, PlannedFile{Path: path, Kind: Lockfile, Before: current, After: s.Locks[path]})
	}
	plan.Config = c
	return plan, nil
}

// Check compares the plan with the hashes of an earlier plan of the same changes, by path, and
// refuses it when any file changed since.
func (p *Plan) Check(expected map[string]string) error {
	if len(expected) != len(p.Files) {
		return errorf(Conflict, "the configuration changed since the plan; plan again")
	}
	for _, f := range p.Files {
		want, ok := expected[f.Path]
		if !ok {
			return errorf(Conflict, "%s was not part of the plan; plan again", f.Path)
		}
		if want != f.BeforeSHA256() {
			return errorf(Conflict, "%s changed since the plan; plan again", f.Path)
		}
	}
	return nil
}

// Write writes the plan's files, after checking that none changed since the plan read them, and
// returns their paths.
func (p *Plan) Write() ([]string, error) {
	for _, f := range p.Files {
		current, err := os.ReadFile(f.Path)
		if errors.Is(err, fs.ErrNotExist) {
			current = nil
		} else if err != nil {
			return nil, err
		}
		if digest(current) != f.BeforeSHA256() {
			return nil, errorf(Conflict, "%s changed since the plan; plan again", f.Path)
		}
	}
	written := []string{}
	for _, f := range p.Files {
		if err := securefs.Write(f.Path, f.After); err != nil {
			return written, err
		}
		written = append(written, f.Path)
	}
	return written, nil
}
