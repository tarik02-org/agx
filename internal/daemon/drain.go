package daemon

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"

	"github.com/tarik02-org/agx/gen/agx/v1/agxv1connect"
	"github.com/tarik02-org/agx/internal/control"
)

const (
	// forcedGrace bounds draining for a stop that revokes launches anyway.
	forcedGrace = 2 * time.Second
)

var errStopping = errors.New("daemon stopping; retry with the next daemon")

// drain counts the requests in flight so a stopping daemon can let them finish. Long-lived
// streams are not counted; they end when draining starts.
type drain struct {
	mu     sync.Mutex
	active int
	// idle is closed when active drops to zero while wait waits.
	idle chan struct{}
	// stopping ends when draining starts.
	stopping context.Context
	// start begins draining: new work is refused from then on, and streams end.
	start context.CancelFunc
}

func newDrain() *drain {
	d := &drain{}
	d.stopping, d.start = context.WithCancel(context.Background())
	return d
}

// enter counts a request in flight until the returned function runs.
func (d *drain) enter() func() {
	d.mu.Lock()
	d.active++
	d.mu.Unlock()
	return sync.OnceFunc(func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		d.active--
		if d.active == 0 && d.idle != nil {
			close(d.idle)
			d.idle = nil
		}
	})
}

// draining reports whether the daemon stopped taking new work.
func (d *drain) draining() bool { return d.stopping.Err() != nil }

// wait waits until no counted request is in flight, or ctx ends, and reports whether they all
// finished. Draining must have started, so that no new work arrives.
func (d *drain) wait(ctx context.Context) bool {
	for {
		d.mu.Lock()
		if d.active == 0 {
			d.mu.Unlock()
			return true
		}
		if d.idle == nil {
			d.idle = make(chan struct{})
		}
		idle := d.idle
		d.mu.Unlock()
		select {
		case <-idle:
		case <-ctx.Done():
			return false
		}
	}
}

// streams are the Connect procedures that stay open until their caller or the daemon ends them.
var streams = map[string]bool{
	agxv1connect.RuntimeServiceWatchRuntimeProcedure: true,
	agxv1connect.SecretServiceLoginProcedure:         true,
}

// track counts the requests next serves, except long-lived streams: an MCP event stream (GET)
// ends cleanly when draining starts, and Connect streams watch stopping themselves.
func (d *drain) track(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/launch/"):
			ctx, cancel := context.WithCancel(r.Context())
			defer cancel()
			stop := context.AfterFunc(d.stopping, cancel)
			defer stop()
			r = r.WithContext(ctx)
		case !streams[r.URL.Path]:
			defer d.enter()()
		}
		next.ServeHTTP(w, r)
	})
}

// refusal is the error for new work while the daemon stops.
func refusal() error {
	err := rpcError(connect.CodeUnavailable, errStopping.Error())
	err.Meta().Set(control.StoppingHeader, "1")
	return err
}

// refuse answers an HTTP request for new work while the daemon stops.
func refuse(w http.ResponseWriter) {
	w.Header().Set(control.StoppingHeader, "1")
	w.Header().Set("Retry-After", "1")
	http.Error(w, errStopping.Error(), http.StatusServiceUnavailable)
}
