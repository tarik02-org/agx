// Package ui holds the web UI's built files, which the daemon serves at /ui/. A build without
// them, such as go install from source, serves a page saying so.
package ui

import (
	"bytes"
	"compress/gzip"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
)

//go:embed all:dist
var dist embed.FS

// Built reports whether the binary contains the web UI.
func Built() bool {
	_, err := fs.Stat(dist, "dist/index.html")
	return err == nil
}

// Handler serves the UI under prefix: files as built, and index.html for any other path, which
// the UI's router resolves.
func Handler(prefix string) http.Handler {
	files, _ := fs.Sub(dist, "dist")
	server := http.FileServerFS(files)
	return http.StripPrefix(prefix, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !Built() {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(placeholder))
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" || name == ".keep" {
			name = "index.html"
		}
		// A directory is a route too: the file server would redirect it to a listing.
		if info, err := fs.Stat(files, name); err != nil || info.IsDir() {
			name = "index.html"
		}
		if name == "index.html" {
			// The page names the hashed assets of this build, so it is never cached. It is served
			// directly: the file server would redirect /index.html to the directory.
			page, err := fs.ReadFile(files, name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(page)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		if compressible(name) {
			w.Header().Set("Vary", "Accept-Encoding")
		}
		if compressible(name) && acceptsGzip(r) {
			content, err := gzipped(files, name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(name)))
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			_, _ = w.Write(content)
			return
		}
		r.URL.Path = "/" + name
		server.ServeHTTP(w, r)
	}))
}

// compressible reports whether a file is text, which gzip shrinks to about a third; fonts and
// images are compressed already.
func compressible(name string) bool {
	switch path.Ext(name) {
	case ".js", ".css", ".svg", ".json", ".wasm":
		return true
	}
	return false
}

// acceptsGzip reports whether the request lists gzip among its encodings, and not with q=0.
func acceptsGzip(r *http.Request) bool {
	for _, encoding := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		token, params, _ := strings.Cut(encoding, ";")
		if strings.TrimSpace(token) != "gzip" {
			continue
		}
		quality, found := strings.CutPrefix(strings.TrimSpace(params), "q=")
		value, err := strconv.ParseFloat(quality, 64)
		return !found || err != nil || value > 0
	}
	return false
}

// The embedded files are compressed once each, on first request, at the best level: they never
// change while the daemon runs, and the browser caches the hashed assets for good.
var compressed sync.Map

func gzipped(files fs.FS, name string) ([]byte, error) {
	if content, ok := compressed.Load(name); ok {
		return content.([]byte), nil
	}
	content, err := fs.ReadFile(files, name)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buffer, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := writer.Write(content); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	compressed.Store(name, buffer.Bytes())
	return buffer.Bytes(), nil
}

const placeholder = `<!doctype html>
<meta charset="utf-8">
<title>agx</title>
<p>This agx was built without its web UI. Release binaries and the Nix package include it;
to build it from source, run <code>pnpm --dir web install && pnpm --dir web build</code> before
<code>go build</code>.</p>
`
