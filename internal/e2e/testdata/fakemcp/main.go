// Command fakemcp is a stdio MCP server for end-to-end tests. Its echo tool answers
// echo:<text>:<$ECHO_TOKEN>[:<$ECHO_EXTRA>], proving the daemon injected the server's environment.
// With the arguments started and sleep_ms, it first creates the file started and then waits,
// standing in for a long tool call.
package main

import (
	"bufio"
	"encoding/json"
	"os"
	"time"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64<<10), 16<<20)
	out := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				ProtocolVersion string `json:"protocolVersion"`
				Arguments       struct {
					Text    string `json:"text"`
					Started string `json:"started"`
					SleepMS int    `json:"sleep_ms"`
				} `json:"arguments"`
			} `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &m) != nil || len(m.ID) == 0 {
			continue
		}
		var result any = map[string]any{}
		switch m.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": m.Params.ProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "fakemcp", "version": "1"},
			}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{
				"name": "echo", "description": "Echo text.", "inputSchema": map[string]any{"type": "object"},
			}}}
		case "tools/call":
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
