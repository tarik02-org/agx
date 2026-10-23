package daemon

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPIcons(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)
	var fetched atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetched.Add(1)
		switch r.URL.Path {
		case "/icon.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write(svg)
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "mcp-icons.json")
	icons := &mcpIcons{path: path, client: server.Client()}
	inline := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("png"))
	wantSVG := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(svg)

	// A dark-theme icon wins over a light one, and a URL is fetched and inlined.
	got := icons.update(t.Context(), "docs", []mcp.Icon{
		{Source: inline, Theme: "light"},
		{Source: server.URL + "/icon.svg", Theme: "dark"},
	})
	if got != wantSVG || fetched.Load() != 1 {
		t.Fatalf("update = %q after %d fetches, want the dark SVG after 1", got, fetched.Load())
	}
	// The same URL is not fetched again, and the icon survives a restart.
	icons.update(t.Context(), "docs", []mcp.Icon{{Source: server.URL + "/icon.svg", Theme: "dark"}})
	if fetched.Load() != 1 {
		t.Fatalf("fetched an unchanged icon again")
	}
	if got := (&mcpIcons{path: path}).get("docs"); got != wantSVG {
		t.Fatalf("remembered icon = %q, want %q", got, wantSVG)
	}
	// What is not an image, or not HTTPS, is not drawn; a server declaring none forgets its icon.
	for _, declared := range [][]mcp.Icon{
		{{Source: server.URL + "/page"}},
		{{Source: "http://example.com/icon.png"}},
		{{Source: "data:text/html;base64,PGh0bWw+"}},
		nil,
	} {
		if got := icons.update(t.Context(), "docs", declared); got != "" {
			t.Fatalf("update(%v) = %q, want none", declared, got)
		}
	}
	if got := icons.get("docs"); got != "" {
		t.Fatalf("icon kept after the server declared none: %q", got)
	}
	// A data URI is taken as it is.
	if got := icons.update(t.Context(), "search", []mcp.Icon{{Source: inline}}); got != inline {
		t.Fatalf("update = %q, want %q", got, inline)
	}
	var none *mcpIcons
	if none.get("docs") != "" || none.update(t.Context(), "docs", []mcp.Icon{{Source: inline}}) != "" {
		t.Fatal("a nil cache remembered an icon")
	}
}
