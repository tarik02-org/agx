package securefs

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

func Token() string { return rand.Text() }

// Write never changes permissions of an existing parent; callers harden agx-owned directories with EnsureDir.
func Write(path string, data []byte, mode os.FileMode) error {
	if err := createPrivate(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".agx-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if err = f.Chmod(mode); err == nil {
		err = Private(f.Name())
	}
	if err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

func WriteJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return Write(path, append(b, '\n'), 0o600)
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
			f, err := fsys.Open(name)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()
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

func EnsureDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return Private(path)
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
		if err := Private(missing[i]); err != nil {
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
