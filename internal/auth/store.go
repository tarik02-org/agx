package auth

import "errors"

var ErrNotStored = errors.New("not stored")

// Locker is a Store that other processes use too, which Lock keeps off one key's credential
// until the returned function runs.
type Locker interface {
	Lock(key string) (unlock func(), err error)
}

type Store interface {
	Get(string) (string, error)
	Set(string, string) error
	Delete(string) error
	Reset() error
}
