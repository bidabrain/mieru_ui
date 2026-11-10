// Package web serves the panel UI and its JSON API.
package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"mieru-ui/database"
)

//go:embed assets
var assetsFS embed.FS

const cookieName = "mieru_ui_session"

// Server is the panel HTTP server.
type Server struct {
	store    *database.Store
	basePath string

	failMu sync.Mutex
	fails  map[string]*failRecord
}

type failRecord struct {
	count int
	until time.Time
	last  time.Time
}

// pruneFails drops stale login-failure records so the map cannot grow without bound.
func (s *Server) pruneFails() {
	for range time.Tick(5 * time.Minute) {
		cutoff := time.Now().Add(-15 * time.Minute)
		s.failMu.Lock()
		for ip, rec := range s.fails {
			if rec.last.Before(cutoff) && rec.until.Before(time.Now()) {
				delete(s.fails, ip)
			}
		}
		s.failMu.Unlock()
	}
}

// NormalizeBasePath turns "abc" / "/abc" / "/abc/" into "/abc/".
func NormalizeBasePath(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" {
		return "/"
	}
	return "/" + p + "/"
}

// New creates the server.
func New(store *database.Store) *Server {
	d := store.Snapshot()
	s := &Server{
		store:    store,
		basePath: NormalizeBasePath(d.Settings.BasePath),
		fails:    map[string]*failRecord{},
	}
	go s.pruneFails()
	return s
}

// Run listens and serves forever.
func (s *Server) Run() error {
	d := s.store.Snapshot()
	addr := net.JoinHostPort(d.Settings.ListenIP, strconv.Itoa(d.Settings.Port))
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// Generous: some actions (mita restart, apt-get install ufw) legitimately take minutes.
		WriteTimeout:   6 * time.Minute,
		IdleTimeout:    2 * time.Minute,
		MaxHeaderBytes: 64 << 10,
		TLSConfig:      &tls.Config{MinVersion: tls.VersionTLS12},
	}
	if d.Settings.CertFile != "" && d.Settings.KeyFile != "" {
		log.Printf("mieru-ui listening on https://%s%s", addr, s.basePath)
		return srv.ListenAndServeTLS(d.Settings.CertFile, d.Settings.KeyFile)
	}
	log.Printf("mieru-ui listening on http://%s%s", addr, s.basePath)
	return srv.ListenAndServe()
}

// Handler builds the full routing tree.
func (s *Server) Handler() http.Handler {
	inner := http.NewServeMux()
	inner.HandleFunc("GET /{$}", s.index)
	assets := http.FileServerFS(assetsFS)
	inner.HandleFunc("GET /assets/", func(w http.ResponseWriter, r *http.Request) {
		// Embedded files have no mtime; force revalidation so upgrades take effect immediately.
		w.Header().Set("Cache-Control", "no-cache")
		assets.ServeHTTP(w, r)
	})
	inner.HandleFunc("POST /api/login", s.login)
	inner.HandleFunc("POST /api/logout", s.logout)

	api := func(pattern string, h http.HandlerFunc) { inner.Handle(pattern, s.auth(h)) }
	api("GET /api/me", s.me)
	api("GET /api/status", s.status)

	api("GET /api/mita", s.mitaInfo)
	api("POST /api/mita/{action}", s.mitaAction)
	api("GET /api/mita/users", s.mitaUsers)
	api("GET /api/mita/metrics", s.mitaMetrics)

	api("GET /api/users", s.listUsers)
	api("POST /api/users", s.createUser)
	api("POST /api/users/batch", s.batchCreateUsers)
	api("PUT /api/users/{id}", s.updateUser)
	api("DELETE /api/users/{id}", s.deleteUser)
	api("POST /api/users/{id}/enable", s.enableUser)
	api("GET /api/users/{id}/share", s.shareUser)
	api("GET /api/qrcode", s.qrcode)

	api("GET /api/mihomo", s.getMihomo)
	api("PUT /api/mihomo", s.putMihomo)
	api("POST /api/mihomo/preview", s.previewMihomo)

	api("GET /api/server", s.getServer)
	api("PUT /api/server", s.putServer)
	api("GET /api/server/preview", s.previewServer)

	api("GET /api/settings", s.getSettings)
	api("PUT /api/settings", s.putSettings)
	api("PUT /api/settings/credentials", s.putCredentials)
	api("POST /api/logout-all", s.logoutAll)
	api("POST /api/panel/restart", s.restartPanel)

	api("GET /api/firewall", s.firewallStatus)
	api("POST /api/firewall/allow", s.firewallAllow)
	api("POST /api/firewall/open-mieru", s.firewallOpenMieru)
	api("POST /api/firewall/delete", s.firewallDelete)
	api("POST /api/firewall/enable", s.firewallEnable)
	api("POST /api/firewall/disable", s.firewallDisable)
	api("POST /api/firewall/install-ufw", s.firewallInstall)

	api("GET /api/logs", s.logs)
	api("GET /api/backup", s.backup)
	api("POST /api/restore", s.restore)

	var h http.Handler = inner
	if s.basePath != "/" {
		root := http.NewServeMux()
		trimmed := strings.TrimSuffix(s.basePath, "/")
		root.Handle(s.basePath, http.StripPrefix(trimmed, inner))
		root.HandleFunc(trimmed, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, s.basePath, http.StatusFound)
		})
		h = root
	}
	return securityHeaders(h)
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("Cross-Origin-Opener-Policy", "same-origin")
		// Only our own scripts may run; inline style attributes are used by the UI.
		hd.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
			"img-src 'self' data: blob:; connect-src 'self'; object-src 'none'; base-uri 'none'; "+
			"form-action 'self'; frame-ancestors 'none'")
		h.ServeHTTP(w, r)
	})
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	raw, err := fs.ReadFile(assetsFS, "assets/index.html")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(raw)
}

