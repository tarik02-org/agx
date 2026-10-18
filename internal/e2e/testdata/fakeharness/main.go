// Command fakeharness stands in for Claude Code and Codex in end-to-end tests. As Codex, it
// installs plugins (see plugins) and serves a minimal app server (see appServer). Otherwise it
// records its arguments,
// selected environment variables, and launch artifacts as JSON in $FAKE_DUMP, and calls the echo
// tool on every MCP server passed with --mcp-config. --exit=N sets its exit status. With
// $FAKE_STDIN set, it copies its standard input there until EOF. With $FAKE_HOLD set, it then
// keeps running and calls the tools again on request (see hold).
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
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
	if command := subcommand(args); command == "plugin" || command == "features" {
		plugins(args)
		return
	}
	if slices.Contains(args, "app-server") {
		appServer(args)
		return
	}
	r := record{Args: args, Env: map[string]string{}, Skills: map[string]string{}, Endpoints: map[string]string{},
		Tools: map[string]string{}}
	r.Directory, _ = os.Getwd()
	for _, name := range strings.Split(os.Getenv("FAKE_ENV"), ",") {
		if value, ok := os.LookupEnv(name); ok && name != "" {
			r.Env[name] = value
		}
	}
	exit := 0
	clients := map[string]*mcpClient{}
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
				clients[name] = &mcpClient{url: s.URL, authorization: os.ExpandEnv(s.Headers["Authorization"])}
			}
			r.Tools, _ = echoAll(clients, nil)
		}
	}
	if path := os.Getenv("FAKE_STDIN"); path != "" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fail(err)
		}
		if err := os.WriteFile(path, b, 0o600); err != nil {
			fail(err)
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
	if dir := os.Getenv("FAKE_HOLD"); dir != "" {
		hold(dir, clients)
	}
	os.Exit(exit)
}

// subcommand is the first argument after the global options.
func subcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-c", "--enable", "--disable":
			i++
		default:
			return args[i]
		}
	}
	return ""
}

// plugins answers Codex's plugin and features commands, keeping marketplaces as lines of a file
// in $CODEX_HOME and installing plugins as Codex does: an entry in config.toml and a cached version. Like Codex, it lists no marketplace and finds no plugin while
// plugins are off: when $CODEX_HOME/config.toml sets plugins = false, unless --enable plugins
// or -c features.plugins=true says otherwise. It appends the arguments of each invocation as a
// JSON line to $CODEX_HOME/fake-plugin-log.jsonl.
func plugins(args []string) {
	home := os.Getenv("CODEX_HOME")
	if err := os.MkdirAll(home, 0o700); err != nil {
		fail(err)
	}
	add := func(file, line string) {
		f, err := os.OpenFile(filepath.Join(home, file), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			fail(err)
		}
		_, _ = fmt.Fprintln(f, line)
		_ = f.Close()
	}
	line, _ := json.Marshal(args)
	add("fake-plugin-log.jsonl", string(line))
	config, _ := os.ReadFile(filepath.Join(home, "config.toml"))
	enabled := !strings.Contains(string(config), "plugins = false")
	rest := args
	for len(rest) > 1 && strings.HasPrefix(rest[0], "-") {
		switch {
		case rest[0] == "--enable" && rest[1] == "plugins", rest[0] == "-c" && rest[1] == "features.plugins=true":
			enabled = true
		}
		rest = rest[2:]
	}
	switch strings.Join(rest[:min(len(rest), 3)], " ") {
	case "features list":
		fmt.Printf("plugins  stable  %t\n", enabled)
	case "plugin marketplace add":
		add("fake-marketplaces", rest[3])
	case "plugin marketplace list":
		if b, err := os.ReadFile(filepath.Join(home, "fake-marketplaces")); err == nil && enabled {
			fmt.Print(string(b))
		} else {
			fmt.Println("No plugin marketplaces in scope.")
		}
	default:
		if len(rest) == 3 && rest[0] == "plugin" && rest[1] == "add" {
			if !enabled {
				fmt.Fprintf(os.Stderr, "Error: plugin `%s` was not found in marketplace\n", rest[2])
				os.Exit(1)
			}
			// Like Codex, enable the plugin in config.toml and cache a version of it.
			name, marketplace, _ := strings.Cut(rest[2], "@")
			add("config.toml", "\n[plugins.\""+rest[2]+"\"]\nenabled = true")
			if err := os.MkdirAll(filepath.Join(home, "plugins", "cache", marketplace, name, "1.0.0"), 0o700); err != nil {
				fail(err)
			}
			return
		}
		if len(rest) == 3 && rest[0] == "plugin" && rest[1] == "remove" {
			name, marketplace, _ := strings.Cut(rest[2], "@")
			header := "[plugins.\"" + rest[2] + "\"]"
			lines := strings.Split(string(config), "\n")
			if i := slices.Index(lines, header); i >= 0 {
				lines = slices.Delete(lines, i, min(i+2, len(lines)))
			}
			if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(strings.Join(lines, "\n")), 0o600); err != nil {
				fail(err)
			}
			if err := os.RemoveAll(filepath.Join(home, "plugins", "cache", marketplace, name)); err != nil {
				fail(err)
			}
			return
		}
		fail(fmt.Errorf("unexpected plugin command %q", args))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "fakeharness:", err)
	os.Exit(99)
}

// errSessionNotFound is the HTTP 404 a server answers for a session it does not know.
var errSessionNotFound = errors.New("session not found")

// mcpClient is a streamable HTTP MCP client that keeps its session between calls and, as the
// specification asks, starts a new one when the server answers 404 for it.
type mcpClient struct {
	url, authorization string
	session            string
	// reinitialized counts sessions started again after a 404.
	reinitialized int
}

