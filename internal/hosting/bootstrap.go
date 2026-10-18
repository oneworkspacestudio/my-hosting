package hosting

import (
	"crypto/hmac"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) prepareBootstrap() error {
	if a.cfg.Issuer != "" {
		return nil
	}
	u, _ := url.Parse(a.cfg.Origin)
	host := u.Hostname()
	ip := net.ParseIP(host)
	a.localSetup = host == "localhost" || ip != nil && ip.IsLoopback()
	var n int
	if e := a.store.db.QueryRow("SELECT COUNT(*) FROM users WHERE role='owner'").Scan(&n); e != nil {
		return e
	}
	if n > 0 || !a.localSetup {
		return nil
	}
	a.bootstrapToken = id() + id()
	raw, e := protect([]byte(a.bootstrapToken))
	if e != nil {
		return e
	}
	return atomicFile(filepath.Join(a.cfg.Data, "secrets", "setup.dpapi"), raw)
}
func (a *App) bootstrapCode() string {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	return a.bootstrapToken
}
func (a *App) setupURL() string {
	if token := a.bootstrapCode(); token != "" {
		return a.cfg.Origin + "/#setup/" + token
	}
	return a.cfg.Origin
}
func (a *App) bootstrap(w http.ResponseWriter, r *http.Request) {
	token := a.bootstrapCode()
	if a.cfg.Issuer != "" || !a.localSetup || token == "" || !a.sameOrigin(r) {
		fail(w, 403, errors.New("Local setup is unavailable. Use MyAccounts or the protected standalone bootstrap command"))
		return
	}
	var in struct {
		Token    string `json:"token"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Confirm  string `json:"confirm"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !hmac.Equal([]byte(in.Token), []byte(token)) {
		fail(w, 403, errors.New("Open the private setup link from this computer's My Hosting launcher"))
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	address, e := mail.ParseAddress(email)
	if e != nil || address.Address != email || len(in.Password) < 8 || len(in.Password) > 72 || in.Password != in.Confirm {
		fail(w, 400, errors.New("Enter an email and matching passwords of 8–72 bytes"))
		return
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if e != nil {
		fail(w, 500, errors.New("Could not protect the password"))
		return
	}
	u := User{ID: id(), Issuer: "local", Subject: email, Name: "Administrator", Email: email, Role: "owner"}
	_, e = a.store.db.Exec("INSERT INTO users(id,issuer,subject,name,email,role,password) VALUES(?,'local',?,?,?,'owner',?)", u.ID, email, u.Name, email, hash)
	if e != nil {
		fail(w, 409, errors.New("An owner already exists; setup cannot reset an account"))
		return
	}
	a.authMu.Lock()
	a.bootstrapToken = ""
	a.authMu.Unlock()
	_ = os.Remove(filepath.Join(a.cfg.Data, "secrets", "setup.dpapi"))
	if e = a.createSession(w, u, time.Now().Add(8*time.Hour)); e != nil {
		fail(w, 500, dbError(e))
		return
	}
	jsonOut(w, 201, map[string]bool{"ok": true})
}
func OpenSetup(cfg Config) error {
	raw, e := os.ReadFile(filepath.Join(cfg.Data, "secrets", "setup.dpapi"))
	if e != nil {
		return errors.New("No pending setup link. Start MyHosting.exe normally to create one")
	}
	token, e := unprotect(raw)
	if e != nil {
		return e
	}
	if len(token) != 64 {
		return errors.New("Setup link is invalid; restart My Hosting")
	}
	openBrowser(strings.TrimRight(cfg.Origin, "/") + "/#setup/" + string(token))
	return nil
}
