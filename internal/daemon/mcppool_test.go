package daemon

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tarik02-org/agx/internal/mcpruntime"
)

// poolOpener opens real sessions to an in-process MCP server and counts opens and closes.
func poolOpener(t *testing.T, opened, closed *atomic.Int32) opener {
	server := mcp.NewServer(&mcp.Implementation{Name: "pool-test", Version: "1"}, nil)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	return func(ctx context.Context) (*mcp.ClientSession, *mcpruntime.Endpoint, func(), error) {
		session, closer, err := connectHandler(ctx, handler)
		if err != nil {
			t.Error(err)
			return nil, nil, nil, err
		}
		opened.Add(1)
		return session, nil, func() {
			closer()
			closed.Add(1)
		}, nil
	}
}

func eventually(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestClientPool(t *testing.T) {
	ctx := t.Context()
	p := newClientPool()
	var opened, closed atomic.Int32
	open := poolOpener(t, &opened, &closed)

	var wg sync.WaitGroup
	clients := make([]*pooledClient, 4)
	for i := range clients {
		wg.Go(func() {
			c, err := p.acquire(ctx, ctx, "launch\x00L\x00echo", "L", open)
			if err != nil {
				t.Error(err)
				return
			}
			clients[i] = c
		})
	}
	wg.Wait()
	if opened.Load() != 1 {
		t.Fatalf("concurrent callers opened %d sessions, want 1", opened.Load())
	}
	for _, c := range clients {
		if c != clients[0] {
			t.Fatal("callers got different clients")
		}
		p.release(c)
	}

	p.expire(time.Now())
	if _, err := p.acquire(ctx, ctx, "launch\x00L\x00echo", "L", open); err != nil || opened.Load() != 1 {
		t.Fatalf("a recently used session was not reused: %v, %d opens", err, opened.Load())
	}
	p.release(clients[0])
	p.expire(time.Now().Add(pooledIdleTimeout + time.Second))
	eventually(t, "the idle session to close", func() bool { return closed.Load() == 1 })

	c, err := p.acquire(ctx, ctx, "launch\x00L\x00echo", "L", open)
	if err != nil || opened.Load() != 2 {
		t.Fatalf("no new session after expiry: %v, %d opens", err, opened.Load())
	}
	other, err := p.acquire(ctx, ctx, "cli\x00echo\x00digest", "", open)
	if err != nil {
		t.Fatal(err)
	}
	p.release(c)
	p.release(other)
	p.closeLaunch("L")
	eventually(t, "the launch's session to close", func() bool { return closed.Load() == 2 })

	p.discard(other)
	eventually(t, "the discarded session to close", func() bool { return closed.Load() == 3 })

	failures := 0
	failing := func(context.Context) (*mcp.ClientSession, *mcpruntime.Endpoint, func(), error) {
		failures++
		return nil, nil, nil, errors.New("cannot start")
	}
	for range 2 {
		if _, err := p.acquire(ctx, ctx, "cli\x00broken", "", failing); err == nil {
			t.Fatal("a failing open succeeded")
		}
	}
	if failures != 2 {
		t.Fatalf("a failed open was cached: %d attempts", failures)
	}

	if _, err := p.acquire(ctx, ctx, "cli\x00echo\x00digest", "", open); err != nil {
		t.Fatal(err)
	}
	p.closeAll()
	if closed.Load() != 4 {
		t.Fatalf("closeAll left sessions open: %d closed", closed.Load())
	}
	if _, err := p.acquire(ctx, ctx, "cli\x00echo\x00digest", "", open); err == nil {
		t.Fatal("a closed pool opened a session")
	}
}
