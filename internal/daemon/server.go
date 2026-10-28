package daemon

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/gofrs/flock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/gen/agx/v1/agxv1connect"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/mcpruntime"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/securefs"
)

// Version is set at build time with -ldflags "-X agx/internal/daemon.Version=..."; otherwise it
// is the main module's version, which go install module@version records.
var Version = "dev"

func init() {
	if Version != "dev" {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		Version = strings.TrimPrefix(info.Main.Version, "v")
	}
}

// ErrVersionMismatch is returned by ReadDiscovery together with the decoded
// discovery when the running daemon has a different version.
var ErrVersionMismatch = errors.New("daemon version mismatch")

// ErrUnavailable marks ReadDiscovery errors that leave no daemon to reach: it is not running,
// or its discovery file is invalid.
var ErrUnavailable = errors.New("daemon unavailable")

// unavailable keeps a discovery error's own message while matching ErrUnavailable.
type unavailable string

func (u unavailable) Error() string { return string(u) }

func (u unavailable) Is(target error) bool { return target == ErrUnavailable }

// historyRetention keeps ended launches around for status inspection.
const historyRetention = time.Hour

type Discovery struct {
	Version    string `json:"version"`
	Build      string `json:"build"`
	InstanceID string `json:"instance_id"`
	URL        string `json:"url"`
	Token      string `json:"token"`
	Config     string `json:"config"`
	// PID is the daemon's process, for diagnosing one that stopped answering.
	PID int `json:"pid,omitempty"`
}

// verifier hashes a launch credential, so the daemon never holds one it could leak.
func verifier(value string) [32]byte { return sha256.Sum256([]byte(value)) }

func matches(hash [32]byte, value string) bool { return hash == verifier(value) }

type launch struct {
	// view.ExpiresAt shows expires, which alone decides expiry: it holds a monotonic clock
	// reading, so that a wall clock jump expires no lease early or late.
	view      *v1.Launch
	expires   time.Time
	grant     [32]byte
	lifecycle [32]byte
	ctx       context.Context
	cancel    context.CancelFunc
	endpoints map[config.ID]*mcpruntime.Endpoint
	snapshot  *config.Snapshot
	// target is the harness process a launch started; its thread launches share it.
	target  adapters.Target
	ended   time.Time
	created time.Time
	// lease is how long a renewal extends the launch.
	lease time.Duration
	// threadServer marks a harness process serving many threads; its launch has no endpoints.
	threadServer bool
	// unavailable says why an attached launch's MCP servers cannot be served, when its directory
	// no longer resolves to the snapshot it started with.
	unavailable string
}

type Server struct {
	mu        sync.Mutex
	reloadMu  sync.Mutex
	paths     platform.Paths
	adapters  adapters.Registry
	builtins  config.Builtins
	config    *config.Config
	configErr error
	inputs    *config.Inputs
	discovery Discovery
	manager   *auth.Manager
	launches  map[string]*launch
	ctx       context.Context
	cancel    context.CancelFunc
	stop      chan struct{}
	// forced is set by a stop that revokes active launches instead of keeping them.
	forced bool
	// revoked maps launches revoked within revokedRetention, by this daemon or one before it, to
	// when; their wrappers cannot attach them again.
	revoked map[string]time.Time
	// attachMu serializes AttachLaunch.
	attachMu sync.Mutex
	// grace, when set by a stop, replaces ShutdownGrace.
	grace time.Duration
	drain *drain
	// stopped is set once a stopping daemon collected its launches; new launches are refused
	// from then on, for their callers to retry with the next daemon.
	stopped bool
	// installs counts package installations running off their requests; none starts once
	// installsClosed is set, so that waiting for them is final.
	installs       sync.WaitGroup
	installsClosed bool
	port           int
	// changed wakes runtime watchers when launches or their MCP servers change.
	changed *notifier
	// clients pools the daemon's own MCP client sessions for ListTools and CallTool.
	clients *clientPool
	// shared runs the stdio MCP servers launches share.
	shared *mcpruntime.Shared
	// editMu serializes planning and applying configuration changes, so an apply checks and
	// writes what no other apply is writing.
	editMu sync.Mutex
	// previews holds the Git sources PreviewSource fetched.
	previews previews
	// icons remembers the icons MCP servers declared, for the web UI.
	icons *mcpIcons
}

