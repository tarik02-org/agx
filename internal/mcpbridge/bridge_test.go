package mcpbridge

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRun(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "upstream"}, nil)
	type args struct {
		Name string `json:"name"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "greet"}, func(
		_ context.Context,
		_ *mcp.CallToolRequest,
		in args,
	) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "hi " + in.Name}}}, nil, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	var mu sync.Mutex
	var authorizations, versions []string
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authorizations = append(authorizations, r.Header.Get("Authorization"))
		versions = append(versions, r.Header.Get("Mcp-Protocol-Version"))
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer grant" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer endpoint.Close()

	toBridge, clientOut := io.Pipe()
	clientIn, fromBridge := io.Pipe()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, endpoint.URL, "grant", toBridge, fromBridge) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "harness"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: clientIn, Writer: clientOut}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "greet", Arguments: map[string]any{"name": "agx"}})
	if err != nil {
		t.Fatal(err)
	}
	if text := result.Content[0].(*mcp.TextContent).Text; text != "hi agx" {
		t.Errorf("result = %q", text)
	}

	// The harness ending its input ends the bridge.
	_ = session.Close()
	if err := <-done; err != nil {
		t.Errorf("Run = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, authorization := range authorizations {
		if authorization != "Bearer grant" {
			t.Errorf("authorizations = %q", authorizations)
			break
		}
	}
	// Initialize carries its version in the body; every request after it names the negotiated one.
	initialize := slices.Index(versions, "")
	if initialize < 0 || initialize == len(versions)-1 || slices.Contains(versions[initialize+1:], "") {
		t.Errorf("versions = %q", versions)
	}
}
