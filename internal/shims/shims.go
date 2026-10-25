// Package shims maintains executables that run harness definitions from any shell. A shim is
// agx itself under a definition's name, which agx dispatches on. agx owns the shim directory:
// syncing it removes whatever else is there.
package shims

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/tarik02-org/agx/internal/securefs"
)

// targetFile records, on Windows, the agx that copy shims hand off to.
const targetFile = "agx-target"

type State string

const (
	// Current shims run their definition through the current agx executable.
	Current State = "current"
	// Stale shims belong to a removed definition or an earlier agx path; install fixes them.
	Stale State = "stale"
	// Missing shims are for definitions without one; install creates them.
	Missing State = "missing"
)

type Entry struct {
	Name  string `json:"name"`
	File  string `json:"file"`
	State State  `json:"state"`
}

type Action string

const (
	Created   Action = "created"
	Updated   Action = "updated"
	Removed   Action = "removed"
	Unchanged Action = "unchanged"
)

type Change struct {
	Name   string `json:"name"`
	File   string `json:"file"`
	Action Action `json:"action"`
}

// Dir is agx's shim directory, $XDG_DATA_HOME/agx/shims (~/.local/share/agx/shims), or
// %LOCALAPPDATA%\agx\shims on Windows. macOS uses the XDG path too rather than Application
// Support: the directory goes on PATH, where a space is easy to break.
func Dir() (string, error) {
	if runtime.GOOS == "windows" {
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			return "", errors.New("LOCALAPPDATA is not set")
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

// A Unix shim is a symbolic link, so signals, exit codes, and arguments pass unchanged. Windows
// symbolic links need Developer Mode, and a hard link keeps running the build it was made from
// after agx updates in place, so a Windows shim is a copy that hands off to the recorded agx.
func fileName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func shimName(file string) string {
	if runtime.GOOS == "windows" {
		return strings.TrimSuffix(file, ".exe")
	}
	return file
}

// current reports whether the shim file runs target.
func current(dir, file, target string) bool {
	path := filepath.Join(dir, file)
	if runtime.GOOS != "windows" {
		link, err := os.Readlink(path)
		return err == nil && link == target
	}
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && recorded(dir) == target
}

func recorded(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, targetFile))
	if err != nil {
		return ""
	}
	return string(b)
}

// files lists the shim directory, without the Windows target record.
func files(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, e := range entries {
		if e.Name() != targetFile {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// replace puts a shim for target at file, through a temporary file renamed over it.
func replace(dir, file, target string) error {
	tmp := filepath.Join(dir, ".agx-shim-"+securefs.Token()[:16])
	var err error
	if runtime.GOOS == "windows" {
		err = copyFile(target, tmp)
	} else {
		err = os.Symlink(target, tmp)
	}
	if err == nil {
		err = os.Rename(tmp, filepath.Join(dir, file))
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
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

// Origin reports the agx executable the shim at path runs, when path is one of agx's shims.
func Origin(path string) (string, bool) {
	if !IsShim(path) {
		return "", false
	}
	if runtime.GOOS != "windows" {
		target, err := os.Readlink(path)
		return target, err == nil
	}
	target := recorded(filepath.Dir(path))
	return target, target != ""
}

// List reports the shims in dir and the definitions without one.
func List(dir, target string, names []string) ([]Entry, error) {
	existing, err := files(dir)
	if err != nil {
		return nil, err
	}
	entries := []Entry{}
	for _, file := range existing {
		state := Stale
		if slices.Contains(names, shimName(file)) && current(dir, file, target) {
			state = Current
		}
		entries = append(entries, Entry{Name: shimName(file), File: file, State: state})
	}
	for _, name := range names {
		if !slices.Contains(existing, fileName(name)) {
			entries = append(entries, Entry{Name: name, File: fileName(name), State: Missing})
		}
	}
	slices.SortFunc(entries, func(a, b Entry) int { return strings.Compare(a.Name, b.Name) })
	return entries, nil
}

// Install makes dir hold exactly one shim per name, each running target. Failures for one shim
// do not stop the others.
func Install(dir, target string, names []string) ([]Change, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	// Windows copies hand off to the recorded agx, so retargeting them rewrites only the record.
	retarget := runtime.GOOS == "windows" && recorded(dir) != target
	if retarget {
		if err := os.WriteFile(filepath.Join(dir, targetFile), []byte(target), 0o644); err != nil {
			return nil, err
		}
	}
	existing, err := files(dir)
	if err != nil {
		return nil, err
	}
	changes := []Change{}
	var errs []error
	for _, file := range existing {
		if slices.Contains(names, shimName(file)) && file == fileName(shimName(file)) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, file)); err != nil {
			errs = append(errs, err)
			continue
		}
		changes = append(changes, Change{Name: shimName(file), File: file, Action: Removed})
	}
	for _, name := range slices.Sorted(slices.Values(names)) {
		file := fileName(name)
		change := Change{Name: name, File: file, Action: Created}
		switch {
		case !slices.Contains(existing, file):
		case current(dir, file, target):
			change.Action = Unchanged
			if retarget {
				change.Action = Updated
			}
		default:
			change.Action = Updated
		}
		if !current(dir, file, target) {
			if err := replace(dir, file, target); err != nil {
				errs = append(errs, err)
				continue
			}
		}
		changes = append(changes, change)
	}
	return changes, errors.Join(errs...)
}

// Remove deletes the shim directory.
func Remove(dir string) ([]Change, error) {
	existing, err := files(dir)
	if err != nil {
		return nil, err
	}
	changes := []Change{}
	for _, file := range existing {
		changes = append(changes, Change{Name: shimName(file), File: file, Action: Removed})
	}
	return changes, os.RemoveAll(dir)
}
