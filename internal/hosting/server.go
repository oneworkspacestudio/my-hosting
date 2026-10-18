package hosting

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type App struct {
	bootstrapToken string
	localSetup     bool
	cfg            Config
	base           string
	store          *Store
	web            fs.FS
	ctx            context.Context
	cancel         context.CancelFunc
	started        time.Time
	authMu         sync.Mutex
	flows          map[string]loginFlow
	attempts       map[string]attempt
	oauth          oauth2.Config
	verifier       *oidc.IDTokenVerifier
	jobsMu         sync.Mutex
	active         map[string]context.CancelFunc
	wake           chan struct{}
	jobsGate       sync.RWMutex
	publishMu      sync.Mutex
	processMu      sync.Mutex
	processes      map[string]*managedProcess
	caddy          *ProcessJob
	caddyError     string
	caddyDone      chan struct{}
	caddyAdminPort int
	runtime        *runtimeManager
	lock           *os.File
}

func New(ctx context.Context, cfg Config, web fs.FS) (*App, error) {
	u, e := url.Parse(cfg.Origin)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("Set a valid http(s) public origin, optionally including /hosting")
	}
	cfg.Origin = strings.TrimRight(cfg.Origin, "/")
	if cfg.SiteDomain != "" && (!validDomain(cfg.SiteDomain) || len(cfg.SiteDomain) > 189) {
		return nil, errors.New("Use a valid sites parent domain with room for a 63-character site label")
	}
	if cfg.SiteTransport != "" && cfg.SiteTransport != "direct" && cfg.SiteTransport != "tunnel" {
		return nil, errors.New("Sites transport must be direct or tunnel")
	}
	if cfg.SiteStyle != "" && cfg.SiteStyle != "flat" && cfg.SiteStyle != "nested" {
		return nil, errors.New("Site address style must be flat or nested")
	}
	if cfg.Issuer != "" && (cfg.ClientID == "" || cfg.Owner == "") {
		return nil, errors.New("MyAccounts mode requires a registered client and explicit Workspace owner subject")
	}
	if e = prepareData(cfg.Data); e != nil {
		return nil, e
	}
	lock, e := lockFile(filepath.Join(cfg.Data, "runtime.lock"))
	if e != nil {
		return nil, e
	}
	s, e := openStore(cfg.Data)
	if e != nil {
		lock.Close()
		return nil, e
	}
	ctx, cancel := context.WithCancel(ctx)
	a := &App{cfg: cfg, base: strings.TrimRight(u.Path, "/"), store: s, web: web, ctx: ctx, cancel: cancel, started: time.Now(), flows: map[string]loginFlow{}, attempts: map[string]attempt{}, active: map[string]context.CancelFunc{}, wake: make(chan struct{}, 1), processes: map[string]*managedProcess{}, lock: lock}
	a.runtime = &runtimeManager{root: filepath.Join(cfg.Data, "runtimes"), status: map[string]string{}}
	if e = a.prepareBootstrap(); e != nil {
		a.Close()
		return nil, e
	}
	if e = a.recoverJobs(); e != nil {
		a.Close()
		return nil, e
	}
	go a.scheduler()
	go a.supervise()
	return a, nil
}
func (a *App) Close() {
	a.cancel()
	a.jobsMu.Lock()
	for _, cancel := range a.active {
		cancel()
	}
	a.jobsMu.Unlock()
	a.jobsGate.Lock()
	defer a.jobsGate.Unlock()
	a.processMu.Lock()
	processIDs := []string{}
	for did := range a.processes {
		processIDs = append(processIDs, did)
	}
	a.processMu.Unlock()
	for _, did := range processIDs {
		a.stopNode(did)
	}
	_ = a.stopCaddy()
	a.store.db.Close()
	if a.lock != nil {
		a.lock.Close()
		a.lock = nil
	}
}
func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, e error) {
	jsonOut(w, status, map[string]string{"error": e.Error()})
}
func jsonDecode(r io.Reader, v any) error {
	d := json.NewDecoder(r)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("Only one JSON object is allowed")
	}
	return nil
}
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if e := jsonDecode(r.Body, v); e != nil {
		fail(w, 400, errors.New("Invalid request; check the fields and try again"))
		return false
	}
	return true
}
func (a *App) Handler() http.Handler { return http.HandlerFunc(a.serve) }
func (a *App) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'self'")
	path := r.URL.Path
	if a.base != "" && (path == a.base || strings.HasPrefix(path, a.base+"/")) {
		path = strings.TrimPrefix(path, a.base)
		r.URL.Path = path
	}
	if path == "" {
		http.Redirect(w, r, a.base+"/", 303)
		return
	}
	if path == "/healthz" && r.Method == "GET" {
		jsonOut(w, 200, map[string]any{"status": "ok", "version": Version})
		return
	}
	if strings.HasPrefix(path, "/hooks/github/") {
		a.webhook(w, r, strings.TrimPrefix(path, "/hooks/github/"))
		return
	}
	if path == "/auth/login" && r.Method == "GET" {
		a.login(w, r)
		return
	}
	if path == "/auth/callback" && r.Method == "GET" {
		a.callback(w, r)
		return
	}
	if path == "/auth/bootstrap" && r.Method == "POST" {
		a.bootstrap(w, r)
		return
	}
	if path == "/auth/local" && r.Method == "POST" {
		a.localLogin(w, r)
		return
	}
	if path == "/auth/logout" && r.Method == "POST" {
		a.logout(w, r)
		return
	}
	if path == "/api/bootstrap-status" && r.Method == "GET" {
		var n int
		_ = a.store.db.QueryRow("SELECT COUNT(*) FROM users WHERE role='owner'").Scan(&n)
		jsonOut(w, 200, map[string]any{"oidc": a.cfg.Issuer != "", "configured": n > 0 || a.cfg.Issuer != "", "base": a.base, "version": Version, "setup": a.localSetup && a.bootstrapCode() != ""})
		return
	}
	if strings.HasPrefix(path, "/api/") {
		s, ok := a.session(r)
		if !ok {
			fail(w, 401, errors.New("Sign in to My Hosting"))
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if !a.sameOrigin(r) || !hmac.Equal([]byte(s.CSRF), []byte(r.Header.Get("X-CSRF-Token"))) {
				fail(w, 403, errors.New("Your form expired. Refresh My Hosting and retry"))
				return
			}
		}
		a.api(w, r, s, strings.TrimPrefix(path, "/api/"))
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		http.NotFound(w, r)
		return
	}
	if strings.Contains(path, "..") || strings.ContainsAny(path, "\\\x00") {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(path, "/")
	if name == "" {
		name = "index.html"
	}
	if name == "index.html" {
		b, e := fs.ReadFile(a.web, name)
		if e != nil {
			fail(w, 503, errors.New("Dashboard assets are missing; rebuild the release"))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(b)
		return
	}
	http.FileServer(http.FS(a.web)).ServeHTTP(w, r)
}
func (a *App) api(w http.ResponseWriter, r *http.Request, s Session, path string) {
	if path == "me" && r.Method == "GET" {
		jsonOut(w, 200, map[string]any{"user": s.User, "csrf": s.CSRF, "version": Version, "origin": a.cfg.Origin, "publication": a.settings()})
		return
	}
	if path == "projects" {
		if r.Method == "GET" {
			ps, e := a.store.projects(s.User)
			if e != nil {
				fail(w, 500, dbError(e))
				return
			}
			jsonOut(w, 200, ps)
			return
		}
		if r.Method == "POST" && requireAdmin(w, s) {
			var p Project
			if !readJSON(w, r, &p) {
				return
			}
			p.ID = id()
			p.Live = ""
			p.Created = now()
			p.Role = ""
			if e := p.validate(); e != nil {
				fail(w, 400, e)
				return
			}
			if e := a.store.saveProject(p); e != nil {
				fail(w, 409, errors.New("That site name is already in use"))
				return
			}
			_ = a.store.audit(s.User.ID, p.ID, "project.created", map[string]any{"slug": p.Slug, "adapter": p.Adapter, "trusted": p.Trusted})
			jsonOut(w, 201, p)
			return
		}
		return
	}
	if path == "settings" {
		a.settingsAPI(w, r, s)
		return
	}
	if path == "team" || strings.HasPrefix(path, "team/") {
		a.teamAPI(w, r, s, path)
		return
	}
	if path == "audit" && r.Method == "GET" {
		a.auditAPI(w, r, s)
		return
	}
	if path == "runtimes" && r.Method == "GET" {
		if !requireAdmin(w, s) {
			return
		}
		jsonOut(w, 200, a.runtime.summary())
		return
	}
	if strings.HasPrefix(path, "runtimes/") {
		a.runtimeAPI(w, r, s, strings.TrimPrefix(path, "runtimes/"))
		return
	}
	if path == "backups" {
		a.backupAPI(w, r, s)
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] != "projects" || !validID.MatchString(parts[1]) {
		http.NotFound(w, r)
		return
	}
	pid := parts[1]
	p, e := a.store.project(pid)
	role := a.store.role(s.User, pid)
	if e != nil || rank(role) == 0 {
		fail(w, 404, errors.New("Project not found or access not granted"))
		return
	}
	p.Role = role
	if len(parts) == 2 {
		if r.Method == "GET" {
			jsonOut(w, 200, p)
			return
		}
		if r.Method == "PUT" && requireAdmin(w, s) {
			var next Project
			if !readJSON(w, r, &next) {
				return
			}
			next.ID = p.ID
			next.Created = p.Created
			next.Live = p.Live
			next.Role = ""
			if next.Slug != p.Slug {
				fail(w, 400, errors.New("Site names are permanent; create a new project to change the preview address"))
				return
			}
			if e = next.validate(); e != nil {
				fail(w, 400, e)
				return
			}
			if e = a.store.saveProject(next); e != nil {
				fail(w, 500, dbError(e))
				return
			}
			_ = a.store.audit(s.User.ID, pid, "project.updated", map[string]string{"adapter": next.Adapter, "branch": next.Branch})
			jsonOut(w, 200, next)
			return
		}
		return
	}
	switch parts[2] {
	case "deployments":
		a.deployAPI(w, r, s, p, parts[3:])
	case "domains":
		a.domainAPI(w, r, s, p, parts[3:])
	case "secrets":
		a.secretAPI(w, r, s, p)
	case "github":
		a.githubAPI(w, r, s, p, parts[3:])
	case "members":
		a.membersAPI(w, r, s, p)
	case "process":
		a.processAPI(w, r, s, p)
	case "metrics":
		if r.Method == "GET" {
			a.metricsAPI(w, r, s, p)
		} else {
			http.NotFound(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}
func (a *App) settings() Settings {
	s := defaults()
	s.PreviewDomain = a.cfg.SiteDomain
	s.AddressStyle = "nested"
	if a.cfg.SiteStyle != "" {
		s.AddressStyle = a.cfg.SiteStyle
	}
	if a.cfg.SiteTransport != "" {
		s.Transport = a.cfg.SiteTransport
	}
	if s.Transport == "tunnel" && s.AddressStyle == "flat" && a.cfg.SiteCFToken != "" {
		s.Public = true
	}
	_ = a.store.setting("hosting", &s)
	if s.Transport == "" {
		s.Transport = "direct"
	}
	if s.AddressStyle == "" {
		s.AddressStyle = "nested"
	}
	return s
}
func (a *App) settingsAPI(w http.ResponseWriter, r *http.Request, s Session) {
	if !requireAdmin(w, s) {
		return
	}
	if r.Method == "GET" {
		jsonOut(w, 200, map[string]any{"settings": a.settings(), "dataPath": a.cfg.Data, "runtimes": a.runtime.summary(), "uptimeSeconds": int(time.Since(a.started).Seconds()), "caddy": a.caddyState(), "securityScope": "Trusted administrators and projects only; Windows restricted tokens are not a VM security boundary", "backup": "Password-encrypted export; keep its password separately. Stop jobs before exporting."})
		return
	}
	if r.Method != "PUT" {
		http.NotFound(w, r)
		return
	}
	var next Settings
	if !readJSON(w, r, &next) {
		return
	}
	if next.Transport == "" {
		next.Transport = "direct"
	}
	if next.AddressStyle == "" {
		next.AddressStyle = "nested"
	}
	if next.AddressStyle != "nested" && next.AddressStyle != "flat" {
		fail(w, 400, errors.New("Choose nested or flat site addresses"))
		return
	}
	if next.CaddyPort < 1024 || next.CaddyPort > 65535 || next.CaddyPort == mustPort(a.cfg.Listen) || next.Retain < 2 || next.Retain > 100 || next.Concurrency != 1 || (next.Transport != "direct" && next.Transport != "tunnel") || (next.PreviewDomain != "" && (!validDomain(next.PreviewDomain) || len(next.PreviewDomain) > 189)) {
		fail(w, 400, errors.New("Choose a free site port (1024–65535), retain 2–100 releases and concurrency 1; use a valid sites parent domain and direct or tunnel HTTPS"))
		return
	}
	if a.runtime.ready("caddy") != nil {
		if next.Public {
			fail(w, 409, errors.New("Provision Caddy before enabling public HTTPS"))
			return
		}
		if e := a.store.setSetting("hosting", next); e != nil {
			fail(w, 500, dbError(e))
			return
		}
		_ = a.store.audit(s.User.ID, "", "settings.updated", next)
		jsonOut(w, 200, next)
		return
	}
	old := a.settings()
	if e := a.store.setSetting("hosting", next); e != nil {
		fail(w, 500, dbError(e))
		return
	}
	a.publishMu.Lock()
	e := a.reloadRoutes(r.Context(), nil)
	a.publishMu.Unlock()
	if e != nil {
		_ = a.store.setSetting("hosting", old)
		fail(w, 409, e)
		return
	}
	_ = a.store.audit(s.User.ID, "", "settings.updated", next)
	jsonOut(w, 200, next)
}
func mustPort(listen string) int {
	_, port, _ := net.SplitHostPort(listen)
	n, _ := strconv.Atoi(port)
	return n
}
func (a *App) runtimeAPI(w http.ResponseWriter, r *http.Request, s Session, key string) {
	if !requireAdmin(w, s) {
		return
	}
	if _, ok := runtimePins[key]; !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method != "POST" {
		http.NotFound(w, r)
		return
	}
	a.jobsMu.Lock()
	busy := len(a.active) > 0
	a.jobsMu.Unlock()
	if busy {
		fail(w, 409, errors.New("Wait for the active deployment before provisioning a runtime"))
		return
	}
	var offline string
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/zip") {
		offline = filepath.Join(a.cfg.Data, "cache", "runtime-upload-"+id()+".zip")
		r.Body = http.MaxBytesReader(w, r.Body, 128<<20)
		f, e := os.OpenFile(offline, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			fail(w, 500, dbError(e))
			return
		}
		_, e = io.Copy(f, r.Body)
		f.Close()
		if e != nil {
			os.Remove(offline)
			fail(w, 400, errors.New("Runtime upload failed or exceeds 128 MiB"))
			return
		}
	}
	go func() {
		if offline != "" {
			defer os.Remove(offline)
		}
		e := a.runtime.provision(a.ctx, key, offline, nil)
		state := "ready"
		if e != nil {
			state = "needs attention: " + e.Error()
		}
		a.runtime.mu.Lock()
		a.runtime.status[key] = state
		a.runtime.mu.Unlock()
		if e == nil && key == "caddy" {
			a.publishMu.Lock()
			_ = a.reloadRoutes(a.ctx, nil)
			a.publishMu.Unlock()
		}
	}()
	_ = a.store.audit(s.User.ID, "", "runtime.provision", map[string]string{"runtime": key})
	jsonOut(w, 202, map[string]string{"state": "provisioning", "message": "Download/extraction is running. Settings shows the current status."})
}
func Run(ctx context.Context, cfg Config, web fs.FS, browser bool) error {
	a, e := New(ctx, cfg, web)
	if e != nil {
		return e
	}
	defer a.Close()
	host, _, e := net.SplitHostPort(cfg.Listen)
	if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("The management server must listen on an explicit loopback address. Use Workspace or an authenticated HTTPS reverse proxy for remote access")
	}
	ln, e := net.Listen("tcp", cfg.Listen)
	if e != nil {
		return e
	}
	srv := &http.Server{Handler: a.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	fmt.Println("My Hosting", Version, "ready at", cfg.Origin)
	if browser {
		openBrowser(a.setupURL())
	}
	select {
	case <-ctx.Done():
	case e = <-done:
		if !errors.Is(e, http.ErrServerClosed) {
			return e
		}
	}
	stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(stop)
}
