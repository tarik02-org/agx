package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Inputs records the state of every file and glob a load consulted.
type Inputs struct{ entries []input }

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
	return files, nil
}
