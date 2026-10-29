package mcpruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/securefs"
)

const (
	maxSessions = 8
	// Harnesses rarely DELETE their sessions, so abandoned ones would otherwise
	// hold one of the session slots and, for stdio, a child process forever.
	sessionIdleTimeout = 30 * time.Minute
	// A stdio server whose session ends gets its input closed, then, after inputGrace, a
	// termination request, and after terminateGrace more it is killed.
	inputGrace     = 2 * time.Second
	terminateGrace = time.Second
)

var (
	errEndpointClosed = errors.New("launch ended")
	errSessionGone    = errors.New("session ended")
)

// needsAuth marks failures that logging in or storing a secret can fix.
type needsAuth struct{ error }

func (e needsAuth) Unwrap() error { return e.error }

type Endpoint struct {
	mu             sync.Mutex
	id             config.ID
	ctx            context.Context
	cancel         context.CancelFunc
	definition     config.MCPServer
	auth           *auth.Manager
	secrets        map[config.ID]config.Secret
	inputs         map[string]string
	shared         *Shared
	shareKey       string
	shareRoot      string
	sessions       map[string]*stdioSession
	starting       int
	remoteSessions map[string]*remoteSession
	generation     uint64
	closed         bool
	state          string
	// problem explains the latest failure for runtime views; it never contains values.
	problem string
}

// stdioSession is a harness session on a stdio server's process: one it shares, or its own.
type stdioSession struct {
	transport  *mcp.StreamableServerTransport
	downstream mcp.Connection
	client     *sharedClient
	cancel     context.CancelFunc
	done       sync.Once
	ended      chan struct{}
	active     int
	lastUsed   time.Time
}

type remoteSession struct {
	upstreamID string
	credential string
	headers    map[string]string
	active     int
	lastUsed   time.Time
}

// New returns the endpoint of MCP server id. A stdio server whose definition sets share joins
// a server of shared rooted at root: the launch directory or repository root its share mode
// keys on, empty for global; any other gets a process of shared per session. Without shared,
// the endpoint runs its processes itself.
func New(
	ctx context.Context,
	id config.ID,
	c config.MCPServer,
	secrets map[config.ID]config.Secret,
	inputs map[string]string,
	manager *auth.Manager,
	shared *Shared,
	root string,
) *Endpoint {
	ctx, cancel := context.WithCancel(ctx)
	e := &Endpoint{
		ctx:            ctx,
		cancel:         cancel,
		id:             id,
		definition:     c,
		secrets:        secrets,
		inputs:         inputs,
		auth:           manager,
		sessions:       map[string]*stdioSession{},
		remoteSessions: map[string]*remoteSession{},
		state:          "connecting",
		shared:         shared,
		shareRoot:      root,
	}
	if shared == nil {
		e.shared = NewShared(ctx, "", nil)
	}
	if c.Transport == "stdio" {
		e.shareKey = shareKey(id, c, root, secrets, inputs)
	}
	go e.expireIdle()
	return e
}

// State reports the endpoint state and, unless ready, why the last attempt failed.
func (e *Endpoint) State() (string, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state == "ready" {
		return e.state, ""
	}
	return e.state, e.problem
}

// resolve reads an injected value when a session starts, so edited files and rotated secrets
// reach new sessions. Secret failures mark the endpoint as needing authorization.
func (e *Endpoint) resolve(
	ctx context.Context,
	target string,
	v config.Value,
	inputs map[string]string,
) (string, error) {
	return v.Resolve(
		target,
		func(name string) (string, bool) {
			value, ok := inputs[name]
			return value, ok
		},
		func(id config.ID) (string, error) {
			value, err := e.auth.Value(ctx, id, e.secrets[id], e.secrets, inputs, "")
			if err != nil {
				return "", needsAuth{err}
			}
			return value, nil
		},
	)
}

// setState must be called with e.mu held.
func (e *Endpoint) setState(state, problem string) {
	if e.state != state || e.problem != problem {
		e.state, e.problem = state, problem
		e.shared.notify()
	}
}

// fail must be called with e.mu held.
func (e *Endpoint) fail(err error) {
	if e.closed {
		return
	}
	state := "failed"
	if errors.As(err, &needsAuth{}) {
		state = "needs_auth"
	}
	e.setState(state, err.Error())
}

