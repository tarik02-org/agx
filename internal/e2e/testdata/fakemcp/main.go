// Command fakemcp is a stdio MCP server for end-to-end tests. Its echo tool answers
// echo:<text>:<$ECHO_TOKEN>[:<$ECHO_EXTRA>], proving the daemon injected the server's environment.
// With the arguments started and sleep_ms, it first creates the file started and then waits,
// standing in for a long tool call. shape answers the arguments it received, as text and as
// structured content; media answers an image, an embedded binary resource, and text; fail
// answers a result marked as an error, and explode a JSON-RPC error. Each process appends a
// line to $FAKEMCP_STARTS when set, so tests can count server starts.
package main

import (
	"bufio"
	"encoding/json"
	"os"
	"time"
)

var tools = []any{
	map[string]any{
		"name": "echo", "description": "Echo text.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":     map[string]any{"type": "string"},
				"started":  map[string]any{"type": "string"},
				"sleep_ms": map[string]any{"type": "integer"},
			},
			"additionalProperties": true,
		},
	},
	map[string]any{
		"name": "shape", "description": "Answer the arguments.\nSecond line.",
		"inputSchema": json.RawMessage(`{"type":"object","properties":{` +
			`"query":{"type":"string","description":"What to find."},` +
			`"status":{"enum":["open","closed"]},` +
			`"limit":{"type":"integer"},` +
			`"ratio":{"type":"number"},` +
			`"exact":{"type":"boolean"},` +
			`"note":{"type":["string","null"]},` +
			`"tags":{"type":"array","items":{"type":"string"}},` +
			`"filter":{"type":"object","properties":{"owner":{"type":"string"}},"required":["owner"]}` +
			`},"required":["query"],"additionalProperties":false}`),
		"outputSchema": json.RawMessage(`{"type":"object","properties":{"received":{"type":"object"}},` +
			`"required":["received"]}`),
	},
	map[string]any{"name": "media", "description": "Answer an image.", "inputSchema": map[string]any{"type": "object"}},
	map[string]any{"name": "fail", "description": "Fail.", "inputSchema": map[string]any{"type": "object"}},
	map[string]any{"name": "explode", "description": "Break.", "inputSchema": map[string]any{"type": "object"}},
}

// png is a 1x1 transparent PNG.
var png = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

func main() {
	if path := os.Getenv("FAKEMCP_STARTS"); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			_, _ = f.WriteString("start\n")
			_ = f.Close()
		}
	}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64<<10), 16<<20)
	out := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				ProtocolVersion string          `json:"protocolVersion"`
				Name            string          `json:"name"`
				Raw             json.RawMessage `json:"arguments"`
				Arguments       struct {
					Text    string `json:"text"`
					Started string `json:"started"`
					SleepMS int    `json:"sleep_ms"`
				} `json:"-"`
			} `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &m) != nil || len(m.ID) == 0 {
			continue
		}
		_ = json.Unmarshal(m.Params.Raw, &m.Params.Arguments)
		var result any = map[string]any{}
		switch m.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": m.Params.ProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "fakemcp", "version": "1"},
			}
		case "tools/list":
			result = map[string]any{"tools": tools}
		case "tools/call":
			switch m.Params.Name {
			case "shape":
				text := string(m.Params.Raw)
				result = map[string]any{
					"content":           []any{map[string]any{"type": "text", "text": text}},
					"structuredContent": map[string]any{"received": m.Params.Raw},
				}
				_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": result})
				continue
			case "media":
				result = map[string]any{"content": []any{
					map[string]any{"type": "text", "text": "a picture"},
					map[string]any{"type": "image", "mimeType": "image/png", "data": png},
					map[string]any{"type": "resource", "resource": map[string]any{
						"uri": "file:///report.pdf", "mimeType": "application/pdf", "blob": []byte("%PDF-1.4"),
					}},
				}}
				_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": result})
				continue
			case "fail":
				result = map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "failed on purpose"}},
					"isError": true,
				}
				_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": result})
				continue
			case "explode":
				_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{
					"code": -32000, "message": "exploded",
				}})
				continue
			}
			if path := m.Params.Arguments.Started; path != "" {
				_ = os.WriteFile(path, nil, 0o600)
			}
			time.Sleep(time.Duration(m.Params.Arguments.SleepMS) * time.Millisecond)
			text := "echo:" + m.Params.Arguments.Text + ":" + os.Getenv("ECHO_TOKEN")
			if extra := os.Getenv("ECHO_EXTRA"); extra != "" {
				text += ":" + extra
			}
			if _, leaked := os.LookupEnv("AGX_MCP_GRANT"); leaked {
				text += ":grant-leaked"
			}
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}
		}
		_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": result})
	}
}
