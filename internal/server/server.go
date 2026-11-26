// Package server serves the Finnick UI and its JSON API on the loopback interface.
//
// Threat model: the server runs on the user's machine with the user's keys, so any web
// page the user visits must not be able to use it. Defenses:
//   - it listens on 127.0.0.1 only;
//   - every request needs a session cookie, obtained once through a random token the CLI
//     prints/opens (/auth?token=...); the cookie is HttpOnly and SameSite=Strict;
//   - the Host header must be the loopback address we listen on (blocks DNS rebinding);
//   - state-changing and secret-revealing calls are POST with a custom header, which
//     cross-origin pages cannot send without a CORS preflight we never approve;
//   - responses are never cached and the page has a strict Content-Security-Policy.
package server

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zipluciano/finnick/internal/settings"
	"github.com/zipluciano/finnick/internal/workspace"
)

const (
	cookieName = "finnick_session"
	csrfHeader = "X-Finnick"
)

type Server struct {
	ws      *workspace.Workspace
	assets  fs.FS
	token   string
	session string
	port    int
	version string
}

// New prepares a server for ws. assets is the built web UI (may lack index.html in dev builds).
func New(ws *workspace.Workspace, assets fs.FS, version string) *Server {
	return &Server{ws: ws, assets: assets, token: randomHex(24), session: randomHex(32), version: version}
}

// Listen binds to 127.0.0.1:port (0 picks a free port) and returns the listener and the URL to open.
func (s *Server) Listen(port int) (net.Listener, string, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, "", err
	}
	s.port = ln.Addr().(*net.TCPAddr).Port
	return ln, "http://127.0.0.1:" + strconv.Itoa(s.port) + "/auth?token=" + s.token, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth", s.handleAuth)
	mux.Handle("GET /api/workspace", s.authed(http.HandlerFunc(s.handleWorkspace)))
	mux.Handle("POST /api/reveal", s.authed(s.csrf(http.HandlerFunc(s.handleReveal))))
	mux.Handle("POST /api/edit", s.authed(s.csrf(http.HandlerFunc(s.handleEdit))))
	mux.Handle("POST /api/create", s.authed(s.csrf(http.HandlerFunc(s.handleCreate))))
	mux.Handle("GET /api/settings", s.authed(http.HandlerFunc(s.handleGetSettings)))
	mux.Handle("PUT /api/settings", s.authed(s.csrf(http.HandlerFunc(s.handlePutSettings))))
	mux.Handle("GET /", s.authed(http.HandlerFunc(s.handleUI)))
	return s.guard(mux)
}

// Serve runs the HTTP server on ln until it fails or is shut down.
func (s *Server) Serve(ln net.Listener) *http.Server {
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	return srv
}

// guard applies host validation and security headers to every response.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.allowedHost(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) allowedHost(host string) bool {
	h, p, err := net.SplitHostPort(host)
	if err != nil {
		return false
	}
	if s.port != 0 && p != strconv.Itoa(s.port) {
		return false
	}
	return h == "127.0.0.1" || h == "localhost" || h == "[::1]" || h == "::1"
}

