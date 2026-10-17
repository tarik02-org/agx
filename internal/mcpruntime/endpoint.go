package mcpruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tarik02/weft/internal/auth"
	"github.com/tarik02/weft/internal/config"
	"github.com/tarik02/weft/internal/platform"
	"github.com/tarik02/weft/internal/securefs"
)

const (
	maxSessions = 8
	// Harnesses rarely DELETE their sessions, so abandoned ones would otherwise
	// hold one of the session slots and, for stdio, a child process forever.
	sessionIdleTimeout = 30 * time.Minute
)

var (
	errEndpointClosed = errors.New("launch ended")
	errSessionGone    = errors.New("session ended")
	errNeedsAuth      = errors.New("upstream credential unavailable")
)

type Endpoint struct {
	mu             sync.Mutex
	ctx            context.Context
	cancel         context.CancelFunc
	definition     config.Connection
	auth           *auth.Manager
	sources        map[config.ID]config.Auth
	inputs         map[string]string
	sessions       map[string]*stdioSession
	starting       int
	remoteSessions map[string]*remoteSession
	generation     uint64
	closeDone      chan struct{}
	closed         bool
	state          string
}

type stdioSession struct {
	transport  *mcp.StreamableServerTransport
	downstream mcp.Connection
	upstream   mcp.Connection
	cancel     context.CancelFunc
	kill       func()
	done       sync.Once
	ended      chan struct{}
	active     int
	lastUsed   time.Time
}

type remoteSession struct {
	upstreamID string
	credential string
	active     int
	lastUsed   time.Time
}

func New(
	ctx context.Context,
	c config.Connection,
	sources map[config.ID]config.Auth,
	inputs map[string]string,
	manager *auth.Manager,
) *Endpoint {
	ctx, cancel := context.WithCancel(ctx)
	e := &Endpoint{
		ctx:            ctx,
		cancel:         cancel,
		definition:     c,
		sources:        sources,
		inputs:         inputs,
		auth:           manager,
		sessions:       map[string]*stdioSession{},
		remoteSessions: map[string]*remoteSession{},
		closeDone:      make(chan struct{}),
		state:          "connecting",
	}
	go e.expireIdle()
	return e
}

func (e *Endpoint) State() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state
}

func (e *Endpoint) Close() {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		<-e.closeDone
		return
	}
	e.closed = true
	e.state = "failed"
	e.cancel()
	e.inputs = nil
	stdio, remote := e.takeSessionsLocked(func(int, time.Time) bool { return true })
	e.mu.Unlock()
	defer close(e.closeDone)
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
	e.state = "connecting"
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
			e.deleteUpstream(s.upstreamID, s.credential)
		}()
	}
	wg.Wait()
}

func (e *Endpoint) deleteUpstream(upstreamID, credential string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, e.definition.URL, nil)
	if err != nil {
		return
	}
	req.Header.Set("Mcp-Session-Id", upstreamID)
	for name, value := range e.definition.Headers {
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

func (s *stdioSession) close() {
	s.done.Do(func() { s.cancel(); s.kill(); _ = s.upstream.Close(); _ = s.downstream.Close(); close(s.ended) })
}

func (e *Endpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost && r.Method != http.MethodGet && r.Method != http.MethodDelete {
		w.Header().Set("Allow", "POST, GET, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("Origin") != "" {
		u, err := url.Parse(r.Header.Get("Origin"))
		if err != nil || u.Host != r.Host {
			http.Error(w, "invalid origin", http.StatusForbidden)
			return
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024*1024)
	if r.Method == http.MethodPost {
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		if !strings.Contains(r.Header.Get("Accept"), "application/json") ||
			!strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			http.Error(w, "Accept must contain application/json and text/event-stream", http.StatusNotAcceptable)
			return
		}
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Second))
		defer func() { _ = http.NewResponseController(w).SetReadDeadline(time.Time{}) }()
	}
	if e.definition.Transport == "http" {
		e.serveRemote(w, r)
		return
	}
	e.serveStdio(w, r)
}

