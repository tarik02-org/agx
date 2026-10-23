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
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

// ErrNewerDiscovery is returned by ReadDiscovery when the discovery file has a format newer than
// this build reads, written by a newer agx; this build must neither use nor replace that daemon.
var ErrNewerDiscovery = errors.New("daemon discovery from a newer agx")

// discoveryFormat is the format version of the discovery file.
const discoveryFormat = 1

// ErrUnavailable marks ReadDiscovery errors that leave no daemon to reach: it is not running,
// or its discovery file is invalid.
var ErrUnavailable = errors.New("daemon unavailable")

// unavailable keeps a discovery error's own message while matching ErrUnavailable.
type unavailable string

func (u unavailable) Error() string { return string(u) }

func (u unavailable) Is(target error) bool { return target == ErrUnavailable }

// KeptLaunchesHeader carries, in a response to a stop without force, how many active launches
// the daemon keeps for the next one to restore.
const KeptLaunchesHeader = "Agx-Kept-Launches"

// historyRetention keeps ended launches and finished operations
// around long enough for idempotent replays and status inspection.
const historyRetention = time.Hour

type Discovery struct {
	// Format is the file's format version, discoveryFormat; files from builds before it have none.
	// Version names the agx release, which older builds compare, so it keeps its name.
	Format     int    `json:"format"`
	Version    string `json:"version"`
	Build      string `json:"build"`
	InstanceID string `json:"instance_id"`
	URL        string `json:"url"`
	Token      string `json:"token"`
	Config     string `json:"config"`
	// RestoresLaunches marks a daemon that keeps its active launches for the next one when it
	// stops without force, so replacing it does not end their MCP access.
	RestoresLaunches bool `json:"restores_launches,omitempty"`
}

type launch struct {
	// view.ExpiresAt shows and saves expires, which alone decides expiry: it holds a monotonic
	// clock reading, so that a wall clock jump expires no lease early or late.
	view      *v1.Launch
	expires   time.Time
	grant     [32]byte
	lifecycle [32]byte
	ctx       context.Context
	cancel    context.CancelFunc
	endpoints map[config.ID]*mcpruntime.Endpoint
	snapshot  *config.Snapshot
	// target is the harness process a launch started; its thread launches share it.
	target adapters.Target
	ended  time.Time
	// created and lease are kept with the launch's state; lease is how long a renewal extends it.
	created time.Time
	lease   time.Duration
	// threadServer marks a harness process serving many threads; its launch has no endpoints.
	threadServer bool
	// restored marks a launch a restarted daemon restored from its state file, until its wrapper
	// resent what that file does not hold (RestoreLaunch). Until then, endpoints and snapshot
	// stay nil, except that endpoints needing no environment inputs are rebuilt on first use.
	restored bool
	// unavailable says why a restored launch's MCP servers cannot be served, when its snapshot no
	// longer resolves to the same ID.
	unavailable string
	// reviving serializes rebuilding a restored launch.
	reviving sync.Mutex
}

type prepareRecord struct {
	fingerprint string
	response    []byte
	launchID    string
}

type operation struct {
	fingerprint string
	requestID   string
	state       *v1.WatchOperationResponse
	finished    time.Time
}

