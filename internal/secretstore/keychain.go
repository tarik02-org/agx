package secretstore

import (
	"errors"
	"fmt"
	"time"

	"github.com/zalando/go-keyring"
)

// keychainTimeout bounds an OS keychain call, which blocks while the keychain is locked or shows
// an access prompt, so callers get an explanation before their own deadline instead of a timeout.
var keychainTimeout = 10 * time.Second

// keychainAdvice tells how to proceed after an OS keychain failure.
const keychainAdvice = "unlock the OS keychain (or start its secret service) and retry, " +
	"or use a secret of kind command or sops instead"

func keychainGet(k string) (string, error) {
	return keychainCall(func() (string, error) { return keyring.Get("agx", k) })
}

func keychainSet(k, value string) error {
	_, err := keychainCall(func() (struct{}, error) { return struct{}{}, keyring.Set("agx", k, value) })
	return err
}

// keychainCall runs op with keychainTimeout. A call that times out keeps running in the
// background, as keychain APIs cannot be cancelled; its result is discarded.
func keychainCall[T any](op func() (T, error)) (T, error) {
	type result struct {
		value T
		err   error
	}
	done := make(chan result, 1)
	go func() {
		value, err := op()
		done <- result{value, err}
	}()
	timer := time.NewTimer(keychainTimeout)
	defer timer.Stop()
	select {
	case r := <-done:
		if r.err != nil && !errors.Is(r.err, keyring.ErrNotFound) {
			return r.value, fmt.Errorf("cannot use the OS keychain: %w; %s", r.err, keychainAdvice)
		}
		return r.value, r.err
	case <-timer.C:
		var zero T
		return zero, fmt.Errorf(
			"the OS keychain did not answer within %s; it may be locked or waiting for an access prompt: %s",
			keychainTimeout, keychainAdvice,
		)
	}
}
