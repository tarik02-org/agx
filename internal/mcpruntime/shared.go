package mcpruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tarik02-org/agx/internal/config"
)

const (
	maxSharedClients = 64
	// sharedIdleTimeout keeps a shared server warm after its last client left, so the next
	// launch in the project finds its index and caches.
	sharedIdleTimeout  = 5 * time.Minute
	sharedStartTimeout = 30 * time.Second
	// sharedProtocol is the version agx asks a shared server for; the server may answer with
	// any version the stdio bridge supports.
	sharedProtocol = "2025-11-25"
)

var (
	errSharedStopped = errors.New("shared MCP server stopped")
	// logLevels are MCP's logging levels from the most to the least verbose.
	logLevels = []string{"debug", "info", "notice", "warning", "error", "critical", "alert", "emergency"}
)

func supportedProtocol(version string) bool {
	switch version {
	case "2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25":
		return true
	}
	return false
}

// Shared runs the stdio MCP servers whose definitions set share: one process per share key,
// which every matching harness session and agx mcp call joins as a client session
// (ADR 0024). A stdio server talks to exactly one client, so a shared server multiplexes:
// it owns the upstream session and maps each client's requests, progress, and
// subscriptions onto it.
type Shared struct {
	ctx     context.Context
	version string
	idle    time.Duration
	mu      sync.Mutex
	servers map[string]*sharedServer
	closed  bool
}

func NewShared(ctx context.Context, version string) *Shared {
	return &Shared{ctx: ctx, version: version, idle: sharedIdleTimeout, servers: map[string]*sharedServer{}}
}

// SetIdleTimeout changes how long a server without clients stays warm; for tests.
func (sh *Shared) SetIdleTimeout(d time.Duration) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.idle = d
}

// Servers counts the running shared servers.
func (sh *Shared) Servers() int {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	return len(sh.servers)
}

// Reset stops the shared servers using secret id, so their clients reconnect with the
// current credential.
func (sh *Shared) Reset(id config.ID) {
	for _, s := range sh.take(func(s *sharedServer) bool { return slices.Contains(s.secrets, id) }) {
		s.stop()
	}
}

// Close stops every shared server and waits until their processes exited; Kill ends those
// a Close in progress waits for.
func (sh *Shared) Close() {
	sh.mu.Lock()
	sh.closed = true
	sh.mu.Unlock()
	servers := sh.take(func(*sharedServer) bool { return true })
	for _, s := range servers {
		s.stop()
	}
	for _, s := range servers {
		<-s.exited
	}
}

func (sh *Shared) Kill() {
	sh.mu.Lock()
	sh.closed = true
	servers := []*sharedServer{}
	for _, s := range sh.servers {
		servers = append(servers, s)
	}
	sh.mu.Unlock()
	for _, s := range servers {
		s.kill()
	}
	sh.Close()
}

// Processes lists the running shared processes; Sessions counts their client sessions.
func (sh *Shared) Processes() []Process {
	sh.mu.Lock()
	servers := []*sharedServer{}
	for _, s := range sh.servers {
		servers = append(servers, s)
	}
	sh.mu.Unlock()
	processes := []Process{}
	for _, s := range servers {
		s.mu.Lock()
		if s.process != nil && !s.stopping {
			processes = append(processes, Process{
				ID:        s.id,
				PID:       s.process.pid,
				Directory: s.cwd,
				Share:     s.share,
				Root:      s.root,
				Sessions:  len(s.clients),
				Started:   s.process.started,
				StopsAt:   s.stopsAt,
			})
		}
		s.mu.Unlock()
	}
	return processes
}

// StopProcess stops the shared process with this process ID; its client sessions end. It
// reports whether one ran.
func (sh *Shared) StopProcess(pid int) bool {
	taken := sh.take(func(s *sharedServer) bool { return s.pid() == pid })
	for _, s := range taken {
		s.stop()
	}
	return len(taken) > 0
}

