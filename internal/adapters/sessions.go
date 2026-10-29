package adapters

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"golang.org/x/sync/errgroup"
)

// Session is one conversation in a harness's own store, reduced to what a listing shows. Title
// and Message are excerpts; nothing else of the transcript leaves the store.
type Session struct {
	ID        string    `json:"id"`
	Directory string    `json:"directory"`
	Title     string    `json:"title,omitempty"`
	Message   string    `json:"message,omitempty"`
	Started   time.Time `json:"started"`
	Updated   time.Time `json:"updated"`
}

// ParseFunc reads one session file. It returns ok false for a file that holds no listable
// session, such as a subagent's, and an error for a malformed one.
type ParseFunc func(f *os.File, info fs.FileInfo) (session Session, ok bool, err error)

// SessionFiles parses session files, reusing earlier results for files whose size and
// modification time are unchanged. It is safe for concurrent use.
type SessionFiles interface {
	Parse(path string, parse ParseFunc) (Session, bool)
}

// Bounds of what a session parser reads: the head holds metadata and the first message; a title
// set later in a long session is found in the tail.
const (
	SessionHead = 1 << 20
	SessionTail = 64 << 10
)

// ParseAll parses paths concurrently and returns the listable sessions.
func ParseAll(ctx context.Context, files SessionFiles, paths []string, parse ParseFunc) []Session {
	results := make([]*Session, len(paths))
	var group errgroup.Group
	group.SetLimit(runtime.GOMAXPROCS(0))
	for i, path := range paths {
		if ctx.Err() != nil {
			break
		}
		group.Go(func() error {
			if ctx.Err() == nil {
				if session, ok := files.Parse(path, parse); ok {
					results[i] = &session
				}
			}
			return nil
		})
	}
	_ = group.Wait()
	sessions := []Session{}
	for _, s := range results {
		if s != nil {
			sessions = append(sessions, *s)
		}
	}
	return sessions
}

// StoreHome is the directory named by the harness's override variable, or dir under the home
// directory.
func StoreHome(variable, dir, directory string, lookup func(string) string) (string, error) {
	if value := lookup(variable); variable != "" && value != "" {
		return StorePath(directory, value)
	}
	home := lookup("HOME")
	if runtime.GOOS == "windows" {
		home = lookup("USERPROFILE")
	}
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}
	return StorePath(directory, filepath.Join(home, dir))
}

func StorePath(directory, path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(directory, path)
	}
	return filepath.Abs(path)
}

// FindSessionFiles lists the regular files under root matching pattern, at most depth directories
// down. A missing root holds no sessions.
func FindSessionFiles(ctx context.Context, root string, depth int, pattern string) ([]string, error) {
	paths := []string{}
	problems := []error{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			if path == root && errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			if d != nil && d.IsDir() && path != root {
				problems = append(problems, err)
				return fs.SkipDir
			}
			return err
		}
		rel, _ := filepath.Rel(root, path)
		levels := strings.Count(rel, string(filepath.Separator))
		if d.IsDir() {
			if path != root && levels >= depth {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			if ok, _ := filepath.Match(pattern, d.Name()); ok {
				paths = append(paths, path)
			}
		}
		return nil
	})
	return paths, errors.Join(append(problems, err)...)
}

// ScanHead calls fn for each line within the head of f until fn returns false. A line longer than
// what remains of the head ends the scan.
func ScanHead(f *os.File, fn func(line []byte) bool) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	scanner := bufio.NewScanner(io.LimitReader(f, SessionHead))
	scanner.Buffer(make([]byte, 0, 64<<10), SessionHead)
	for scanner.Scan() {
		if !fn(scanner.Bytes()) {
			return nil
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, bufio.ErrTooLong) {
		return err
	}
	return nil
}

// ScanTail calls fn for each complete line in the tail of f that the head did not already cover.
func ScanTail(f *os.File, size int64, fn func(line []byte)) error {
	if size <= SessionHead {
		return nil
	}
	start := max(size-SessionTail, SessionHead)
	b := make([]byte, size-start)
	if _, err := f.ReadAt(b, start); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	// The first line is usually cut by the window.
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		b = b[i+1:]
	}
	for line := range bytes.SplitSeq(b, []byte("\n")) {
		if len(line) > 0 {
			fn(line)
		}
	}
	return nil
}

// Excerpt collapses whitespace and shortens text to at most limit characters.
func Excerpt(text string, limit int) string {
	text = strings.Join(strings.FieldsFunc(text, unicode.IsSpace), " ")
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return strings.TrimRightFunc(string(runes[:limit-1]), unicode.IsSpace) + "…"
}

// Timestamp parses an RFC 3339 timestamp, returning the zero time when it is missing or invalid.
func Timestamp(value string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

// ContainsAny reports whether line contains one of markers, which spares parsing lines that
// cannot hold what a store scan looks for.
func ContainsAny(line []byte, markers [][]byte) bool {
	for _, m := range markers {
		if bytes.Contains(line, m) {
			return true
		}
	}
	return false
}
