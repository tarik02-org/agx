// Package resources describes portable content independently of its storage or delivery.
package resources

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// File names a file within a resource, using a relative slash-separated path.
type File struct {
	Path       string
	Content    []byte
	Executable bool
}

// Tree carries content rather than a path in the producer's filesystem.
type Tree struct {
	Directories []string
	Files       []File
}

// Digest identifies these exact bytes, paths and executable permissions.
func (t Tree) Digest() string {
	b, err := json.Marshal(t)
	if err != nil {
		panic(err)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type Skill struct {
	ID      string
	Name    string
	Digest  string
	Content Tree
}

type Set struct {
	Skills       []Skill
	Instructions string
}
