package daemon

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/gofrs/flock"
	"google.golang.org/protobuf/proto"

	v1 "github.com/tarik02/weft/gen/weft/v1"
	"github.com/tarik02/weft/gen/weft/v1/weftv1connect"
	"github.com/tarik02/weft/internal/auth"
	"github.com/tarik02/weft/internal/config"
	"github.com/tarik02/weft/internal/mcpruntime"
	"github.com/tarik02/weft/internal/platform"
	"github.com/tarik02/weft/internal/securefs"
)

// Version is set at build time with -ldflags "-X weft/internal/daemon.Version=...".
var Version = "dev"

// ErrVersionMismatch is returned by ReadDiscovery together with the decoded
// discovery when the running daemon has a different version.
var ErrVersionMismatch = errors.New("daemon version mismatch")

// historyRetention keeps ended launches, login requests and finished operations
// around long enough for idempotent replays and status inspection.
const historyRetention = time.Hour

type Discovery struct {
	Version    string `json:"version"`
	InstanceID string `json:"instance_id"`
	URL        string `json:"url"`
	Token      string `json:"token"`
	Config     string `json:"config"`
}

type launch struct {
	view      *v1.Launch
	grant     [32]byte
	lifecycle [32]byte
	ctx       context.Context
	cancel    context.CancelFunc
	endpoints map[config.ID]*mcpruntime.Endpoint
	snapshot  *config.Snapshot
	ended     time.Time
}

type prepareRecord struct {
	fingerprint string
	response    []byte
	launchID    string
}
type loginRecord struct {
	fingerprint string
	response    *v1.BeginOAuthLoginResponse
	created     time.Time
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
	loginRequests     map[string]loginRecord
	ctx               context.Context
	cancel            context.CancelFunc
	stop              chan struct{}
}

func Serve(ctx context.Context, paths platform.Paths) error {
	if err := securefs.EnsureDir(paths.State); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(paths.State, "daemon.lock"))
	ok, err := lock.TryLock()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("daemon is already running")
	}
	defer func() { _ = lock.Unlock() }()
	discoveryPath := filepath.Join(paths.State, "daemon.json")
	// The startup lock makes stale discovery replacement safe.
	if err := os.Remove(discoveryPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	c, inputs, err := config.LoadTracked(paths.Config, paths.Cache)
	if err != nil {
		return err
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	discovery := Discovery{
		Version:    Version,
		InstanceID: securefs.Token(),
		URL:        "http://" + listener.Addr().String(),
		Token:      securefs.Token(),
		Config:     paths.Config,
	}
	s := &Server{
		paths:             paths,
		config:            c,
		inputs:            inputs,
		discovery:         discovery,
		ctx:               ctx,
		cancel:            cancel,
		stop:              make(chan struct{}),
		launches:          map[string]*launch{},
		preparations:      map[string]prepareRecord{},
		operations:        map[string]*operation{},
		operationRequests: map[string]*operation{},
		loginRequests:     map[string]loginRecord{},
	}
	s.manager = auth.New(discovery.URL + "/oauth/callback")
	mux := http.NewServeMux()
	interceptor := connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if req.Spec().Procedure != weftv1connect.LaunchServiceRenewLaunchProcedure &&
				req.Spec().Procedure != weftv1connect.LaunchServiceFinishLaunchProcedure {
				if !equalBearer(req.Header().Get("Authorization"), discovery.Token) {
					return nil, rpcError(connect.CodeUnauthenticated, "management credential required")
				}
			}
			return next(ctx, req)
		}
	})
	opts := []connect.HandlerOption{connect.WithInterceptors(interceptor), connect.WithReadMaxBytes(16 * 1024 * 1024)}
	path, handler := weftv1connect.NewConfigurationServiceHandler(s, opts...)
	mux.Handle(path, handler)
	path, handler = weftv1connect.NewPackageServiceHandler(s, opts...)
	mux.Handle(path, handler)
	path, handler = weftv1connect.NewLaunchServiceHandler(s, opts...)
	mux.Handle(path, handler)
	path, handler = weftv1connect.NewRuntimeServiceHandler(s, opts...)
	mux.Handle(path, handler)
	path, handler = weftv1connect.NewAuthServiceHandler(s, opts...)
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
		s.expire()
		s.mu.Lock()
		defer s.mu.Unlock()
		if r.URL.Query().Get("force") != "true" {
			for _, l := range s.launches {
				if active(l.view.State) {
					http.Error(w, "active launches; stop with --force to revoke MCP access", http.StatusConflict)
					return
				}
			}
		}
		select {
		case <-s.stop:
		default:
			close(s.stop)
		}
		w.WriteHeader(http.StatusNoContent)
	})
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
		mux.ServeHTTP(w, r)
	}), ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 64 * 1024}
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(listener) }()
	if err := securefs.WriteJSON(discoveryPath, discovery); err != nil {
		_ = server.Close()
		return err
	}
	defer func() { _ = os.Remove(discoveryPath) }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var serveErr error
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-s.stop:
			break loop
		case serveErr = <-errCh:
			break loop
		case <-ticker.C:
			s.expire()
		}
	}
	cancel()
	s.mu.Lock()
	launches := []*launch{}
	for _, l := range s.launches {
		s.end(l, v1.Launch_STATE_REVOKED)
		launches = append(launches, l)
	}
	s.mu.Unlock()
	// end releases asynchronously; wait here so shutdown does not leave children or cache dirs.
	var cleanup sync.WaitGroup
	for _, l := range launches {
		cleanup.Add(1)
		go func() { defer cleanup.Done(); s.release(l.view.Id, l.endpoints) }()
	}
	cleanup.Wait()
	shutdownCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	_ = server.Shutdown(shutdownCtx)
	_ = server.Close()
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return nil
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
	c, inputs, err := config.LoadTracked(s.paths.Config, s.paths.Cache)
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

