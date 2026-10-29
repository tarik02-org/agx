package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/tarik02-org/agx/internal/platform"
)

func TestRefusalsExhaustAfterWait(t *testing.T) {
	refused := connect.NewError(connect.CodeFailedPrecondition, errors.New("directory cannot be resolved"))
	unavailable := connect.NewError(connect.CodeUnavailable, errors.New("no daemon"))
	start := time.Now()
	var r refusals
	if r.exhausted(refused, start) || r.exhausted(refused, start.Add(attachRefusedWait-time.Second)) {
		t.Fatal("gave up before the wait")
	}
	if !r.exhausted(refused, start.Add(attachRefusedWait)) {
		t.Fatal("kept retrying past the wait")
	}
	// Another failure in between restarts the wait: the daemon was gone, not refusing.
	r = refusals{}
	r.exhausted(refused, start)
	r.exhausted(unavailable, start.Add(time.Minute))
	if r.exhausted(refused, start.Add(attachRefusedWait)) {
		t.Fatal("an interrupted refusal counted from its first attempt")
	}
}

// Polling reads the discovery file only when its size or modification time changed.
func TestDiscoveryChanged(t *testing.T) {
	l := &lease{paths: platform.Paths{Runtime: t.TempDir()}}
	path := filepath.Join(l.paths.Runtime, "daemon.json")
	write := func(content string, modified time.Time) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	if l.discoveryChanged() {
		t.Fatal("a missing file changed")
	}
	start := time.Now().Truncate(time.Second)
	write(`{"instance_id":"a"}`, start)
	if !l.discoveryChanged() || l.discoveryChanged() {
		t.Fatal("a written file is not one change")
	}
	write(`{"instance_id":"b"}`, start.Add(time.Second))
	if !l.discoveryChanged() {
		t.Fatal("a rewritten file did not change")
	}
	write(`{"instance_id":"bc"}`, start.Add(time.Second))
	if !l.discoveryChanged() {
		t.Fatal("a file of another size did not change")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if !l.discoveryChanged() || l.discoveryChanged() {
		t.Fatal("a removed file is not one change")
	}
}
