package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
)

const shapeConfig = `
instructions:
  conventions:
    text: Use pnpm.
skills:
  hello:
    description: Says hello.
    content: Hi.
mcp:
  fs:
    command: [npx, server-filesystem]
  docs:
    url: https://docs.example/mcp
    auth: none
secrets:
  token:
    kind: keychain
harnesses:
  codex-work:
    base: codex
scopes:
  - add:
      instructions: [conventions]
      skills: [hello]
      mcp: [fs, docs]
`

// shape renders the structure of a decoded JSON value: object keys with the shapes of their
// values, an array's first element, and the type of a scalar.
func shape(value any) string {
	switch v := value.(type) {
	case map[string]any:
		fields := []string{}
		for _, key := range slices.Sorted(maps.Keys(v)) {
			fields = append(fields, key+":"+shape(v[key]))
		}
		return "{" + strings.Join(fields, ",") + "}"
	case []any:
		if len(v) == 0 {
			return "[]"
		}
		return "[" + shape(v[0]) + "]"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	}
	return fmt.Sprintf("%T", value)
}

// TestJSONShapes pins the document of each command that runs without a daemon: renaming,
// dropping, or retyping a field changes its shape.
func TestJSONShapes(t *testing.T) {
	isolate(t, shapeConfig)
	t.Setenv("AGX_NO_AUTOSTART", "1")
	t.Chdir(t.TempDir())
	source := "{column:number,line:number,path:string}"
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"config", "validate"}, "{files:[string],root:string}"},
		{
			[]string{"explain", "--harness", "codex-work"},
			"{directory:string,harness:string,id:string,required_environment:[],resources:[{id:string,kind:string}]," +
				"settings:[],traces:[{resource:{id:string,kind:string},selected:bool," +
				"steps:[{change:string,scope_directory:string,source:" + source + "}]}]}",
		},
		{
			[]string{"mcp", "list"},
			"{docs:{auth:{none:bool},load:string,source:" + source + ",transport:string,url:string}," +
				"fs:{auth:{},command:[string],load:string,source:" + source + ",transport:string}}",
		},
		{
			[]string{"skill", "list"},
			"{hello:{content:string,description:string,name:string,source:" + source + "}}",
		},
		{[]string{"instructions", "list"}, "{conventions:{source:" + source + ",text:string}}"},
		{
			[]string{"harness", "list"},
			"[{args:null,builtin:string,chain:[string],environment:{},executable:string,hidden:bool,name:string," +
				"setting_sources:null,settings:{}}]",
		},
		{
			[]string{"sessions"},
			"{errors:{},files:{cached:number,malformed:number,parsed:number},sessions:[]}",
		},
		{[]string{"install", "--frozen"}, "{lockfiles:[],package_ids:[]}"},
		{[]string{"daemon", "stop"}, "{kept_launches:number,running:bool}"},
		{
			[]string{"use", "fs"},
			"{defined:null,disabled:null,enabled:[{id:string,kind:string}],files:null,removed:null," +
				"scope:{directory:string,harnesses:null}}",
		},
	} {
		var data any
		okJSON(t, &data, test.args...)
		if got := shape(data); got != test.want {
			t.Errorf("agx %s --json:\n got %s\nwant %s", strings.Join(test.args, " "), got, test.want)
		}
	}

	changes := "{changes:[{action:string,file:string,name:string}],directory:string,on_path:bool}"
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"shims", "list"}, "{directory:string,on_path:bool,shims:[{file:string,name:string,state:string}],target:string}"},
		{[]string{"shims", "install"}, changes},
		{[]string{"shims", "remove"}, changes},
	} {
		var data any
		okJSON(t, &data, test.args...)
		if got := shape(data); got != test.want {
			t.Errorf("agx %s --json:\n got %s\nwant %s", strings.Join(test.args, " "), got, test.want)
		}
	}

	var out bytes.Buffer
	run([]string{"agx", "launch", "list", "--json"}, &out)
	var document map[string]any
	if err := json.Unmarshal(out.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if got, want := shape(document), "{message:string}"; got != want {
		t.Errorf("error document:\n got %s\nwant %s", got, want)
	}
}

// TestListRedactsLiterals keeps credentials typed into headers and environment values out of
// list documents, while tagged values keep their reference.
func TestListRedactsLiterals(t *testing.T) {
	isolate(t, `
mcp:
  docs:
    url: https://docs.example/mcp
    auth: none
    headers:
      X-Token: literal-header-token
      X-Key: !env DOCS_KEY
  fs:
    command: [server-filesystem]
    environment:
      TOKEN: literal-env-token
harnesses:
  codex-work:
    base: codex
    environment:
      OPENAI_API_KEY: literal-harness-token
`)
	t.Setenv("AGX_NO_AUTOSTART", "1")
	for _, args := range [][]string{{"mcp", "list"}, {"harness", "list"}} {
		_, docs := runJSON(t, append([]string{"--json"}, args...)...)
		if len(docs) != 1 || docs[0].Data == nil {
			t.Fatalf("agx %q: %+v", args, docs[0].Message)
		}
		if data := string(docs[0].Data); strings.Contains(data, "literal-") {
			t.Errorf("agx %q --json shows a literal value: %s", args, data)
		}
	}
	var servers map[string]struct {
		Headers map[string]map[string]string `json:"headers"`
	}
	okJSON(t, &servers, "mcp", "list")
	if got := servers["docs"].Headers["X-Key"]["env"]; got != "DOCS_KEY" {
		t.Errorf("tagged header = %q, want its reference", got)
	}
	if _, ok := servers["docs"].Headers["X-Token"]; !ok {
		t.Error("redacted header is missing")
	}
}

// TestProtoDocuments pins how daemon messages print: field names as in the API, and every
// field present.
func TestProtoDocuments(t *testing.T) {
	var out bytes.Buffer
	scripting.stdout = &out
	t.Cleanup(func() { scripting.stdout = os.Stdout })
	expires := timestamppb.New(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	for _, message := range []proto.Message{
		&v1.LoginResponse{Event: &v1.LoginResponse_Started{Started: &v1.LoginStarted{
			AuthorizationUrl: "https://example.com/authorize",
			ExpiresAt:        expires,
			McpId:            "github",
		}}},
		&v1.LoginResponse{Event: &v1.LoginResponse_Finished{Finished: &v1.LoginFinished{
			Outcome: v1.LoginFinished_OUTCOME_SUCCEEDED,
		}}},
		&v1.SetSecretResponse{},
	} {
		if err := printJSON(message); err != nil {
			t.Fatal(err)
		}
	}
	want := `{"version":1,"data":{"started":{"authorization_url":"https://example.com/authorize",` +
		`"expires_at":"2026-01-02T03:04:05Z","mcp_id":"github"}}}
{"version":1,"data":{"finished":{"outcome":"OUTCOME_SUCCEEDED","message":""}}}
{"version":1,"data":{}}
`
	if out.String() != want {
		t.Fatalf("documents:\n%s\nwant:\n%s", out.String(), want)
	}
}