func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("token")), []byte(s.token)) != 1 {
		http.Error(w, "invalid or expired link: open the URL printed by `finnick ui` again", http.StatusUnauthorized)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: s.session, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) authed(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil || subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.session)) != 1 {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				writeError(w, http.StatusUnauthorized, errSessionExpired)
				return
			}
			http.Error(w, "Open the URL printed by `finnick ui` in your terminal.", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(csrfHeader) != "1" {
			writeError(w, http.StatusForbidden, errors.New("missing "+csrfHeader+" header"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

type workspaceResponse struct {
	Version  string             `json:"version"`
	Root     string             `json:"root"`
	Config   *workspace.Config  `json:"config"`
	Identity workspace.Identity `json:"identity"`
	Files    []workspace.File   `json:"files"`
}

func (s *Server) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	id := workspace.LocalIdentity()
	files, err := s.ws.Scan(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if files == nil {
		files = []workspace.File{}
	}
	writeJSON(w, workspaceResponse{Version: s.version, Root: s.ws.Root, Config: s.ws.Config, Identity: id, Files: files})
}

func (s *Server) handleReveal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if !readJSON(w, r, &req) || req.Path == "" {
		writeError(w, http.StatusBadRequest, errors.New("body must be {\"path\": \"...\"}"))
		return
	}
	res, err := s.ws.Reveal(req.Path)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, map[string]any{"path": req.Path, "entries": res.Entries, "version": res.Version, "editable": res.Editable})
}

func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string           `json:"path"`
		Version string           `json:"version"`
		Edits   []workspace.Edit `json:"edits"`
	}
	if !readJSON(w, r, &req) || req.Path == "" || req.Version == "" || len(req.Edits) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("body must be {\"path\", \"version\", \"edits\": [...]}"))
		return
	}
	version, entries, err := s.ws.Edit(req.Path, req.Version, req.Edits)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, map[string]any{"path": req.Path, "entries": entries, "version": version, "editable": true})
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path  string           `json:"path"`
		Edits []workspace.Edit `json:"edits"`
	}
	if !readJSON(w, r, &req) || req.Path == "" || len(req.Edits) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("body must be {\"path\", \"edits\": [...]}"))
		return
	}
	version, entries, err := s.ws.Create(req.Path, req.Edits)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, map[string]any{"path": req.Path, "entries": entries, "version": version, "editable": true})
}

// readJSON decodes a small request body. Bodies are capped: none of the API needs more.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v) == nil
}

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, errors.New("no such API call: GET "+r.URL.Path))
		return
	}
	if _, err := fs.Stat(s.assets, "index.html"); err != nil {
		http.Error(w, "The web UI was not built into this binary. Run `make build` (or `npm --prefix web run build`) and rebuild.", http.StatusNotImplemented)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if _, err := fs.Stat(s.assets, name); name == "" || name == "index.html" || err != nil {
		s.serveIndex(w) // SPA fallback
		return
	}
	http.ServeFileFS(w, r, s.assets, name)
}

// serveIndex applies saved preferences to <html> so the first paint already has the right
// theme and language, without an inline script (which the CSP forbids).
func (s *Server) serveIndex(w http.ResponseWriter) {
	page, err := fs.ReadFile(s.assets, "index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	prefs := settings.Load()
	attrs := `<html lang="en"`
	if prefs.Locale != "" {
		attrs = `<html lang="` + prefs.Locale + `"`
	}
	if prefs.Theme != "system" {
		attrs += ` data-theme="` + prefs.Theme + `"`
	}
	page = bytes.Replace(page, []byte(`<html lang="en"`), []byte(attrs), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(page)
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, settings.Load())
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var next settings.Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&next); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := settings.Save(next); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, next)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

var errSessionExpired = errors.New("session expired: open the URL printed by `finnick ui` again")

// errorCode lets the UI show a translated message for errors it knows about.
func errorCode(err error) string {
	for _, c := range []struct {
		err  error
		code string
	}{
		{errSessionExpired, "session_expired"},
		{workspace.ErrNotRecipient, "not_recipient"},
		{workspace.ErrIntegrity, "integrity"},
		{workspace.ErrInvalidPath, "invalid_path"},
		{workspace.ErrStale, "stale"},
		{workspace.ErrUnsupported, "unsupported"},
		{workspace.ErrKeyExists, "key_exists"},
		{workspace.ErrKeyNotFound, "key_not_found"},
		{workspace.ErrInvalidKey, "invalid_key"},
		{workspace.ErrNoRule, "no_rule"},
		{workspace.ErrFileExists, "file_exists"},
	} {
		if errors.Is(err, c.err) {
			return c.code
		}
	}
	return ""
}

// statusFor maps workspace errors to HTTP statuses.
func statusFor(err error) int {
	switch {
	case errors.Is(err, workspace.ErrStale), errors.Is(err, workspace.ErrKeyExists), errors.Is(err, workspace.ErrFileExists):
		return http.StatusConflict
	case errors.Is(err, workspace.ErrInvalidPath), errors.Is(err, workspace.ErrInvalidKey):
		return http.StatusBadRequest
	}
	return http.StatusUnprocessableEntity
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := map[string]string{"error": err.Error()}
	if code := errorCode(err); code != "" {
		body["code"] = code
	}
	json.NewEncoder(w).Encode(body)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
