package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

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
	source := "{line:number,path:string}"
	for _, test := range []struct {
		args []string
		want string
	}{
		{
			[]string{"config", "validate"},
			"{counts:{harnesses:number,instructions:number,mcp:number,packages:number,scopes:number," +
				"secrets:number,skills:number},diagnostics:[],files:[string],root:string,valid:bool}",
		},
		{
			[]string{"explain", "--harness", "codex-work"},
			"{directory:string,harness:string,id:string,injections:[],required_environment:[]," +
				"resources:[{id:string,kind:string,load:null,share:null}],settings:[],traces:[{order:null,resource:{id:string,kind:string,load:null,share:null},selected:bool," +
				"steps:[{bundle_chain:[],change:string,harness:null,scope_directory:null,source:" + source + "}]}]}",
		},
		{
			[]string{"mcp", "list"},
			"{mcp:[{auth:{kind:string,on_demand:bool,secret_id:null},command:null,description:null,id:string," +
				"inline:bool,load:string,share:string,source:" + source + ",transport:string,url:string}]}",
		},
		{
			[]string{"skill", "list"},
			"{skills:[{from:null,id:string,inline:bool,kind:string,source:" + source + "}]}",
		},
		{
			[]string{"instructions", "list"},
			"{instructions:[{id:string,inline:bool,kind:string,order:null,path:null,source:" + source + "}]}",
		},
		{
			[]string{"harness", "list"},
			"{harnesses:[{base:null,builtin:string,executable:null,name:string,resolved_settings:{},settings:null,source:null}]}",
		},
		{
			[]string{"sessions"},
			"{directory:string,errors:{},files:{cached:number,malformed:number,parsed:number},query:null,scope:string,sessions:[]}",
		},
		{
			[]string{"install", "--frozen"},
			"{frozen:bool,lockfiles:[],packages:[],updated:bool}",
		},
		{
			[]string{"daemon", "stop"},
			"{forced:bool,kept_launches:number,running:bool,stopping:bool,version:null}",
		},
		{
			[]string{"use", "fs"},
			"{defined:[],disabled:[],enabled:[{id:string,kind:string}],files:[],needs_login:null,removed:[]," +
				"scope:{directory:null,harnesses:[]}}",
		},
	} {
		var data any
		okJSON(t, &data, test.args...)
		if got := shape(data); got != test.want {
			t.Errorf("agx %s --json:\n got %s\nwant %s", strings.Join(test.args, " "), got, test.want)
		}
	}

	dir := t.TempDir()
	for _, test := range []struct {
		args []string
		want string
	}{
		{
			[]string{"shims", "list", "--dir", dir},
			"{directory:string,on_path:bool,shims:[{file:string,kind:null,name:string,state:string,target:null}],target:string}",
		},
		{
			[]string{"shims", "install", "--dir", dir},
			"{changes:[{action:string,file:string,kind:string,name:string,state:null,target:string}]," +
				"directory:string,on_path:bool,target:string}",
		},
		{
			[]string{"shims", "remove", "--dir", dir},
			"{changes:[{action:string,file:string,kind:string,name:string,state:null,target:string}]," +
				"directory:string,on_path:bool,target:null}",
		},
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
	if got, want := shape(document), "{error:{code:string,message:string,source:null},version:number}"; got != want {
		t.Errorf("error document:\n got %s\nwant %s", got, want)
	}
}

// TestDaemonDocuments pins the documents built from daemon messages, values included: enums,
// times, and absent values.
func TestDaemonDocuments(t *testing.T) {
	expires := timestamppb.New(time.Date(2026, 1, 2, 3, 4, 5, 6, time.FixedZone("x", 3600)))
	view := &v1.RuntimeView{
		DaemonInstanceId: "instance",
		Version:          "0.1.0",
		Launches: []*v1.Launch{
			{
				Id:         "l1",
				SnapshotId: "s1",
				Directory:  "/work",
				Harness:    "codex",
				State:      v1.Launch_STATE_RUNNING,
				ExpiresAt:  expires,
				Mode:       "app-server",
			},
			{
				Id:             "l2",
				SnapshotId:     "s2",
				Directory:      "/work/sub",
				Harness:        "codex",
				State:          v1.Launch_State(99),
				Mode:           "interactive",
				ParentLaunchId: "l1",
				SessionId:      "thread",
			},
		},
		McpServers: []*v1.McpRuntime{{
			LaunchId: "l1",
			McpId:    "docs",
			State:    v1.McpRuntime_STATE_NEEDS_AUTH,
			Diagnostics: []*v1.Diagnostic{{
				Severity: v1.Diagnostic_SEVERITY_WARNING,
				Code:     "mcp_needs_auth",
				Summary:  "log in",
				Source:   &v1.SourceLocation{Path: "/agx.yaml", Line: 3},
			}},
		}},
	}
	for _, test := range []struct {
		name     string
		document any
		want     string
	}{
		{
			"daemon status",
			daemonStatusDocument(view),
			`{"running":true,"version":"0.1.0","instance_id":"instance","launch_count":2,"running_launch_count":1,` +
				`"mcp_server_count":1,"diagnostics":[]}`,
		},
		{
			"launch list",
			launchListDocument(view),
			`{"launches":[` +
				`{"id":"l1","harness":"codex","mode":"app-server","state":"running","directory":"/work",` +
				`"snapshot_id":"s1","expires_at":"2026-01-02T02:04:05Z","parent_launch_id":null,"session_id":null,` +
				`"mcp_servers":[{"mcp_id":"docs","state":"needs_auth","diagnostics":[{"severity":"warning",` +
				`"code":"mcp_needs_auth","message":"log in","source":{"path":"/agx.yaml","line":3}}]}]},` +
				`{"id":"l2","harness":"codex","mode":"interactive","state":"unknown","directory":"/work/sub",` +
				`"snapshot_id":"s2","expires_at":null,"parent_launch_id":"l1","session_id":"thread","mcp_servers":[]}]}`,
		},
		{
			"empty launch list",
			launchListDocument(&v1.RuntimeView{}),
			`{"launches":[]}`,
		},
		{
			"secret status",
			secretStatusDocument([]*v1.SecretStatus{
				{SecretId: "token", State: v1.SecretStatus_STATE_NEEDS_LOGIN, Kind: "keychain"},
				{SecretId: "docs", State: v1.SecretStatus_STATE_READY, Kind: "oauth", ExpiresAt: expires, McpId: "docs", OnDemand: true},
			}),
			`{"secrets":[` +
				`{"id":"token","kind":"keychain","state":"needs_login","expires_at":null,"mcp_id":null,` +
				`"on_demand":false,"diagnostics":[]},` +
				`{"id":"docs","kind":"oauth","state":"ready","expires_at":"2026-01-02T02:04:05Z","mcp_id":"docs",` +
				`"on_demand":true,"diagnostics":[]}]}`,
		},
		{
			"empty secret status",
			secretStatusDocument(nil),
			`{"secrets":[]}`,
		},
	} {
		b, err := json.Marshal(test.document)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != test.want {
			t.Errorf("%s:\n got %s\nwant %s", test.name, b, test.want)
		}
	}
}
