package mcpruntime_test

import (
	"context"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/mcpruntime"
)

const fakeSharedServerEnv = "AGX_TEST_FAKE_SHARED_MCP"

// fakeSharedServer is a stdio MCP server whose tools report its process ID and the roots
// its client gives, send progress, ask for sampling, and exit.
func fakeSharedServer() {
	server := mcp.NewServer(&mcp.Implementation{Name: "fake-shared", Version: "1"}, nil)
	text := func(s string) *mcp.CallToolResult {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
	}
	mcp.AddTool(server, &mcp.Tool{Name: "pid"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return text(strconv.Itoa(os.Getpid())), nil, nil
		})
	mcp.AddTool(server, &mcp.Tool{Name: "roots"},
		func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			// Shared servers still answer roots for servers written against older protocols.
			roots, err := req.Session.ListRoots(ctx, nil) //nolint:staticcheck
			if err != nil {
				return text("error: " + err.Error()), nil, nil //nolint:nilerr // the test reads the failure
			}
			uris := []string{}
			for _, root := range roots.Roots {
				uris = append(uris, root.URI)
			}
			return text(strings.Join(uris, ",")), nil, nil
		})
	mcp.AddTool(server, &mcp.Tool{Name: "progress"},
		func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			token := req.Params.GetProgressToken()
			for i := range 3 {
				_ = req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
					ProgressToken: token,
					Progress:      float64(i + 1),
					Total:         3,
				})
			}
			return text("done"), nil, nil
		})
	mcp.AddTool(server, &mcp.Tool{Name: "sample"},
		func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			_, err := req.Session.CreateMessage(ctx, &mcp.CreateMessageParams{MaxTokens: 1}) //nolint:staticcheck
			if err != nil {
				return text("refused"), nil, nil //nolint:nilerr // the test reads the refusal
			}
			return text("sampled"), nil, nil
		})
	mcp.AddTool(server, &mcp.Tool{Name: "exit"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			os.Exit(3)
			return nil, nil, nil
		})
	_ = server.Run(context.Background(), &mcp.StdioTransport{})
}

type sharedFixture struct {
	t      *testing.T
	shared *mcpruntime.Shared
	server config.MCPServer
}

func newSharedFixture(t *testing.T, share string) *sharedFixture {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	shared := mcpruntime.NewShared(t.Context(), "test")
	t.Cleanup(shared.Close)
	return &sharedFixture{
		t:      t,
		shared: shared,
		// -test.run keeps a child that somehow misses the variable from running the tests again.
		server: config.MCPServer{
			Transport:   "stdio",
			Command:     []string{executable, "-test.run=^$"},
			Environment: map[string]config.Value{fakeSharedServerEnv: {Literal: "1"}},
			Share:       share,
			Cwd:         t.TempDir(),
		},
	}
}

// endpoint is a launch's endpoint of the fake server, rooted at root.
func (f *sharedFixture) endpoint(root string) *mcpruntime.Endpoint {
	e := mcpruntime.New(
		f.t.Context(), "fake", f.server, nil, nil, auth.New("http://127.0.0.1:1/callback"), f.shared, root,
	)
	f.t.Cleanup(e.Close)
	return e
}

type sharedSession struct {
	*mcp.ClientSession
	mu       sync.Mutex
	progress []any
}

func connect(t *testing.T, e *mcpruntime.Endpoint) *sharedSession {
	t.Helper()
	h := httptest.NewServer(e)
	t.Cleanup(h.Close)
	s := &sharedSession{}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			s.mu.Lock()
			s.progress = append(s.progress, req.Params.ProgressToken)
			s.mu.Unlock()
		},
	})
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: h.URL, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	s.ClientSession = session
	return s
}

func call(t *testing.T, s *sharedSession, params *mcp.CallToolParams) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	result, err := s.CallTool(ctx, params)
	if err != nil {
		t.Fatalf("%s: %v", params.Name, err)
	}
	return result.Content[0].(*mcp.TextContent).Text
}

func tool(name string) *mcp.CallToolParams { return &mcp.CallToolParams{Name: name} }