// Close ends every session, which lets the stdio processes of none else exit, and deletes remote
// sessions upstream.
func (e *Endpoint) Close() {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.setState("failed", e.problem)
	e.closed = true
	e.cancel()
	e.inputs = nil
	stdio, remote := e.takeSessionsLocked(func(int, time.Time) bool { return true })
	e.mu.Unlock()
	e.release(stdio, remote)
}

// Reset ends all sessions and the credentials cached with them while keeping the
// endpoint usable, so harnesses can reconnect after the user logs in again.
func (e *Endpoint) Reset() {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.generation++
	e.setState("connecting", e.problem)
	stdio, remote := e.takeSessionsLocked(func(int, time.Time) bool { return true })
	e.mu.Unlock()
	e.release(stdio, remote)
}

func (e *Endpoint) expireIdle() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.mu.Lock()
			stdio, remote := e.takeIdleLocked()
			e.mu.Unlock()
			e.release(stdio, remote)
		}
	}
}

func (e *Endpoint) takeIdleLocked() ([]*stdioSession, []*remoteSession) {
	now := time.Now()
	return e.takeSessionsLocked(func(active int, lastUsed time.Time) bool {
		return active == 0 && now.Sub(lastUsed) > sessionIdleTimeout
	})
}

func (e *Endpoint) takeSessionsLocked(
	match func(active int, lastUsed time.Time) bool,
) ([]*stdioSession, []*remoteSession) {
	stdio := []*stdioSession{}
	for id, s := range e.sessions {
		if match(s.active, s.lastUsed) {
			delete(e.sessions, id)
			stdio = append(stdio, s)
		}
	}
	remote := []*remoteSession{}
	for id, s := range e.remoteSessions {
		if match(s.active, s.lastUsed) {
			delete(e.remoteSessions, id)
			remote = append(remote, s)
		}
	}
	return stdio, remote
}

func (e *Endpoint) release(stdio []*stdioSession, remote []*remoteSession) {
	for _, s := range stdio {
		s.close()
	}
	var wg sync.WaitGroup
	for _, s := range remote {
		if s.upstreamID == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.deleteUpstream(s.upstreamID, s.credential, s.headers)
		}()
	}
	wg.Wait()
}

func (e *Endpoint) deleteUpstream(upstreamID, credential string, headers map[string]string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, e.definition.URL, nil)
	if err != nil {
		return
	}
	req.Header.Set("Mcp-Session-Id", upstreamID)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	if response, err := client.Do(req); err == nil {
		_ = response.Body.Close()
	}
}

// close ends the session without waiting for its server, which stops once no session uses it.
func (s *stdioSession) close() {
	s.done.Do(func() { s.cancel(); s.client.end(); _ = s.downstream.Close(); close(s.ended) })
}

func (e *Endpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost && r.Method != http.MethodGet && r.Method != http.MethodDelete {
		w.Header().Set("Allow", "POST, GET, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024*1024)
	if r.Method == http.MethodPost {
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Second))
		defer func() { _ = http.NewResponseController(w).SetReadDeadline(time.Time{}) }()
	}
	if e.definition.Transport == "http" {
		e.serveRemote(w, r)
		return
	}
	e.serveStdio(w, r)
}

