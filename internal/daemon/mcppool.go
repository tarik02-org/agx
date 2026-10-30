package daemon

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tarik02-org/agx/internal/control"
	"github.com/tarik02-org/agx/internal/mcpruntime"
)

// pooledIdleTimeout closes a pooled client session nobody used for this long, well before the
// endpoint's own idle expiry.
const pooledIdleTimeout = 5 * time.Minute

// clientPool keeps the daemon's own MCP client sessions, one per launch, or CLI scope, and
// server, so repeated calls neither respawn stdio servers nor initialize again.
type clientPool struct {
	mu      sync.Mutex
	clients map[string]*pooledClient
	closed  bool
}

type pooledClient struct {
	key string
	// launchID is the launch whose endpoint the session uses; empty for a CLI scope, whose
	// endpoint the session owns.
	launchID string
	// ready closes once opening finished; err then says whether it failed.
	ready    chan struct{}
	err      error
	session  *mcp.ClientSession
	endpoint *mcpruntime.Endpoint
	close    func()
	active   int
	lastUsed time.Time
}

func newClientPool() *clientPool {
	return &clientPool{clients: map[string]*pooledClient{}}
}

// opener opens a session for a pooled client; ctx bounds only the opening. close ends the
// session and whatever it owns.
type opener func(ctx context.Context) (session *mcp.ClientSession, endpoint *mcpruntime.Endpoint, close func(), err error)

// acquire returns the open client of key, opening it with open, which runs under base so that
// a caller giving up does not abandon a session others wait for. The caller releases it.
func (p *clientPool) acquire(
	ctx, base context.Context,
	key, launchID string,
	open opener,
) (*pooledClient, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, refusal()
	}
	c := p.clients[key]
	if c == nil {
		c = &pooledClient{key: key, launchID: launchID, ready: make(chan struct{})}
		p.clients[key] = c
		go func() {
			openCtx, cancel := context.WithTimeout(base, 30*time.Second)
			defer cancel()
			session, endpoint, closer, err := open(openCtx)
			p.mu.Lock()
			c.session, c.endpoint, c.close, c.err = session, endpoint, closer, err
			if err == nil && p.clients[key] != c {
				// The pool closed, or the launch ended, while the session opened.
				go closer()
			}
			if err != nil && p.clients[key] == c {
				delete(p.clients, key)
			}
			c.lastUsed = time.Now()
			p.mu.Unlock()
			close(c.ready)
		}()
	}
	c.active++
	p.mu.Unlock()
	select {
	case <-c.ready:
	case <-ctx.Done():
		p.release(c)
		return nil, ctx.Err()
	}
	if c.err != nil {
		p.release(c)
		return nil, c.err
	}
	return c, nil
}

func (p *clientPool) release(c *pooledClient) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c.active--
	c.lastUsed = time.Now()
}

// discard closes c once it broke, so the next caller opens a new session.
func (p *clientPool) discard(c *pooledClient) {
	p.mu.Lock()
	if p.clients[c.key] == c {
		delete(p.clients, c.key)
		go c.close()
	}
	p.mu.Unlock()
}

// expire closes sessions idle for pooledIdleTimeout.
func (p *clientPool) expire(now time.Time) {
	for _, closer := range p.take(func(c *pooledClient) bool {
		return c.session != nil && c.active == 0 && now.Sub(c.lastUsed) > pooledIdleTimeout
	}) {
		go closer()
	}
}

// closeLaunch closes the sessions on a launch's endpoints once it ended.
func (p *clientPool) closeLaunch(id string) {
	for _, closer := range p.take(func(c *pooledClient) bool { return c.launchID == id }) {
		go closer()
	}
}

// closeAll closes every session, waiting until they closed, and opens none from then on.
func (p *clientPool) closeAll() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	p.reset()
}

func (p *clientPool) reset() {
	var wg sync.WaitGroup
	for _, closer := range p.take(func(*pooledClient) bool { return true }) {
		wg.Go(closer)
	}
	wg.Wait()
}

// take removes the clients match selects and returns the closers of those already open; one
// still opening closes once it opened.
func (p *clientPool) take(match func(*pooledClient) bool) []func() {
	p.mu.Lock()
	closers := []func(){}
	for key, c := range p.clients {
		if match(c) {
			delete(p.clients, key)
			if c.session != nil {
				closers = append(closers, c.close)
			}
		}
	}
	p.mu.Unlock()
	return closers
}

// connectHandler opens an MCP client session to handler over in-memory connections, so no other
// process can reach the endpoint it serves.
func connectHandler(ctx context.Context, handler http.Handler) (*mcp.ClientSession, func(), error) {
	listener := newPipeListener()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = server.Serve(listener) }()
	transport := &http.Transport{DialContext: listener.dial, ForceAttemptHTTP2: false}
	client := mcp.NewClient(&mcp.Implementation{Name: "agx", Version: control.Version}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             "http://agx.invalid/",
		HTTPClient:           &http.Client{Transport: transport},
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		_ = server.Close()
		transport.CloseIdleConnections()
		return nil, nil, err
	}
	return session, func() {
		_ = session.Close()
		_ = server.Close()
		transport.CloseIdleConnections()
	}, nil
}

// pipeListener accepts the server ends of in-memory connections its dial creates.
type pipeListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{conns: make(chan net.Conn), done: make(chan struct{})}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr{} }

func (l *pipeListener) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	client, server := net.Pipe()
	select {
	case l.conns <- server:
		return client, nil
	case <-l.done:
	case <-ctx.Done():
	}
	_ = client.Close()
	_ = server.Close()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return nil, net.ErrClosed
}

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "agx" }
