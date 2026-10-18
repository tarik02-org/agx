package securefs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func Token() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

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
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func Contained(root, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("package paths must be relative: %s", rel)
	}
	p := filepath.Join(root, rel)
	if !Within(root, p) {
		return "", fmt.Errorf("path escapes package: %s", rel)
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	if !Within(canonicalRoot, resolved) {
		return "", fmt.Errorf("path escapes package: %s", rel)
	}
	return resolved, nil
}

// CopyTree dereferences internal symlinks so cached content cannot change later.
func CopyTree(src, dst string) error {
	root, err := filepath.EvalSymlinks(src)
	if err != nil {
		return err
	}
	active := map[string]bool{}
	var copyPath func(string, string) error
	copyPath = func(from, to string) error {
		real, err := filepath.EvalSymlinks(from)
		if err != nil {
			return err
		}
		if !Within(root, real) {
			return fmt.Errorf("symlink escapes package: %s", from)
		}
		info, err := os.Stat(real)
		if err != nil {
			return err
		}
		if info.IsDir() {
			if active[real] {
				return fmt.Errorf("symlink cycle at %s", from)
			}
			active[real] = true
			defer delete(active, real)
			if err := os.MkdirAll(to, 0o700); err != nil {
				return err
			}
			entries, err := os.ReadDir(real)
			if err != nil {
				return err
			}
			for _, e := range entries {
				if e.Name() == ".git" {
					continue
				}
				if err := copyPath(filepath.Join(real, e.Name()), filepath.Join(to, e.Name())); err != nil {
					return err
				}
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported package file: %s", from)
		}
		in, err := os.Open(real)
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600|info.Mode().Perm()&0o111)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, in)
		closeErr := out.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	return copyPath(root, dst)
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
