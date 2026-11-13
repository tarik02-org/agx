package acp

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/adapters/relay/relaytest"
)

// server is the MCP server a launch in directory gives its sessions.
func server(directory, id string) string {
	return fmt.Sprintf(
		`{"type":"http","name":"agx-m","url":"%s","headers":[{"name":"Authorization","value":"Bearer %s"}]}`,
		directory,
		id,
	)
}

// bridged is the stdio server that bridges a launch's server to an agent without HTTP ones.
func bridged(t *testing.T, directory, id, typed string) string {
	t.Helper()
	agx, err := executable()
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf(
		`{%s"name":"agx-m","command":"%s","args":["mcp","bridge","%s"],"env":[{"name":"AGX_MCP_GRANT","value":"%s"}]}`,
		typed,
		agx,
		directory,
		id,
	)
}

func start(t *testing.T) *relaytest.Session {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the scripted protocol uses POSIX paths")
	}
	return relaytest.Start(t, &relaytest.Host{
		Config: func(directory, id string) string {
			return `{"mcpServers":[` + server(directory, id) + `]}`
		},
		Instructions: func(directory string) string { return "AGX " + directory },
	}, run)
}

// initialize negotiates version with the agent's capabilities, as agent answers them.
func initialize(s *relaytest.Session, agent string) {
	s.Expect(
		s.Client(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":1}}`),
		`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":1}}`,
	)
	answer := `{"jsonrpc":"2.0","id":0,"result":` + agent + `}`
	s.Expect(s.Server(answer), answer)
}

func TestRelaySessionsWithCapabilities(t *testing.T) {
	s := start(t)
	initialize(s, `{"protocolVersion":1,"agentCapabilities":{"mcpCapabilities":{"http":true},`+
		`"sessionCapabilities":{"systemPrompt":{},"close":{}}}}`)

	// A new session gets its directory's servers after the client's and the instructions as a
	// system prompt append.
	s.Expect(
		s.Client(`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/a",`+
			`"mcpServers":[{"type":"http","name":"t3","url":"t3","headers":[]}]}}`),
		`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/a",`+
			`"mcpServers":[{"type":"http","name":"t3","url":"t3","headers":[]},`+server("/a", "L1")+`],`+
			`"systemPrompt":{"mode":"append","content":[{"type":"text","text":"AGX /a"}]}}}`,
	)
	s.Server(`{"jsonrpc":"2.0","id":1,"result":{"sessionId":"A"}}`)

	// The client's system prompt keeps its mode and content first; a server name the client
	// uses stays the client's.
	s.Expect(
		s.Client(`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/b",`+
			`"mcpServers":[{"type":"http","name":"agx-m","url":"theirs","headers":[]}],`+
			`"systemPrompt":{"mode":"override","content":[{"type":"text","text":"T3"}]}}}`),
		`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/b",`+
			`"mcpServers":[{"type":"http","name":"agx-m","url":"theirs","headers":[]}],`+
			`"systemPrompt":{"mode":"override","content":[{"type":"text","text":"T3"},{"type":"text","text":"AGX /b"}]}}}`,
	)
	s.Server(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"B"}}`)
	s.Eventually(func(bound map[string]string, _ []string) bool { return bound["A"] == "L1" && bound["B"] == "L2" })

	// Prompts and everything else pass unchanged.
	prompt := `{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"A","prompt":[]}}`
	s.Expect(s.Client(prompt), prompt)
	cancel := `{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"A"}}`
	s.Expect(s.Client(cancel), cancel)

	// Loading a session gives it a fresh launch for its directory and ends the old one; the
	// session keeps the directory it had when the request names none.
	s.Expect(
		s.Client(
			`{"jsonrpc":"2.0","id":4,"method":"session/load","params":{"sessionId":"A","cwd":"/a","mcpServers":[]}}`,
		),
		`{"jsonrpc":"2.0","id":4,"method":"session/load","params":{"sessionId":"A","cwd":"/a",`+
			`"mcpServers":[`+server("/a", "L3")+`],"systemPrompt":{"mode":"append","content":[{"type":"text","text":"AGX /a"}]}}}`,
	)
	s.Server(`{"jsonrpc":"2.0","id":4,"result":null}`)
	s.Eventually(func(bound map[string]string, finished []string) bool {
		return bound["A"] == "L3" && slices.Equal(finished, []string{"L1"})
	})

	// A failed setup ends its launch; one that cannot prepare passes unchanged.
	s.Client(`{"jsonrpc":"2.0","id":5,"method":"session/new","params":{"cwd":"/a","mcpServers":[]}}`)
	s.Server(`{"jsonrpc":"2.0","id":5,"error":{"code":-32603,"message":"no"}}`)
	s.Eventually(func(_ map[string]string, finished []string) bool { return slices.Contains(finished, "L4") })
	failing := `{"jsonrpc":"2.0","id":6,"method":"session/new","params":{"cwd":"/fail","mcpServers":[]}}`
	s.Expect(s.Client(failing), failing)

	// Closing a session ends its launch.
	s.Client(`{"jsonrpc":"2.0","id":7,"method":"session/close","params":{"sessionId":"B"}}`)
	s.Server(`{"jsonrpc":"2.0","id":7,"result":{}}`)
	s.Eventually(func(_ map[string]string, finished []string) bool { return slices.Contains(finished, "L2") })

	// When the agent exits, every remaining launch ends.
	s.Exit()
	if _, finished := s.Host.State(); !slices.Contains(finished, "L3") {
		t.Errorf("launch L3 not finished: %v", finished)
	}
	if warnings := s.Host.Warnings(); len(warnings) != 1 || !strings.Contains(warnings[0], "/fail") {
		t.Errorf("warnings = %q", warnings)
	}
}

func TestRelaySessionsWithoutCapabilities(t *testing.T) {
	s := start(t)
	initialize(s, `{"protocolVersion":1,"agentCapabilities":{"mcpCapabilities":{"http":false}}}`)

	// An agent without HTTP MCP servers gets them bridged over stdio, and no system prompt.
	s.Expect(
		s.Client(`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/a","mcpServers":[]}}`),
		`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/a","mcpServers":[`+
			bridged(t, "/a", "L1", "")+`]}}`,
	)
	s.Server(`{"jsonrpc":"2.0","id":1,"result":{"sessionId":"A"}}`)

	// The new session's first prompt carries the instructions instead; later ones do not.
	s.Expect(
		s.Client(`{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"sessionId":"A",`+
			`"prompt":[{"type":"text","text":"hi"}]}}`),
		`{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"sessionId":"A",`+
			`"prompt":[{"type":"text","text":"AGX /a"},{"type":"text","text":"hi"}]}}`,
	)
	again := `{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"A","prompt":[]}}`
	s.Expect(s.Client(again), again)

	// A forked session has the instructions in the history it copies.
	s.Expect(
		s.Client(
			`{"jsonrpc":"2.0","id":4,"method":"session/fork","params":{"sessionId":"A","cwd":"/a","mcpServers":[]}}`,
		),
		`{"jsonrpc":"2.0","id":4,"method":"session/fork","params":{"sessionId":"A","cwd":"/a","mcpServers":[`+
			bridged(t, "/a", "L2", "")+`]}}`,
	)
	s.Server(`{"jsonrpc":"2.0","id":4,"result":{"sessionId":"F"}}`)
	s.Eventually(func(bound map[string]string, _ []string) bool { return bound["F"] == "L2" })
	first := `{"jsonrpc":"2.0","id":5,"method":"session/prompt","params":{"sessionId":"F","prompt":[]}}`
	s.Expect(s.Client(first), first)

	if warnings := s.Host.Warnings(); len(warnings) != 0 {
		t.Errorf("warnings = %q", warnings)
	}
}

