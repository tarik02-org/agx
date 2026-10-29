package cli

import (
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
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
