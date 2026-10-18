package hosting

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) teamAPI(w http.ResponseWriter, r *http.Request, s Session, path string) {
	if !requireAdmin(w, s) {
		return
	}
	if r.Method == "GET" {
		rows, e := a.store.db.Query("SELECT id,issuer,subject,name,email,role FROM users ORDER BY name")
		if e != nil {
			fail(w, 500, dbError(e))
			return
		}
		defer rows.Close()
		users := []User{}
		for rows.Next() {
			var u User
			_ = rows.Scan(&u.ID, &u.Issuer, &u.Subject, &u.Name, &u.Email, &u.Role)
			users = append(users, u)
		}
		jsonOut(w, 200, map[string]any{"users": users, "invitations": "Members first sign in with MyAccounts, then an administrator grants access. MyAccounts invitations are managed by MyAccounts."})
		return
	}
	parts := strings.Split(path, "/")
	if r.Method != "PUT" || len(parts) != 2 || !validID.MatchString(parts[1]) {
		http.NotFound(w, r)
		return
	}
	if s.User.Role != "owner" {
		fail(w, 403, errors.New("Only the configured owner can change global roles"))
		return
	}
	var in struct {
		Role string `json:"role"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Role != "admin" && in.Role != "viewer" && in.Role != "disabled" {
		fail(w, 400, errors.New("Choose admin, viewer or disabled; owner identity is configured by Workspace"))
		return
	}
	var current string
	if a.store.db.QueryRow("SELECT role FROM users WHERE id=?", parts[1]).Scan(&current) != nil || current == "owner" {
		fail(w, 409, errors.New("The configured owner cannot be removed or demoted here"))
		return
	}
	_, e := a.store.db.Exec("UPDATE users SET role=? WHERE id=?", in.Role, parts[1])
	if e != nil {
		fail(w, 500, dbError(e))
		return
	}
	if in.Role == "disabled" {
		_, _ = a.store.db.Exec("DELETE FROM sessions WHERE user_id=?", parts[1])
	}
	_ = a.store.audit(s.User.ID, "", "member.role", map[string]string{"user": parts[1], "role": in.Role})
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (a *App) membersAPI(w http.ResponseWriter, r *http.Request, s Session, p Project) {
	if r.Method == "GET" {
		rows, e := a.store.db.Query("SELECT u.id,u.name,u.email,m.role FROM members m JOIN users u ON u.id=m.user_id WHERE m.project_id=? ORDER BY u.name", p.ID)
		if e != nil {
			fail(w, 500, dbError(e))
			return
		}
		defer rows.Close()
		out := []map[string]string{}
		for rows.Next() {
			var uid, name, email, role string
			_ = rows.Scan(&uid, &name, &email, &role)
			out = append(out, map[string]string{"id": uid, "name": name, "email": email, "role": role})
		}
		jsonOut(w, 200, out)
		return
	}
	if !requireAdmin(w, s) {
		return
	}
	var in struct {
		UserID string `json:"userId"`
		Role   string `json:"role"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !validID.MatchString(in.UserID) || (in.Role != "viewer" && in.Role != "developer" && in.Role != "admin") {
		fail(w, 400, errors.New("Select an existing member and viewer, developer or admin role"))
		return
	}
	var e error
	if r.Method == "PUT" {
		_, e = a.store.db.Exec("INSERT INTO members(project_id,user_id,role) VALUES(?,?,?) ON CONFLICT(project_id,user_id) DO UPDATE SET role=excluded.role", p.ID, in.UserID, in.Role)
	} else if r.Method == "DELETE" {
		_, e = a.store.db.Exec("DELETE FROM members WHERE project_id=? AND user_id=?", p.ID, in.UserID)
	} else {
		http.NotFound(w, r)
		return
	}
	if e != nil {
		fail(w, 400, errors.New("The member is unavailable; ask them to sign in first"))
		return
	}
	_ = a.store.audit(s.User.ID, p.ID, "project.member", map[string]string{"user": in.UserID, "role": in.Role, "method": r.Method})
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (a *App) auditAPI(w http.ResponseWriter, r *http.Request, s Session) {
	pid := r.URL.Query().Get("project")
	if pid == "" && !requireAdmin(w, s) {
		return
	}
	if pid != "" && rank(a.store.role(s.User, pid)) == 0 {
		fail(w, 404, errors.New("Project not found"))
		return
	}
	query := "SELECT seq,at,actor,project_id,action,detail,previous,hash FROM audit"
	args := []any{}
	if pid != "" {
		query += " WHERE project_id=?"
		args = append(args, pid)
	}
	query += " ORDER BY seq DESC LIMIT 100"
	rows, e := a.store.db.Query(query, args...)
	if e != nil {
		fail(w, 500, dbError(e))
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var seq int
		var at, actor, project, action, detail, previous, hash string
		_ = rows.Scan(&seq, &at, &actor, &project, &action, &detail, &previous, &hash)
		var body any
		_ = json.Unmarshal([]byte(detail), &body)
		out = append(out, map[string]any{"seq": seq, "at": at, "actor": actor, "projectId": project, "action": action, "detail": body, "previous": previous, "hash": hash})
	}
	jsonOut(w, 200, out)
}
func (a *App) domains(pid string) ([]Domain, error) {
	rows, e := a.store.db.Query("SELECT detail FROM domains WHERE project_id=? ORDER BY host", pid)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Domain{}
	for rows.Next() {
		var raw string
		var d Domain
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(raw), &d); e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (a *App) verifiedDomains(pid string) ([]string, error) {
	ds, e := a.domains(pid)
	if e != nil {
		return nil, e
	}
	hosts := []string{}
	for _, d := range ds {
		if d.Verified {
			hosts = append(hosts, d.Host)
		}
	}
	return hosts, nil
}
func (a *App) saveDomain(d Domain) error {
	_, e := a.store.db.Exec("INSERT INTO domains(host,project_id,detail) VALUES(?,?,?) ON CONFLICT(host) DO UPDATE SET detail=excluded.detail", d.Host, d.ProjectID, encode(d))
	return e
}
func publicAddress(ip net.IP) bool {
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified() && !ip.Equal(net.ParseIP("169.254.169.254"))
}
func (a *App) verifyDomain(r *http.Request, d *Domain) {
	d.Verified = false
	d.HTTPS = "pending"
	d.Error = ""
	txt, e := net.DefaultResolver.LookupTXT(r.Context(), "_one-workspace."+d.Host)
	if e != nil {
		d.Error = "TXT record not found. Add _one-workspace." + d.Host + " with value " + d.Token + ", wait for DNS propagation, then verify again."
		return
	}
	found := false
	for _, v := range txt {
		if v == d.Token {
			found = true
		}
	}
	if !found {
		d.Error = "The domain verification TXT value does not match this installation."
		return
	}
	ips, e := net.DefaultResolver.LookupIPAddr(r.Context(), d.Host)
	if e != nil || len(ips) == 0 {
		d.Error = "Add an A/AAAA record pointing to this host's public address, then verify again."
		return
	}
	for _, ip := range ips {
		if !publicAddress(ip.IP) {
			d.Error = "Public hosting domains must resolve to public addresses; private, loopback and link-local targets are rejected."
			return
		}
	}
	d.Verified = true
	if !a.settings().Public {
		d.Error = "DNS ownership verified. Enable public HTTPS in Settings and allow inbound TCP 80/443 (and router forwarding where required). A Cloudflare Tunnel does not provide these listeners automatically."
		return
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	target := net.JoinHostPort(ips[0].IP.String(), "443")
	conn, e := tls.DialWithDialer(dialer, "tcp", target, &tls.Config{ServerName: d.Host, MinVersion: tls.VersionTLS12})
	if e != nil {
		d.Error = "DNS verified; HTTPS is not yet reachable with a valid certificate. Check public A/AAAA records, inbound 80/443, router forwarding, Caddy logs and ACME rate limits."
		return
	}
	conn.Close()
	d.HTTPS = "certificate-valid"
}
func (a *App) domainAPI(w http.ResponseWriter, r *http.Request, s Session, p Project, tail []string) {
	if r.Method == "GET" {
		ds, e := a.domains(p.ID)
		if e != nil {
			fail(w, 500, dbError(e))
			return
		}
		jsonOut(w, 200, map[string]any{"domains": ds, "localURL": fmt.Sprintf("http://%s.localhost:%d", p.Slug, a.settings().CaddyPort), "publicEnabled": a.settings().Public, "guidance": "Own the domain with a TXT record, point A/AAAA records at this Windows host, then permit TCP 80/443. Automatic HTTPS remains pending until a real certificate is observed."})
		return
	}
	if !requireAdmin(w, s) {
		return
	}
	if len(tail) == 0 && r.Method == "POST" {
		var in struct {
			Host string `json:"host"`
		}
		if !readJSON(w, r, &in) {
			return
		}
		in.Host = strings.ToLower(strings.TrimSpace(in.Host))
		if !validDomain(in.Host) || a.reservedSiteDomain(in.Host) {
			fail(w, 400, errors.New("Enter a separate custom hostname; managed site addresses and dashboard/identity hostnames cannot be reassigned"))
			return
		}
		var existing string
		if a.store.db.QueryRow("SELECT project_id FROM domains WHERE host=?", in.Host).Scan(&existing) == nil {
			fail(w, 409, errors.New("That domain is already registered; remove its previous binding first"))
			return
		}
		d := Domain{Host: in.Host, ProjectID: p.ID, Token: "one-workspace=" + id(), HTTPS: "unverified"}
		if e := a.saveDomain(d); e != nil {
			fail(w, 500, dbError(e))
			return
		}
		_ = a.store.audit(s.User.ID, p.ID, "domain.add", map[string]string{"host": d.Host})
		jsonOut(w, 201, d)
		return
	}
	if len(tail) == 0 {
		http.NotFound(w, r)
		return
	}
	host := tail[0]
	if !validDomain(host) {
		http.NotFound(w, r)
		return
	}
	var raw string
	if a.store.db.QueryRow("SELECT detail FROM domains WHERE host=? AND project_id=?", host, p.ID).Scan(&raw) != nil {
		fail(w, 404, errors.New("Domain not found"))
		return
	}
	var d Domain
	_ = json.Unmarshal([]byte(raw), &d)
	if len(tail) == 2 && tail[1] == "verify" && r.Method == "POST" {
		a.verifyDomain(r, &d)
		if e := a.saveDomain(d); e != nil {
			fail(w, 500, dbError(e))
			return
		}
		if d.Verified && a.settings().Public {
			a.publishMu.Lock()
			e := a.reloadRoutes(r.Context(), nil)
			a.publishMu.Unlock()
			if e != nil {
				d.Error = e.Error()
				_ = a.saveDomain(d)
			}
		}
		_ = a.store.audit(s.User.ID, p.ID, "domain.verify", map[string]any{"host": host, "verified": d.Verified, "https": d.HTTPS})
		jsonOut(w, 200, d)
		return
	}
	if len(tail) == 1 && r.Method == "DELETE" {
		_, e := a.store.db.Exec("DELETE FROM domains WHERE host=? AND project_id=?", host, p.ID)
		if e != nil {
			fail(w, 500, dbError(e))
			return
		}
		a.publishMu.Lock()
		e = a.reloadRoutes(r.Context(), nil)
		a.publishMu.Unlock()
		if e != nil {
			_ = a.saveDomain(d)
			fail(w, 409, e)
			return
		}
		_ = a.store.audit(s.User.ID, p.ID, "domain.remove", map[string]string{"host": host})
		jsonOut(w, 200, map[string]bool{"ok": true})
		return
	}
	http.NotFound(w, r)
}
func (a *App) metricsAPI(w http.ResponseWriter, r *http.Request, s Session, p Project) {
	hosts := map[string]bool{p.Slug + ".localhost": true}
	rows, e := a.store.db.Query("SELECT detail FROM deployments WHERE project_id=? AND state='published'", p.ID)
	if e == nil {
		for rows.Next() {
			var raw string
			var d Deploy
			if rows.Scan(&raw) == nil && json.Unmarshal([]byte(raw), &d) == nil {
				hosts[d.Snapshot.SlugHostLocal(d)] = true
				hosts[a.siteHost(d)] = true
			}
		}
		rows.Close()
	}
	if custom, e := a.verifiedDomains(p.ID); e == nil {
		for _, host := range custom {
			hosts[host] = true
		}
	}
	f, e := os.Open(filepath.Join(a.cfg.Data, "logs", "access.json"))
	count, errorsCount := 0, 0
	if e == nil {
		defer f.Close()
		if info, e := f.Stat(); e == nil && info.Size() > 2<<20 {
			f.Seek(-(2 << 20), io.SeekEnd)
		}
		raw, _ := io.ReadAll(io.LimitReader(f, 2<<20))
		for _, line := range strings.Split(string(raw), "\n") {
			var item struct {
				Status  int `json:"status"`
				Request struct {
					Host string `json:"host"`
				} `json:"request"`
			}
			if json.Unmarshal([]byte(line), &item) != nil {
				continue
			}
			host := strings.ToLower(item.Request.Host)
			if name, _, err := net.SplitHostPort(host); err == nil {
				host = name
			}
			if hosts[strings.TrimSuffix(host, ".")] {
				count++
				if item.Status >= 500 {
					errorsCount++
				}
			}
		}
	}
	jsonOut(w, 200, map[string]any{"requests": count, "serverErrors": errorsCount, "scope": "Actual records in the latest 2 MiB of retained local Caddy access logs; not lifetime analytics", "uptimeSeconds": int(time.Since(a.started).Seconds()), "uptimeScope": "Current control-plane process uptime; not a historical availability percentage"})
}
