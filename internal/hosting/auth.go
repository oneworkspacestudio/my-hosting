package hosting

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"errors"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type loginFlow struct {
	Verifier, Nonce string
	Expires         time.Time
}
type Session struct {
	User    User
	CSRF    string
	Expires time.Time
}
type attempt struct {
	Count int
	Since time.Time
}

func (a *App) cookie(w http.ResponseWriter, name, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: a.base + "/", HttpOnly: true, Secure: strings.HasPrefix(a.cfg.Origin, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: age})
}
func (a *App) sameOrigin(r *http.Request) bool {
	u, _ := url.Parse(a.cfg.Origin)
	return r.Header.Get("Origin") == u.Scheme+"://"+u.Host
}
func (a *App) session(r *http.Request) (Session, bool) {
	var s Session
	c, e := r.Cookie("myhosting_session")
	if e != nil || len(c.Value) != 64 {
		return s, false
	}
	var expiry string
	e = a.store.db.QueryRow(`SELECT u.id,u.issuer,u.subject,u.name,u.email,u.role,s.csrf,s.expires FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.id=?`, digest([]byte(c.Value))).Scan(&s.User.ID, &s.User.Issuer, &s.User.Subject, &s.User.Name, &s.User.Email, &s.User.Role, &s.CSRF, &expiry)
	if e != nil {
		return s, false
	}
	s.Expires, e = time.Parse(time.RFC3339Nano, expiry)
	return s, e == nil && time.Now().Before(s.Expires) && s.User.Role != "disabled"
}
func (a *App) createSession(w http.ResponseWriter, u User, expires time.Time) error {
	token := id() + id()
	csrf := id() + id()
	_, e := a.store.db.Exec("INSERT INTO sessions(id,user_id,csrf,expires) VALUES(?,?,?,?)", digest([]byte(token)), u.ID, csrf, expires.UTC().Format(time.RFC3339Nano))
	if e == nil {
		a.cookie(w, "myhosting_session", token, int(time.Until(expires).Seconds()))
		_ = a.store.audit(u.ID, "", "sign-in", map[string]string{"issuer": u.Issuer})
	}
	return e
}
func (a *App) discover(ctx context.Context) error {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if a.verifier != nil {
		return nil
	}
	if a.cfg.Issuer == "" {
		return errors.New("MyAccounts is not configured")
	}
	ctx = oidc.ClientContext(ctx, &http.Client{Timeout: 20 * time.Second})
	p, e := oidc.NewProvider(ctx, a.cfg.Issuer)
	if e != nil {
		return errors.New("MyAccounts discovery is unavailable; check its configured address and connection")
	}
	a.oauth = oauth2.Config{ClientID: a.cfg.ClientID, ClientSecret: a.cfg.ClientSecret, RedirectURL: a.cfg.Origin + "/auth/callback", Scopes: []string{oidc.ScopeOpenID, "profile", "email"}, Endpoint: p.Endpoint()}
	a.verifier = p.Verifier(&oidc.Config{ClientID: a.cfg.ClientID})
	return nil
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if a.cfg.Issuer == "" {
		http.Redirect(w, r, a.base+"/#login", 303)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if e := a.discover(ctx); e != nil {
		fail(w, 503, e)
		return
	}
	state, nonce, v := id()+id(), id(), oauth2.GenerateVerifier()
	a.authMu.Lock()
	for k, f := range a.flows {
		if time.Now().After(f.Expires) {
			delete(a.flows, k)
		}
	}
	if len(a.flows) >= 500 {
		a.authMu.Unlock()
		fail(w, 429, errors.New("Too many sign-in attempts; try again shortly"))
		return
	}
	a.flows[state] = loginFlow{v, nonce, time.Now().Add(5 * time.Minute)}
	a.authMu.Unlock()
	a.cookie(w, "myhosting_login", state, 300)
	http.Redirect(w, r, a.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(v), oidc.Nonce(nonce)), 302)
}
func (a *App) callback(w http.ResponseWriter, r *http.Request) {
	bad := func() {
		fail(w, 401, errors.New("Sign-in did not finish. Open My Hosting from Workspace to try again"))
	}
	c, e := r.Cookie("myhosting_login")
	state := r.URL.Query().Get("state")
	if e != nil || state == "" || !hmac.Equal([]byte(c.Value), []byte(state)) {
		bad()
		return
	}
	a.authMu.Lock()
	f, ok := a.flows[state]
	delete(a.flows, state)
	a.authMu.Unlock()
	a.cookie(w, "myhosting_login", "", -1)
	if !ok || time.Now().After(f.Expires) || r.URL.Query().Get("error") != "" || a.verifier == nil {
		bad()
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	ctx = oidc.ClientContext(ctx, &http.Client{Timeout: 20 * time.Second})
	token, e := a.oauth.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(f.Verifier))
	if e != nil {
		bad()
		return
	}
	raw, _ := token.Extra("id_token").(string)
	identity, e := a.verifier.Verify(ctx, raw)
	if e != nil || identity.Nonce != f.Nonce || identity.Subject == "" {
		bad()
		return
	}
	var claims struct {
		Name     string `json:"name"`
		Display  string `json:"displayName"`
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
	}
	if identity.Claims(&claims) != nil {
		bad()
		return
	}
	if claims.Display != "" {
		claims.Name = claims.Display
	}
	if claims.Name == "" {
		claims.Name = "Workspace member"
	}
	if !claims.Verified {
		claims.Email = ""
	}
	role := "viewer"
	if identity.Subject == a.cfg.Owner && a.cfg.Owner != "" {
		role = "owner"
	}
	_, e = a.store.db.Exec(`INSERT INTO users(id,issuer,subject,name,email,role) VALUES(?,?,?,?,?,?) ON CONFLICT(issuer,subject) DO UPDATE SET name=excluded.name,email=excluded.email`, id(), identity.Issuer, identity.Subject, claims.Name, claims.Email, role)
	if e != nil {
		fail(w, 500, dbError(e))
		return
	}
	var u User
	e = a.store.db.QueryRow("SELECT id,issuer,subject,name,email,role FROM users WHERE issuer=? AND subject=?", identity.Issuer, identity.Subject).Scan(&u.ID, &u.Issuer, &u.Subject, &u.Name, &u.Email, &u.Role)
	if e != nil || u.Role == "disabled" {
		bad()
		return
	}
	expires := time.Now().Add(8 * time.Hour)
	if identity.Expiry.Before(expires) {
		expires = identity.Expiry
	}
	if e = a.createSession(w, u, expires); e != nil {
		fail(w, 500, dbError(e))
		return
	}
	http.Redirect(w, r, a.base+"/", 303)
}
func (a *App) localLogin(w http.ResponseWriter, r *http.Request) {
	if a.cfg.Issuer != "" {
		fail(w, 403, errors.New("Use your MyAccounts session; independent authentication is disabled"))
		return
	}
	if !a.sameOrigin(r) {
		fail(w, 403, errors.New("Open the configured My Hosting address to sign in"))
		return
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	a.authMu.Lock()
	at := a.attempts[host]
	if time.Since(at.Since) > 10*time.Minute {
		at = attempt{Since: time.Now()}
	}
	at.Count++
	a.attempts[host] = at
	a.authMu.Unlock()
	if at.Count > 20 {
		fail(w, 429, errors.New("Too many attempts. Wait ten minutes before retrying"))
		return
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	var u User
	var password []byte
	e := a.store.db.QueryRow("SELECT id,issuer,subject,name,email,role,password FROM users WHERE issuer='local' AND subject=?", strings.ToLower(strings.TrimSpace(in.Email))).Scan(&u.ID, &u.Issuer, &u.Subject, &u.Name, &u.Email, &u.Role, &password)
	if e != nil || u.Role == "disabled" || bcrypt.CompareHashAndPassword(password, []byte(in.Password)) != nil {
		fail(w, 401, errors.New("Email or password is incorrect"))
		return
	}
	if e = a.createSession(w, u, time.Now().Add(8*time.Hour)); e != nil {
		fail(w, 500, dbError(e))
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if !a.sameOrigin(r) {
		fail(w, 403, errors.New("Sign out from My Hosting or Workspace"))
		return
	}
	if c, e := r.Cookie("myhosting_session"); e == nil {
		_, _ = a.store.db.Exec("DELETE FROM sessions WHERE id=?", digest([]byte(c.Value)))
	}
	a.cookie(w, "myhosting_session", "", -1)
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func Bootstrap(cfg Config, email, password string) error {
	if cfg.Issuer != "" {
		return errors.New("Standalone bootstrap is disabled when MyAccounts is configured")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") || len(password) < 8 || len(password) > 72 {
		return errors.New("Enter an email and an 8–72 byte password")
	}
	if e := prepareData(cfg.Data); e != nil {
		return e
	}
	s, e := openStore(cfg.Data)
	if e != nil {
		return e
	}
	defer s.db.Close()
	var count int
	if e = s.db.QueryRow("SELECT COUNT(*) FROM users WHERE role='owner'").Scan(&count); e != nil {
		return e
	}
	if count != 0 {
		return errors.New("This installation already has an owner; bootstrap cannot reset credentials")
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	_, e = s.db.Exec("INSERT INTO users(id,issuer,subject,name,email,role,password) VALUES(?,'local',?,?,?,'owner',?)", id(), email, "Administrator", email, hash)
	return e
}
func rank(role string) int {
	switch role {
	case "owner":
		return 4
	case "admin":
		return 3
	case "developer":
		return 2
	case "viewer":
		return 1
	}
	return 0
}
func requireAdmin(w http.ResponseWriter, s Session) bool {
	if rank(s.User.Role) < 3 {
		fail(w, 403, errors.New("Only a Hosting administrator can change this setting"))
		return false
	}
	return true
}

var _ = sql.ErrNoRows