// Serve runs the daemon until stop closes or a stop request arrives, then shuts down gracefully:
// it takes no new launches or MCP sessions, lets requests in flight finish for the grace period,
// and lets its MCP servers exit. When ctx ends, it shuts down at once. Unless the stop forces,
// wrappers attach their launches to the next daemon. builtins must describe the adapters in
// registry.
func Serve(
	ctx context.Context,
	stop <-chan struct{},
	paths platform.Paths,
	registry adapters.Registry,
	builtins config.Builtins,
) error {
	if err := securefs.EnsureDir(paths.Runtime); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(paths.Runtime, "daemon.lock"))
	ok, err := lock.TryLock()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("daemon is already running")
	}
	defer func() { _ = lock.Unlock() }()
	discoveryPath := filepath.Join(paths.Runtime, "daemon.json")
	// Discovery left behind names the port of a daemon that was killed, which its launches'
	// harnesses use; the state file names the port of one that stopped.
	port := 0
	if previous, err := ReadDiscovery(paths); err == nil || errors.Is(err, ErrVersionMismatch) {
		if u, err := url.Parse(previous.URL); err == nil {
			port, _ = strconv.Atoi(u.Port())
		}
		slog.Warn("the previous daemon stopped without shutting down", "pid", previous.PID, "url", previous.URL)
	}
	// The startup lock makes stale discovery replacement safe.
	if err := os.Remove(discoveryPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	inputs := &config.Inputs{}
	c, err := config.Load(paths.Config, config.Options{Builtins: builtins, Cache: paths.Cache, Inputs: inputs})
	if err != nil {
		return err
	}
	// Holding the startup lock, no other daemon writes these directories.
	removeAbandonedTemporaries(paths.State, time.Now())
	state, err := loadState(StatePath(paths), time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "agx daemon: %v\n", err)
	}
	if port == 0 {
		port = state.Port
	}
	listener, err := listen(ctx, port)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	// Launches and their MCP servers outlive ctx until shutdown ends them.
	serverCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	discovery := Discovery{
		Version:    Version,
		Build:      Build(),
		InstanceID: securefs.Token(),
		URL:        "http://" + listener.Addr().String(),
		Token:      securefs.Token(),
		Config:     paths.Config,
		PID:        os.Getpid(),
	}
	s := &Server{
		paths:     paths,
		adapters:  registry,
		builtins:  builtins,
		config:    c,
		inputs:    inputs,
		discovery: discovery,
		ctx:       serverCtx,
		cancel:    cancel,
		stop:      make(chan struct{}),
		drain:     newDrain(),
		launches:  map[string]*launch{},
		revoked:   state.Revoked,
		port:      listener.Addr().(*net.TCPAddr).Port,
		clients:   newClientPool(),
		icons:     newMCPIcons(filepath.Join(paths.Cache, "mcp-icons.json")),
		changed:   &notifier{},
	}
	s.shared = mcpruntime.NewShared(serverCtx, Version, s.changed.notify)
	if port != 0 && port != s.port {
		fmt.Fprintf(
			os.Stderr,
			"agx daemon: port %d is taken; harnesses holding endpoint URLs of port %d lose MCP access "+
				"until they get new ones\n",
			port,
			port,
		)
	}
	s.mu.Lock()
	s.saveState()
	s.mu.Unlock()
	// Launch files of launches whose wrappers did not attach them within a lease belong to
	// harnesses that ended while no daemon ran.
	started := time.Now()
	sweep := time.AfterFunc(time.Duration(c.Daemon.LeaseSeconds)*time.Second, func() { s.sweep(started) })
	defer sweep.Stop()
	slog.Info("daemon started", "version", Version, "build", Build(), "pid", os.Getpid(), "url", discovery.URL)
	s.manager = auth.New(discovery.URL+"/oauth/callback", auth.OpenStore(paths.State))
	mux := http.NewServeMux()
	interceptor := connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Lifecycle procedures check the lifecycle token, and tool procedures accept a launch
			// grant instead of the management token; they authorize themselves.
			if !selfAuthorized[req.Spec().Procedure] {
				if !equalBearer(req.Header().Get("Authorization"), discovery.Token) {
					return nil, rpcError(connect.CodeUnauthenticated, "management credential required")
				}
			}
			if s.drain.draining() && startsWork[req.Spec().Procedure] {
				return nil, refusal()
			}
			return next(ctx, req)
		}
	})
	opts := []connect.HandlerOption{connect.WithInterceptors(interceptor), connect.WithReadMaxBytes(16 * 1024 * 1024)}
	path, handler := agxv1connect.NewConfigurationServiceHandler(s, opts...)
	mux.Handle(path, handler)
	path, handler = agxv1connect.NewPackageServiceHandler(s, opts...)
	mux.Handle(path, handler)
	path, handler = agxv1connect.NewLaunchServiceHandler(s, opts...)
	mux.Handle(path, handler)
	path, handler = agxv1connect.NewRuntimeServiceHandler(s, opts...)
	mux.Handle(path, handler)
	path, handler = agxv1connect.NewSecretServiceHandler(s, opts...)
	mux.Handle(path, handler)
	mux.HandleFunc("/launch/", s.serveMCP)
	mux.HandleFunc("/oauth/callback", s.manager.Callback)
	tracked := s.drain.track(mux)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != listener.Addr().String() {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != discovery.URL {
			http.Error(w, "invalid origin", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		tracked.ServeHTTP(w, r)
	}), ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 64 * 1024}
	// Discovery goes first: the listener queues connections meanwhile, and a wrapper whose old
	// client reaches this daemon on a reused port must find the token it needs then.
	if err := securefs.WriteJSON(discoveryPath, discovery); err != nil {
		return err
	}
	defer func() { _ = os.Remove(discoveryPath) }()
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(listener) }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var serveErr error
	graceful := false
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-stop:
			graceful = true
			break loop
		case <-s.stop:
			graceful = true
			break loop
		case serveErr = <-errCh:
			break loop
		case <-ticker.C:
			s.expire()
			s.clients.expire(time.Now())
		}
	}
	s.drain.start()
	if graceful {
		s.settle(ctx)
	}
	launches := s.stopLaunches()
	s.clients.closeAll()
	// Shutdown closes the listener at once; requests that outlasted the grace period end as
	// their MCP sessions close.
	shutdownCtx, stopShutdown := context.WithCancel(ctx)
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		_ = server.Shutdown(shutdownCtx)
	}()
	s.closeLaunches(launches)
	stopKilling := context.AfterFunc(ctx, s.shared.Kill)
	s.shared.Close()
	stopKilling()
	select {
	case <-shutdownDone:
	case <-time.After(time.Second):
	}
	stopShutdown()
	<-shutdownDone
	_ = server.Close()
	s.cancel()
	// Cancelled installations still remove what they started; they must not be cut off by exit.
	installCtx, stopInstalls := context.WithTimeout(context.Background(), forcedGrace)
	if !s.awaitInstalls(installCtx) {
		fmt.Fprintln(os.Stderr, "agx daemon: package installation did not stop in time")
	}
	stopInstalls()
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return nil
}

