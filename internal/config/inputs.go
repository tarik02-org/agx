package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// Inputs records the state of every file and glob a load consulted.
type Inputs struct {
	entries        []input
	revision       func() string
	loadedRevision string
}

type input struct {
	path  string
	glob  bool
	state string
}

func (i *Inputs) file(path string) {
	i.entries = append(i.entries, input{path: path, state: fileState(path)})
}

func (i *Inputs) glob(pattern string, matches []string) {
	i.entries = append(i.entries, input{path: pattern, glob: true, state: Digest(matches)})
}

// Changed reports whether any recorded input differs from its state at load time.
func (i *Inputs) Changed() bool {
	if i.revision != nil && i.revision() != i.loadedRevision {
		return true
	}
	for _, e := range i.entries {
		if e.glob {
			if globState(e.path) != e.state {
				return true
			}
		} else if fileState(e.path) != e.state {
			return true
		}
	}
	return false
}

func fileState(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "error:" + err.Error()
	}
	return fmt.Sprintf("%d:%d:%s", info.ModTime().UnixNano(), info.Size(), info.Mode())
}

func globState(pattern string) string {
	files, err := globFiles(pattern)
	if err != nil {
		return "error:" + err.Error()
	}
	return Digest(files)
}

// globFiles returns regular files matching pattern in lexical order.
func globFiles(pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	files := []string{}
	for _, match := range matches {
		if info, err := os.Stat(match); err == nil && info.Mode().IsRegular() {
			files = append(files, match)
		}
	}
	// Glob sorts each directory's names, not whole paths: agents/x sorts before agents-windows/x
	// there, after it here. Loading sorts whole paths, and a different order would read as a change.
	slices.Sort(files)
	return files, nil
}

// TrackRevision includes externally loaded configuration in change detection.
func (i *Inputs) TrackRevision(loaded string, read func() string) {
	i.loadedRevision, i.revision = loaded, read
}