func TestRelayVersion2Stdio(t *testing.T) {
	// Version 2 stdio servers are typed, and only an agent that advertises them gets them.
	s := start(t)
	initialize(s, `{"protocolVersion":2,"capabilities":{"session":{"mcp":{"stdio":{}}}}}`)
	s.Expect(
		s.Client(`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/a"}}`),
		`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/a","mcpServers":[`+
			bridged(t, "/a", "L1", `"type":"stdio",`)+`]}}`,
	)

	s = start(t)
	initialize(s, `{"protocolVersion":2,"capabilities":{"session":{}}}`)
	created := `{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/a"}}`
	s.Expect(s.Client(created), created)
	if warnings := s.Host.Warnings(); len(warnings) != 1 || !strings.Contains(warnings[0], "neither HTTP nor stdio") {
		t.Errorf("warnings = %q", warnings)
	}
}

func TestRelayVersion2(t *testing.T) {
	s := start(t)
	initialize(
		s,
		`{"protocolVersion":2,"info":{"name":"a"},"capabilities":{"session":{"mcp":{"http":{}},"systemPrompt":{}}}}`,
	)
	s.Expect(
		s.Client(`{"jsonrpc":"2.0","id":1,"method":"session/resume","params":{"sessionId":"R","cwd":"/a"}}`),
		`{"jsonrpc":"2.0","id":1,"method":"session/resume","params":{"sessionId":"R","cwd":"/a",`+
			`"mcpServers":[`+server("/a", "L1")+`],"systemPrompt":{"mode":"append","content":[{"type":"text","text":"AGX /a"}]}}}`,
	)
	s.Server(`{"jsonrpc":"2.0","id":1,"result":{}}`)
	s.Eventually(func(bound map[string]string, _ []string) bool { return bound["R"] == "L1" })

	// Version 2 has no session/load.
	load := `{"jsonrpc":"2.0","id":2,"method":"session/load","params":{"sessionId":"R","cwd":"/a"}}`
	s.Expect(s.Client(load), load)
}