// selfAuthorized are the procedures that check their own credentials.
var selfAuthorized = map[string]bool{
	agxv1connect.LaunchServiceRenewLaunchProcedure:  true,
	agxv1connect.LaunchServiceFinishLaunchProcedure: true,
	agxv1connect.RuntimeServiceListToolsProcedure:   true,
	agxv1connect.RuntimeServiceCallToolProcedure:    true,
}

// startsWork are the procedures a stopping daemon refuses, so that callers retry them with the
// next daemon.
var startsWork = map[string]bool{
	agxv1connect.LaunchServicePrepareLaunchProcedure:       true,
	agxv1connect.LaunchServiceAttachLaunchProcedure:        true,
	agxv1connect.LaunchServicePrepareThreadLaunchProcedure: true,
	agxv1connect.PackageServiceInstallProcedure:            true,
	agxv1connect.PackageServicePreviewSourceProcedure:      true,
	agxv1connect.ConfigurationServicePlanChangesProcedure:  true,
	agxv1connect.ConfigurationServiceApplyChangesProcedure: true,
	agxv1connect.RuntimeServiceTestMcpServerProcedure:      true,
	agxv1connect.RuntimeServiceListToolsProcedure:          true,
	agxv1connect.RuntimeServiceCallToolProcedure:           true,
}