// initialRequest checks that a request without a session ID initializes one, and returns the
// protocol version it asks for.
func initialRequest(r *http.Request) (string, error) {
	if r.Method != http.MethodPost {
		return "", fmt.Errorf("session ID required")
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	var msg struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return "", fmt.Errorf("invalid initialization request")
	}
	if msg.JSONRPC != "2.0" || msg.Method != "initialize" || len(msg.ID) == 0 || string(msg.ID) == "null" {
		return "", fmt.Errorf("expected initialize request with an ID")
	}
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if json.Unmarshal(msg.Params, &params) != nil || params.ProtocolVersion == "" {
		return "", fmt.Errorf("missing protocolVersion")
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	return params.ProtocolVersion, nil
}

func (e *Endpoint) serveStdio(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get("Mcp-Session-Id")
	var s *stdioSession
	if id == "" {
		version, err := initialRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !slices.Contains(protocolVersions, version) {
			http.Error(w, "stdio bridge supports MCP through "+sharedProtocol, http.StatusBadRequest)
			return
		}
		id, s = e.openStdio(w)
		if s == nil {
			return
		}
	} else {
		e.mu.Lock()
		if e.closed {
			e.mu.Unlock()
			http.Error(w, "launch ended", http.StatusUnauthorized)
			return
		}
		s = e.sessions[id]
		if s == nil {
			e.mu.Unlock()
			http.Error(w, "session not found for this endpoint", http.StatusNotFound)
			return
		}
		s.active++
		s.lastUsed = time.Now()
		e.mu.Unlock()
	}
	defer func() {
		e.mu.Lock()
		s.active--
		s.lastUsed = time.Now()
		e.mu.Unlock()
	}()
	if r.Method == http.MethodDelete {
		e.mu.Lock()
		if e.sessions[id] == s {
			delete(e.sessions, id)
		}
		e.mu.Unlock()
		s.close()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method == http.MethodGet && !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		http.Error(w, "GET requires text/event-stream", http.StatusNotAcceptable)
		return
	}
	w.Header().Set("Mcp-Session-Id", id)
	s.transport.ServeHTTP(w, r)
}

// openStdio starts a session outside e.mu, holding a slot via e.starting so
// credential helpers and process startup do not block other sessions. The
// returned session is already marked active.
func (e *Endpoint) openStdio(w http.ResponseWriter) (string, *stdioSession) {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		http.Error(w, "launch ended", http.StatusUnauthorized)
		return "", nil
	}
	idleStdio, idleRemote := e.takeIdleLocked()
	if len(e.sessions)+e.starting >= maxSessions {
		e.mu.Unlock()
		go e.release(idleStdio, idleRemote)
		http.Error(w, "too many sessions for this MCP server", http.StatusTooManyRequests)
		return "", nil
	}
	e.starting++
	generation := e.generation
	inputs := e.inputs
	e.mu.Unlock()
	go e.release(idleStdio, idleRemote)

	id := securefs.Token()
	s, err := e.join(id, inputs)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.starting--
	if err != nil {
		if e.closed {
			http.Error(w, "launch ended", http.StatusUnauthorized)
			return "", nil
		}
		e.fail(err)
		http.Error(
			w,
			"upstream connection failed; check the MCP server command and credential source",
			http.StatusBadGateway,
		)
		return "", nil
	}
	select {
	case <-s.ended:
		e.fail(errors.New("upstream process exited during startup"))
		http.Error(
			w,
			"upstream connection failed; check the MCP server command and credential source",
			http.StatusBadGateway,
		)
		return "", nil
	default:
	}
	if e.closed || e.generation != generation {
		go s.close()
		if e.closed {
			http.Error(w, "launch ended", http.StatusUnauthorized)
		} else {
			http.Error(w, "MCP server was reset; retry", http.StatusServiceUnavailable)
		}
		return "", nil
	}
	s.active = 1
	s.lastUsed = time.Now()
	e.sessions[id] = s
	e.setState("ready", "")
	return id, s
}

// process is a running stdio MCP server.
type process struct {
	pid     int
	started time.Time
	conn    mcp.Connection
	// cancel starts its shutdown; kill ends its process tree at once.
	cancel context.CancelFunc
	kill   func()
	exited chan struct{}
}

// startProcess starts the server's command under base, resolving its environment with resolve.
// Cancelling base, or the returned cancel, closes the server's input, then terminates and
// kills it.
func (e *Endpoint) startProcess(base, resolve context.Context, inputs map[string]string) (*process, error) {
	ctx, cancel := context.WithCancel(base)
	cmd := exec.CommandContext(ctx, e.definition.Command[0], e.definition.Command[1:]...)
	cmd.Dir = e.definition.Cwd
	cmd.Env = auth.SystemEnvironment()
	for name, v := range e.definition.Environment {
		value, err := e.resolve(resolve, fmt.Sprintf("mcp %s environment %s", e.id, name), v, inputs)
		if err != nil {
			cancel()
			return nil, err
		}
		cmd.Env = append(cmd.Env, name+"="+value)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	// Most stdio servers exit when their input ends; a termination request and then a kill of
	// their group cover those that do not, and WaitDelay those that escaped the group.
	cmd.Cancel = func() error {
		_ = stdin.Close()
		return nil
	}
	cmd.WaitDelay = inputGrace + 2*terminateGrace
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		cancel()
		return nil, err
	}
	// Upstream stderr is deliberately not copied: it can contain credentials.
	group, err := platform.Start(cmd)
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		cancel()
		return nil, err
	}
	context.AfterFunc(ctx, func() {
		time.AfterFunc(inputGrace, group.Terminate)
		time.AfterFunc(inputGrace+terminateGrace, group.Kill)
	})
	conn, err := (&mcp.IOTransport{Reader: stdout, Writer: stdin}).Connect(ctx)
	if err != nil {
		group.Kill()
		_ = group.Wait()
		cancel()
		return nil, err
	}
	exited := make(chan struct{})
	go func() {
		_ = group.Wait()
		close(exited)
	}()
	return &process{
		pid:     cmd.Process.Pid,
		started: time.Now(),
		conn:    conn,
		cancel:  cancel,
		kill:    group.Kill,
		exited:  exited,
	}, nil
}