// take removes the servers match selects and marks them stopping, so no client joins them.
func (sh *Shared) take(match func(*sharedServer) bool) []*sharedServer {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	taken := []*sharedServer{}
	for key, s := range sh.servers {
		if match(s) {
			delete(sh.servers, key)
			s.mu.Lock()
			s.stopping = true
			s.mu.Unlock()
			taken = append(taken, s)
		}
	}
	return taken
}

// forget removes s from the registry once it stopped by itself.
func (sh *Shared) forget(s *sharedServer) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if sh.servers[s.key] == s {
		delete(sh.servers, s.key)
	}
}

// shareKey identifies the process a definition may share: the same server, root, command,
// environment, secret definitions, and environment inputs. Sources and descriptions are left
// out, so editing other lines of a configuration file keeps the process.
func shareKey(
	id config.ID,
	definition config.MCPServer,
	root string,
	secrets map[config.ID]config.Secret,
	inputs map[string]string,
) string {
	definition.Source, definition.Description, definition.Load = config.Source{}, "", ""
	used := map[config.ID]config.Secret{}
	pending := definition.Secrets()
	for len(pending) > 0 {
		secretID := pending[0]
		pending = pending[1:]
		if _, ok := used[secretID]; ok {
			continue
		}
		secret := secrets[secretID]
		secret.Source = config.Source{}
		used[secretID] = secret
		if secret.AgeKey.Secret != "" {
			pending = append(pending, secret.AgeKey.Secret)
		}
	}
	return config.Digest([]any{id, definition, root, used, inputs})
}

// join returns the shared server of e's share key, starting it with e's definition, secrets,
// and inputs when none runs. The caller attaches a client or calls unreserve.
func (sh *Shared) join(ctx context.Context, e *Endpoint, inputs map[string]string) (*sharedServer, error) {
	sh.mu.Lock()
	if sh.closed {
		sh.mu.Unlock()
		return nil, errEndpointClosed
	}
	s := sh.servers[e.shareKey]
	if s == nil {
		idle := sh.idle
		if configured := e.definition.Share.Idle; configured != nil {
			idle = *configured
		}
		s = &sharedServer{
			shared:        sh,
			key:           e.shareKey,
			id:            e.id,
			root:          e.shareRoot,
			share:         e.definition.Share.Type,
			cwd:           e.definition.Cwd,
			secrets:       e.definition.Secrets(),
			idle:          idle,
			ready:         make(chan struct{}),
			exited:        make(chan struct{}),
			clients:       map[*sharedClient]bool{},
			pending:       map[jsonrpc.ID]*pendingCall{},
			progress:      map[string]progressRoute{},
			subscriptions: map[string]int{},
		}
		sh.servers[s.key] = s
		go s.start(e, inputs)
	}
	s.mu.Lock()
	s.reserved++
	s.mu.Unlock()
	sh.mu.Unlock()
	select {
	case <-s.ready:
	case <-ctx.Done():
		s.unreserve()
		return nil, ctx.Err()
	}
	if s.err != nil {
		s.unreserve()
		return nil, s.err
	}
	return s, nil
}

// sharedServer is one upstream process and the client sessions multiplexed onto it.
type sharedServer struct {
	shared  *Shared
	key     string
	id      config.ID
	root    string
	share   string
	cwd     string
	secrets []config.ID
	idle    time.Duration
	// ready closes once the process started and initialized; err then says whether it failed.
	ready chan struct{}
	err   error
	// exited closes once the process exited, or failed to start.
	exited chan struct{}
	// process is set once it started; conn is used only after that.
	process *process
	// initialize is the upstream initialize result, given to every client.
	initialize map[string]json.RawMessage

	mu            sync.Mutex
	clients       map[*sharedClient]bool
	reserved      int
	stopping      bool
	idleTimer     *time.Timer
	stopsAt       time.Time
	next          uint64
	pending       map[jsonrpc.ID]*pendingCall
	progress      map[string]progressRoute
	subscriptions map[string]int
	level         string
}