// settle waits for the grace period until requests in flight finished, or ctx ends.
func (s *Server) settle(ctx context.Context) {
	s.mu.Lock()
	grace := ShutdownGrace
	if s.grace > 0 {
		grace = s.grace
	}
	if s.forced {
		grace = min(grace, forcedGrace)
	}
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, grace)
	defer cancel()
	if !s.drain.wait(ctx) {
		fmt.Fprintf(os.Stderr, "agx daemon: requests still in flight after %s end now\n", grace)
	}
	if !s.awaitInstalls(ctx) {
		fmt.Fprintf(os.Stderr, "agx daemon: package installations still running after %s are cancelled\n", grace)
	}
}

// awaitInstalls refuses new package installations and waits until those running finished, or
// ctx ends; it reports whether they all finished.
func (s *Server) awaitInstalls(ctx context.Context) bool {
	s.mu.Lock()
	s.installsClosed = true
	s.mu.Unlock()
	done := make(chan struct{})
	go func() {
		s.installs.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

// reload loads configuration on demand, skipping the load while every file it read is unchanged.
func (s *Server) reload() (*config.Config, error) {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
	s.mu.Lock()
	current, currentErr, inputs := s.config, s.configErr, s.inputs
	s.mu.Unlock()
	if inputs != nil && !inputs.Changed() {
		if currentErr != nil {
			return nil, currentErr
		}
		return current, nil
	}
	inputs = &config.Inputs{}
	c, err := config.Load(s.paths.Config, config.Options{Builtins: s.builtins, Cache: s.paths.Cache, Inputs: inputs})
	s.mu.Lock()
	defer s.mu.Unlock()
	if (err == nil) != (s.configErr == nil) {
		s.changed.notify()
	}
	s.configErr = err
	s.inputs = inputs
	if err == nil {
		s.config = c
	}
	return c, err
}

func active(state v1.Launch_State) bool {
	return state == v1.Launch_STATE_PREPARED || state == v1.Launch_STATE_RUNNING
}

// extend sets when the launch's lease runs out; s.mu must be held.
func (l *launch) extend(expires time.Time) {
	l.expires = expires
	l.view.ExpiresAt = timestamppb.New(expires)
}

// end must be called with s.mu held; endpoint and cache cleanup run outside it. A launch's
// thread launches end with it. A revocation is saved by the caller.
func (s *Server) end(l *launch, state v1.Launch_State) {
	if !active(l.view.State) {
		return
	}
	l.view.State = state
	l.ended = time.Now()
	l.cancel()
	s.changed.notify()
	if state == v1.Launch_STATE_REVOKED {
		if s.revoked == nil {
			s.revoked = map[string]time.Time{}
		}
		s.revoked[l.view.Id] = l.ended
	}
	// A finished launch is the normal outcome; the others cut a running harness off.
	if state != v1.Launch_STATE_FINISHED && l.view.ParentLaunchId == "" {
		slog.Info("launch ended", "launch", shortID(l.view.Id), "harness", l.view.Harness, "state", savedEnd(state))
	}
	s.clients.closeLaunch(l.view.Id)
	go s.release(l.view.Id, l.endpoints)
	for _, child := range s.children(l) {
		s.end(child, state)
	}
}

// children lists a launch's thread launches; s.mu must be held.
func (s *Server) children(l *launch) []*launch {
	children := []*launch{}
	for _, child := range s.launches {
		if child.view.ParentLaunchId == l.view.Id {
			children = append(children, child)
		}
	}
	return children
}

func (s *Server) release(id string, endpoints map[config.ID]*mcpruntime.Endpoint) {
	closeEndpoints(endpoints)
	_ = os.RemoveAll(filepath.Join(s.paths.Cache, "launches", id))
}

func closeEndpoints(endpoints map[config.ID]*mcpruntime.Endpoint) {
	var wg sync.WaitGroup
	for _, e := range endpoints {
		wg.Go(e.Close)
	}
	wg.Wait()
}

// Stop makes the daemon stop; without force, wrappers attach its active launches to the next
// daemon.
func (s *Server) Stop(
	ctx context.Context,
	req *connect.Request[v1.StopRequest],
) (*connect.Response[v1.StopResponse], error) {
	grace := req.Msg.Grace.AsDuration()
	if req.Msg.Grace != nil && grace <= 0 {
		return nil, rpcError(connect.CodeInvalidArgument, "grace must be a positive duration")
	}
	s.expire()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.forced = s.forced || req.Msg.Force
	if grace > 0 {
		s.grace = grace
	}
	var kept uint32
	for _, l := range s.launches {
		if active(l.view.State) && l.view.ParentLaunchId == "" && !s.forced {
			kept++
		}
	}
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	return connect.NewResponse(&v1.StopResponse{KeptLaunches: kept}), nil
}

func (s *Server) expire() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, l := range s.launches {
		if active(l.view.State) && now.After(l.expires) {
			s.end(l, v1.Launch_STATE_EXPIRED)
		}
	}
	s.prune(now)
}

// stopLaunches refuses new launches from now on and returns the daemon's launches. A forced stop
// revokes them; otherwise their launch files stay for the next daemon.
func (s *Server) stopLaunches() []*launch {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	launches := []*launch{}
	for _, l := range s.launches {
		if s.forced {
			s.end(l, v1.Launch_STATE_REVOKED)
		}
		launches = append(launches, l)
	}
	if s.forced {
		s.saveState()
	}
	slog.Info("daemon stopping", "launches", len(launches), "forced", s.forced)
	return launches
}

// closeLaunches ends the MCP sessions of launches, letting their servers exit; for a forced
// stop, it also removes their launch files.
func (s *Server) closeLaunches(launches []*launch) {
	s.mu.Lock()
	forced := s.forced
	s.mu.Unlock()
	var cleanup sync.WaitGroup
	for _, l := range launches {
		s.mu.Lock()
		endpoints := l.endpoints
		s.mu.Unlock()
		// end releases asynchronously; wait here so shutdown does not leave cache dirs.
		if forced {
			cleanup.Go(func() { s.release(l.view.Id, endpoints) })
		} else {
			cleanup.Go(func() { closeEndpoints(endpoints) })
		}
	}
	cleanup.Wait()
}

// sweep removes the launch files, older than started, of launches the daemon does not hold.
func (s *Server) sweep(started time.Time) {
	root := filepath.Join(s.paths.Cache, "launches")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		s.mu.Lock()
		held := s.launches[entry.Name()] != nil
		s.mu.Unlock()
		if info, err := entry.Info(); err == nil && !held && info.ModTime().Before(started) {
			_ = os.RemoveAll(filepath.Join(root, entry.Name()))
		}
	}
}