// ---------- JSON helpers ----------

type apiResp struct {
	Success bool   `json:"success"`
	Msg     string `json:"msg,omitempty"`
	Obj     any    `json:"obj,omitempty"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, obj any) { writeJSON(w, 200, apiResp{Success: true, Obj: obj}) }

func okMsg(w http.ResponseWriter, msg string, obj any) {
	writeJSON(w, 200, apiResp{Success: true, Msg: msg, Obj: obj})
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, apiResp{Success: false, Msg: msg})
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<20))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("请求格式错误: %v", err)
	}
	return nil
}

// ---------- sessions ----------

func sign(secret, payload string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(payload))
	return hex.EncodeToString(m.Sum(nil))
}

func (s *Server) makeCookie(set database.Settings) *http.Cookie {
	exp := time.Now().Add(time.Duration(set.SessionHours) * time.Hour)
	// The password hash is part of the signature so changing the password logs everyone out.
	payload := base64.RawURLEncoding.EncodeToString([]byte(set.Username)) + "." + strconv.FormatInt(exp.Unix(), 10)
	return &http.Cookie{
		Name:     cookieName,
		Value:    payload + "." + sign(set.SessionSecret+set.PasswordHash, payload),
		Path:     s.basePath,
		Expires:  exp,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   set.CertFile != "",
	}
}

func (s *Server) validSession(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 3 {
		return false
	}
	set := s.store.Snapshot().Settings
	payload := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(parts[2]), []byte(sign(set.SessionSecret+set.PasswordHash, payload))) {
		return false
	}
	name, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || string(name) != set.Username {
		return false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	return err == nil && time.Now().Unix() < exp
}

func (s *Server) auth(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.validSession(r) {
			fail(w, http.StatusUnauthorized, "未登录或登录已过期")
			return
		}
		// Simple CSRF guard: mutating requests must carry a custom header,
		// which cross-site forms cannot set without a CORS preflight.
		if r.Method != http.MethodGet && r.Header.Get("X-Mieru-UI") != "1" {
			fail(w, http.StatusForbidden, "缺少请求头")
			return
		}
		h(w, r)
	})
}

func clientIP(r *http.Request) string {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	s.failMu.Lock()
	rec := s.fails[ip]
	if rec != nil && time.Now().Before(rec.until) {
		s.failMu.Unlock()
		fail(w, http.StatusTooManyRequests, "失败次数过多，请 5 分钟后再试")
		return
	}
	s.failMu.Unlock()

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, 400, err.Error())
		return
	}
	set := s.store.Snapshot().Settings
	if !database.CheckLogin(set, req.Username, req.Password) {
		s.failMu.Lock()
		now := time.Now()
		rec := s.fails[ip]
		// Start a fresh window only if the previous failure was long ago.
		if rec == nil || now.Sub(rec.last) > 15*time.Minute {
			rec = &failRecord{}
			s.fails[ip] = rec
		}
		rec.count++
		rec.last = now
		if rec.count >= 5 {
			rec.until = now.Add(5 * time.Minute)
			rec.count = 0
		}
		s.failMu.Unlock()
		log.Printf("login failed from %s (user %q)", ip, req.Username)
		fail(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	s.failMu.Lock()
	delete(s.fails, ip)
	s.failMu.Unlock()
	http.SetCookie(w, s.makeCookie(set))
	ok(w, nil)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: s.basePath, MaxAge: -1, HttpOnly: true})
	ok(w, nil)
}

// logoutAll rotates the session secret, which invalidates every issued cookie.
func (s *Server) logoutAll(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Update(func(d *database.Data) error {
		d.Settings.SessionSecret = database.RandomHex(32)
		return nil
	}); err != nil {
		fail(w, 500, err.Error())
		return
	}
	s.logout(w, r)
}
