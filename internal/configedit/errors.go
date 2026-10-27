// Package configedit makes configuration edits: it changes fragments through their YAML trees,
// validates the configuration they produce, and writes them. It knows nothing of the CLI, so
// anything else that edits configuration can share it and write identical YAML.
package configedit

import (
	"fmt"
)

// Code classifies an edit error. The values are the codes of the CLI's JSON error documents.
type Code string

const (
	// ConfigInvalid: the edited configuration would not load.
	ConfigInvalid Code = "config_invalid"
	// ReadOnly: a file the edit needs cannot be written, or a package defines the target.
	ReadOnly Code = "read_only"
	// Conflict: the target already exists, is still referenced, or changed since a plan.
	Conflict Code = "conflict"
	// NotFound: the target does not exist.
	NotFound Code = "not_found"
	// InvalidRequest: the edit is malformed or names something it may not.
	InvalidRequest Code = "invalid_request"
	// ChooseFile: new definitions or rules could go to several fragments, listed in Files, and
	// the edit names none.
	ChooseFile Code = "choose_file"
)

// Error is an edit error with its classification.
type Error struct {
	Code Code
	Err  error
	// Files lists the fragments to choose from, with ChooseFile.
	Files []string
}

func (e *Error) Error() string { return e.Err.Error() }

func (e *Error) Unwrap() error { return e.Err }

func errorf(code Code, format string, args ...any) error {
	return &Error{Code: code, Err: fmt.Errorf(format, args...)}
}

func coded(code Code, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Code: code, Err: err}
}
