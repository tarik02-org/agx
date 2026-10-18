package cli

import (
	"context"
	"errors"
	"sync"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/daemon"
	"github.com/tarik02-org/agx/internal/platform"
)

const (
	// restartWait bounds retrying new work while a daemon restarts: it drains for its grace
	// period, exits, and the next one starts.
	restartWait = 30 * time.Second
	// discoveryPoll is how often a wrapper looks for a daemon that replaced its own, so a
	// restarted daemon gets its launches restored promptly.
	discoveryPoll = 250 * time.Millisecond
)

// stopping reports whether err comes from a daemon that stopped taking new work, so the request
// is worth retrying with the next one.
func stopping(err error) bool {
	var e *connect.Error
	return errors.As(err, &e) && e.Meta().Get(daemon.StoppingHeader) != ""
}

// backoff is the pause before retry attempt n (from 0) with a daemon that is restarting.
func backoff(n int) time.Duration {
	return min(200*time.Millisecond<<min(n, 4), 2*time.Second)
}

// pause waits d, or less if ctx ends first, and returns ctx's error then.
func pause(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// lease is a wrapper's connection to the daemon serving its launch. When that daemon stops and
// another one restores the launch, possibly of another build, lease follows the discovery file to
// it and resends what the new daemon did not keep: environment inputs and native arguments.
type lease struct {
	paths     platform.Paths
	launch    string
	lifecycle string
	mu        sync.Mutex
	client    *client
	// restore holds what RestoreLaunch resends, per launch: the wrapper's own and its threads'.
	restore map[string]*v1.RestoreLaunchRequest
}

func newLease(paths platform.Paths, c *client, launch, lifecycle string) *lease {
	return &lease{
		paths:     paths,
		launch:    launch,
		lifecycle: lifecycle,
		client:    c,
		restore:   map[string]*v1.RestoreLaunchRequest{},
	}
}

func (l *lease) current() *client {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.client
}

// remember keeps what launch id was prepared with, for a daemon that restores it.
func (l *lease) remember(id string, inputs map[string]string, native []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.restore[id] = &v1.RestoreLaunchRequest{
		LaunchId:          id,
		LifecycleToken:    l.lifecycle,
		EnvironmentInputs: inputs,
		NativeArguments:   native,
	}
}

func (l *lease) forget(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.restore, id)
}

// reconnect switches to the daemon the discovery file names when it is another instance than
// used's, and reports whether it did. A daemon of another build serves launches all the same.
func (l *lease) reconnect(used *client) bool {
	next, err := newAnyVersionClient(l.paths)
	if err != nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if next.discovery.InstanceID == l.client.discovery.InstanceID {
		// Another caller may have switched already.
		return l.client != used
	}
	l.client = next
	return true
}

// replaced reports whether discovery names another daemon instance than the one in use, which
// then waits to have this wrapper's launches restored.
func (l *lease) replaced() bool {
	d, err := daemon.ReadDiscovery(l.paths)
	if err != nil && !errors.Is(err, daemon.ErrVersionMismatch) {
		return false
	}
	return d.InstanceID != l.current().discovery.InstanceID
}

// await waits d, and returns early once another daemon replaced the one in use, switching to it.
// It reports false when done closes first.
func (l *lease) await(done <-chan struct{}, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	poll := time.NewTicker(discoveryPoll)
	defer poll.Stop()
	for {
		select {
		case <-done:
			return false
		case <-timer.C:
			return true
		case <-poll.C:
			// The next daemon may listen on the same port, where the old client would reach it
			// with the old management token.
			if l.replaced() && l.reconnect(l.current()) {
				return true
			}
		}
	}
}

// call runs fn against the current daemon and, when it fails, once more against a daemon that
// replaced it.
func (l *lease) call(fn func(*client) error) error {
	used := l.current()
	err := fn(used)
	if err != nil && l.reconnect(used) {
		err = fn(l.current())
	}
	return err
}

// renew renews the launch, then resends what a restarted daemon asks for.
func (l *lease) renew(ctx context.Context, msg *v1.RenewLaunchRequest) (*v1.RenewLaunchResponse, error) {
	var response *v1.RenewLaunchResponse
	err := l.call(func(c *client) error {
		r, err := c.launches.RenewLaunch(ctx, request(msg, l.lifecycle))
		if err == nil {
			response = r.Msg
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	for _, id := range response.RestoreLaunchIds {
		l.mu.Lock()
		restore := l.restore[id]
		l.mu.Unlock()
		if restore == nil {
			continue
		}
		err := l.call(func(c *client) error {
			_, err := c.launches.RestoreLaunch(ctx, request(restore, c.discovery.Token))
			return err
		})
		// The daemon asks again with the next renewal.
		if err != nil && connect.CodeOf(err) != connect.CodeUnavailable {
			ui.Diagnose(ui.Warn, "cannot restore launch %s with the restarted daemon: %v", id, err)
		}
	}
	return response, nil
}

// sync renews the wrapper's own launch, so a restarted daemon has it restored before a thread
// is prepared under it.
func (l *lease) sync(ctx context.Context) error {
	_, err := l.renew(ctx, &v1.RenewLaunchRequest{LaunchId: l.launch})
	return err
}
