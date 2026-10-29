//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestMCPCalls covers agx mcp tools, call, and types from a shell and from inside a launch.
func TestMCPCalls(t *testing.T) {
	e := newEnv(t)
	starts := filepath.Join(e.dir, "starts")
	mcp := filepath.ToSlash(binaries.mcp)
	e.write("config/main.yaml", `mcp:
  echo:
    command: ['`+mcp+`']
    environment:
      ECHO_TOKEN: !env E2E_TOKEN
      FAKEMCP_STARTS: '`+filepath.ToSlash(starts)+`'
  tracker:
    command: ['`+mcp+`']
    description: Issues of the platform team
    load: on_demand
  elsewhere:
    command: ['`+mcp+`']
scopes:
  - directory: ../work
    add:
      mcp: [echo, tracker]
harnesses:
  claude:
    executable: '`+filepath.ToSlash(binaries.harness)+`'
`)
	e.ok("daemon", "start")
	t.Cleanup(func() { e.stopDaemon(true) })

	type tool struct {
		Name         string `json:"name"`
		InputSchema  string `json:"input_schema_json"`
		OutputSchema string `json:"output_schema_json"`
	}
	type server struct {
		ID          string `json:"mcp_id"`
		Description string `json:"description"`
		Load        string `json:"load"`
		State       string `json:"state"`
		Tools       []tool `json:"tools"`
	}
	var listed struct {
		MCP []server `json:"servers"`
	}

	t.Run("tools", func(t *testing.T) {
		e.jsonOK(&listed, "mcp", "tools")
		ids := []string{}
		for _, s := range listed.MCP {
			ids = append(ids, s.ID+"/"+s.Load+"/"+s.State)
		}
		if !slices.Equal(ids, []string{"echo/LOAD_NATIVE/STATE_READY", "tracker/LOAD_ON_DEMAND/STATE_READY"}) {
			t.Fatalf("servers = %q", ids)
		}
		if d := listed.MCP[1].Description; d != "Issues of the platform team" {
			t.Fatalf("description = %v", d)
		}
		e.jsonOK(&listed, "mcp", "tools", "echo")
		if len(listed.MCP) != 1 || listed.MCP[0].Tools[1].Name != "shape" || listed.MCP[0].Tools[1].InputSchema == "" ||
			listed.MCP[0].Tools[1].OutputSchema == "" {
			t.Fatalf("tools echo = %+v", listed)
		}
		e.jsonOK(&listed, "mcp", "tools", "--all")
		if len(listed.MCP) != 3 {
			t.Fatalf("tools --all = %+v", listed)
		}
		e.jsonFail("not_found", "mcp", "tools", "elsewhere")
		human := e.ok("mcp", "tools", "echo")
		if !strings.Contains(
			human.stdout,
			"shape(query: string, exact?: boolean, filter?: { owner: string }, limit?: number",
		) ||
			!strings.Contains(human.stdout, "  Answer the arguments.\n") {
			t.Fatalf("tools echo printed %q", human.stdout)
		}
	})

	type content struct {
		Type     string  `json:"type"`
		Text     *string `json:"text"`
		MIMEType *string `json:"mimeType"`
		Path     *string `json:"path"`
		Size     *int64  `json:"size"`
		// Resource holds an embedded resource, whose binary content goes to a file too.
		Resource *struct {
			Text *string `json:"text"`
			Path *string `json:"path"`
		} `json:"resource"`
	}
	var called struct {
		MCPID             string          `json:"mcp_id"`
		Tool              string          `json:"tool"`
		IsError           bool            `json:"is_error"`
		Content           []content       `json:"content"`
		StructuredContent json.RawMessage `json:"structured_content"`
	}

	t.Run("call", func(t *testing.T) {
		e.jsonOK(&called, "mcp", "call", "echo.echo", "text=hi")
		if called.MCPID != "echo" || called.Tool != "echo" || called.IsError || len(called.Content) != 1 ||
			*called.Content[0].Text != "echo:hi:"+token || string(called.StructuredContent) != "null" {
			t.Fatalf("call echo.echo = %+v", called)
		}
		e.jsonOK(&called, "mcp", "call", "echo.shape", "query=bug", "status=open", "limit=5", "tags=a", "tags=b")
		if want := `{"received":{"limit":5,"query":"bug","status":"open","tags":["a","b"]}}`; string(
			called.StructuredContent,
		) != want {
			t.Fatalf("structured content = %s, want %s", called.StructuredContent, want)
		}
		r := e.runInput(binaries.agx, nil, `{"query":"stdin"}`, "mcp", "call", "echo.shape", "--args", "-")
		var echoed struct {
			Query string `json:"query"`
		}
		if r.code != 0 || json.Unmarshal([]byte(r.stdout), &echoed) != nil || echoed.Query != "stdin" {
			t.Fatalf("--args - exited %d: %q %q", r.code, r.stdout, r.stderr)
		}
		e.jsonOK(&called, "mcp", "call", "tracker.echo", "text=on-demand")
		if *called.Content[0].Text != "echo:on-demand:" {
			t.Fatalf("on-demand call = %+v", called)
		}
		e.jsonOK(&called, "mcp", "call", "elsewhere.echo", "text=x", "--all")

		e.jsonFail("not_found", "mcp", "call", "elsewhere.echo")
		e.jsonFail("not_found", "mcp", "call", "missing.echo")
		e.jsonFail("invalid_request", "mcp", "call", "echo.shap")
		e.jsonFail("invalid_request", "mcp", "call", "echo.shape", "query=q", "status=bogus")
		e.jsonFail("invalid_request", "mcp", "call", "echo.shape", "limit=1")
		e.jsonFail("mcp_failed", "mcp", "call", "echo.explode")
		if r := e.agx(nil, "mcp", "call", "echo.shape", "query"); r.code != 2 {
			t.Fatalf("malformed argument exited %d", r.code)
		}
		if r := e.agx(nil, "mcp", "call", "echo"); r.code != 2 {
			t.Fatalf("a call without a tool exited %d", r.code)
		}

		// A result marked as an error still prints its document, and exits 1.
		failed := e.agx(nil, "mcp", "call", "echo.fail", "--json")
		var doc envelope
		if failed.code != 1 || json.Unmarshal([]byte(failed.stdout), &doc) != nil ||
			json.Unmarshal(
				doc.Data,
				&called,
			) != nil || !called.IsError || *called.Content[0].Text != "failed on purpose" {
			t.Fatalf("isError call exited %d: %s", failed.code, failed.stdout)
		}
		if r := e.agx(nil, "mcp", "call", "echo.fail"); r.code != 1 || r.stdout != "failed on purpose\n" {
			t.Fatalf("isError call exited %d: %q", r.code, r.stdout)
		}
	})

	t.Run("binary content goes to files", func(t *testing.T) {
		dir := filepath.Join(e.dir, "saved")
		for range 2 {
			e.jsonOK(&called, "mcp", "call", "echo.media", "--save-dir", dir)
		}
		image, resource := called.Content[1], called.Content[2]
		if len(called.Content) != 3 || image.Type != "image" || *image.MIMEType != "image/png" ||
			*image.Size != 67 || resource.Type != "resource" || resource.Resource == nil || resource.Resource.Text != nil {
			t.Fatalf("media = %+v", called.Content)
		}
		imagePath, pdf := *image.Path, *resource.Resource.Path
		if filepath.Dir(imagePath) != dir || filepath.Ext(imagePath) != ".png" || filepath.Ext(pdf) != ".pdf" ||
			filepath.Base(imagePath) == "1.png" {
			t.Fatalf("a second call into the same directory wrote %s and %s", imagePath, pdf)
		}
		if b, err := os.ReadFile(pdf); err != nil || string(b) != "%PDF-1.4" {
			t.Fatalf("pdf = %q, %v", b, err)
		}
		r := e.ok("mcp", "call", "echo.media")
		lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
		if len(lines) != 3 || lines[0] != "a picture" || !strings.HasPrefix(lines[1], "[image/png, 67 B] ") {
			t.Fatalf("media printed %q", r.stdout)
		}
		temporary := strings.TrimPrefix(lines[1], "[image/png, 67 B] ")
		if !strings.HasPrefix(filepath.Base(filepath.Dir(temporary)), "agx-mcp-") {
			t.Fatalf("image went to %s", temporary)
		}
		_ = os.RemoveAll(filepath.Dir(temporary))
	})

	t.Run("sessions are pooled", func(t *testing.T) {
		b, err := os.ReadFile(starts)
		if err != nil || strings.Count(string(b), "start") != 1 {
			t.Fatalf("echo started %d times for every call from one directory", strings.Count(string(b), "start"))
		}
	})

	t.Run("inside a launch", func(t *testing.T) {
		agx := filepath.ToSlash(binaries.agx)
		commands, _ := json.Marshal([][]string{
			{agx, "mcp", "tools", "--json"},
			{agx, "mcp", "call", "tracker.shape", "query=from-launch", "--json"},
			{agx, "mcp", "call", "elsewhere.echo", "--json"},
			{agx, "mcp", "call", "elsewhere.echo", "text=all", "--all", "--json"},
		})
		path := filepath.Join(e.dir, "dump.json")
		r := e.agx(
			[]string{"FAKE_DUMP=" + path, "FAKE_EXEC=" + string(commands), "FAKE_ENV=AGX_LAUNCH_ID,AGX_MCP_GRANT"},
			"claude",
			"-p",
			"hi",
		)
		if r.code != 0 {
			t.Fatalf("launch exited %d: %s", r.code, r.stderr)
		}
		var d struct {
			dump
			Exec []struct {
				Code   int    `json:"code"`
				Stdout string `json:"stdout"`
				Stderr string `json:"stderr"`
			} `json:"exec"`
		}
		b, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(b, &d) != nil {
			t.Fatalf("dump: %v", err)
		}
		if _, ok := d.Endpoints["agx-tracker"]; ok || d.Endpoints["agx-echo"] == "" {
			t.Fatalf("harness endpoints = %v; on-demand servers stay out", d.Endpoints)
		}
		if d.Env["AGX_LAUNCH_ID"] == "" || d.Env["AGX_MCP_GRANT"] == "" {
			t.Fatalf("harness environment = %v", d.Env)
		}
		var doc envelope
		decode := func(i int) {
			t.Helper()
			if err := json.Unmarshal([]byte(d.Exec[i].Stdout), &doc); err != nil {
				t.Fatalf("command %d printed %q (%q)", i, d.Exec[i].Stdout, d.Exec[i].Stderr)
			}
		}
		decode(0)
		if err := json.Unmarshal(
			doc.Data,
			&listed,
		); err != nil || len(listed.MCP) != 2 ||
			listed.MCP[1].ID != "tracker" {
			t.Fatalf("tools in the launch = %s", d.Exec[0].Stdout)
		}
		decode(1)
		if d.Exec[1].Code != 0 || !strings.Contains(string(doc.Data), `from-launch`) {
			t.Fatalf("on-demand call in the launch exited %d: %s", d.Exec[1].Code, d.Exec[1].Stdout)
		}
		decode(2)
		if d.Exec[2].Code != 1 || doc.Error == nil || doc.Error.Code != "not_found" ||
			!strings.Contains(doc.Error.Message, "not selected for this launch") {
			t.Fatalf("out-of-launch call exited %d: %s", d.Exec[2].Code, d.Exec[2].Stdout)
		}
		decode(3)
		if d.Exec[3].Code != 0 || !strings.Contains(string(doc.Data), "echo:all:") {
			t.Fatalf("--all in the launch exited %d: %s %s", d.Exec[3].Code, d.Exec[3].Stdout, d.Exec[3].Stderr)
		}

		forged := e.agx([]string{"AGX_LAUNCH_ID=" + d.Env["AGX_LAUNCH_ID"], "AGX_MCP_GRANT=forged"},
			"mcp", "call", "echo.echo", "--json")
		if forged.code != 1 || !strings.Contains(forged.stdout, "invalid or expired launch grant") {
			t.Fatalf("a forged grant exited %d: %s", forged.code, forged.stdout)
		}
		ended := e.agx([]string{"AGX_LAUNCH_ID=" + d.Env["AGX_LAUNCH_ID"], "AGX_MCP_GRANT=" + d.Env["AGX_MCP_GRANT"]},
			"mcp", "call", "echo.echo", "--json")
		if ended.code != 1 || !strings.Contains(ended.stdout, "invalid or expired launch grant") {
			t.Fatalf("the grant of a finished launch exited %d: %s", ended.code, ended.stdout)
		}
	})

	t.Run("types", func(t *testing.T) {
		out := filepath.Join(e.dir, "generated")
		var written struct {
			MCP   []string `json:"mcp"`
			Files []string `json:"files"`
		}
		e.jsonOK(&written, "mcp", "types", "--out", out)
		if !slices.Equal(written.MCP, []string{"echo", "tracker"}) || len(written.Files) != 2 {
			t.Fatalf("types = %+v", written)
		}
		declarations, err := os.ReadFile(filepath.Join(out, "agx-mcp.d.mts"))
		if err != nil || !strings.Contains(string(declarations), "export declare const tracker:") {
			t.Fatalf("declarations: %v\n%s", err, declarations)
		}
		r := e.ok("mcp", "types", "echo")
		if !strings.Contains(r.stdout, "shape(args: {") {
			t.Fatalf("types echo printed %q", r.stdout)
		}
	})
}