type Server struct {
	mu           sync.Mutex
	reloadMu     sync.Mutex
	paths        platform.Paths
	adapters     adapters.Registry
	builtins     config.Builtins
	config       *config.Config
	configErr    error
	inputs       *config.Inputs
	discovery    Discovery
	manager      *auth.Manager
	launches     map[string]*launch
	preparations map[string]prepareRecord
	operations   map[string]*operation
	// operationRequests maps idempotency keys to operations also held in operations.
	operationRequests map[string]*operation
	ctx               context.Context
	cancel            context.CancelFunc
	stop              chan struct{}
	// forced is set by a stop that revokes active launches instead of keeping them.
	forced bool
	// grace, when set by a stop, replaces daemon.shutdown_grace_seconds.
	grace time.Duration
	drain *drain
	// flushed is set once a stopping daemon collected the state the next one gets; new launches
	// are refused from then on, for their callers to retry with the next daemon.
	flushed bool
	// installs counts package installations running off their requests; none starts once
	// installsClosed is set, so that waiting for them is final.
	installs       sync.WaitGroup
	installsClosed bool
	// store keeps active launches across restarts; generation numbers the states built for it.
	store      *stateStore
	generation uint64
	port       int
	// clients pools the daemon's own MCP client sessions for ListTools and CallTool.
	clients *clientPool
	// shared runs the stdio MCP servers launches share.
	shared *mcpruntime.Shared
	// editMu serializes planning and applying configuration changes, so an apply checks and
	// writes what no other apply is writing.
	editMu sync.Mutex
	// previews holds the Git sources PreviewSource fetched.
	previews previews
}

// Serve runs the daemon until stop closes or a stop request arrives, then shuts down gracefully:
// it takes no new launches or MCP sessions, lets requests in flight finish for the grace period,
// saves its launches, and lets its MCP servers exit. When ctx ends, it shuts down at once, still
// saving its launches. builtins must describe the adapters in registry.
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
	// The startup lock makes stale discovery replacement safe.
	if err := os.Remove(discoveryPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	c, inputs, err := config.LoadTracked(paths.Config, paths.Cache, builtins)
	if err != nil {
		return err
	}
	store := &stateStore{path: StatePath(paths), interval: renewalWriteInterval}
	port, restored, err := loadState(store.path, paths.Config, time.Now())
	// Without a readable state, launch files may still belong to launches a daemon could restore.
	sweep := err == nil
	var newer newerState
	var invalid invalidState
	switch {
	case errors.As(err, &newer):
		return err
	case errors.As(err, &invalid):
		aside := store.path + ".bad-" + time.Now().UTC().Format("20060102T150405Z")
		if renameErr := os.Rename(store.path, aside); renameErr != nil {
			fmt.Fprintf(
				os.Stderr,
				"agx daemon: ignoring saved launches: %v; cannot move it aside: %v\n",
				err,
				renameErr,
			)
		} else {
			fmt.Fprintf(os.Stderr, "agx daemon: ignoring saved launches: %v; moved it to %s\n", err, aside)
		}
	case err != nil:
		fmt.Fprintf(os.Stderr, "agx daemon: ignoring saved launches: %v\n", err)
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
		Format:           discoveryFormat,
		Version:          Version,
		Build:            Build(),
		InstanceID:       securefs.Token(),
		URL:              "http://" + listener.Addr().String(),
		Token:            securefs.Token(),
		Config:           paths.Config,
		RestoresLaunches: true,
	}
	s := &Server{
		paths:             paths,
		adapters:          registry,
		builtins:          builtins,
		config:            c,
		inputs:            inputs,
		discovery:         discovery,
		ctx:               serverCtx,
		cancel:            cancel,
		stop:              make(chan struct{}),
		drain:             newDrain(),
		launches:          map[string]*launch{},
		preparations:      map[string]prepareRecord{},
		operations:        map[string]*operation{},
		operationRequests: map[string]*operation{},
		store:             store,
		port:              listener.Addr().(*net.TCPAddr).Port,
		clients:           newClientPool(),
		shared:            mcpruntime.NewShared(serverCtx, Version),
	}
	if port != 0 && port != s.port && len(restored) > 0 {
		fmt.Fprintf(
			os.Stderr,
			"agx daemon: port %d is taken; restored launches keep their lease, but harnesses holding "+
				"endpoint URLs of port %d lose MCP access until they get new ones\n",
			port,
			port,
		)
	}
	s.restore(restored, sweep)
	s.manager = auth.New(discovery.URL + "/oauth/callback")
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
	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !equalBearer(r.Header.Get("Authorization"), discovery.Token) {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		var grace time.Duration
		if value := r.URL.Query().Get("grace"); value != "" {
			parsed, err := time.ParseDuration(value)
			if err != nil || parsed <= 0 {
				http.Error(w, "grace must be a positive duration", http.StatusBadRequest)
				return
			}
			grace = parsed
		}
		s.expire()
		s.mu.Lock()
		defer s.mu.Unlock()
		// Without force, active launches stay in the state file for the next daemon to restore.
		s.forced = s.forced || r.URL.Query().Get("force") == "true"
		if grace > 0 {
			s.grace = grace
		}
		kept := 0
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
		w.Header().Set(KeptLaunchesHeader, strconv.Itoa(kept))
		w.WriteHeader(http.StatusNoContent)
	})
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
			if s.store.due(time.Now()) {
				s.persist(false)
			}
		}
	}
	s.drain.start()
	if graceful {
		s.settle(ctx)
	}
	launches := s.flush()
	s.clients.closeAll()
	// Shutdown closes the listener at once; requests that outlasted the grace period end as
	// their MCP sessions close.
	shutdownCtx, stopShutdown := context.WithCancel(ctx)
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		_ = server.Shutdown(shutdownCtx)
	}()
	s.closeLaunches(ctx, launches)
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
	agxv1connect.LaunchServicePrepareThreadLaunchProcedure: true,
	agxv1connect.PackageServiceInstallProcedure:            true,
	agxv1connect.PackageServiceUpdateProcedure:             true,
	agxv1connect.PackageServicePreviewSourceProcedure:      true,
	agxv1connect.ConfigurationServicePlanChangesProcedure:  true,
	agxv1connect.ConfigurationServiceApplyChangesProcedure: true,
	agxv1connect.RuntimeServiceTestMcpServerProcedure:      true,
	agxv1connect.RuntimeServiceListToolsProcedure:          true,
	agxv1connect.RuntimeServiceCallToolProcedure:           true,
}