// pendingCall is a request sent upstream: a client's, answered to client under its own ID, or
// agx's own when client is nil. after runs with the response before it is delivered.
type pendingCall struct {
	client   *sharedClient
	id       jsonrpc.ID
	progress string
	after    func(*jsonrpc.Response)
}

type progressRoute struct {
	client *sharedClient
	token  json.RawMessage
}

func (s *sharedServer) start(e *Endpoint, inputs map[string]string) {
	ctx, cancel := context.WithTimeout(s.shared.ctx, sharedStartTimeout)
	defer cancel()
	p, err := e.startProcess(s.shared.ctx, ctx, inputs)
	if err != nil {
		s.err = err
		close(s.exited)
		close(s.ready)
		s.shared.forget(s)
		return
	}
	s.mu.Lock()
	s.process = p
	stopping := s.stopping
	s.mu.Unlock()
	if stopping {
		// The daemon stopped, or a secret changed, while the process started.
		p.cancel()
	}
	go s.readUpstream()
	go func() {
		<-p.exited
		s.ended()
	}()
	capabilities := map[string]any{}
	if s.root != "" {
		capabilities["roots"] = map[string]any{}
	}
	result, err := s.call(ctx, "initialize", map[string]any{
		"protocolVersion": sharedProtocol,
		"capabilities":    capabilities,
		"clientInfo":      map[string]any{"name": "agx", "version": s.shared.version},
	})
	if err == nil {
		err = json.Unmarshal(result, &s.initialize)
	}
	if err == nil {
		var version string
		_ = json.Unmarshal(s.initialize["protocolVersion"], &version)
		if !supportedProtocol(version) {
			err = fmt.Errorf("MCP server answered with unsupported protocol version %q", version)
		}
	}
	if err == nil {
		err = s.notify("notifications/initialized", map[string]any{})
	}
	if err != nil {
		s.err = fmt.Errorf("shared MCP server did not initialize: %w", err)
		s.shared.forget(s)
		s.stop()
	}
	close(s.ready)
}

// call sends agx's own request upstream and waits for its result.
func (s *sharedServer) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	answer := make(chan *jsonrpc.Response, 1)
	if err := s.send(&jsonrpc.Request{Method: method, Params: data}, &pendingCall{
		after: func(r *jsonrpc.Response) { answer <- r },
	}); err != nil {
		return nil, err
	}
	select {
	case r := <-answer:
		if r.Error != nil {
			return nil, r.Error
		}
		return r.Result, nil
	case <-s.exited:
		return nil, errors.New("process exited")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *sharedServer) notify(method string, params any) error {
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return s.process.conn.Write(s.shared.ctx, &jsonrpc.Request{Method: method, Params: data})
}

// send writes request upstream under a new ID, which call answers to.
func (s *sharedServer) send(request *jsonrpc.Request, call *pendingCall) error {
	s.mu.Lock()
	s.next++
	id, _ := jsonrpc.MakeID(fmt.Sprintf("agx-%d", s.next))
	s.pending[id] = call
	s.mu.Unlock()
	request.ID = id
	if err := s.process.conn.Write(s.shared.ctx, request); err != nil {
		s.mu.Lock()
		delete(s.pending, id)
		if call.progress != "" {
			delete(s.progress, call.progress)
		}
		s.mu.Unlock()
		return err
	}
	return nil
}

// stop starts the process's shutdown: its input closes, then it is terminated and killed.
func (s *sharedServer) stop() {
	s.mu.Lock()
	s.stopping = true
	if s.idleTimer != nil {
		s.idleTimer.Stop()
	}
	p := s.process
	s.mu.Unlock()
	if p != nil {
		p.cancel()
	}
}

func (s *sharedServer) kill() {
	s.stop()
	s.mu.Lock()
	p := s.process
	s.mu.Unlock()
	if p != nil {
		p.kill()
	}
}

