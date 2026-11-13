package appserver

import (
	"encoding/json"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/adapters/relay"
	"github.com/tarik02-org/agx/internal/adapters/relay/relaytest"
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
		theirs, ok := relay.Parse(json.RawMessage(tt.theirs))
		if !ok {
			t.Fatalf("parse %s", tt.theirs)
		}
		if got := string(merge(theirs, ours).Marshal()); got != tt.want {
			t.Errorf("merge(%s):\n got %s\nwant %s", tt.theirs, got, tt.want)
		}
	}
}

func TestRelayThreads(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the scripted protocol uses POSIX paths; the e2e suite runs the relay with native Windows paths")
	}
	s := relaytest.Start(t, &relaytest.Host{
		Config: func(directory, id string) string {
			return fmt.Sprintf(
				`{"mcp_servers":{"agx-m":{"url":"%s","http_headers":{"Authorization":"Bearer %s"}}}}`,
				directory,
				id,
			)
		},
		Instructions: func(directory string) string { return "AGX " + directory },
		Disabled:     map[string][]string{"/a": {"b@agx-managed"}, "/b": {"a@agx-managed"}},
	}, Relay)

	// Unrelated messages pass byte for byte, both ways.
	s.Expect(
		s.Client(`{"id": 1, "method":"initialize","params":{"x":"<&>"}}`),
		`{"id": 1, "method":"initialize","params":{"x":"<&>"}}`,
	)
	s.Expect(s.Server(`{"id":1,"result":{}}`), `{"id":1,"result":{}}`)

	// Two threads in different directories get their own servers, grants, and instructions.
	s.Expect(
		s.Client(`{"id":2,"method":"thread/start","params":{"cwd":"/a","model":"m",`+
			`"config":{"tools.update_plan.enabled":true,"mcp_servers":{"t3-code":{"url":"t3"}}},"developerInstructions":"T3"}}`),
		`{"id":2,"method":"thread/start","params":{"cwd":"/a","model":"m",`+
			`"config":{"tools.update_plan.enabled":true,"mcp_servers":{"agx-m":{"http_headers":{"Authorization":"Bearer L1"},"url":"/a"},"t3-code":{"url":"t3"}}},`+
			`"developerInstructions":"AGX /a\n\nT3"}}`,
	)
	s.Expect(s.Server(`{"id":2,"result":{"thread":{"id":"A"}}}`), `{"id":2,"result":{"thread":{"id":"A"}}}`)
	s.Expect(
		s.Client(`{"id":3,"method":"thread/start","params":{"cwd":"/b"}}`),
		`{"id":3,"method":"thread/start","params":{"cwd":"/b","config":{"mcp_servers":{"agx-m":`+
			`{"http_headers":{"Authorization":"Bearer L2"},"url":"/b"}}},"developerInstructions":"AGX /b"}}`,
	)
	s.Server(`{"id":3,"result":{"thread":{"id":"B"}}}`)
	s.Eventually(func(bound map[string]string, _ []string) bool { return bound["A"] == "L1" && bound["B"] == "L2" })

	// Turns keep the other directory's plugins disabled, merged with the client's list.
	s.Expect(
		s.Client(`{"id":4,"method":"turn/start","params":{"threadId":"A","cwd":"/a","disabledPluginIds":["x"]}}`),
		`{"id":4,"method":"turn/start","params":{"threadId":"A","cwd":"/a","disabledPluginIds":["x","b@agx-managed"]}}`,
	)
	s.Expect(
		s.Client(`{"id":5,"method":"turn/start","params":{"threadId":"A","cwd":"/b"}}`),
		`{"id":5,"method":"turn/start","params":{"threadId":"A","cwd":"/b","disabledPluginIds":["a@agx-managed"]}}`,
	)
	s.Expect(
		s.Client(`{"id":6,"method":"turn/start","params":{"threadId":"unknown"}}`),
		`{"id":6,"method":"turn/start","params":{"threadId":"unknown"}}`,
	)

	// skills/list shows unselected agx plugin skills as disabled for each directory.
	s.Expect(
		s.Client(`{"id":7,"method":"skills/list","params":{"cwds":["/a"]}}`),
		`{"id":7,"method":"skills/list","params":{"cwds":["/a"]}}`,
	)
	s.Expect(
		s.Server(`{"id":7,"result":{"data":[{"cwd":"/a","skills":[`+
			`{"name":"a","enabled":true,"pluginId":"a@agx-managed"},`+
			`{"name":"b","enabled":true,"pluginId":"b@agx-managed"},`+
			`{"name":"own","enabled":true,"pluginId":null}],"errors":[]}]}}`),
		`{"id":7,"result":{"data":[{"cwd":"/a","skills":[`+
			`{"name":"a","enabled":true,"pluginId":"a@agx-managed"},`+
			`{"name":"b","enabled":false,"pluginId":"b@agx-managed"},`+
			`{"name":"own","enabled":true,"pluginId":null}],"errors":[]}]}}`,
	)

	// A subagent shares its parent's launch, which ends when both have closed.
	s.Server(`{"method":"thread/started","params":{"thread":{"id":"A1","parentThreadId":"A"}}}`)
	s.Server(`{"method":"thread/closed","params":{"threadId":"A"}}`)
	s.Server(`{"method":"thread/archived","params":{"threadId":"A1"}}`)
	s.Eventually(func(_ map[string]string, finished []string) bool { return slices.Equal(finished, []string{"L1"}) })

	// A failed start ends its launch; a start that cannot prepare passes unchanged.
	s.Client(`{"id":8,"method":"thread/start","params":{"cwd":"/a"}}`)
	s.Server(`{"id":8,"error":{"code":-1,"message":"no"}}`)
	s.Eventually(func(_ map[string]string, finished []string) bool { return slices.Contains(finished, "L3") })
	s.Expect(
		s.Client(`{"id":9,"method":"thread/start","params":{"cwd":"/fail"}}`),
		`{"id":9,"method":"thread/start","params":{"cwd":"/fail"}}`,
	)

	// Resuming gives the thread a fresh launch without instructions and ends the old one.
	s.Expect(
		s.Client(`{"id":10,"method":"thread/resume","params":{"threadId":"B","excludeTurns":true}}`),
		`{"id":10,"method":"thread/resume","params":{"threadId":"B","excludeTurns":true,"config":{"mcp_servers":{"agx-m":`+
			`{"http_headers":{"Authorization":"Bearer L4"},"url":"/b"}}}}}`,
	)
	s.Server(`{"id":10,"result":{"thread":{"id":"B"}}}`)
	s.Eventually(func(bound map[string]string, finished []string) bool {
		return bound["B"] == "L4" && slices.Contains(finished, "L2")
	})

	// Forking a thread the relay has not seen asks codex for its directory first.
	s.Send(`{"id":11,"method":"thread/fork","params":{"threadId":"old"}}`)
	s.Expect(s.Received(), `{"id":"agx-relay-1","method":"thread/read","params":{"threadId":"old"}}`)
	s.Answer(`{"id":"agx-relay-1","result":{"thread":{"id":"old","cwd":"/a"}}}`)
	s.Expect(
		s.Received(),
		`{"id":11,"method":"thread/fork","params":{"threadId":"old","config":{"mcp_servers":{"agx-m":`+
			`{"http_headers":{"Authorization":"Bearer L5"},"url":"/a"}}},"developerInstructions":"AGX /a"}}`,
	)

	// When codex exits, every remaining launch ends.
	s.Exit()
	_, finished := s.Host.State()
	for _, id := range []string{"L4", "L5"} {
		if !slices.Contains(finished, id) {
			t.Errorf("launch %s not finished: %v", id, finished)
		}
	}
	if warnings := s.Host.Warnings(); len(warnings) != 1 || !strings.Contains(warnings[0], "/fail") {
		t.Errorf("warnings = %q", warnings)
	}
}