func (c *mcpClient) post(message map[string]any) (map[string]any, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	message["jsonrpc"] = "2.0"
	body, _ := json.Marshal(message)
	req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", c.authorization)
	if c.session != "" {
		req.Header.Set("Mcp-Session-Id", c.session)
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
	if resp.StatusCode == http.StatusNotFound && c.session != "" {
		return nil, errSessionNotFound
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if id := resp.Header.Get("Mcp-Session-Id"); id != "" {
		c.session = id
	}
	return decode(resp)
}

func (c *mcpClient) initialize() error {
	c.session = ""
	if _, err := c.post(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-11-25", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "fakeharness", "version": "1"},
	}}); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	if _, err := c.post(map[string]any{"method": "notifications/initialized"}); err != nil {
		return fmt.Errorf("initialized: %w", err)
	}
	return nil
}

// echo calls the echo tool with arguments added to its text, starting a session first when
// there is none.
func (c *mcpClient) echo(arguments map[string]any) (string, error) {
	if c.session == "" {
		if err := c.initialize(); err != nil {
			return "", err
		}
	}
	args := map[string]any{"text": "hello"}
	for name, value := range arguments {
		args[name] = value
	}
	call := map[string]any{"id": 2, "method": "tools/call", "params": map[string]any{
		"name": "echo", "arguments": args,
	}}
	result, err := c.post(call)
	if errors.Is(err, errSessionNotFound) {
		c.reinitialized++
		if err := c.initialize(); err != nil {
			return "", err
		}
		result, err = c.post(call)
	}
	if err != nil {
		return "", fmt.Errorf("tools/call: %w", err)
	}
	var reply struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	b, _ := json.Marshal(result)
	if err := json.Unmarshal(b, &reply); err != nil || len(reply.Result.Content) == 0 {
		return "", fmt.Errorf("unexpected tools/call response %s", b)
	}
	return reply.Result.Content[0].Text, nil
}

// callEcho runs the MCP handshake over streamable HTTP and calls the echo tool.
func callEcho(url, authorization string) (string, error) {
	return (&mcpClient{url: url, authorization: authorization}).echo(nil)
}

// echoAll calls the echo tool of every client, keyed like clients, with arguments added, and
// reports how many sessions started again.
func echoAll(clients map[string]*mcpClient, arguments map[string]any) (map[string]string, int) {
	tools, reinitialized := map[string]string{}, 0
	for name, c := range clients {
		before := c.reinitialized
		text, err := c.echo(arguments)
		if err != nil {
			text = "error: " + err.Error()
		}
		tools[name] = text
		reinitialized += c.reinitialized - before
	}
	return tools, reinitialized
}

// hold keeps the harness running after its first calls, as a session does: for each file
// call-<n> that appears in dir, it calls the echo tools again over the same sessions, adding the
// JSON object the file may hold to their arguments, and writes result-<n>.json; the file exit
// ends it.
func hold(dir string, clients map[string]*mcpClient) {
	for n := 1; ; {
		if _, err := os.Stat(filepath.Join(dir, "exit")); err == nil {
			return
		}
		call, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("call-%d", n)))
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		arguments := map[string]any{}
		if len(call) > 0 {
			if err := json.Unmarshal(call, &arguments); err != nil {
				fail(err)
			}
		}
		tools, reinitialized := echoAll(clients, arguments)
		b, _ := json.Marshal(map[string]any{"tools": tools, "reinitialized": reinitialized})
		path := filepath.Join(dir, fmt.Sprintf("result-%d.json", n))
		if err := os.WriteFile(path+".tmp", b, 0o600); err != nil {
			fail(err)
		}
		if err := os.Rename(path+".tmp", path); err != nil {
			fail(err)
		}
		n++
	}
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

// appServer answers newline-delimited JSON-RPC like codex app-server. Every result echoes the
// request's params as "received"; initialize adds the process arguments, and thread/start a
// new thread in the requested cwd plus the echo tool's reply from every MCP server in its
// config. test/call calls a thread's echo tools again over the same sessions, and test/close
// closes a thread with a thread/closed notification.
func appServer(args []string) {
	reader := bufio.NewReader(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	threads := 0
	// sessions keeps each thread's MCP clients, as Codex keeps a thread's MCP connections.
	sessions := map[string]map[string]*mcpClient{}
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params map[string]any  `json:"params"`
		}
		if json.Unmarshal(line, &request) != nil || request.ID == nil {
			continue
		}
		result := map[string]any{"received": request.Params}
		switch request.Method {
		case "initialize":
			result["argv"] = args
		case "thread/start":
			threads++
			id := fmt.Sprintf("thread-%d", threads)
			result["thread"] = map[string]any{"id": id, "cwd": request.Params["cwd"]}
			clients := map[string]*mcpClient{}
			config, _ := request.Params["config"].(map[string]any)
			servers, _ := config["mcp_servers"].(map[string]any)
			for name, s := range servers {
				server, _ := s.(map[string]any)
				url, _ := server["url"].(string)
				headers, _ := server["http_headers"].(map[string]any)
				authorization, _ := headers["Authorization"].(string)
				clients[name] = &mcpClient{url: url, authorization: authorization}
			}
			sessions[id] = clients
			result["tools"], _ = echoAll(clients, nil)
		case "test/call":
			id, _ := request.Params["threadId"].(string)
			result["tools"], result["reinitialized"] = echoAll(sessions[id], nil)
		case "test/close":
			_ = out.Encode(map[string]any{"id": request.ID, "result": result})
			_ = out.Encode(map[string]any{"method": "thread/closed", "params": map[string]any{"threadId": request.Params["threadId"]}})
			continue
		}
		_ = out.Encode(map[string]any{"id": request.ID, "result": result})
	}
}