// ended runs once the process exited: requests in flight fail and every client session ends,
// so harnesses initialize again and start a new process.
func (s *sharedServer) ended() {
	s.shared.forget(s)
	s.mu.Lock()
	s.stopping = true
	clients := []*sharedClient{}
	for c := range s.clients {
		clients = append(clients, c)
	}
	failed := []*pendingCall{}
	for id, call := range s.pending {
		delete(s.pending, id)
		if call.client != nil {
			failed = append(failed, call)
		}
	}
	s.mu.Unlock()
	close(s.exited)
	for _, call := range failed {
		call.client.fail(call.id, jsonrpc.CodeInternalError, "shared MCP server exited")
	}
	for _, c := range clients {
		c.end()
	}
}

// pid is the process's ID, or 0 before it started.
func (s *sharedServer) pid() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.process == nil {
		return 0
	}
	return s.process.pid
}

func (s *sharedServer) unreserve() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reserved--
	s.idleLocked()
}

// idleLocked arms the idle timer once no client uses or is about to join s.
func (s *sharedServer) idleLocked() {
	if len(s.clients) > 0 || s.reserved > 0 || s.stopping {
		return
	}
	if s.idleTimer != nil {
		s.idleTimer.Stop()
	}
	s.stopsAt = time.Now().Add(s.idle)
	s.idleTimer = time.AfterFunc(s.idle, func() {
		taken := s.shared.take(func(candidate *sharedServer) bool {
			if candidate != s {
				return false
			}
			candidate.mu.Lock()
			defer candidate.mu.Unlock()
			return len(candidate.clients) == 0 && candidate.reserved == 0
		})
		for _, t := range taken {
			t.stop()
		}
	})
}

// attach adds a client session reading from and writing to down, for a reservation join made.
func (s *sharedServer) attach(ctx context.Context, down mcp.Connection) (*sharedClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reserved--
	switch {
	case s.stopping:
		s.idleLocked()
		return nil, errSharedStopped
	case len(s.clients) >= maxSharedClients:
		s.idleLocked()
		return nil, errors.New("too many sessions for this shared MCP server")
	}
	if s.idleTimer != nil {
		s.idleTimer.Stop()
		s.idleTimer = nil
	}
	s.stopsAt = time.Time{}
	c := &sharedClient{
		server:        s,
		ctx:           ctx,
		down:          down,
		subscriptions: map[string]bool{},
		ended:         make(chan struct{}),
	}
	s.clients[c] = true
	go c.read()
	return c, nil
}

func (s *sharedServer) clientsLocked() []*sharedClient {
	clients := []*sharedClient{}
	for c := range s.clients {
		clients = append(clients, c)
	}
	return clients
}

func (s *sharedServer) readUpstream() {
	for {
		message, err := s.process.conn.Read(s.shared.ctx)
		if err != nil {
			s.stop()
			return
		}
		switch m := message.(type) {
		case *jsonrpc.Response:
			s.answered(m)
		case *jsonrpc.Request:
			if m.IsCall() {
				s.serverRequest(m)
			} else {
				s.serverNotification(m)
			}
		}
	}
}

func (s *sharedServer) answered(r *jsonrpc.Response) {
	s.mu.Lock()
	call := s.pending[r.ID]
	delete(s.pending, r.ID)
	if call != nil && call.progress != "" {
		delete(s.progress, call.progress)
	}
	s.mu.Unlock()
	if call == nil {
		return
	}
	if call.after != nil {
		call.after(r)
	}
	if call.client != nil {
		call.client.write(&jsonrpc.Response{ID: call.id, Result: r.Result, Error: r.Error})
	}
}

