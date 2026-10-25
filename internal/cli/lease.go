package cli

import (
	"context"
	"errors"
	"sync"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

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
	// restarted daemon gets its launches attached promptly.
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

// lease is a wrapper's connection to the daemon serving its launch. It follows the discovery
// file to the daemon that replaced its own, of any build, and attaches the launch to a daemon
// that does not hold it, sending everything a daemon needs to serve it: the launch, its grant,
// environment inputs, native arguments, and thread launches.
type lease struct {
	paths     platform.Paths
	lifecycle string
	mu        sync.Mutex
	client    *client
	// launch is the wrapper's launch as AttachLaunch sends it, without threads; threads holds its
	// active thread launches.
	launch  *v1.AttachLaunchRequest
	threads map[string]*v1.AttachedThread
	// started is when the wrapper last started a daemon to replace a crashed one.
	started time.Time
}

// crashRestartInterval spaces the daemons a wrapper starts for a crashed one.
const crashRestartInterval = 10 * time.Second

// replaceCrashed starts a daemon when the one discovery names is gone without having stopped:
// a stopping daemon removes its discovery file last, so one that stays behind unanswered was
// killed. Nothing else would start a daemon for the harness, whose MCP requests go straight to
// the port. It reports whether a daemon answers afterwards.
func (l *lease) replaceCrashed(ctx context.Context) bool {
	if time.Since(l.started) < crashRestartInterval {
		return false
	}
	c, err := newAnyVersionClient(l.paths)
	if err != nil {
		return false
	}
	probe, cancel := context.WithTimeout(ctx, time.Second)
	_, err = c.runtime.GetRuntime(probe, connect.NewRequest(&v1.GetRuntimeRequest{}))
	cancel()
	if err == nil || connect.CodeOf(err) != connect.CodeUnavailable {
		return false
	}
	l.started = time.Now()
	ui.Diagnose(ui.Warn, "the agx daemon stopped answering without shutting down; starting a new one")
	if _, err := startDaemon(context.WithoutCancel(ctx), l.paths); err != nil {
		ui.Diagnose(ui.Warn, "cannot start a new agx daemon: %v", err)
		return false
	}
	return l.reconnect(l.current())
}

func newLease(paths platform.Paths, c *client, launch *v1.AttachLaunchRequest) *lease {
	return &lease{
		paths:     paths,
		lifecycle: launch.LifecycleToken,
		client:    c,
		launch:    launch,
		threads:   map[string]*v1.AttachedThread{},
	}
}

func (l *lease) current() *client {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.client
}

// addThread keeps a thread launch for attaching.
func (l *lease) addThread(launch *v1.Launch, grant string, inputs map[string]string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.threads[launch.Id] = &v1.AttachedThread{Launch: launch, Grant: grant, EnvironmentInputs: inputs}
}

// bindThread records the thread a thread launch serves.
func (l *lease) bindThread(id, thread string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if t := l.threads[id]; t != nil {
		t.Launch.SessionId = thread
	}
}

func (l *lease) forget(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.threads, id)
}

// attach hands the launch to the current daemon, which takes it when it does not hold it.
func (l *lease) attach(ctx context.Context) error {
	l.mu.Lock()
	msg := proto.Clone(l.launch).(*v1.AttachLaunchRequest)
	for _, t := range l.threads {
		msg.Threads = append(msg.Threads, proto.Clone(t).(*v1.AttachedThread))
	}
	l.mu.Unlock()
	_, err := l.current().launches.AttachLaunch(ctx, connect.NewRequest(msg))
	return err
}

// reconnect switches to the daemon the discovery file names when it is another instance than
// used's, and reports whether it did. A daemon of another build serves launches all the same.
func (l *lease) reconnect(used *client) bool {
	d, err := daemon.ReadDiscovery(l.paths)
	if err != nil && !errors.Is(err, daemon.ErrVersionMismatch) {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if d.InstanceID == l.client.discovery.InstanceID {
		// Another caller may have switched already.
		return l.client != used
	}
	l.client = dial(d)
	return true
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
			if l.reconnect(l.current()) {
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

// renew renews a launch, attaching it first to a daemon that does not hold it or let it expire.
func (l *lease) renew(ctx context.Context, msg *v1.RenewLaunchRequest) (*v1.RenewLaunchResponse, error) {
	var response *v1.RenewLaunchResponse
	renew := func(c *client) error {
		r, err := c.launches.RenewLaunch(ctx, withToken(msg, l.lifecycle))
		if err == nil {
			response = r.Msg
		}
		return err
	}
	err := l.call(renew)
	if connect.CodeOf(err) == connect.CodeNotFound || ended(err) == "expired" {
		if err = l.attach(ctx); err == nil {
			err = renew(l.current())
		}
	}
	return response, err
}

// ended is how a launch ended, when err refuses it for that.
func ended(err error) string {
	var e *connect.Error
	if errors.As(err, &e) {
		return e.Meta().Get(daemon.EndedHeader)
	}
	return ""
}

// sync renews the wrapper's own launch, so the daemon has it attached before a thread is
// prepared under it.
func (l *lease) sync(ctx context.Context) error {
	_, err := l.renew(ctx, &v1.RenewLaunchRequest{LaunchId: l.launch.Launch.Id})
	return err
}
