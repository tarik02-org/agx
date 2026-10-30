package auth

import "errors"

var ErrNotStored = errors.New("not stored")

type Store interface {
	Get(string) (string, error)
	Set(string, string) error
	Delete(string) error
	Reset() error
}