// listen binds the loopback port of the previous daemon when there was one and it is free, so
// endpoint URLs given to running harnesses keep working, and any free port otherwise.
func listen(ctx context.Context, port int) (net.Listener, error) {
	config := &net.ListenConfig{}
	if port != 0 {
		listener, err := config.Listen(ctx, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			return listener, nil
		}
	}
	return config.Listen(ctx, "tcp4", "127.0.0.1:0")
}

// prune must be called with s.mu held.
func (s *Server) prune(now time.Time) {
	cutoff := now.Add(-historyRetention)
	for id, l := range s.launches {
		if !active(l.view.State) && l.ended.Before(cutoff) {
			delete(s.launches, id)
		}
	}
}

// notifier wakes those waiting for the next change.
type notifier struct {
	mu sync.Mutex
	ch chan struct{}
}

func (n *notifier) notify() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ch != nil {
		close(n.ch)
		n.ch = nil
	}
}

// next closes at the next change.
func (n *notifier) next() <-chan struct{} {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ch == nil {
		n.ch = make(chan struct{})
	}
	return n.ch
}

func equalBearer(header, token string) bool {
	return subtle.ConstantTimeCompare([]byte(header), []byte("Bearer "+token)) == 1
}

func diagnostic(code, summary string) *v1.Diagnostic {
	return &v1.Diagnostic{Severity: v1.Diagnostic_SEVERITY_ERROR, Code: code, Summary: summary}
}