// join opens session id on the endpoint's server process, starting the process when none runs
// for its share key; a server that shares no process gets one for the session alone.
func (e *Endpoint) join(id string, inputs map[string]string) (*stdioSession, error) {
	ctx, cancel := context.WithCancel(e.ctx)
	joinCtx, stopJoin := context.WithTimeout(ctx, sharedStartTimeout)
	key := e.shareKey
	if e.definition.ShareMode() == config.ShareNone {
		key += "/" + id
	}
	server, err := e.shared.join(joinCtx, e, key, inputs)
	stopJoin()
	if err != nil {
		cancel()
		return nil, err
	}
	t := &mcp.StreamableServerTransport{SessionID: id}
	down, err := t.Connect(ctx)
	if err != nil {
		server.unreserve()
		cancel()
		return nil, err
	}
	client, err := server.attach(ctx, down)
	if err != nil {
		_ = down.Close()
		cancel()
		return nil, err
	}
	s := &stdioSession{transport: t, downstream: down, client: client, cancel: cancel, ended: make(chan struct{})}
	go func() {
		<-client.Ended()
		e.sessionEnded(id, s)
	}()
	return s, nil
}

// Process is a running stdio MCP server process, for runtime views.
type Process struct {
	ID        config.ID
	PID       int
	Directory string
	// Share is the server's share mode; Root is the directory a shared process is keyed on.
	Share    string
	Root     string
	Sessions int
	Started  time.Time
	// StopsAt is when an idle shared process stops; zero while sessions use it.
	StopsAt time.Time
}

// Processes lists the IDs of the processes the endpoint's sessions use.
func (e *Endpoint) Processes() []int {
	e.mu.Lock()
	defer e.mu.Unlock()
	pids := []int{}
	for _, s := range e.sessions {
		if pid := s.client.server.pid(); pid != 0 {
			pids = append(pids, pid)
		}
	}
	return pids
}

// sessionEnded forgets session id once its process exited or it left its shared server.
func (e *Endpoint) sessionEnded(id string, s *stdioSession) {
	s.close()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sessions[id] == s {
		delete(e.sessions, id)
	}
	if !e.closed {
		e.setState("connecting", e.problem)
	}
}

