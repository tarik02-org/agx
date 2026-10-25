package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

func TestKeychainCallTimesOut(t *testing.T) {
	previous := keychainTimeout
	keychainTimeout = 10 * time.Millisecond
	t.Cleanup(func() { keychainTimeout = previous })
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	_, err := keychainCall(func() (string, error) {
		<-release
		return "", nil
	})
	if err == nil || !strings.Contains(err.Error(), "did not answer within 10ms") ||
		!strings.Contains(err.Error(), "locked or waiting for an access prompt") {
		t.Fatalf("blocked keychain call = %v", err)
	}
}

func TestChunkedKeychainItems(t *testing.T) {
	keyring.MockInit()
	long := strings.Repeat("x", 5000)
	for _, value := range []string{long, "short", long + "y", "agx-chunks:3"} {
		if err := keychainSet("item", value); err != nil {
			t.Fatal(err)
		}
		got, err := keychainGet("item")
		if err != nil || got != value {
			t.Fatalf("round trip of %d bytes: %d bytes, %v", len(value), len(got), err)
		}
	}
	if err := keychainDelete("item"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"item", "item#1", "item#2", "item#3"} {
		if _, err := keyring.Get("agx", key); !errors.Is(err, keyring.ErrNotFound) {
			t.Errorf("%s left after delete: %v", key, err)
		}
	}
}
