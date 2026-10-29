package securefs

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

func Token() string { return rand.Text() }

// WritePrivate replaces path atomically with a synced file only the current user can access, for
// secrets, credentials and state. Directories it creates are private too; existing ones are left
// as they are, so callers make agx's private directories private with EnsureDir.
func WritePrivate(path string, data []byte) error {
	if err := createPrivate(filepath.Dir(path)); err != nil {
		return err
	}
	tmp, err := temporary(filepath.Dir(path), data, true, makePrivate)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp) }()
	return os.Rename(tmp, path)
}

func WritePrivateJSON(path string, value any) error {
	b, err := encodeJSON(value)
	if err != nil {
		return err
	}
	return WritePrivate(path, b)
}

// Staged is data written to a temporary file beside the file it is to replace.
type Staged struct {
	path, temp string
}

// Stage writes data beside path, synced, for Commit to put in place; Write explains the rest.
func Stage(path string, data []byte) (*Staged, error) {
	return stage(path, data, true)
}

func (s *Staged) Commit() error { return os.Rename(s.temp, s.path) }

// Discard removes the temporary file of data not committed.
func (s *Staged) Discard() { _ = os.Remove(s.temp) }

// Write replaces path atomically with a synced file, for files of the user such as configuration
// and lockfiles. An existing file keeps its mode, a new one gets 0644, and a symlinked file is
// written at its target.
func Write(path string, data []byte) error {
	return commit(stage(path, data, true))
}

// WriteCacheJSON is Write for content agx can recreate, so it skips the sync.
func WriteCacheJSON(path string, value any) error {
	b, err := encodeJSON(value)
	if err != nil {
		return err
	}
	return commit(stage(path, b, false))
}

func commit(s *Staged, err error) error {
	if err != nil {
		return err
	}
	defer s.Discard()
	return s.Commit()
}

func stage(path string, data []byte, sync bool) (*Staged, error) {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	tmp, err := temporary(filepath.Dir(path), data, sync, func(name string) error { return os.Chmod(name, mode) })
	if err != nil {
		return nil, err
	}
	return &Staged{path: path, temp: tmp}, nil
}

// temporary writes data to a new temporary file in dir after protect sets its permissions, and
// returns its name.
func temporary(dir string, data []byte, sync bool, protect func(string) error) (string, error) {
	f, err := os.CreateTemp(dir, ".agx-*")
	if err != nil {
		return "", err
	}
	err = protect(f.Name())
	if err == nil {
		_, err = f.Write(data)
	}
	if err == nil && sync {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func encodeJSON(value any) ([]byte, error) {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func Within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && filepath.IsLocal(rel)
}

// Contained resolves rel inside root; os.Root rejects symlinks that leave it.
func Contained(root, rel string) (string, error) {
	if !filepath.IsLocal(rel) {
		return "", fmt.Errorf("path escapes package: %s", rel)
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer func() { _ = r.Close() }()
	if _, err := r.Stat(rel); err != nil {
		return "", err
	}
	return filepath.Join(root, rel), nil
}

// Walk visits src and, for a directory, everything under it but .git in lexical order, with rel
// slash-separated and content set for files. Internal symlinks are followed so a walk sees what
// a later read would; escaping or cyclic ones are rejected.
func Walk(src string, visit func(rel string, info fs.FileInfo, content io.Reader) error) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	// A single file has no tree for a symlink to escape.
	fsys, start := os.DirFS(filepath.Dir(src)), filepath.Base(src)
	if info.IsDir() {
		root, err := os.OpenRoot(src)
		if err != nil {
			return err
		}
		defer func() { _ = root.Close() }()
		fsys, start = root.FS(), "."
	}
	var active []fs.FileInfo
	var walk func(name, rel string) error
	walk = func(name, rel string) error {
		info, err := fs.Stat(fsys, name)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("unsupported package file: %s", rel)
			}
			f := &lazyFile{fsys: fsys, name: name}
			defer func() {
				if f.file != nil {
					_ = f.file.Close()
				}
			}()
			return visit(rel, info, f)
		}
		for _, a := range active {
			if os.SameFile(a, info) {
				return fmt.Errorf("symlink cycle at %s", rel)
			}
		}
		if err := visit(rel, info, nil); err != nil {
			return err
		}
		active = append(active, info)
		defer func() { active = active[:len(active)-1] }()
		entries, err := fs.ReadDir(fsys, name)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.Name() == ".git" {
				continue
			}
			if err := walk(path.Join(name, e.Name()), path.Join(rel, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(start, ".")
}

// lazyFile opens a file on its first read, so a walk that checks only the structure of a tree
// opens no file.
type lazyFile struct {
	fsys fs.FS
	name string
	file fs.File
}

func (f *lazyFile) open() error {
	if f.file != nil {
		return nil
	}
	var err error
	f.file, err = f.fsys.Open(f.name)
	return err
}

func (f *lazyFile) Read(p []byte) (int, error) {
	if err := f.open(); err != nil {
		return 0, err
	}
	return f.file.Read(p)
}

// WriteTo keeps the copy fast paths of the opened file.
func (f *lazyFile) WriteTo(w io.Writer) (int64, error) {
	if err := f.open(); err != nil {
		return 0, err
	}
	return io.Copy(w, f.file)
}

// CopyTree dereferences internal symlinks so cached content cannot change later.
func CopyTree(src, dst string) error {
	return Walk(src, func(rel string, info fs.FileInfo, content io.Reader) error {
		to := filepath.Join(dst, filepath.FromSlash(rel))
		if content == nil {
			return os.MkdirAll(to, 0o700)
		}
		out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600|info.Mode().Perm()&0o111)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, content)
		closeErr := out.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}

// EnsureDir creates path private, or makes an existing one private, for directories holding
// credentials and state.
func EnsureDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return makePrivate(path)
}

// createPrivate makes only the directories it creates private.
func createPrivate(path string) error {
	var missing []string
	for dir := path; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(dir); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return err
		}
		missing = append(missing, dir)
		if filepath.Dir(dir) == dir {
			break
		}
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	for i := len(missing) - 1; i >= 0; i-- {
		if err := makePrivate(missing[i]); err != nil {
			return err
		}
	}
	return nil
}

// NewerFormatError reports a file agx owns that a newer agx wrote in a format this one cannot
// read; rewriting it would lose what the newer version recorded.
type NewerFormatError struct {
	Path               string
	Version, Supported int
}

func (e *NewerFormatError) Error() string {
	return fmt.Sprintf(
		"%s uses format version %d, but this agx reads only version %d; upgrade agx to use it",
		e.Path, e.Version, e.Supported,
	)
}

// CheckFormat rejects data of an agx-owned JSON file written in a newer format version than
// supported, before stricter decoding fails on fields this version does not know.
func CheckFormat(path string, data []byte, supported int) error {
	var header struct {
		Version int `json:"version"`
	}
	if json.Unmarshal(data, &header) == nil && header.Version > supported {
		return &NewerFormatError{Path: path, Version: header.Version, Supported: supported}
	}
	return nil
}