// end must be called with s.mu held; endpoint and cache cleanup run outside it.
func (s *Server) end(l *launch, state v1.Launch_State) {
	if !active(l.view.State) {
		return
	}
	l.view.State = state
	l.ended = time.Now()
	l.cancel()
	go s.release(l.view.Id, l.endpoints)
}

func (s *Server) release(id string, endpoints map[config.ID]*mcpruntime.Endpoint) {
	var wg sync.WaitGroup
	for _, e := range endpoints {
		wg.Add(1)
		go func() { defer wg.Done(); e.Close() }()
	}
	wg.Wait()
	_ = os.RemoveAll(filepath.Join(s.paths.Cache, "launches", id))
}

func (s *Server) expire() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, l := range s.launches {
		if active(l.view.State) && now.After(l.view.ExpiresAt.AsTime()) {
			s.end(l, v1.Launch_STATE_EXPIRED)
		}
	}
	s.prune(now)
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
	for requestID, record := range s.loginRequests {
		if record.created.Before(cutoff) {
			delete(s.loginRequests, requestID)
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

func rpcError(code connect.Code, message string) error {
	return connect.NewError(code, fmt.Errorf("%s", message))
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
		time.Now().After(l.view.ExpiresAt.AsTime()) ||
		!auth.Match(l.grant, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) {
		s.mu.Unlock()
		http.Error(w, "invalid or expired launch grant", http.StatusUnauthorized)
		return
	}
	e := l.endpoints[config.ID(parts[1])]
	s.mu.Unlock()
	if e == nil {
		http.Error(w, "connection not selected for launch", http.StatusForbidden)
		return
	}
	e.ServeHTTP(w, r)
}

func (s *Server) runtime() *v1.RuntimeView {
	_, _ = s.reload()
	type connection struct {
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
				"Configuration reload failed; active launches retain their snapshots. Run weft config validate.",
			),
		)
	}
	connections := []connection{}
	for _, l := range s.launches {
		view.Launches = append(view.Launches, proto.Clone(l.view).(*v1.Launch))
		if active(l.view.State) {
			for id, e := range l.endpoints {
				connections = append(connections, connection{launchID: l.view.Id, id: id, endpoint: e})
			}
		}
	}
	s.mu.Unlock()
	for _, c := range connections {
		state := v1.ConnectionRuntime_STATE_CONNECTING
		switch c.endpoint.State() {
		case "ready":
			state = v1.ConnectionRuntime_STATE_READY
		case "needs_auth":
			state = v1.ConnectionRuntime_STATE_NEEDS_AUTH
		case "failed":
			state = v1.ConnectionRuntime_STATE_FAILED
		}
		view.Connections = append(
			view.Connections,
			&v1.ConnectionRuntime{LaunchId: c.launchID, ConnectionId: string(c.id), State: state},
		)
	}
	sort.Slice(view.Launches, func(i, j int) bool { return view.Launches[i].Id < view.Launches[j].Id })
	sort.Slice(view.Connections, func(i, j int) bool {
		a, b := view.Connections[i], view.Connections[j]
		return a.LaunchId+"/"+a.ConnectionId < b.LaunchId+"/"+b.ConnectionId
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
				"Configuration is invalid; run weft config validate for local source diagnostics.",
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
		return nil, rpcError(connect.CodeFailedPrecondition, "configuration invalid; run weft config validate")
	}
	if req.Msg.Directory == "" || !filepath.IsAbs(req.Msg.Directory) {
		return nil, rpcError(connect.CodeInvalidArgument, "directory must be absolute")
	}
	snapshot, err := c.Resolve(req.Msg.Directory)
	if err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, "cannot resolve directory; run weft explain locally")
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
		case <-s.ctx.Done():
			return rpcError(connect.CodeUnavailable, "daemon stopping")
		case <-ticker.C:
		}
	}
}

