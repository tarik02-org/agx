// Package shims maintains executables that run harness definitions from any shell. A shim is
// agx itself under a definition's name, which agx dispatches on; a manifest in the shim directory
// records what agx created, so it never replaces or deletes anyone else's files.
package shims

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/tarik02-org/agx/internal/securefs"
)

const manifestName = ".agx-shims.json"

type Kind string

const (
	Symlink  Kind = "symlink"
	Hardlink Kind = "hardlink"
	Copy     Kind = "copy"
	Script   Kind = "script"
)

// Shim is one executable agx created.
type Shim struct {
	Name   string `json:"name"`
	File   string `json:"file"`
	Kind   Kind   `json:"kind,omitempty"`
	Target string `json:"target,omitempty"`
	// Hash identifies a regular-file shim's content: a hard link or copy cannot name its source,
	// and agx must recognize its own file before replacing or deleting it.
	Hash string `json:"hash,omitempty"`
}

type State string

const (
	// Current shims run their definition through the current agx executable.
	Current State = "current"
	// Stale shims belong to a removed definition or an earlier agx path; install fixes them.
	Stale State = "stale"
	// Missing shims are for definitions without one; install creates them.
	Missing State = "missing"
	// Modified shims changed since agx created them; agx leaves them alone.
	Modified State = "modified"
	// Conflict means a file agx did not create already has the shim's name.
	Conflict State = "conflict"
)

type Entry struct {
	Shim
	State State `json:"state"`
}

type Action string

const (
	Created   Action = "created"
	Updated   Action = "updated"
	Removed   Action = "removed"
	Unchanged Action = "unchanged"
	// Kept files are left in place because agx did not create them, or they changed since.
	Kept Action = "kept"
)

type Change struct {
	Shim
	Action Action `json:"action"`
	// State explains a kept shim.
	State State `json:"state,omitempty"`
}

// manifestVersion is the format version of the shim manifest; agx rejects manifests of others.
const manifestVersion = 1

type manifest struct {
	Version int    `json:"version"`
	Shims   []Shim `json:"shims"`
}

// DefaultDir is a directory of agx's own, $XDG_DATA_HOME/agx/shims (~/.local/share/agx/shims), or
// %LOCALAPPDATA%\agx\shims on Windows. macOS uses the XDG path too rather than Application
// Support: the directory goes on PATH, where a space is easy to break.
func DefaultDir() (string, error) {
	if runtime.GOOS == "windows" {
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			return "", errors.New("LOCALAPPDATA is not set; pass --dir")
		}
		return filepath.Join(local, "agx", "shims"), nil
	}
	if data := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(data) {
		return filepath.Join(data, "agx", "shims"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "agx", "shims"), nil
}

// kinds lists how shims are made, most faithful first. A link runs agx itself, so signals, exit
// codes, and arguments pass unchanged; Windows symlinks need Developer Mode, so it links hard.
func kinds() []Kind {
	if runtime.GOOS == "windows" {
		return []Kind{Hardlink, Copy, Script}
	}
	return []Kind{Symlink, Script}
}

func fileName(name string, kind Kind) string {
	switch {
	case runtime.GOOS != "windows":
		return name
	case kind == Script:
		return name + ".cmd"
	default:
		return name + ".exe"
	}
}