func (e *Endpoint) serveRemote(w http.ResponseWriter, r *http.Request) {
	localID := r.Header.Get("Mcp-Session-Id")
	initial := localID == ""
	if initial {
		if _, err := initialRequest(r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		http.Error(w, "launch ended", http.StatusUnauthorized)
		return
	}
	var session *remoteSession
	if initial {
		idleStdio, idleRemote := e.takeIdleLocked()
		go e.release(idleStdio, idleRemote)
		if len(e.remoteSessions) >= maxSessions {
			e.mu.Unlock()
			http.Error(w, "too many sessions", http.StatusTooManyRequests)
			return
		}
		localID = securefs.Token()
		session = &remoteSession{}
		e.remoteSessions[localID] = session
	} else if session = e.remoteSessions[localID]; session == nil {
		e.mu.Unlock()
		http.Error(w, "session not found for this endpoint", http.StatusNotFound)
		return
	}
	session.active++
	session.lastUsed = time.Now()
	upstreamID := session.upstreamID
	inputs := e.inputs
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		session.active--
		session.lastUsed = time.Now()
		e.mu.Unlock()
	}()
	// current reports whether the session is still registered; callers hold e.mu.
	current := func() bool { return !e.closed && e.remoteSessions[localID] == session }
	forget := func() {
		if e.remoteSessions[localID] == session {
			delete(e.remoteSessions, localID)
		}
	}

	target, _ := url.Parse(e.definition.URL)
	origin, _ := auth.Origin(e.definition.URL)
	// Values resolve per request like the bearer credential, so rotation applies without reconnecting.
	credential, headers, err := e.remoteCredentials(r.Context(), inputs, origin)
	if err != nil {
		e.mu.Lock()
		e.fail(err)
		if initial {
			forget()
		}
		e.mu.Unlock()
		switch {
		case errors.As(err, &needsAuth{}):
			http.Error(w, "upstream credential unavailable; see agx secret status", http.StatusUnauthorized)
		case errors.Is(err, auth.ErrRefreshFailed):
			http.Error(w, "upstream credential refresh failed; see agx launch show", http.StatusServiceUnavailable)
		default:
			http.Error(w, "upstream header unavailable; see agx launch show", http.StatusBadGateway)
		}
		return
	}
	secretID := e.definition.Auth.Secret
	secret := e.secrets[secretID]
	// A 401 for an OAuth token is answered by refreshing it once and resending the request, so
	// the body must be replayable; MCP POSTs are single JSON-RPC messages.
	refreshable := secret.Kind == "oauth" && credential != ""
	if refreshable && r.Body != nil && r.Body != http.NoBody {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "cannot read request", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(data))
		r.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }
	}
	// refreshErr holds why a rejected token could not be renewed, for the 401's explanation.
	var refreshErr error
	transport := roundTripper(func(req *http.Request) (*http.Response, error) {
		resp, err := http.DefaultTransport.RoundTrip(req)
		if err != nil || resp.StatusCode != http.StatusUnauthorized || !refreshable {
			return resp, err
		}
		var renewed string
		renewed, refreshErr = e.auth.Refresh(req.Context(), secretID, secret, origin, credential)
		if refreshErr == nil {
			retry := req.Clone(req.Context())
			if req.GetBody != nil {
				// The buffered body above cannot fail to replay.
				retry.Body, _ = req.GetBody()
			}
			retry.Header.Set("Authorization", "Bearer "+renewed)
			_ = resp.Body.Close()
			credential = renewed
			return http.DefaultTransport.RoundTrip(retry)
		}
		return resp, nil
	})
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stop := context.AfterFunc(e.ctx, cancel)
	defer stop()
	proxy := &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			// SetURL appends the incoming launch path; the upstream is the configured URL itself.
			pr.Out.URL.Path, pr.Out.URL.RawPath, pr.Out.URL.RawQuery = target.Path, target.RawPath, target.RawQuery
			pr.Out.Host = target.Host
			pr.Out.Header.Del("Authorization")
			pr.Out.Header.Del("Proxy-Authorization")
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("Origin")
			pr.Out.Header.Del("Mcp-Session-Id")
			for name, value := range headers {
				pr.Out.Header.Set(name, value)
			}
			if credential != "" {
				pr.Out.Header.Set("Authorization", "Bearer "+credential)
			}
			if upstreamID != "" {
				pr.Out.Header.Set("Mcp-Session-Id", upstreamID)
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("Set-Cookie")
			resp.Header.Del("Location")
			// Harness credentials never reach the upstream, so its challenge is agx's to answer.
			challenge := strings.Join(resp.Header.Values("WWW-Authenticate"), ", ")
			resp.Header.Del("WWW-Authenticate")
			var unauthorized error
			if resp.StatusCode == http.StatusUnauthorized {
				unauthorized = e.unauthorized(resp.Request.Context(), credential, challenge, refreshErr)
			}
			success := resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
			e.mu.Lock()
			defer e.mu.Unlock()
			if !current() {
				// The session was closed while initialize was in flight; end the
				// upstream session it created so it is not leaked.
				if initial && success {
					if id := resp.Header.Get("Mcp-Session-Id"); id != "" {
						go e.deleteUpstream(id, credential, headers)
					}
				}
				if e.closed {
					return errEndpointClosed
				}
				return errSessionGone
			}
			if unauthorized != nil {
				if initial {
					forget()
				}
				e.fail(unauthorized)
				return unauthorized
			}
			switch {
			case initial && success:
				session.upstreamID = resp.Header.Get("Mcp-Session-Id")
				session.credential, session.headers = credential, headers
				e.setState("ready", "")
			case initial:
				forget()
				e.fail(fmt.Errorf("upstream rejected initialization with HTTP %d", resp.StatusCode))
			case success:
				session.credential, session.headers = credential, headers
			case resp.StatusCode == http.StatusNotFound && upstreamID != "":
				// The upstream expired its session; the client must initialize again.
				forget()
			}
			if success {
				resp.Header.Set("Mcp-Session-Id", localID)
			} else {
				resp.Header.Del("Mcp-Session-Id")
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			switch {
			case errors.Is(err, errEndpointClosed):
				http.Error(w, "launch ended", http.StatusUnauthorized)
				return
			case errors.Is(err, errSessionGone):
				http.Error(w, "session not found for this endpoint", http.StatusNotFound)
				return
			case errors.As(err, &needsAuth{}):
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}
			e.mu.Lock()
			e.fail(errors.New("upstream MCP unavailable"))
			if initial {
				forget()
			}
			e.mu.Unlock()
			http.Error(w, "upstream MCP unavailable", http.StatusBadGateway)
		},
		FlushInterval: -1,
	}
	proxy.ServeHTTP(w, r.WithContext(ctx))
	if r.Method == http.MethodDelete {
		e.mu.Lock()
		forget()
		e.mu.Unlock()
	}
}