func (s *Server) serveMCP(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/launch/"), "/mcp/", 2)
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	l := s.launches[parts[0]]
	_, revoked := s.revoked[parts[0]]
	grant, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	attaching := !revoked && (l == nil || l.view.State == v1.Launch_STATE_EXPIRED || time.Now().After(l.expires))
	valid := !revoked && l != nil && active(l.view.State) && bearer && matches(l.grant, grant)
	s.mu.Unlock()
	if attaching {
		// The launch's wrapper attaches it once it notices this daemon.
		w.Header().Set("Retry-After", "1")
		http.Error(w, "launch not attached yet; retry shortly", http.StatusServiceUnavailable)
		return
	}
	if !valid {
		http.Error(w, "invalid or ended launch grant", http.StatusUnauthorized)
		return
	}
	// A stopping daemon finishes the sessions it serves but opens none; event streams would
	// outlast it.
	if s.drain.draining() && (r.Header.Get("Mcp-Session-Id") == "" || r.Method == http.MethodGet) {
		refuse(w)
		return
	}
	s.mu.Lock()
	e, unavailable := l.endpoints[config.ID(parts[1])], l.unavailable
	s.mu.Unlock()
	if unavailable != "" {
		http.Error(w, unavailable, http.StatusForbidden)
		return
	}
	if e == nil {
		http.Error(w, "MCP server not selected for launch", http.StatusForbidden)
		return
	}
	e.ServeHTTP(w, r)
}

// runtime reports the last configuration load rather than reloading: commands probe the daemon
// with it before every launch, and a reload can take seconds where reading files is slow.
func (s *Server) runtime() *v1.RuntimeView {
	type server struct {
		launchID string
		id       config.ID
		endpoint *mcpruntime.Endpoint
	}
	s.mu.Lock()
	view := &v1.RuntimeView{DaemonInstanceId: s.discovery.InstanceID, Version: Version}
	if s.configErr != nil {
		view.Diagnostics = append(
			view.Diagnostics,
			diagnostic(
				"invalid_config",
				"Configuration reload failed; active launches retain their snapshots. Run agx config validate.",
			),
		)
	}
	servers := []server{}
	for _, l := range s.launches {
		view.Launches = append(view.Launches, proto.Clone(l.view).(*v1.Launch))
		if active(l.view.State) {
			for id, e := range l.endpoints {
				servers = append(servers, server{launchID: l.view.Id, id: id, endpoint: e})
			}
		}
	}
	s.mu.Unlock()
	for _, c := range servers {
		state := v1.McpRuntime_STATE_CONNECTING
		current, problem := c.endpoint.State()
		switch current {
		case "ready":
			state = v1.McpRuntime_STATE_READY
		case "needs_auth":
			state = v1.McpRuntime_STATE_NEEDS_AUTH
		case "failed":
			state = v1.McpRuntime_STATE_FAILED
		}
		runtime := &v1.McpRuntime{LaunchId: c.launchID, McpId: string(c.id), State: state}
		if problem != "" {
			runtime.Diagnostics = append(runtime.Diagnostics, diagnostic("mcp_"+current, problem))
		}
		view.McpServers = append(view.McpServers, runtime)
	}
	sort.Slice(view.Launches, func(i, j int) bool { return view.Launches[i].Id < view.Launches[j].Id })
	sort.Slice(view.McpServers, func(i, j int) bool {
		a, b := view.McpServers[i], view.McpServers[j]
		return a.LaunchId+"/"+a.McpId < b.LaunchId+"/"+b.McpId
	})
	return view
}