func initialRequest(r *http.Request) ([]byte, error) {
	if r.Method != http.MethodPost {
		return nil, fmt.Errorf("session ID required")
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return nil, fmt.Errorf("Content-Type must be application/json")
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	var msg struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("invalid initialization request")
	}
	if msg.JSONRPC != "2.0" || msg.Method != "initialize" || len(msg.ID) == 0 || string(msg.ID) == "null" {
		return nil, fmt.Errorf("expected initialize request with an ID")
	}
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if json.Unmarshal(msg.Params, &params) != nil || params.ProtocolVersion == "" {
		return nil, fmt.Errorf("missing protocolVersion")
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	return data, nil
}

func (e *Endpoint) serveStdio(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get("Mcp-Session-Id")
	var s *stdioSession
	if id == "" {
		initial, err := initialRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var init struct {
			Params struct {
				ProtocolVersion string `json:"protocolVersion"`
			} `json:"params"`
		}
		_ = json.Unmarshal(initial, &init)
		switch init.Params.ProtocolVersion {
		case "2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25":
		default:
			http.Error(w, "stdio bridge supports MCP through 2025-11-25", http.StatusBadRequest)
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
	accept := r.Header.Get("Accept")
	if r.Method == http.MethodPost &&
		(!strings.Contains(accept, "application/json") || !strings.Contains(accept, "text/event-stream")) {
		http.Error(w, "Accept must contain application/json and text/event-stream", http.StatusNotAcceptable)
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
		http.Error(w, "too many sessions for this connection", http.StatusTooManyRequests)
		return "", nil
	}
	e.starting++
	generation := e.generation
	inputs := e.inputs
	e.mu.Unlock()
	go e.release(idleStdio, idleRemote)

	id := securefs.Token()
	s, err := e.startStdio(id, inputs)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.starting--
	if err != nil {
		if e.closed {
			http.Error(w, "launch ended", http.StatusUnauthorized)
			return "", nil
		}
		if errors.Is(err, errNeedsAuth) {
			e.state = "needs_auth"
		} else {
			e.state = "failed"
		}
		http.Error(
			w,
			"upstream connection failed; check connection command and credential source",
			http.StatusBadGateway,
		)
		return "", nil
	}
	select {
	case <-s.ended:
		if !e.closed {
			e.state = "failed"
		}
		http.Error(
			w,
			"upstream connection failed; check connection command and credential source",
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
			http.Error(w, "connection was reset; retry", http.StatusServiceUnavailable)
		}
		return "", nil
	}
	s.active = 1
	s.lastUsed = time.Now()
	e.sessions[id] = s
	e.state = "ready"
	return id, s
}

func (e *Endpoint) startStdio(id string, inputs map[string]string) (*stdioSession, error) {
	ctx, cancel := context.WithCancel(e.ctx)
	cmd := exec.CommandContext(ctx, e.definition.Command[0], e.definition.Command[1:]...)
	cmd.Dir = e.definition.Cwd
	cmd.Env = auth.SystemEnvironment()
	for key, value := range e.definition.Environment {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	for key, authID := range e.definition.AuthEnvironment {
		value, err := e.auth.Value(ctx, authID, e.sources[authID], inputs, "")
		if err != nil {
			cancel()
			return nil, fmt.Errorf("%w: %w", errNeedsAuth, err)
		}
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	platform.IsolateOwned(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		cancel()
		return nil, err
	}
	// Upstream stderr is deliberately not copied: it can contain credentials.
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		cancel()
		return nil, err
	}
	kill, err := platform.Own(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = stdin.Close()
		_ = stdout.Close()
		cancel()
		return nil, err
	}
	up, err := (&mcp.IOTransport{Reader: stdout, Writer: stdin}).Connect(ctx)
	if err != nil {
		kill()
		_ = cmd.Wait()
		cancel()
		return nil, err
	}
	t := &mcp.StreamableServerTransport{SessionID: id}
	down, err := t.Connect(ctx)
	if err != nil {
		kill()
		_ = up.Close()
		_ = cmd.Wait()
		cancel()
		return nil, err
	}
	s := &stdioSession{
		transport:  t,
		downstream: down,
		upstream:   up,
		cancel:     cancel,
		kill:       kill,
		ended:      make(chan struct{}),
	}
	relay := func(from, to mcp.Connection) {
		defer s.close()
		for {
			message, err := from.Read(ctx)
			if err != nil {
				return
			}
			if err := to.Write(ctx, message); err != nil {
				return
			}
		}
	}
	go relay(down, up)
	go relay(up, down)
	go func() {
		_ = cmd.Wait()
		s.close()
		e.mu.Lock()
		if e.sessions[id] == s {
			delete(e.sessions, id)
		}
		if !e.closed {
			e.state = "connecting"
		}
		e.mu.Unlock()
	}()
	return s, nil
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
	credential := ""
	if id := e.definition.Auth; id != "" {
		value, err := e.auth.Value(r.Context(), id, e.sources[id], inputs, origin)
		if err != nil {
			e.mu.Lock()
			if !e.closed {
				e.state = "needs_auth"
			}
			if initial {
				forget()
			}
			e.mu.Unlock()
			http.Error(w, "upstream credential unavailable; use weft auth", http.StatusUnauthorized)
			return
		}
		credential = value
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stop := context.AfterFunc(e.ctx, cancel)
	defer stop()
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = target.Host
			pr.Out.Header.Del("Authorization")
			pr.Out.Header.Del("Proxy-Authorization")
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("Origin")
			pr.Out.Header.Del("Mcp-Session-Id")
			for name, value := range e.definition.Headers {
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
			success := resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
			e.mu.Lock()
			defer e.mu.Unlock()
			if !current() {
				// The session was closed while initialize was in flight; end the
				// upstream session it created so it is not leaked.
				if initial && success {
					if id := resp.Header.Get("Mcp-Session-Id"); id != "" {
						go e.deleteUpstream(id, credential)
					}
				}
				if e.closed {
					return errEndpointClosed
				}
				return errSessionGone
			}
			switch {
			case initial && success:
				session.upstreamID = resp.Header.Get("Mcp-Session-Id")
				session.credential = credential
				e.state = "ready"
			case initial:
				forget()
				e.state = "failed"
			case success:
				session.credential = credential
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
			}
			e.mu.Lock()
			if !e.closed {
				e.state = "failed"
			}
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
