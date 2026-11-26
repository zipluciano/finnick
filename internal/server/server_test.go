package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/zipluciano/finnick/internal/workspace"
)

func newTestServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0o755) // a repository without SOPS files yet
	ws, err := workspace.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := New(ws, fstest.MapFS{"index.html": {Data: []byte("<!doctype html>ok")}}, "test")
	s.port = 4321
	return s, s.Handler()
}

func do(h http.Handler, method, target, host string, mut func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(`{"path":"x.env"}`))
	r.Host = host
	if mut != nil {
		mut(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestRejectsForeignHost(t *testing.T) {
	s, h := newTestServer(t)
	if w := do(h, "GET", "/auth?token="+s.token, "evil.example:4321", nil); w.Code != http.StatusForbidden {
		t.Fatalf("DNS-rebinding host accepted: %d", w.Code)
	}
	if w := do(h, "GET", "/auth?token="+s.token, "127.0.0.1:9999", nil); w.Code != http.StatusForbidden {
		t.Fatalf("wrong port accepted: %d", w.Code)
	}
}

func TestAuthFlow(t *testing.T) {
	s, h := newTestServer(t)
	host := "127.0.0.1:4321"
	if w := do(h, "GET", "/api/workspace", host, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("api without session: %d", w.Code)
	}
	if w := do(h, "GET", "/auth?token=wrong", host, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("bad token: %d", w.Code)
	}
	w := do(h, "GET", "/auth?token="+s.token, host, nil)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("good token: %d", w.Code)
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie flags: %+v", cookie)
	}
	withCookie := func(r *http.Request) { r.AddCookie(cookie) }
	w = do(h, "GET", "/api/workspace", host, withCookie)
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("workspace: %d %s", w.Code, w.Body)
	}
	// Revealing needs the custom header too (CSRF).
	if w := do(h, "POST", "/api/reveal", host, withCookie); w.Code != http.StatusForbidden {
		t.Fatalf("reveal without csrf header: %d", w.Code)
	}
	w = do(h, "POST", "/api/reveal", host, func(r *http.Request) { withCookie(r); r.Header.Set(csrfHeader, "1") })
	if w.Code != http.StatusUnprocessableEntity { // x.env does not exist in the empty workspace
		t.Fatalf("reveal with csrf header: %d %s", w.Code, w.Body)
	}
}

func TestSettingsAndIndexInjection(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, h := newTestServer(t)
	host := "127.0.0.1:4321"
	cookie := do(h, "GET", "/auth?token="+s.token, host, nil).Result().Cookies()[0]
	authed := func(r *http.Request) { r.AddCookie(cookie); r.Header.Set(csrfHeader, "1") }

	put := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(body))
		r.Host = host
		authed(r)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := put(`{"theme":"neon","locale":"en"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid theme accepted: %d", w.Code)
	}
	if w := put(`{"theme":"dark","locale":"pt-BR"}`); w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	w := do(h, "GET", "/api/settings", host, authed)
	if !strings.Contains(w.Body.String(), `"pt-BR"`) {
		t.Fatalf("get: %s", w.Body)
	}
	// The page is served with the saved theme and language already applied.
	s.assets = fstest.MapFS{"index.html": {Data: []byte(`<!doctype html><html lang="en"><body></body></html>`)}}
	page := do(h, "GET", "/", host, authed)
	if !strings.Contains(page.Body.String(), `<html lang="pt-BR" data-theme="dark">`) {
		t.Fatalf("index not personalized: %s", page.Body)
	}
}
