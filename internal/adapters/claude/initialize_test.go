package claude_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/adapters/claude"
)

func TestRelayInput(t *testing.T) {
	const user = `{"type":"user","message":{"role":"user","content":"a <b> & \"initialize\""}}` + "\n"
	for _, tt := range []struct {
		name, in string
		// append is the expected appendSystemPrompt of the first line, or "-" when the line must
		// pass unchanged.
		append string
	}{
		{
			name: "string append",
			in: `{"type":"control_request","request_id":"r1","request":{"subtype":"initialize","hooks":{},` +
				`"appendSystemPrompt":"T3 <text>","future":[1,2]},"extra":true}` + "\n" + user,
			append: "AGX\n\nT3 <text>",
		},
		{
			name:   "absent append",
			in:     `{"type":"control_request","request_id":"r1","request":{"subtype":"initialize"}}` + "\n" + user,
			append: "-",
		},
		{
			name: "null append",
			in: `{"type":"control_request","request_id":"r1","request":{"subtype":"initialize",` +
				`"appendSystemPrompt":null}}` + "\n" + user,
			append: "-",
		},
		{
			name: "non-string append",
			in: `{"type":"control_request","request_id":"r1","request":{"subtype":"initialize",` +
				`"appendSystemPrompt":["x"]}}` + "\n" + user,
			append: "-",
		},
		{
			name:   "no initialize",
			in:     user + `{"type":"control_request","request":{"subtype":"interrupt"}}` + "\n",
			append: "-",
		},
		{
			name:   "invalid json",
			in:     "{\"initialize\"\n" + user,
			append: "-",
		},
		{
			name:   "no trailing newline",
			in:     `{"type":"control_request","request":{"subtype":"initialize","appendSystemPrompt":""}}`,
			append: "AGX\n\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := claude.RelayInput(&out, strings.NewReader(tt.in), "AGX"); err != nil {
				t.Fatal(err)
			}
			if tt.append == "-" {
				if out.String() != tt.in {
					t.Fatalf("output changed:\n got %q\nwant %q", out.String(), tt.in)
				}
				return
			}
			first, rest, _ := strings.Cut(out.String(), "\n")
			_, wantRest, _ := strings.Cut(tt.in, "\n")
			if rest != wantRest {
				t.Errorf("later lines changed:\n got %q\nwant %q", rest, wantRest)
			}
			var got, want map[string]any
			if err := json.Unmarshal([]byte(first), &got); err != nil {
				t.Fatal(err)
			}
			wantFirst, _, _ := strings.Cut(tt.in, "\n")
			if err := json.Unmarshal([]byte(wantFirst), &want); err != nil {
				t.Fatal(err)
			}
			want["request"].(map[string]any)["appendSystemPrompt"] = tt.append
			if a, b := mustJSON(t, got), mustJSON(t, want); a != b {
				t.Errorf("initialize:\n got %s\nwant %s", a, b)
			}
			if strings.Contains(first, `\u003c`) {
				t.Errorf("initialize escapes HTML: %s", first)
			}
		})
	}
}

func TestRelayInputMergesOnlyFirstInitialize(t *testing.T) {
	init := `{"type":"control_request","request":{"subtype":"initialize","appendSystemPrompt":"x"}}` + "\n"
	big := `{"type":"user","message":"` + strings.Repeat("y", 1<<20) + `"}` + "\n"
	var out bytes.Buffer
	if err := claude.RelayInput(&out, strings.NewReader(big+init+init+big), "AGX"); err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitAfter(out.String(), "\n")
	if len(lines) != 5 || lines[0] != big || lines[2] != init || lines[3] != big {
		t.Fatalf("lines after the first initialize changed: %d lines", len(lines))
	}
	if !strings.Contains(lines[1], `"AGX\n\nx"`) {
		t.Errorf("first initialize = %s", lines[1])
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
