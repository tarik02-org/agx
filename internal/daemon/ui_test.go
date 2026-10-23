package daemon

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
)

func TestUILoginCodes(t *testing.T) {
	u := newUISessions()
	now := time.Now()
	code, expires := u.code(now)
	if !expires.Equal(now.Add(uiCodeLifetime)) {
		t.Fatalf("expires = %v", expires)
	}
	session, ok := u.exchange(code, now)
	if !ok || !u.valid(session, now) {
		t.Fatal("a fresh code did not sign in")
	}
	if _, ok := u.exchange(code, now); ok {
		t.Fatal("a code worked twice")
	}
	late, _ := u.code(now)
	if _, ok := u.exchange(late, now.Add(2*uiCodeLifetime)); ok {
		t.Fatal("an expired code worked")
	}
	if u.valid(session, now.Add(uiSessionIdle+time.Minute)) {
		t.Fatal("an idle session stayed valid")
	}
	if u.valid("", now) {
		t.Fatal("an empty token was valid")
	}
}

func TestUILogin(t *testing.T) {
	s := &Server{ui: newUISessions(), discovery: Discovery{Token: "management"}}
	code, _ := s.ui.code(time.Now())
	w := httptest.NewRecorder()
	s.serveUILogin(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ui/login?code="+code, nil))
	location := w.Header().Get("Location")
	session, ok := strings.CutPrefix(location, "/ui/#session=")
	if w.Code != http.StatusSeeOther || !ok || session == "" {
		t.Fatalf("login answered %d to %q", w.Code, location)
	}
	if cookies := w.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("login set cookies, which every port of the host would receive: %+v", cookies)
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+session)
	if !s.managed(header) {
		t.Fatal("the session token does not authorize management")
	}
	header.Set("Authorization", "Bearer "+session+"x")
	if s.managed(header) {
		t.Fatal("another token authorized management")
	}

	// A session cannot mint login codes, so it cannot outlive its idle limit.
	mint := func(token string) error {
		req := connect.NewRequest(&v1.CreateLoginCodeRequest{})
		req.Header().Set("Authorization", "Bearer "+token)
		_, err := s.CreateLoginCode(t.Context(), req)
		return err
	}
	if err := mint(session); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("a session minted a login code: %v", err)
	}
	if err := mint("management"); err != nil {
		t.Fatalf("the management token could not mint a login code: %v", err)
	}

	scoped := httptest.NewRecorder()
	next, _ := s.ui.code(time.Now())
	s.serveUILogin(scoped, httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/ui/login?code="+next+"&directory="+url.QueryEscape(filepath.Join(t.TempDir(), "a b")), nil))
	if location := scoped.Header().Get("Location"); !strings.HasPrefix(location, "/ui/?dir=") ||
		!strings.Contains(location, "#session=") {
		t.Fatalf("a login with a directory went to %q", location)
	}
	relative := httptest.NewRecorder()
	last, _ := s.ui.code(time.Now())
	s.serveUILogin(relative, httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/ui/login?code="+last+"&directory=relative/dir", nil))
	if location := relative.Header().Get("Location"); !strings.HasPrefix(location, "/ui/#session=") {
		t.Fatalf("a login with a relative directory went to %q", location)
	}
	again := httptest.NewRecorder()
	s.serveUILogin(again, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ui/login?code="+code, nil))
	if again.Code != http.StatusUnauthorized {
		t.Fatalf("a used code answered %d", again.Code)
	}
}
