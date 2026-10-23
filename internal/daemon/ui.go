package daemon

import (
	"context"
	"maps"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/securefs"
	"github.com/tarik02-org/agx/internal/ui"
)

const (
	// uiCodeLifetime bounds how long a login code from agx ui waits for the browser.
	uiCodeLifetime = time.Minute
	// uiSessionIdle ends a UI session nobody used for this long.
	uiSessionIdle = 12 * time.Hour
)

// uiSessions holds the web UI's login codes and sessions, in memory only: a daemon restart signs
// every browser out, and agx ui signs it in again.
type uiSessions struct {
	mu       sync.Mutex
	codes    map[string]time.Time
	sessions map[string]time.Time
}

func newUISessions() *uiSessions {
	return &uiSessions{codes: map[string]time.Time{}, sessions: map[string]time.Time{}}
}

// code issues a single-use login code and when it expires.
func (u *uiSessions) code(now time.Time) (string, time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for code, expires := range u.codes {
		if now.After(expires) {
			delete(u.codes, code)
		}
	}
	code := securefs.Token()
	expires := now.Add(uiCodeLifetime)
	u.codes[code] = expires
	return code, expires
}

// exchange trades a login code for a new session token; a code works once.
func (u *uiSessions) exchange(code string, now time.Time) (string, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	expires, ok := u.codes[code]
	delete(u.codes, code)
	if !ok || now.After(expires) {
		return "", false
	}
	session := securefs.Token()
	u.sessions[session] = now
	return session, true
}

// valid reports whether token is a live session and marks it used. Tokens are random 256-bit
// values, so looking one up leaks nothing a guess could use.
func (u *uiSessions) valid(token string, now time.Time) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	maps.DeleteFunc(u.sessions, func(_ string, used time.Time) bool { return now.Sub(used) > uiSessionIdle })
	if _, ok := u.sessions[token]; !ok || token == "" {
		return false
	}
	u.sessions[token] = now
	return true
}

// managed reports whether request headers carry management authorization: the management
// token, or a web UI session token, both as a bearer token. The UI keeps its session in the
// page's own storage rather than a cookie, because browsers send a host's cookies to all of its
// ports, so any other local server would receive it.
func (s *Server) managed(h http.Header) bool {
	authorization := h.Get("Authorization")
	if equalBearer(authorization, s.discovery.Token) {
		return true
	}
	session, ok := strings.CutPrefix(authorization, "Bearer ")
	return ok && s.ui.valid(session, time.Now())
}

// CreateLoginCode takes the management token only, so a UI session cannot mint new sessions and
// outlive its idle limit.
func (s *Server) CreateLoginCode(
	_ context.Context,
	req *connect.Request[v1.CreateLoginCodeRequest],
) (*connect.Response[v1.CreateLoginCodeResponse], error) {
	if !equalBearer(req.Header().Get("Authorization"), s.discovery.Token) {
		return nil, rpcError(connect.CodeUnauthenticated, "management credential required")
	}
	code, expires := s.ui.code(time.Now())
	return connect.NewResponse(&v1.CreateLoginCodeResponse{Code: code, ExpiresAt: timestamppb.New(expires)}), nil
}

// serveUILogin trades the code agx ui opened the browser with for a session token, and sends the
// browser on to the UI without the code in its address. The token travels in the fragment, which
// browsers never send to a server; the page moves it into its storage and drops it from the
// address.
func (s *Server) serveUILogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session, ok := s.ui.exchange(r.URL.Query().Get("code"), time.Now())
	if !ok {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("<!doctype html><meta charset=\"utf-8\"><title>agx</title>" +
			"<p>This sign-in link expired or was already used. Run <code>agx ui</code> again.</p>"))
		return
	}
	// The directory agx ui ran in becomes the UI's scope. It travels only as a query value of the
	// UI's own address, so the redirect cannot lead anywhere else.
	target := "/ui/"
	if directory := r.URL.Query().Get("directory"); filepath.IsAbs(directory) {
		target += "?" + url.Values{"dir": {directory}}.Encode()
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, target+"#"+url.Values{"session": {session}}.Encode(), http.StatusSeeOther)
}

// serveUISchema serves the configuration JSON Schema of this daemon's version and harnesses, which
// the UI's editors validate and complete against.
func (s *Server) serveUISchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.managed(r.Header) {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	schema, err := config.Schema(false, s.builtins)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/schema+json")
	_, _ = w.Write(schema)
}

func (s *Server) serveUI() http.Handler { return ui.Handler("/ui") }
