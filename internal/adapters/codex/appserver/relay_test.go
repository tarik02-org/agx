package appserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tarik02-org/agx/internal/adapters"
)

func TestMerge(t *testing.T) {
	ours := map[string]any{
		"mcp_servers": map[string]any{"agx-a": map[string]any{"url": "u"}},
		"plugins":     map[string]any{"p@agx-managed": map[string]any{"enabled": true}},
	}
	for _, tt := range []struct{ theirs, want string }{
		{`{}`, `{"mcp_servers":{"agx-a":{"url":"u"}},"plugins":{"p@agx-managed":{"enabled":true}}}`},
		{
			`{"x":1,"mcp_servers":{"t3":{"url":"<t>"}}}`,
			`{"plugins":{"p@agx-managed":{"enabled":true}},"x":1,"mcp_servers":{"agx-a":{"url":"u"},"t3":{"url":"<t>"}}}`,
		},
		{
			`{"mcp_servers":{"agx-a":{"url":"theirs"}}}`,
			`{"plugins":{"p@agx-managed":{"enabled":true}},"mcp_servers":{"agx-a":{"url":"theirs"}}}`,
		},
		{
			`{"mcp_servers.t3":{"url":"t"},"plugins.p@agx-managed.enabled":false}`,
			`{"mcp_servers":{"agx-a":{"url":"u"}},"plugins":{"p@agx-managed":{"enabled":true}},` +
				`"mcp_servers.t3":{"url":"t"},"plugins.p@agx-managed.enabled":false}`,
		},
		{
			`{"mcp_servers":"not an object"}`,
			`{"plugins":{"p@agx-managed":{"enabled":true}},"mcp_servers":"not an object"}`,
		},
	} {
		theirs, ok := parseObject(json.RawMessage(tt.theirs))
		if !ok {
			t.Fatalf("parse %s", tt.theirs)
		}
		if got := string(merge(theirs, ours).marshal()); got != tt.want {
			t.Errorf("merge(%s):\n got %s\nwant %s", tt.theirs, got, tt.want)
		}
	}
}

// fakeHost hands out numbered launches: directory d gets config with server agx-<d>.
type fakeHost struct {
	mu       sync.Mutex
	next     int
	bound    map[string]string
	finished []string
	warnings []string
	disabled map[string][]string
}

func (h *fakeHost) Prepare(_ context.Context, directory string) (adapters.ThreadLaunch, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if directory == "/fail" {
		return adapters.ThreadLaunch{}, fmt.Errorf("cannot resolve")
	}
	h.next++
	id := fmt.Sprintf("L%d", h.next)
	config := fmt.Sprintf(
		`{"mcp_servers":{"agx-m":{"url":"%s","http_headers":{"Authorization":"Bearer %s"}}}}`,
		directory,
		id,
	)
	return adapters.ThreadLaunch{
		ID:              id,
		Directory:       directory,
		Config:          []byte(config),
		Instructions:    "AGX " + directory,
		DisabledPlugins: h.disabled[directory],
	}, nil
}

func (h *fakeHost) DisabledPlugins(_ context.Context, directory string) ([]string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.disabled[directory], nil
}

func (h *fakeHost) Bind(_ context.Context, l adapters.ThreadLaunch, thread string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.bound[thread] = l.ID
	return nil
}

func (h *fakeHost) Finish(_ context.Context, l adapters.ThreadLaunch) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.finished = append(h.finished, l.ID)
	return nil
}

func (h *fakeHost) Warn(message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.warnings = append(h.warnings, message)
}

func (h *fakeHost) state() (map[string]string, []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	bound := map[string]string{}
	for k, v := range h.bound {
		bound[k] = v
	}
	return bound, slices.Clone(h.finished)
}

// session drives a relay: the test plays both the host (client) and codex (server).
type session struct {
	t                  *testing.T
	host               *fakeHost
	toRelay, toServer  *io.PipeWriter
	fromRelay, fromSrv *bufio.Reader
	done               chan error
}

func newSession(t *testing.T) *session {
	t.Helper()
	clientIn, toRelay := io.Pipe()
	fromRelay, clientOut := io.Pipe()
	serverIn, toServer := io.Pipe()
	fromServer, serverOut := io.Pipe()
	s := &session{
		t: t,
		host: &fakeHost{bound: map[string]string{}, disabled: map[string][]string{
			"/a": {"b@agx-managed"},
			"/b": {"a@agx-managed"},
		}},
		toRelay:   toRelay,
		toServer:  toServer,
		fromRelay: bufio.NewReader(fromRelay),
		fromSrv:   bufio.NewReader(fromServer),
		done:      make(chan error, 1),
	}
	go func() {
		s.done <- Relay(
			context.Background(),
			s.host,
			"/base",
			adapters.Stream{In: clientIn, Out: clientOut},
			adapters.Stream{In: serverIn, Out: serverOut},
		)
	}()
	t.Cleanup(func() {
		_ = toRelay.Close()
		_ = toServer.Close()
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
			t.Error("relay did not end")
		}
	})
	return s
}

