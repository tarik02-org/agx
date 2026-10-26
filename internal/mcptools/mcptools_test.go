package mcptools_test

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/mcptools"
)

const issues = `{"type":"object","properties":{
	"query":{"type":"string"},
	"status":{"enum":["open","closed"]},
	"limit":{"type":"integer"},
	"ratio":{"type":"number"},
	"exact":{"type":"boolean"},
	"note":{"type":["string","null"]},
	"tags":{"type":"array","items":{"type":"string"}},
	"ids":{"type":"array","items":{"type":"integer"}},
	"filter":{"type":"object","properties":{"owner":{"type":"string"},"deep":{"type":"object","properties":{"x":{"type":"string"}}}},"required":["owner"]},
	"kind":{"$ref":"#/$defs/kind"},
	"mode":{"anyOf":[{"const":"fast"},{"const":"slow"}]},
	"weird-name":{"type":"string"}
},"required":["query"],"additionalProperties":false,"$defs":{"kind":{"enum":["bug","task"]}}}`

func schema(t *testing.T, text string) *mcptools.Schema {
	t.Helper()
	s, err := mcptools.ParseSchema(text)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSignature(t *testing.T) {
	output := schema(t, `{"type":"object","properties":{"total":{"type":"integer"}},"required":["total"]}`)
	got := mcptools.Signature("search_issues", schema(t, issues), output)
	want := `search_issues(query: string, exact?: boolean, filter?: { owner: string; deep?: {…} }, ` +
		`ids?: number[], kind?: "bug" | "task", limit?: number, mode?: "fast" | "slow", ` +
		`note?: string | null, ratio?: number, status?: "open" | "closed", tags?: string[], ` +
		`"weird-name"?: string): { total: number }`
	if got != want {
		t.Fatalf("signature\n got %s\nwant %s", got, want)
	}
	for _, test := range []struct{ input, want string }{
		{``, "ping()"},
		{`{"type":"object"}`, "ping()"},
		{`{"type":"object","additionalProperties":{"type":"string"}}`, "ping(...args: Record<string, string>)"},
		{
			`{"type":"object","properties":{"a":{"type":"array","items":{"type":["string","number"]}}}}`,
			"ping(a?: (string | number)[])",
		},
		{`{"type":"object","properties":{"self":{"$ref":"#"}}}`, "ping(self?: { self?: {…} })"},
		{`{"type":"object","properties":{"a":true,"b":false},"additionalProperties":false}`, "ping(a?: unknown, b?: unknown)"},
	} {
		if got := mcptools.Signature("ping", schema(t, test.input), nil); got != test.want {
			t.Errorf("Signature(%s) = %s, want %s", test.input, got, test.want)
		}
	}
}

func TestFirstLine(t *testing.T) {
	if got := mcptools.FirstLine("  Search issues.\nMore detail.", 80); got != "Search issues." {
		t.Fatalf("FirstLine = %q", got)
	}
	if got := mcptools.FirstLine(strings.Repeat("a", 20), 10); got != "aaaaaaaaa…" {
		t.Fatalf("FirstLine = %q", got)
	}
}

func TestClosest(t *testing.T) {
	names := []string{"search_issues", "get_issue", "create_issue", "list_projects"}
	if got := mcptools.Closest("serch_issues", names, 3); !slices.Equal(got, []string{"search_issues"}) {
		t.Fatalf("Closest = %q", got)
	}
	if got := mcptools.Closest(
		"issue",
		names,
		3,
	); !slices.Equal(
		got,
		[]string{"get_issue", "create_issue", "search_issues"},
	) {
		t.Fatalf("Closest = %q", got)
	}
	if got := mcptools.Suggest("zzz", names); got != "; it has search_issues, get_issue, create_issue, list_projects" {
		t.Fatalf("Suggest = %q", got)
	}
}

func arguments(t *testing.T, s *mcptools.Schema, base string, raw bool, pairs ...string) (string, error) {
	t.Helper()
	var object *mcptools.Object
	if base != "" {
		var err error
		if object, err = mcptools.ParseObject([]byte(base)); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{"body.md": "# Body\n", "filter.json": `{"owner":"me"}`}
	read := func(path string) ([]byte, error) {
		if content, ok := files[path]; ok {
			return []byte(content), nil
		}
		return nil, os.ErrNotExist
	}
	out, err := mcptools.Arguments(s, object, pairs, raw, read)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(b), nil
}

func TestArguments(t *testing.T) {
	s := schema(t, issues)
	for _, test := range []struct {
		name  string
		base  string
		raw   bool
		pairs []string
		want  string
	}{
		{"coercion", "", false, []string{
			"query=login bug", "status=open", "limit=5", "ratio=0.5", "exact=true", "note=null",
			"tags=a", "tags=b", "ids=7", `filter={"owner":"me"}`, "kind=bug", "mode=slow", "weird-name=x",
		}, `{"query":"login bug","status":"open","limit":5,"ratio":0.5,"exact":true,"note":null,` +
			`"tags":["a","b"],"ids":[7],"filter":{"owner":"me"},"kind":"bug","mode":"slow","weird-name":"x"}`},
		{"strings stay strings", "", false, []string{"query=5", "note=true"}, `{"query":"5","note":"true"}`},
		{"json array", "", false, []string{"query=q", `tags=["x","y"]`, "tags=z"}, `{"query":"q","tags":["x","y","z"]}`},
		{"files", "", false, []string{"query=@body.md", "filter=@filter.json"}, `{"query":"# Body\n","filter":{"owner":"me"}}`},
		{"escaped at", "", false, []string{"query=@@user"}, `{"query":"@user"}`},
		{"empty value", "", false, []string{"query="}, `{"query":""}`},
		{"raw strings", "", true, []string{"query=q", "limit=5"}, `{"query":"q","limit":"5"}`},
		{"args then pairs", `{"query":"base","limit":1}`, false, []string{"limit=2"}, `{"query":"base","limit":2}`},
		{"equals in value", "", false, []string{"query=a=b"}, `{"query":"a=b"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := arguments(t, s, test.base, test.raw, test.pairs...)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("\n got %s\nwant %s", got, test.want)
			}
		})
	}
}

func TestArgumentErrors(t *testing.T) {
	s := schema(t, issues)
	for _, test := range []struct {
		pairs  []string
		syntax bool
		want   string
	}{
		{[]string{"query"}, true, `argument "query" must be key=value`},
		{[]string{"=x"}, true, `must be key=value`},
		{[]string{"query=@missing.md"}, true, "argument query:"},
		{[]string{"query=q", "status=bogus"}, false, `argument status must be one of "open", "closed", not "bogus"`},
		{[]string{"query=q", "limit=1.5"}, false, `argument limit must be number, not "1.5"`},
		{[]string{"query=q", "exact=yes"}, false, `argument exact must be boolean`},
		{[]string{"query=q", "statsu=open"}, false, "unknown argument statsu; did you mean status?"},
		{[]string{"limit=1"}, false, "missing required arguments: query (string)"},
		{[]string{"query=a", "query=b"}, false, "argument query is given twice"},
		{[]string{"query=q", "filter={bad"}, false, "argument filter is not a valid JSON object"},
	} {
		_, err := arguments(t, s, "", false, test.pairs...)
		var syntax mcptools.SyntaxError
		var invalid mcptools.ArgumentError
		switch {
		case err == nil:
			t.Errorf("%q: no error", test.pairs)
		case test.syntax && !errors.As(err, &syntax), !test.syntax && !errors.As(err, &invalid):
			t.Errorf("%q: error %T %v", test.pairs, err, err)
		case !strings.Contains(err.Error(), test.want):
			t.Errorf("%q: %v, want %q", test.pairs, err, test.want)
		}
	}
	// Without declared properties, any key is accepted and values stay strings unless JSON.
	got, err := arguments(t, schema(t, `{"type":"object"}`), "", false, "a=1", `b={"c":2}`)
	if err != nil || got != `{"a":"1","b":{"c":2}}` {
		t.Fatalf("open schema: %s %v", got, err)
	}
}
