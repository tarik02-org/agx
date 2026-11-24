package secretstore

import (
	"strings"
	"testing"
	"time"
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