// serverRequest answers what the server asks agx: no single client owns the session, so
// sampling, elicitation, and the like are refused rather than handed to an arbitrary client.
func (s *sharedServer) serverRequest(r *jsonrpc.Request) {
	response := &jsonrpc.Response{ID: r.ID}
	switch {
	case r.Method == "ping":
		response.Result = json.RawMessage(`{}`)
	case r.Method == "roots/list" && s.root != "":
		root := url.URL{Scheme: "file", Path: filepath.ToSlash(s.root)}
		if filepath.VolumeName(s.root) != "" {
			root.Path = "/" + root.Path
		}
		result, _ := json.Marshal(map[string]any{
			"roots": []map[string]string{{"uri": root.String(), "name": filepath.Base(s.root)}},
		})
		response.Result = result
	default:
		response.Error = &jsonrpc.Error{
			Code:    jsonrpc.CodeMethodNotFound,
			Message: fmt.Sprintf("%s is not supported: MCP server %s is shared by several sessions", r.Method, s.id),
		}
	}
	_ = s.process.conn.Write(s.shared.ctx, response)
}

func (s *sharedServer) serverNotification(r *jsonrpc.Request) {
	switch r.Method {
	case "notifications/progress":
		var params map[string]json.RawMessage
		if json.Unmarshal(r.Params, &params) != nil {
			return
		}
		s.mu.Lock()
		route, ok := s.progress[string(params["progressToken"])]
		s.mu.Unlock()
		if !ok {
			return
		}
		params["progressToken"] = route.token
		data, _ := json.Marshal(params)
		route.client.write(&jsonrpc.Request{Method: r.Method, Params: data})
	case "notifications/tools/list_changed",
		"notifications/resources/list_changed",
		"notifications/prompts/list_changed":
		s.mu.Lock()
		clients := s.clientsLocked()
		s.mu.Unlock()
		for _, c := range clients {
			c.write(r)
		}
	case "notifications/resources/updated":
		var params struct {
			URI string `json:"uri"`
		}
		_ = json.Unmarshal(r.Params, &params)
		s.mu.Lock()
		clients := []*sharedClient{}
		for c := range s.clients {
			if c.subscriptions[params.URI] {
				clients = append(clients, c)
			}
		}
		s.mu.Unlock()
		for _, c := range clients {
			c.write(r)
		}
	case "notifications/message":
		var params struct {
			Level string `json:"level"`
		}
		_ = json.Unmarshal(r.Params, &params)
		s.mu.Lock()
		clients := []*sharedClient{}
		for c := range s.clients {
			if c.level == "" || slices.Index(logLevels, params.Level) >= slices.Index(logLevels, c.level) {
				clients = append(clients, c)
			}
		}
		s.mu.Unlock()
		for _, c := range clients {
			c.write(r)
		}
	}
}

// updateLevelLocked runs upstream at the most verbose level any client asked for.
func (s *sharedServer) updateLevelLocked() {
	level := ""
	for c := range s.clients {
		if c.level != "" && (level == "" || slices.Index(logLevels, c.level) < slices.Index(logLevels, level)) {
			level = c.level
		}
	}
	if level == "" || level == s.level {
		return
	}
	s.level = level
	params, _ := json.Marshal(map[string]string{"level": level})
	go func() { _ = s.send(&jsonrpc.Request{Method: "logging/setLevel", Params: params}, &pendingCall{}) }()
}

// sharedClient is a harness session, or agx's own client session, on a shared server.
type sharedClient struct {
	server *sharedServer
	ctx    context.Context
	down   mcp.Connection
	ended  chan struct{}
	once   sync.Once
	// Guarded by server.mu.
	initialized   bool
	level         string
	subscriptions map[string]bool
}

// Ended closes once the client session left its server, by detaching or because the server
// stopped.
func (c *sharedClient) Ended() <-chan struct{} { return c.ended }

func (c *sharedClient) write(message jsonrpc.Message) {
	// Delivery is best effort, as for unshared servers: a client without an open stream
	// misses notifications, and a response whose request stream disconnected is dropped.
	_ = c.down.Write(c.ctx, message)
}

func (c *sharedClient) reply(id jsonrpc.ID, result any) {
	data, err := json.Marshal(result)
	if err != nil {
		c.fail(id, jsonrpc.CodeInternalError, err.Error())
		return
	}
	c.write(&jsonrpc.Response{ID: id, Result: data})
}