func TestRelayUnknownVersion(t *testing.T) {
	s := start(t)
	initialize(s, `{"protocolVersion":3}`)
	created := `{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/a","mcpServers":[]}}`
	s.Expect(s.Client(created), created)
	if warnings := s.Host.Warnings(); len(warnings) != 1 || !strings.Contains(warnings[0], "version 3") {
		t.Errorf("warnings = %q", warnings)
	}
}

func TestRelayRefusedSystemPrompt(t *testing.T) {
	s := start(t)
	initialize(s, `{"protocolVersion":1,"agentCapabilities":{"mcpCapabilities":{"http":true},`+
		`"sessionCapabilities":{"systemPrompt":{}}}}`)

	// An agent that implements an earlier draft refuses the system prompt it advertises; the
	// request goes again without it, and the client sees only the second response.
	s.Expect(
		s.Client(`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/a","mcpServers":[]}}`),
		`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/a","mcpServers":[`+server("/a", "L1")+`],`+
			`"systemPrompt":{"mode":"append","content":[{"type":"text","text":"AGX /a"}]}}}`,
	)
	s.Answer(
		`{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"systemPrompt must be an array of content blocks"}}`,
	)
	s.Expect(
		s.Received(),
		`{"jsonrpc":"2.0","id":1,"method":"session/new","params":{"cwd":"/a","mcpServers":[`+server("/a", "L1")+`]}}`,
	)
	s.Expect(
		s.Server(`{"jsonrpc":"2.0","id":1,"result":{"sessionId":"A"}}`),
		`{"jsonrpc":"2.0","id":1,"result":{"sessionId":"A"}}`,
	)
	s.Expect(
		s.Client(`{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"sessionId":"A","prompt":[]}}`),
		`{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"sessionId":"A","prompt":[{"type":"text","text":"AGX /a"}]}}`,
	)

	// Later sessions take instructions in their first prompt at once.
	created := `{"jsonrpc":"2.0","id":3,"method":"session/new","params":{"cwd":"/b","mcpServers":[` + server(
		"/b",
		"L2",
	) + `]}}`
	s.Expect(s.Client(`{"jsonrpc":"2.0","id":3,"method":"session/new","params":{"cwd":"/b","mcpServers":[]}}`), created)
	s.Server(`{"jsonrpc":"2.0","id":3,"result":{"sessionId":"B"}}`)
	s.Eventually(func(bound map[string]string, _ []string) bool { return bound["A"] == "L1" && bound["B"] == "L2" })

	// Other errors pass to the client without a retry.
	s.Client(`{"jsonrpc":"2.0","id":4,"method":"session/load","params":{"sessionId":"A","cwd":"/a","mcpServers":[]}}`)
	failed := `{"jsonrpc":"2.0","id":4,"error":{"code":-32603,"message":"no"}}`
	s.Expect(s.Server(failed), failed)

	if warnings := s.Host.Warnings(); len(warnings) != 1 ||
		!strings.Contains(warnings[0], "refused the system prompt") {
		t.Errorf("warnings = %q", warnings)
	}
}