// settle waits for the grace period until requests in flight finished, or ctx ends.
func (s *Server) settle(ctx context.Context) {
	c, _ := s.reload()
	s.mu.Lock()
	if c == nil {
		c = s.config
	}
	grace := time.Duration(c.Daemon.ShutdownGraceSeconds) * time.Second
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
	c, inputs, err := config.LoadTracked(s.paths.Config, s.paths.Cache, s.builtins)
	s.mu.Lock()
	defer s.mu.Unlock()
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
// thread launches end with it.
func (s *Server) end(l *launch, state v1.Launch_State) {
	if !active(l.view.State) {
		return
	}
	l.view.State = state
	l.ended = time.Now()
	l.cancel()
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

func (s *Server) expire() {
	s.mu.Lock()
	now := time.Now()
	ended := false
	for _, l := range s.launches {
		if active(l.view.State) && now.After(l.expires) {
			s.end(l, v1.Launch_STATE_EXPIRED)
			ended = true
		}
	}
	s.prune(now)
	s.mu.Unlock()
	if ended {
		s.persist(false)
	}
}

// flush writes the state the next daemon gets and returns the daemon's launches. A forced stop
// revokes them; otherwise they stay in the state file, with their launch files, for the next
// daemon to restore.
func (s *Server) flush() []*launch {
	s.mu.Lock()
	s.flushed = true
	launches := []*launch{}
	for _, l := range s.launches {
		if s.forced {
			s.end(l, v1.Launch_STATE_REVOKED)
		}
		launches = append(launches, l)
	}
	s.mu.Unlock()
	// Requests still in flight must not rewrite the state once it holds what the next daemon gets.
	s.persist(false)
	s.store.freeze()
	return launches
}

// closeLaunches ends the MCP sessions of launches, letting their servers exit, or killing them
// once ctx ends; for a forced stop, it also removes their launch files.
func (s *Server) closeLaunches(ctx context.Context, launches []*launch) {
	s.mu.Lock()
	forced := s.forced
	all := map[*mcpruntime.Endpoint]bool{}
	for _, l := range launches {
		for _, e := range l.endpoints {
			all[e] = true
		}
	}
	s.mu.Unlock()
	stopKilling := context.AfterFunc(ctx, func() {
		for e := range all {
			go e.Kill()
		}
	})
	defer stopKilling()
	var cleanup sync.WaitGroup
	for _, l := range launches {
		s.mu.Lock()
		endpoints := l.endpoints
		s.mu.Unlock()
		// end releases asynchronously; wait here so shutdown does not leave children or cache dirs.
		if forced {
			cleanup.Go(func() { s.release(l.view.Id, endpoints) })
		} else {
			cleanup.Go(func() { closeEndpoints(endpoints) })
		}
	}
	cleanup.Wait()
}

// persist writes the active launches to the state file; a renewal alone is throttled, and the
// periodic check writes it later. Failures are logged: launches keep working, but a restarted
// daemon would not restore them.
func (s *Server) persist(renewal bool) {
	s.mu.Lock()
	s.generation++
	generation := s.generation
	state := savedState{Version: stateVersion, Port: s.port, Config: s.paths.Config, Launches: []savedLaunch{}}
	parents := []*launch{}
	for _, l := range s.launches {
		if active(l.view.State) && l.view.ParentLaunchId == "" {
			parents = append(parents, l)
		}
	}
	sort.Slice(parents, func(i, j int) bool { return parents[i].created.Before(parents[j].created) })
	for _, parent := range parents {
		state.Launches = append(state.Launches, saveLaunch(parent))
		children := s.children(parent)
		sort.Slice(children, func(i, j int) bool { return children[i].created.Before(children[j].created) })
		for _, child := range children {
			if active(child.view.State) {
				state.Launches = append(state.Launches, saveLaunch(child))
			}
		}
	}
	s.mu.Unlock()
	if _, err := s.store.write(generation, state, renewal, time.Now()); err != nil {
		fmt.Fprintf(os.Stderr, "agx daemon: cannot save launches for a restart: %v\n", err)
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
	for requestID, record := range s.preparations {
		if s.launches[record.launchID] == nil {
			delete(s.preparations, requestID)
		}
	}
	for id, op := range s.operations {
		if !op.finished.IsZero() && op.finished.Before(cutoff) {
			delete(s.operations, id)
			delete(s.operationRequests, op.requestID)
		}
	}
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
	if l == nil || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || !active(l.view.State) ||
		time.Now().After(l.expires) ||
		!auth.Match(l.grant, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) {
		s.mu.Unlock()
		http.Error(w, "invalid or expired launch grant", http.StatusUnauthorized)
		return
	}
	pending := l.restored && l.endpoints == nil && l.unavailable == "" && !l.threadServer
	s.mu.Unlock()
	// A stopping daemon finishes the sessions it serves but opens none; event streams would
	// outlast it.
	if s.drain.draining() && (r.Header.Get("Mcp-Session-Id") == "" || r.Method == http.MethodGet) {
		refuse(w)
		return
	}
	if pending {
		if err := s.revive(r.Context(), l, nil, false); err != nil {
			w.Header().Set("Retry-After", "1")
			http.Error(w, errAwaitingWrapper.Error(), http.StatusServiceUnavailable)
			return
		}
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

func (s *Server) runtime() *v1.RuntimeView {
	_, _ = s.reload()
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
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	previous := ""
	for {
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
		case <-ticker.C:
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
	// The format decodes first, so a newer one is recognized even when its fields changed.
	var header struct {
		Format int `json:"format"`
	}
	if err := json.Unmarshal(b, &header); err == nil && header.Format > discoveryFormat {
		return Discovery{}, fmt.Errorf(
			"%w: %s has format %d, newer than this agx reads (%d); the daemon runs a newer agx, so upgrade agx",
			ErrNewerDiscovery,
			path,
			header.Format,
			discoveryFormat,
		)
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

	if d.Version != Version || d.Build != Build() {
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
	return reasonError(
		connect.CodeFailedPrecondition,
		v1.ErrorDetail_REASON_CONFIG_INVALID,
		"daemon configuration invalid: "+err.Error(),
	)
}
