package configedit

import (
	"fmt"
)

// Code classifies failures for RPC status codes.
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