func (e *Endpoint) remoteCredentials(
	ctx context.Context,
	inputs map[string]string,
	origin string,
) (string, map[string]string, error) {
	credential := ""
	switch id := e.definition.Auth.Secret; {
	case e.definition.Auth.Env != "" || e.definition.Auth.File != "":
		value, err := e.resolve(ctx, "mcp "+string(e.id)+" auth", e.definition.Auth.Value, inputs)
		if err != nil {
			return "", nil, needsAuth{err}
		}
		credential = value
	case id != "":
		secret := e.secrets[id]
		value, err := e.auth.Value(ctx, id, secret, e.secrets, inputs, origin)
		switch {
		case err == nil:
			credential = value
		case secret.Implicit && errors.Is(err, auth.ErrNoCredential):
			// On-demand OAuth sends no credential until the server asks for a login.
		case errors.Is(err, auth.ErrRefreshFailed):
			return "", nil, fmt.Errorf("mcp %s auth: %w", e.id, err)
		default:
			return "", nil, needsAuth{fmt.Errorf("mcp %s auth: %w", e.id, err)}
		}
	}
	headers := map[string]string{}
	for name, v := range e.definition.Headers {
		target := fmt.Sprintf("mcp %s header %s", e.id, name)
		value, err := e.resolve(ctx, target, v, inputs)
		if err != nil {
			return "", nil, err
		}
		if !validHeaderValue(value) {
			return "", nil, fmt.Errorf("%s: %s resolves to an invalid header value", target, v)
		}
		headers[name] = value
	}
	return credential, headers, nil
}

// unauthorized classifies an upstream 401 as needing authorization when the server has an auth
// secret. On-demand OAuth applies only once the server advertises OAuth discovery; otherwise,
// as for servers without auth, the 401 is an ordinary upstream failure and nil is returned.
func (e *Endpoint) unauthorized(ctx context.Context, credential, challenge string, refreshErr error) error {
	id := e.definition.Auth.Secret
	switch secret := e.secrets[id]; {
	case e.definition.Auth.Env != "" || e.definition.Auth.File != "":
		return needsAuth{fmt.Errorf("MCP server %s rejected the credential of %s", e.id, e.definition.Auth)}
	case id == "":
		return nil
	case secret.Kind != "oauth":
		return needsAuth{fmt.Errorf("MCP server %s rejected the credential of secret %s", e.id, id)}
	}
	secret := e.secrets[id]
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := e.auth.RequireLogin(ctx, id, secret, e.definition.URL, challenge)
	if err != nil && secret.Implicit && credential == "" {
		return nil
	}
	if credential == "" {
		return needsAuth{fmt.Errorf("MCP server %s requires login; run agx secret login %s", e.id, id)}
	}
	if refreshErr != nil {
		return needsAuth{fmt.Errorf("MCP server %s rejected its login: %w", e.id, refreshErr)}
	}
	return needsAuth{fmt.Errorf("MCP server %s rejected its login; run agx secret login %s", e.id, id)}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// validHeaderValue follows RFC 9110 field values: no control characters except tab.
func validHeaderValue(value string) bool {
	for i := 0; i < len(value); i++ {
		if c := value[i]; c < ' ' && c != '\t' || c == 0x7f {
			return false
		}
	}
	return true
}