func (s *Server) Validate(
	ctx context.Context,
	req *connect.Request[v1.ValidateRequest],
) (*connect.Response[v1.ValidateResponse], error) {
	_, err := s.reload()
	out := &v1.ValidateResponse{}
	if err != nil {
		out.Diagnostics = append(
			out.Diagnostics,
			diagnostic(
				"invalid_config",
				"Configuration is invalid; run agx config validate for local source diagnostics.",
			),
		)
	}
	return connect.NewResponse(out), nil
}

func (s *Server) Resolve(
	ctx context.Context,
	req *connect.Request[v1.ResolveRequest],
) (*connect.Response[v1.ResolveResponse], error) {
	c, err := s.reload()
	if err != nil {
		return nil, configError(err)
	}
	if req.Msg.Directory == "" || !filepath.IsAbs(req.Msg.Directory) {
		return nil, rpcError(connect.CodeInvalidArgument, "directory must be absolute")
	}
	if req.Msg.Harness != "" {
		if _, err := c.Harness(req.Msg.Harness); err != nil {
			return nil, rpcError(connect.CodeInvalidArgument, err.Error())
		}
	}
	snapshot, err := c.Resolve(req.Msg.Directory, req.Msg.Harness)
	if err != nil {
		return nil, rpcError(
			connect.CodeFailedPrecondition,
			fmt.Sprintf("cannot resolve directory %s: %v", req.Msg.Directory, err),
		)
	}
	return connect.NewResponse(resolveMessage(snapshot)), nil
}

func (s *Server) GetRuntime(
	ctx context.Context,
	req *connect.Request[v1.GetRuntimeRequest],
) (*connect.Response[v1.GetRuntimeResponse], error) {
	return connect.NewResponse(&v1.GetRuntimeResponse{Runtime: s.runtime()}), nil
}

func (s *Server) WatchRuntime(
	ctx context.Context,
	req *connect.Request[v1.WatchRuntimeRequest],
	stream *connect.ServerStream[v1.WatchRuntimeResponse],
) error {
	if !equalBearer(req.Header().Get("Authorization"), s.discovery.Token) {
		return rpcError(connect.CodeUnauthenticated, "management credential required")
	}
	previous := ""
	for {
		changed := s.changed.next()
		view := s.runtime()
		hash := config.Digest(view)
		if hash != previous {
			if err := stream.Send(&v1.WatchRuntimeResponse{Runtime: view}); err != nil {
				return err
			}
			previous = hash
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.drain.stopping.Done():
			return refusal()
		case <-changed:
		}
	}
}

// warnMissing logs the missing packages a launch goes without, once per package.
func warnMissing(snapshot *config.Snapshot) {
	for _, warning := range snapshot.Config.MissingWarnings(snapshot.Missing) {
		fmt.Fprintf(os.Stderr, "agx daemon: warning: %s\n", warning)
	}
}