func resolveMessage(s *config.Snapshot) *v1.ResolveResponse {
	out := &v1.ResolveResponse{
		Snapshot: &v1.Snapshot{Id: s.ID, Directory: s.Directory, RequiredEnvironment: s.RequiredEnvironment},
	}
	resource := func(r config.Resource) *v1.ResourceRef {
		out := &v1.ResourceRef{}
		switch r.Kind {
		case "skill":
			out.Resource = &v1.ResourceRef_SkillId{SkillId: string(r.ID)}
		case "instructions":
			out.Resource = &v1.ResourceRef_InstructionsId{InstructionsId: string(r.ID)}
		case "connection":
			out.Resource = &v1.ResourceRef_ConnectionId{ConnectionId: string(r.ID)}
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
			if step.Change == "exclude" {
				change = v1.ResolutionStep_CHANGE_EXCLUDE
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
					BundleChain:    chain,
				},
			)
		}
		out.Traces = append(out.Traces, trace)
	}
	return out
}

// ReadDiscovery never returns credentials to inspection commands. On a version
// mismatch it returns the valid discovery with an error wrapping
// ErrVersionMismatch, so callers such as daemon stop can still reach the daemon.
func ReadDiscovery(paths platform.Paths) (Discovery, error) {
	var d Discovery
	b, err := os.ReadFile(filepath.Join(paths.State, "daemon.json"))
	if err != nil {
		return d, fmt.Errorf("daemon unavailable; run weft daemon start")
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return d, fmt.Errorf("invalid daemon discovery; restart the daemon")
	}
	u, err := url.Parse(d.URL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.Path != "" ||
		u.RawQuery != "" ||
		u.Fragment != "" ||
		d.Token == "" ||
		d.InstanceID == "" {
		return d, fmt.Errorf("invalid daemon discovery")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return d, fmt.Errorf("invalid daemon port")
	}

	if d.Version != Version {
		return d, fmt.Errorf("%w; restart the daemon", ErrVersionMismatch)
	}
	return d, nil
}