func eventually(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSharedServerServesLaunchesWithOneProcess(t *testing.T) {
	f := newSharedFixture(t, config.ShareRepository)
	root := t.TempDir()
	first := connect(t, f.endpoint(root))
	second := connect(t, f.endpoint(root))
	again := connect(t, f.endpoint(root))
	pid := call(t, first, tool("pid"))
	if got := call(t, second, tool("pid")); got != pid {
		t.Fatalf("second launch runs pid %s, first %s", got, pid)
	}
	if got := call(t, again, tool("pid")); got != pid {
		t.Fatalf("third launch runs pid %s, first %s", got, pid)
	}
	other := connect(t, f.endpoint(t.TempDir()))
	if got := call(t, other, tool("pid")); got == pid {
		t.Fatal("a launch in another repository joined the same process")
	}
	if got := f.shared.Servers(); got != 2 {
		t.Fatalf("shared servers = %d, want 2", got)
	}
}

func TestUnsharedServerStartsProcessPerSession(t *testing.T) {
	f := newSharedFixture(t, config.ShareNone)
	e := f.endpoint("")
	first, second := connect(t, e), connect(t, e)
	if call(t, first, tool("pid")) == call(t, second, tool("pid")) {
		t.Fatal("sessions of an unshared server share a process")
	}
	if got := f.shared.Servers(); got != 0 {
		t.Fatalf("shared servers = %d, want 0", got)
	}
}

func TestSharedServerAnswersRootsWithItsRoot(t *testing.T) {
	f := newSharedFixture(t, config.ShareDirectory)
	root := t.TempDir()
	session := connect(t, f.endpoint(root))
	if got, want := call(t, session, tool("roots")), "file://"+root; got != want {
		t.Fatalf("roots = %q, want %q", got, want)
	}
	global := newSharedFixture(t, config.ShareGlobal)
	if got := call(t, connect(t, global.endpoint("")), tool("roots")); !strings.HasPrefix(got, "error") {
		t.Fatalf("global server got roots %q", got)
	}
}

func TestSharedServerRefusesSampling(t *testing.T) {
	f := newSharedFixture(t, config.ShareGlobal)
	if got := call(t, connect(t, f.endpoint("")), tool("sample")); got != "refused" {
		t.Fatalf("sample = %q", got)
	}
}

func TestSharedServerRoutesProgressToItsClient(t *testing.T) {
	f := newSharedFixture(t, config.ShareGlobal)
	first, second := connect(t, f.endpoint("")), connect(t, f.endpoint(""))
	params := tool("progress")
	params.SetProgressToken("mine")
	if got := call(t, first, params); got != "done" {
		t.Fatalf("progress = %q", got)
	}
	eventually(t, "progress arrived", func() bool {
		first.mu.Lock()
		defer first.mu.Unlock()
		return len(first.progress) == 3
	})
	for _, token := range first.progress {
		if token != "mine" {
			t.Fatalf("progress token = %v, want the client's own", token)
		}
	}
	second.mu.Lock()
	defer second.mu.Unlock()
	if len(second.progress) != 0 {
		t.Fatalf("another client got progress %v", second.progress)
	}
}

func TestSharedServerStopsWhenIdle(t *testing.T) {
	f := newSharedFixture(t, config.ShareGlobal)
	f.shared.SetIdleTimeout(50 * time.Millisecond)
	session := connect(t, f.endpoint(""))
	call(t, session, tool("pid"))
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the idle server stopped", func() bool { return f.shared.Servers() == 0 })
}

func TestSharedServerRestartsAfterCrash(t *testing.T) {
	f := newSharedFixture(t, config.ShareGlobal)
	e := f.endpoint("")
	first := connect(t, e)
	pid := call(t, first, tool("pid"))
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if _, err := first.CallTool(ctx, tool("exit")); err == nil {
		t.Fatal("a call the server crashed on succeeded")
	}
	eventually(t, "the crashed server was forgotten", func() bool { return f.shared.Servers() == 0 })
	if got := call(t, connect(t, e), tool("pid")); got == pid {
		t.Fatal("a new session reached the crashed process")
	}
}

func TestSharedServerStopsOnSecretReset(t *testing.T) {
	f := newSharedFixture(t, config.ShareGlobal)
	f.server.Environment["TOKEN"] = config.Value{Secret: "token"}
	secrets := map[config.ID]config.Secret{"token": {Kind: "daemon_environment", Variable: "AGX_TEST_SHARED_TOKEN"}}
	t.Setenv("AGX_TEST_SHARED_TOKEN", "value")
	e := mcpruntime.New(
		t.Context(), "fake", f.server, secrets, nil, auth.New("http://127.0.0.1:1/callback"), f.shared, "",
	)
	t.Cleanup(e.Close)
	session := connect(t, e)
	call(t, session, tool("pid"))
	f.shared.Reset("other")
	if got := f.shared.Servers(); got != 1 {
		t.Fatalf("an unrelated secret stopped the server: %d servers", got)
	}
	f.shared.Reset("token")
	if got := f.shared.Servers(); got != 0 {
		t.Fatalf("shared servers after reset = %d, want 0", got)
	}
}