func (c *sharedClient) fail(id jsonrpc.ID, code int64, message string) {
	c.write(&jsonrpc.Response{ID: id, Error: &jsonrpc.Error{Code: code, Message: message}})
}

func (c *sharedClient) read() {
	defer c.end()
	for {
		message, err := c.down.Read(c.ctx)
		if err != nil {
			return
		}
		// Responses would answer server requests, which shared servers never pass to clients.
		if r, ok := message.(*jsonrpc.Request); ok {
			c.handle(r)
		}
	}
}

func (c *sharedClient) handle(r *jsonrpc.Request) {
	s := c.server
	if !r.IsCall() {
		// The client's initialized, roots, and progress notifications concern its own
		// session, which agx answers for; only cancellation reaches upstream.
		if r.Method == "notifications/cancelled" {
			c.cancel(r)
		}
		return
	}
	switch r.Method {
	case "initialize":
		c.initializeSession(r)
	case "ping":
		c.reply(r.ID, map[string]any{})
	case "logging/setLevel":
		var params struct {
			Level string `json:"level"`
		}
		if json.Unmarshal(r.Params, &params) != nil || !slices.Contains(logLevels, params.Level) {
			c.fail(r.ID, jsonrpc.CodeInvalidParams, "unknown logging level")
			return
		}
		s.mu.Lock()
		c.level = params.Level
		s.updateLevelLocked()
		s.mu.Unlock()
		c.reply(r.ID, map[string]any{})
	case "resources/subscribe":
		c.subscribe(r)
	case "resources/unsubscribe":
		c.unsubscribe(r)
	case "tools/list", "tools/call",
		"resources/list", "resources/templates/list", "resources/read",
		"prompts/list", "prompts/get",
		"completion/complete":
		c.forward(r, nil)
	default:
		c.fail(r.ID, jsonrpc.CodeMethodNotFound, "method not supported for shared MCP servers")
	}
}

// initializeSession answers with the upstream result at the version the client asked for;
// the bridge checked the client's version when the session started.
func (c *sharedClient) initializeSession(r *jsonrpc.Request) {
	s := c.server
	s.mu.Lock()
	again := c.initialized
	c.initialized = true
	s.mu.Unlock()
	if again {
		c.fail(r.ID, jsonrpc.CodeInvalidRequest, "session already initialized")
		return
	}
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(r.Params, &params)
	result := map[string]json.RawMessage{}
	for key, value := range s.initialize {
		result[key] = value
	}
	if supportedProtocol(params.ProtocolVersion) {
		result["protocolVersion"], _ = json.Marshal(params.ProtocolVersion)
	}
	c.reply(r.ID, result)
}

// forward sends a client request upstream under a new ID, its progress token too, and
// answers the client under its own ID.
func (c *sharedClient) forward(r *jsonrpc.Request, after func(*jsonrpc.Response)) {
	s := c.server
	call := &pendingCall{client: c, id: r.ID, after: after}
	params := r.Params
	var fields map[string]json.RawMessage
	if json.Unmarshal(r.Params, &fields) == nil {
		var meta map[string]json.RawMessage
		if json.Unmarshal(fields["_meta"], &meta) == nil && len(meta["progressToken"]) > 0 {
			s.mu.Lock()
			s.next++
			token, _ := json.Marshal(fmt.Sprintf("agx-progress-%d", s.next))
			s.progress[string(token)] = progressRoute{client: c, token: meta["progressToken"]}
			s.mu.Unlock()
			call.progress = string(token)
			meta["progressToken"] = token
			fields["_meta"], _ = json.Marshal(meta)
			params, _ = json.Marshal(fields)
		}
	}
	if err := s.send(&jsonrpc.Request{Method: r.Method, Params: params}, call); err != nil {
		c.fail(r.ID, jsonrpc.CodeInternalError, "shared MCP server unavailable")
	}
}

