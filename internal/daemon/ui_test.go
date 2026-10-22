package daemon

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func TestUILoginSetsCookie(t *testing.T) {
	s := &Server{ui: newUISessions()}
	code, _ := s.ui.code(time.Now())
	w := httptest.NewRecorder()
	s.serveUILogin(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ui/login?code="+code, nil))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/ui/" {
		t.Fatalf("login answered %d to %q", w.Code, w.Header().Get("Location"))
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookies = %+v", cookies)
	}
	header := http.Header{}
	header.Add("Cookie", uiCookie+"="+cookies[0].Value)
	if !s.managed(header) {
		t.Fatal("the session cookie does not authorize management")
	}
	again := httptest.NewRecorder()
	s.serveUILogin(again, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ui/login?code="+code, nil))
	if again.Code != http.StatusUnauthorized {
		t.Fatalf("a used code answered %d", again.Code)
	}
}