// script runs agx with the definition as its command, since a script cannot change argv[0].
func script(target, name string) []byte {
	if runtime.GOOS == "windows" {
		// Definition names are [A-Za-z0-9._-], so only the target needs escaping.
		return fmt.Appendf(nil, "@echo off\r\n\"%s\" %s %%*\r\n", strings.ReplaceAll(target, "%", "%%"), name)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
	return fmt.Appendf(nil, "#!/bin/sh\nexec %s %s \"$@\"\n", quote(target), quote(name))
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func readManifest(dir string) (manifest, error) {
	var m manifest
	path := filepath.Join(dir, manifestName)
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	if err := securefs.CheckFormat(path, b, manifestVersion); err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("%s: %w", path, err)
	}
	if m.Version != manifestVersion {
		return m, fmt.Errorf("%s: unsupported format; remove the manifest to forget agx's shims", path)
	}
	for _, s := range m.Shims {
		// A file name that leaves the directory would let the manifest delete arbitrary files.
		valid := s.Name != "" && s.File == filepath.Base(s.File) && s.File != "." && s.File != ".." &&
			s.File != manifestName && slices.Contains([]Kind{Symlink, Hardlink, Copy, Script}, s.Kind)
		if !valid {
			return m, fmt.Errorf("%s: invalid entry for %q; remove the manifest to forget agx's shims", path, s.Name)
		}
	}
	return m, nil
}

func writeManifest(dir string, shims []Shim) error {
	path := filepath.Join(dir, manifestName)
	if len(shims) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	sort.Slice(shims, func(i, j int) bool { return shims[i].Name < shims[j].Name })
	b, err := json.MarshalIndent(manifest{Version: manifestVersion, Shims: shims}, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".agx-shims-"+securefs.Token()[:16])
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// owned reports whether the shim's file exists and is still what agx created.
func owned(dir string, s Shim) (exists, ours bool, err error) {
	path := filepath.Join(dir, s.File)
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if s.Kind == Symlink {
		if info.Mode()&fs.ModeSymlink == 0 {
			return true, false, nil
		}
		link, err := os.Readlink(path)
		return true, err == nil && link == s.Target, err
	}
	if !info.Mode().IsRegular() {
		return true, false, nil
	}
	hash, err := hashFile(path)
	return true, err == nil && hash == s.Hash, err
}

// current reports whether an owned shim already runs target under its name.
func current(dir string, s Shim, target string) (bool, error) {
	if s.Target != target || s.File != fileName(s.Name, s.Kind) {
		return false, nil
	}
	switch s.Kind {
	case Hardlink:
		shim, err := os.Stat(filepath.Join(dir, s.File))
		if err != nil {
			return false, err
		}
		source, err := os.Stat(target)
		if err != nil {
			return false, err
		}
		return os.SameFile(shim, source), nil
	case Copy:
		hash, err := hashFile(target)
		return err == nil && hash == s.Hash, err
	case Script:
		return s.Hash == hashBytes(script(target, s.Name)), nil
	default:
		// owned already compared the link with the recorded target.
		return true, nil
	}
}

// occupied reports whether a file of any kind exists at path.
func occupied(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// create makes the shim for name in the first kind that works, replacing only the file named
// replace, which the caller verified agx owns.
func create(dir, name, target, replace string) (Shim, error) {
	var errs []error
	for _, kind := range kinds() {
		file := fileName(name, kind)
		final := filepath.Join(dir, file)
		if file != replace {
			taken, err := occupied(final)
			if err != nil || taken {
				errs = append(errs, fmt.Errorf("%s: %w", final, errors.Join(err, fs.ErrExist)))
				continue
			}
		}
		s, err := write(dir, name, target, kind)
		if err == nil {
			return s, nil
		}
		errs = append(errs, fmt.Errorf("%s %s: %w", kind, final, err))
	}
	return Shim{}, errors.Join(errs...)
}

func write(dir, name, target string, kind Kind) (Shim, error) {
	s := Shim{Name: name, File: fileName(name, kind), Kind: kind, Target: target}
	tmp := filepath.Join(dir, ".agx-shim-"+securefs.Token()[:16])
	var err error
	switch kind {
	case Symlink:
		err = os.Symlink(target, tmp)
	case Hardlink:
		err = os.Link(target, tmp)
	case Copy:
		err = copyFile(target, tmp)
	case Script:
		err = os.WriteFile(tmp, script(target, name), 0o755)
	}
	if err == nil && kind != Symlink {
		s.Hash, err = hashFile(tmp)
	}
	if err == nil {
		err = os.Rename(tmp, filepath.Join(dir, s.File))
	}
	if err != nil {
		_ = os.Remove(tmp)
		return Shim{}, err
	}
	return s, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return errors.Join(err, out.Close())
}

// List reports agx's shims in dir and the definitions without one.
func List(dir, target string, names []string) ([]Entry, error) {
	m, err := readManifest(dir)
	if err != nil {
		return nil, err
	}
	entries := []Entry{}
	recorded := map[string]bool{}
	for _, s := range m.Shims {
		recorded[s.Name] = true
		exists, ours, err := owned(dir, s)
		if err != nil {
			return nil, err
		}
		wanted := slices.Contains(names, s.Name)
		switch {
		case !exists && wanted:
			entries = append(
				entries,
				Entry{Shim: Shim{Name: s.Name, File: fileName(s.Name, kinds()[0])}, State: Missing},
			)
		case !exists:
		case !ours:
			entries = append(entries, Entry{Shim: s, State: Modified})
		case !wanted:
			entries = append(entries, Entry{Shim: s, State: Stale})
		default:
			ok, err := current(dir, s, target)
			if err != nil {
				return nil, err
			}
			state := Stale
			if ok {
				state = Current
			}
			entries = append(entries, Entry{Shim: s, State: state})
		}
	}
	for _, name := range names {
		if recorded[name] {
			continue
		}
		s := Shim{Name: name, File: fileName(name, kinds()[0])}
		taken, err := occupied(filepath.Join(dir, s.File))
		if err != nil {
			return nil, err
		}
		state := Missing
		if taken {
			state = Conflict
		}
		entries = append(entries, Entry{Shim: s, State: state})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// Install makes dir hold one shim per name, each running target, and removes agx's shims for
// other names. Failures for one shim do not stop the others.
func Install(dir, target string, names []string) ([]Change, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	m, err := readManifest(dir)
	if err != nil {
		return nil, err
	}
	recorded := map[string]Shim{}
	for _, s := range m.Shims {
		recorded[s.Name] = s
	}
	kept := []Shim{}
	changes := []Change{}
	var errs []error
	for _, s := range m.Shims {
		if slices.Contains(names, s.Name) {
			continue
		}
		exists, ours, err := owned(dir, s)
		switch {
		case err != nil:
			errs = append(errs, err)
			kept = append(kept, s)
		case !exists:
		case !ours:
			changes = append(changes, Change{Shim: s, Action: Kept, State: Modified})
		default:
			if err := os.Remove(filepath.Join(dir, s.File)); err != nil {
				errs = append(errs, err)
				kept = append(kept, s)
				continue
			}
			changes = append(changes, Change{Shim: s, Action: Removed})
		}
	}
	for _, name := range slices.Sorted(slices.Values(names)) {
		replace := ""
		if s, ok := recorded[name]; ok {
			exists, ours, err := owned(dir, s)
			if err != nil {
				errs = append(errs, err)
				kept = append(kept, s)
				continue
			}
			if exists && !ours {
				changes = append(changes, Change{Shim: s, Action: Kept, State: Modified})
				continue
			}
			if exists {
				ok, err := current(dir, s, target)
				if err != nil {
					errs = append(errs, err)
					kept = append(kept, s)
					continue
				}
				if ok {
					kept = append(kept, s)
					changes = append(changes, Change{Shim: s, Action: Unchanged})
					continue
				}
				replace = s.File
			}
		} else {
			file := fileName(name, kinds()[0])
			taken, err := occupied(filepath.Join(dir, file))
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if taken {
				changes = append(changes, Change{Shim: Shim{Name: name, File: file}, Action: Kept, State: Conflict})
				continue
			}
		}
		s, err := create(dir, name, target, replace)
		if err != nil {
			errs = append(errs, fmt.Errorf("shim %s: %w", name, err))
			if replace != "" {
				kept = append(kept, recorded[name])
			}
			continue
		}
		action := Created
		if replace != "" {
			action = Updated
			if replace != s.File {
				if err := os.Remove(filepath.Join(dir, replace)); err != nil && !errors.Is(err, fs.ErrNotExist) {
					errs = append(errs, err)
				}
			}
		}
		kept = append(kept, s)
		changes = append(changes, Change{Shim: s, Action: action})
	}
	if err := writeManifest(dir, kept); err != nil {
		errs = append(errs, err)
	}
	return changes, errors.Join(errs...)
}

// Remove deletes every shim agx created in dir that is still as agx left it.
func Remove(dir string) ([]Change, error) {
	m, err := readManifest(dir)
	if err != nil {
		return nil, err
	}
	kept := []Shim{}
	changes := []Change{}
	var errs []error
	for _, s := range m.Shims {
		exists, ours, err := owned(dir, s)
		switch {
		case err != nil:
			errs = append(errs, err)
			kept = append(kept, s)
		case !exists:
		case !ours:
			changes = append(changes, Change{Shim: s, Action: Kept, State: Modified})
		default:
			if err := os.Remove(filepath.Join(dir, s.File)); err != nil {
				errs = append(errs, err)
				kept = append(kept, s)
				continue
			}
			changes = append(changes, Change{Shim: s, Action: Removed})
		}
	}
	if err := writeManifest(dir, kept); err != nil {
		errs = append(errs, err)
	}
	return changes, errors.Join(errs...)
}