// cancel forwards the client's cancellation of one of its own requests.
func (c *sharedClient) cancel(r *jsonrpc.Request) {
	var params map[string]json.RawMessage
	if json.Unmarshal(r.Params, &params) != nil {
		return
	}
	var raw any
	if json.Unmarshal(params["requestId"], &raw) != nil {
		return
	}
	requestID, err := jsonrpc.MakeID(raw)
	if err != nil {
		return
	}
	s := c.server
	s.mu.Lock()
	var upstream jsonrpc.ID
	for id, call := range s.pending {
		if call.client == c && call.id == requestID {
			upstream = id
			break
		}
	}
	s.mu.Unlock()
	if !upstream.IsValid() {
		return
	}
	params["requestId"], _ = json.Marshal(upstream.Raw())
	data, _ := json.Marshal(params)
	_ = s.process.conn.Write(s.shared.ctx, &jsonrpc.Request{Method: r.Method, Params: data})
}

func subscriptionURI(r *jsonrpc.Request) (string, bool) {
	var params struct {
		URI string `json:"uri"`
	}
	if json.Unmarshal(r.Params, &params) != nil || params.URI == "" {
		return "", false
	}
	return params.URI, true
}

// subscribe subscribes upstream for the first client of a URI only, and undoes the count
// when upstream refuses.
func (c *sharedClient) subscribe(r *jsonrpc.Request) {
	uri, ok := subscriptionURI(r)
	if !ok {
		c.fail(r.ID, jsonrpc.CodeInvalidParams, "uri required")
		return
	}
	s := c.server
	s.mu.Lock()
	if c.subscriptions[uri] {
		s.mu.Unlock()
		c.reply(r.ID, map[string]any{})
		return
	}
	c.subscriptions[uri] = true
	s.subscriptions[uri]++
	first := s.subscriptions[uri] == 1
	s.mu.Unlock()
	if !first {
		c.reply(r.ID, map[string]any{})
		return
	}
	c.forward(r, func(response *jsonrpc.Response) {
		if response.Error == nil {
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if c.subscriptions[uri] {
			delete(c.subscriptions, uri)
			s.dropSubscriptionLocked(uri)
		}
	})
}

func (c *sharedClient) unsubscribe(r *jsonrpc.Request) {
	uri, ok := subscriptionURI(r)
	if !ok {
		c.fail(r.ID, jsonrpc.CodeInvalidParams, "uri required")
		return
	}
	s := c.server
	s.mu.Lock()
	last := false
	if c.subscriptions[uri] {
		delete(c.subscriptions, uri)
		s.subscriptions[uri]--
		if s.subscriptions[uri] <= 0 {
			delete(s.subscriptions, uri)
			last = true
		}
	}
	s.mu.Unlock()
	if last {
		c.forward(r, nil)
		return
	}
	c.reply(r.ID, map[string]any{})
}

// dropSubscriptionLocked releases one client's subscription to uri, unsubscribing upstream
// when it was the last.
func (s *sharedServer) dropSubscriptionLocked(uri string) {
	s.subscriptions[uri]--
	if s.subscriptions[uri] > 0 {
		return
	}
	delete(s.subscriptions, uri)
	params, _ := json.Marshal(map[string]string{"uri": uri})
	go func() { _ = s.send(&jsonrpc.Request{Method: "resources/unsubscribe", Params: params}, &pendingCall{}) }()
}

// end detaches the client: its requests in flight are forgotten, its subscriptions and
// logging level released, and the server goes idle once no client is left.
func (c *sharedClient) end() {
	c.once.Do(func() {
		s := c.server
		s.mu.Lock()
		delete(s.clients, c)
		for id, call := range s.pending {
			if call.client == c {
				call.client = nil
				if call.progress != "" {
					delete(s.progress, call.progress)
					call.progress = ""
				}
				s.pending[id] = call
			}
		}
		for uri := range c.subscriptions {
			s.dropSubscriptionLocked(uri)
		}
		c.subscriptions = map[string]bool{}
		c.level = ""
		s.updateLevelLocked()
		s.idleLocked()
		s.mu.Unlock()
		close(c.ended)
	})
}
