// Command fakeharness stands in for Claude Code in end-to-end tests. It records its arguments,
// selected environment variables, and launch artifacts as JSON in $FAKE_DUMP, and calls the echo
// tool on every MCP server passed with --mcp-config. --exit=N sets its exit status.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type record struct {
	Args         []string          `json:"args"`
	Directory    string            `json:"directory"`
	Env          map[string]string `json:"env"`
	Skills       map[string]string `json:"skills"`
	Instructions string            `json:"instructions"`
	Endpoints    map[string]string `json:"endpoints"`
	Tools        map[string]string `json:"tools"`
}

func main() {
	args := os.Args[1:]
	r := record{Args: args, Env: map[string]string{}, Skills: map[string]string{}, Endpoints: map[string]string{},
		Tools: map[string]string{}}
	r.Directory, _ = os.Getwd()
	for _, name := range strings.Split(os.Getenv("FAKE_ENV"), ",") {
		if value, ok := os.LookupEnv(name); ok && name != "" {
			r.Env[name] = value
		}
	}
	exit := 0
	for i := 0; i < len(args); i++ {
		value := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch arg := args[i]; {
		case arg == "--version":
			fmt.Println("fakeharness 1")
		case strings.HasPrefix(arg, "--exit="):
			exit, _ = strconv.Atoi(strings.TrimPrefix(arg, "--exit="))
		case arg == "--plugin-dir":
			dir := value()
			matches, _ := filepath.Glob(filepath.Join(dir, "skills", "*", "SKILL.md"))
			for _, m := range matches {
				b, _ := os.ReadFile(m)
				r.Skills[filepath.Base(filepath.Dir(m))] = string(b)
			}
		case arg == "--append-system-prompt-file":
			b, _ := os.ReadFile(value())
			r.Instructions = string(b)
		case arg == "--mcp-config":
			var cfg struct {
				Servers map[string]struct {
					URL     string            `json:"url"`
					Headers map[string]string `json:"headers"`
				} `json:"mcpServers"`
			}
			if err := json.Unmarshal([]byte(value()), &cfg); err != nil {
				fail(err)
			}
			for name, s := range cfg.Servers {
				r.Endpoints[name] = s.URL
				text, err := callEcho(s.URL, os.ExpandEnv(s.Headers["Authorization"]))
				if err != nil {
					text = "error: " + err.Error()
				}
				r.Tools[name] = text
			}
		}
	}
	if path := os.Getenv("FAKE_DUMP"); path != "" {
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			fail(err)
		}
		if err := os.WriteFile(path, b, 0o600); err != nil {
			fail(err)
		}
	}
	os.Exit(exit)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "fakeharness:", err)
	os.Exit(99)
}

// callEcho runs the MCP handshake over streamable HTTP and calls the echo tool.
func callEcho(url, authorization string) (string, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	session := ""
	post := func(message map[string]any) (map[string]any, error) {
		message["jsonrpc"] = "2.0"
		body, _ := json.Marshal(message)
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", authorization)
		if session != "" {
			req.Header.Set("Mcp-Session-Id", session)
			req.Header.Set("Mcp-Protocol-Version", "2025-11-25")
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == http.StatusAccepted {
			return nil, nil
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		}
		if id := resp.Header.Get("Mcp-Session-Id"); id != "" {
			session = id
		}
		return decode(resp)
	}
	if _, err := post(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-11-25", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "fakeharness", "version": "1"},
	}}); err != nil {
		return "", fmt.Errorf("initialize: %w", err)
	}
	if _, err := post(map[string]any{"method": "notifications/initialized"}); err != nil {
		return "", fmt.Errorf("initialized: %w", err)
	}
	result, err := post(map[string]any{"id": 2, "method": "tools/call", "params": map[string]any{
		"name": "echo", "arguments": map[string]any{"text": "hello"},
	}})
	if err != nil {
		return "", fmt.Errorf("tools/call: %w", err)
	}
	var call struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	b, _ := json.Marshal(result)
	if err := json.Unmarshal(b, &call); err != nil || len(call.Result.Content) == 0 {
		return "", fmt.Errorf("unexpected tools/call response %s", b)
	}
	return call.Result.Content[0].Text, nil
}

// decode reads a JSON response, or the last JSON-RPC response of an event stream.
func decode(resp *http.Response) (map[string]any, error) {
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		var out map[string]any
		return out, json.NewDecoder(resp.Body).Decode(&out)
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		data, ok := strings.CutPrefix(scanner.Text(), "data:")
		if !ok || strings.TrimSpace(data) == "" {
			continue
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(data), &out); err != nil {
			return nil, err
		}
		if _, ok := out["id"]; ok {
			return out, nil
		}
	}
	return nil, fmt.Errorf("event stream ended without a response: %v", scanner.Err())
}
