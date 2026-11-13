// Package mcpbridge serves one of a launch's MCP endpoints over standard input and output, for
// harnesses that start MCP servers only as commands.
package mcpbridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Run copies MCP messages between in and out and the endpoint, authorized by grant, until in
// ends or either side fails. Messages pass as they are, so the harness negotiates with the
// server itself.
//
// Server messages outside a request's response, such as list change notifications, are not
// delivered: the SDK's connection opens the stream that carries them only for its own client.
func Run(ctx context.Context, endpoint, grant string, in io.ReadCloser, out io.WriteCloser) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	headers := &headers{grant: grant, base: http.DefaultTransport}
	remote, err := (&mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: &http.Client{Transport: headers},
		MaxRetries: -1,
	}).Connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = remote.Close() }()
	local, err := (&mcp.IOTransport{Reader: in, Writer: out}).Connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = local.Close() }()

	done := make(chan error, 2)
	go func() {
		for {
			msg, err := local.Read(ctx)
			if err != nil {
				done <- err
				return
			}
			r, call := msg.(*jsonrpc.Request)
			call = call && r.IsCall()
			if call && r.Method == "initialize" {
				headers.initializing(r.ID)
			}
			err = remote.Write(ctx, msg)
			if err == nil || !call {
				continue
			}
			// A request the endpoint did not take fails alone; a broken connection also ends the
			// other direction.
			failed := &jsonrpc.Response{ID: r.ID, Error: &jsonrpc.Error{Code: -32603, Message: err.Error()}}
			if err := local.Write(ctx, failed); err != nil {
				done <- err
				return
			}
		}
	}()
	go func() {
		for {
			msg, err := remote.Read(ctx)
			if err != nil {
				done <- err
				return
			}
			if r, ok := msg.(*jsonrpc.Response); ok {
				headers.initialized(r)
			}
			if err := local.Write(ctx, msg); err != nil {
				done <- err
				return
			}
		}
	}()
	err = <-done
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// headers authorizes the bridge's requests and names the protocol version the harness
// negotiated, which the SDK's connection learns only for its own client.
type headers struct {
	grant string
	base  http.RoundTripper

	mu         sync.Mutex
	initialize *jsonrpc.ID
	version    string
}

func (h *headers) initializing(id jsonrpc.ID) {
	h.mu.Lock()
	h.initialize = &id
	h.mu.Unlock()
}

func (h *headers) initialized(r *jsonrpc.Response) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.initialize == nil || r.ID != *h.initialize || r.Error != nil {
		return
	}
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if json.Unmarshal(r.Result, &result) == nil {
		h.version = result.ProtocolVersion
	}
	h.initialize = nil
}

func (h *headers) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+h.grant)
	h.mu.Lock()
	version := h.version
	h.mu.Unlock()
	if version != "" && r.Header.Get("Mcp-Protocol-Version") == "" {
		r.Header.Set("Mcp-Protocol-Version", version)
	}
	return h.base.RoundTrip(r)
}