func resolveMessage(s *config.Snapshot) *v1.ResolveResponse {
	out := &v1.ResolveResponse{
		Snapshot: &v1.Snapshot{
			Id:                  s.ID,
			Directory:           s.Directory,
			Harness:             s.Harness,
			RequiredEnvironment: s.RequiredEnvironment,
		},
	}
	for _, warning := range s.Config.MissingWarnings(s.Missing) {
		out.Diagnostics = append(out.Diagnostics, &v1.Diagnostic{
			Severity: v1.Diagnostic_SEVERITY_WARNING,
			Code:     "package_not_installed",
			Summary:  warning,
		})
	}
	resource := func(r config.Resource) *v1.ResourceRef {
		out := &v1.ResourceRef{}
		switch r.Kind {
		case "skill":
			out.Resource = &v1.ResourceRef_SkillId{SkillId: string(r.ID)}
		case "instructions":
			out.Resource = &v1.ResourceRef_InstructionsId{InstructionsId: string(r.ID)}
		case "mcp":
			out.Resource = &v1.ResourceRef_McpId{McpId: string(r.ID)}
		}
		return out
	}
	for _, r := range s.Resources {
		out.Snapshot.Resources = append(out.Snapshot.Resources, resource(r))
	}
	for _, t := range s.Traces {
		trace := &v1.ResourceTrace{Resource: resource(t.Resource)}
		for _, step := range t.Steps {
			change := v1.ResolutionStep_CHANGE_ADD
			if step.Change == "remove" {
				change = v1.ResolutionStep_CHANGE_REMOVE
			}
			chain := []string{}
			for _, id := range step.BundleChain {
				chain = append(chain, string(id))
			}
			trace.Steps = append(
				trace.Steps,
				&v1.ResolutionStep{
					Change: change,
					Source: &v1.SourceLocation{
						Path:   step.Source.Path,
						Line:   uint32(step.Source.Line),
						Column: uint32(step.Source.Column),
					},
					ScopeDirectory: step.Directory,
					Harness:        step.Harness,
					BundleChain:    chain,
				},
			)
		}
		out.Traces = append(out.Traces, trace)
	}
	out.Settings = harnessSettings(s)
	return out
}

// ReadDiscovery never returns credentials to inspection commands. On a version
// mismatch it returns the valid discovery with an error wrapping
// ErrVersionMismatch, so callers such as daemon stop can still reach the daemon.
func ReadDiscovery(paths platform.Paths) (Discovery, error) {
	var d Discovery
	path := filepath.Join(paths.Runtime, "daemon.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return d, unavailable("daemon unavailable; run agx daemon start")
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return d, unavailable("invalid daemon discovery; restart the daemon")
	}
	u, err := url.Parse(d.URL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.Path != "" ||
		u.RawQuery != "" ||
		u.Fragment != "" ||
		d.Token == "" ||
		d.InstanceID == "" {
		return d, unavailable("invalid daemon discovery")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return d, unavailable("invalid daemon port")
	}
	if d.Build != Build() {
		return d, fmt.Errorf(
			"%w: it runs a different agx build; run agx daemon stop, then retry",
			ErrVersionMismatch,
		)
	}
	return d, nil
}

// Build identifies the running executable by content, so a rebuilt binary with an unchanged
// version string still notices a daemon started from the old one.
var Build = sync.OnceValue(func() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
})

// configError reports why the daemon's configuration failed to load. Validation messages name
// files, lines, and IDs, never resolved values.
func configError(err error) error {
	return configInvalid("daemon configuration invalid: "+err.Error(), err)
}

// savedEnd names how a launch ended, for the log.
func savedEnd(state v1.Launch_State) string {
	switch state {
	case v1.Launch_STATE_REVOKED:
		return "revoked"
	case v1.Launch_STATE_EXPIRED:
		return "expired"
	case v1.Launch_STATE_FINISHED:
		return "finished"
	case v1.Launch_STATE_UNSPECIFIED, v1.Launch_STATE_PREPARED, v1.Launch_STATE_RUNNING:
	}
	return "unknown"
}

// abandonedTemporary matches the temporary files securefs.Write creates; one that outlives its
// write was left by a process killed mid-write.
var abandonedTemporary = regexp.MustCompile(`^\.agx-[0-9]+$`)

// removeAbandonedTemporaries deletes temporary files in dir that a killed process left. Commands
// write launch records there too, so only files older than any write could take are removed.
func removeAbandonedTemporaries(dir string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !abandonedTemporary.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || now.Sub(info.ModTime()) < 10*time.Minute {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err == nil {
			slog.Info("removed a file an interrupted write left", "file", filepath.Join(dir, entry.Name()))
		}
	}
}
