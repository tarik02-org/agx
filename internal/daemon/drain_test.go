package daemon

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/tarik02-org/agx/internal/control"
)

func TestDrainWaitsForRequestsInFlight(t *testing.T) {
	d := newDrain()
	if !d.wait(t.Context()) {
		t.Fatal("an idle drain did not finish")
	}
	leave := d.enter()
	d.start()
	done := make(chan bool)
	go func() { done <- d.wait(t.Context()) }()
	select {
	case <-done:
		t.Fatal("wait returned with a request in flight")
	case <-time.After(50 * time.Millisecond):
	}
	leave()
	// Leaving twice must not count another request out.
	leave()
	if !<-done {
		t.Fatal("wait did not report the request finished")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if !d.wait(ctx) {
		t.Fatal("a second leave counted the drain below zero")
	}
}

func TestDrainGivesUpAfterGrace(t *testing.T) {
	d := newDrain()
	leave := d.enter()
	defer leave()
	d.start()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if d.wait(ctx) {
		t.Fatal("wait reported a request still in flight as finished")
	}
}

func TestDrainEndsEventStreams(t *testing.T) {
	d := newDrain()
	streamEnded := make(chan struct{})
	callStarted := make(chan struct{})
	finishCall := make(chan struct{})
	handler := d.track(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			<-r.Context().Done()
			close(streamEnded)
			return
		}
		close(callStarted)
		<-finishCall
		select {
		case <-r.Context().Done():
			t.Error("draining cancelled a call in flight")
		default:
		}
	}))
	go handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/launch/l/mcp/m", nil),
	)
	go handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/launch/l/mcp/m", nil),
	)
	<-callStarted
	d.start()
	select {
	case <-streamEnded:
	case <-time.After(5 * time.Second):
		t.Fatal("draining did not end the event stream")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if d.wait(ctx) {
		t.Fatal("drain finished while a call was in flight")
	}
	close(finishCall)
	if !d.wait(t.Context()) {
		t.Fatal("drain did not finish after the call")
	}
}

func TestStoppingRefusals(t *testing.T) {
	var connectErr *connect.Error
	if err := refusal(); connect.CodeOf(err) != connect.CodeUnavailable ||
		!errors.As(err, &connectErr) || connectErr.Meta().Get(control.StoppingHeader) == "" {
		t.Fatalf("refusal = %v", err)
	}
	w := httptest.NewRecorder()
	refuse(w)
	if w.Code != http.StatusServiceUnavailable || w.Header().Get(control.StoppingHeader) == "" ||
		w.Header().Get("Retry-After") == "" {
		t.Fatalf("refuse answered %d with %v", w.Code, w.Header())
	}
}
