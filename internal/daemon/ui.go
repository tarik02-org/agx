package daemon

import (
	"context"
	"crypto/subtle"
	"net/http"
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
	// uiCookie names the web UI's session cookie.
	uiCookie = "agx_ui_session"
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

// valid reports whether token is a live session and marks it used.
func (u *uiSessions) valid(token string, now time.Time) bool {
	if token == "" {
		return false
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	for session, used := range u.sessions {
		if now.Sub(used) > uiSessionIdle {
			delete(u.sessions, session)
			continue
		}
		if subtle.ConstantTimeCompare([]byte(session), []byte(token)) == 1 {
			u.sessions[session] = now
			return true
		}
	}
	return false
}

// managed reports whether request headers carry management authorization: the management
// token, or a web UI session cookie. The daemon's handler already pinned Host and Origin to its
// own address, so a cookie only arrives from the UI's own pages.
func (s *Server) managed(h http.Header) bool {
	if equalBearer(h.Get("Authorization"), s.discovery.Token) {
		return true
	}
	cookie, err := (&http.Request{Header: h}).Cookie(uiCookie)
	return err == nil && s.ui.valid(cookie.Value, time.Now())
}

func (s *Server) CreateLoginCode(
	context.Context,
	*connect.Request[v1.CreateLoginCodeRequest],
) (*connect.Response[v1.CreateLoginCodeResponse], error) {
	code, expires := s.ui.code(time.Now())
	return connect.NewResponse(&v1.CreateLoginCodeResponse{Code: code, ExpiresAt: timestamppb.New(expires)}), nil
}

// serveUILogin trades the code agx ui opened the browser with for a session cookie, and sends
// the browser on to the UI without the code in its address.
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
	http.SetCookie(w, &http.Cookie{
		Name:     uiCookie,
		Value:    session,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/ui/", http.StatusSeeOther)
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