func readLine(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	line := make(chan string, 1)
	go func() {
		s, _ := r.ReadString('\n')
		line <- s
	}()
	select {
	case s := <-line:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("no line")
		return ""
	}
}

// client sends a line from the host and returns what codex receives.
func (s *session) client(line string) string {
	s.t.Helper()
	go func() { _, _ = io.WriteString(s.toRelay, line+"\n") }()
	return strings.TrimSuffix(readLine(s.t, s.fromSrv), "\n")
}

// server sends a line from codex and returns what the host receives.
func (s *session) server(line string) string {
	s.t.Helper()
	go func() { _, _ = io.WriteString(s.toServer, line+"\n") }()
	return strings.TrimSuffix(readLine(s.t, s.fromRelay), "\n")
}

func (s *session) expect(got, want string) {
	s.t.Helper()
	if got != want {
		s.t.Errorf("\n got %s\nwant %s", got, want)
	}
}

// eventually waits for the host to reach a state, since lifecycle calls run off the message loop.
func (s *session) eventually(check func(bound map[string]string, finished []string) bool) {
	s.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if check(s.host.state()) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	bound, finished := s.host.state()
	s.t.Fatalf("host state: bound %v, finished %v", bound, finished)
}

func TestRelayThreads(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the scripted protocol uses POSIX paths; the e2e suite runs the relay with native Windows paths")
	}
	s := newSession(t)

	// Unrelated messages pass byte for byte, both ways.
	s.expect(
		s.client(`{"id": 1, "method":"initialize","params":{"x":"<&>"}}`),
		`{"id": 1, "method":"initialize","params":{"x":"<&>"}}`,
	)
	s.expect(s.server(`{"id":1,"result":{}}`), `{"id":1,"result":{}}`)

	// Two threads in different directories get their own servers, grants, and instructions.
	s.expect(
		s.client(`{"id":2,"method":"thread/start","params":{"cwd":"/a","model":"m",`+
			`"config":{"tools.update_plan.enabled":true,"mcp_servers":{"t3-code":{"url":"t3"}}},"developerInstructions":"T3"}}`),
		`{"id":2,"method":"thread/start","params":{"cwd":"/a","model":"m",`+
			`"config":{"tools.update_plan.enabled":true,"mcp_servers":{"agx-m":{"http_headers":{"Authorization":"Bearer L1"},"url":"/a"},"t3-code":{"url":"t3"}}},`+
			`"developerInstructions":"AGX /a\n\nT3"}}`,
	)
	s.expect(s.server(`{"id":2,"result":{"thread":{"id":"A"}}}`), `{"id":2,"result":{"thread":{"id":"A"}}}`)
	s.expect(
		s.client(`{"id":3,"method":"thread/start","params":{"cwd":"/b"}}`),
		`{"id":3,"method":"thread/start","params":{"cwd":"/b","config":{"mcp_servers":{"agx-m":`+
			`{"http_headers":{"Authorization":"Bearer L2"},"url":"/b"}}},"developerInstructions":"AGX /b"}}`,
	)
	s.server(`{"id":3,"result":{"thread":{"id":"B"}}}`)
	s.eventually(func(bound map[string]string, _ []string) bool { return bound["A"] == "L1" && bound["B"] == "L2" })

	// Turns keep the other directory's plugins disabled, merged with the client's list.
	s.expect(
		s.client(`{"id":4,"method":"turn/start","params":{"threadId":"A","cwd":"/a","disabledPluginIds":["x"]}}`),
		`{"id":4,"method":"turn/start","params":{"threadId":"A","cwd":"/a","disabledPluginIds":["x","b@agx-managed"]}}`,
	)
	s.expect(
		s.client(`{"id":5,"method":"turn/start","params":{"threadId":"A","cwd":"/b"}}`),
		`{"id":5,"method":"turn/start","params":{"threadId":"A","cwd":"/b","disabledPluginIds":["a@agx-managed"]}}`,
	)
	s.expect(
		s.client(`{"id":6,"method":"turn/start","params":{"threadId":"unknown"}}`),
		`{"id":6,"method":"turn/start","params":{"threadId":"unknown"}}`,
	)

	// skills/list shows unselected agx plugin skills as disabled for each directory.
	s.expect(
		s.client(`{"id":7,"method":"skills/list","params":{"cwds":["/a"]}}`),
		`{"id":7,"method":"skills/list","params":{"cwds":["/a"]}}`,
	)
	s.expect(
		s.server(`{"id":7,"result":{"data":[{"cwd":"/a","skills":[`+
			`{"name":"a","enabled":true,"pluginId":"a@agx-managed"},`+
			`{"name":"b","enabled":true,"pluginId":"b@agx-managed"},`+
			`{"name":"own","enabled":true,"pluginId":null}],"errors":[]}]}}`),
		`{"id":7,"result":{"data":[{"cwd":"/a","skills":[`+
			`{"name":"a","enabled":true,"pluginId":"a@agx-managed"},`+
			`{"name":"b","enabled":false,"pluginId":"b@agx-managed"},`+
			`{"name":"own","enabled":true,"pluginId":null}],"errors":[]}]}}`,
	)

	// A subagent shares its parent's launch, which ends when both have closed.
	s.server(`{"method":"thread/started","params":{"thread":{"id":"A1","parentThreadId":"A"}}}`)
	s.server(`{"method":"thread/closed","params":{"threadId":"A"}}`)
	s.server(`{"method":"thread/archived","params":{"threadId":"A1"}}`)
	s.eventually(func(_ map[string]string, finished []string) bool { return slices.Equal(finished, []string{"L1"}) })

	// A failed start ends its launch; a start that cannot prepare passes unchanged.
	s.client(`{"id":8,"method":"thread/start","params":{"cwd":"/a"}}`)
	s.server(`{"id":8,"error":{"code":-1,"message":"no"}}`)
	s.eventually(func(_ map[string]string, finished []string) bool { return slices.Contains(finished, "L3") })
	s.expect(
		s.client(`{"id":9,"method":"thread/start","params":{"cwd":"/fail"}}`),
		`{"id":9,"method":"thread/start","params":{"cwd":"/fail"}}`,
	)

	// Resuming gives the thread a fresh launch without instructions and ends the old one.
	s.expect(
		s.client(`{"id":10,"method":"thread/resume","params":{"threadId":"B","excludeTurns":true}}`),
		`{"id":10,"method":"thread/resume","params":{"threadId":"B","excludeTurns":true,"config":{"mcp_servers":{"agx-m":`+
			`{"http_headers":{"Authorization":"Bearer L4"},"url":"/b"}}}}}`,
	)
	s.server(`{"id":10,"result":{"thread":{"id":"B"}}}`)
	s.eventually(func(bound map[string]string, finished []string) bool {
		return bound["B"] == "L4" && slices.Contains(finished, "L2")
	})

	// Forking a thread the relay has not seen asks codex for its directory first.
	go func() {
		_, _ = io.WriteString(s.toRelay, `{"id":11,"method":"thread/fork","params":{"threadId":"old"}}`+"\n")
	}()
	read := strings.TrimSuffix(readLine(t, s.fromSrv), "\n")
	s.expect(read, `{"id":"agx-relay-1","method":"thread/read","params":{"threadId":"old"}}`)
	go func() {
		_, _ = io.WriteString(s.toServer, `{"id":"agx-relay-1","result":{"thread":{"id":"old","cwd":"/a"}}}`+"\n")
	}()
	s.expect(
		strings.TrimSuffix(readLine(t, s.fromSrv), "\n"),
		`{"id":11,"method":"thread/fork","params":{"threadId":"old","config":{"mcp_servers":{"agx-m":`+
			`{"http_headers":{"Authorization":"Bearer L5"},"url":"/a"}}},"developerInstructions":"AGX /a"}}`,
	)

	// When codex exits, every remaining launch ends.
	_ = s.toServer.Close()
	select {
	case err := <-s.done:
		s.done <- err
	case <-time.After(5 * time.Second):
		t.Fatal("relay did not end with the server")
	}
	_, finished := s.host.state()
	for _, id := range []string{"L4", "L5"} {
		if !slices.Contains(finished, id) {
			t.Errorf("launch %s not finished: %v", id, finished)
		}
	}
	if len(s.host.warnings) != 1 || !strings.Contains(s.host.warnings[0], "/fail") {
		t.Errorf("warnings = %q", s.host.warnings)
	}
}
