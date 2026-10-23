// Package configedit makes configuration edits: it changes fragments through their YAML trees,
// validates the configuration they produce, and writes them. It knows nothing of the CLI, so
// anything else that edits configuration can share it and write identical YAML.
package configedit

import (
	"errors"
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
)

// Error is an edit error with its classification.
type Error struct {
	Code Code
	Err  error
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

// CodeOf returns the classification of err, if it carries one.
func CodeOf(err error) (Code, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.Code, true
	}
	return "", false
}
