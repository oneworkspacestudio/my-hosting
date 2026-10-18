package hosting

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func (a *App) putSecret(pid, name, environment, value string) error {
	encrypted, e := protect([]byte(value))
	if e != nil {
		return e
	}
	_, e = a.store.db.Exec("INSERT INTO secrets(project_id,name,environment,value) VALUES(?,?,?,?) ON CONFLICT(project_id,name,environment) DO UPDATE SET value=excluded.value", pid, name, environment, encrypted)
	return e
}
func (a *App) getSecret(pid, name, environment string) (string, error) {
	var raw []byte
	e := a.store.db.QueryRow("SELECT value FROM secrets WHERE project_id=? AND name=? AND environment=?", pid, name, environment).Scan(&raw)
	if e != nil {
		return "", e
	}
	plain, e := unprotect(raw)
	return string(plain), e
}
func (a *App) secretValues(pid, environment string) (map[string]string, error) {
	rows, e := a.store.db.Query("SELECT name,value FROM secrets WHERE project_id=? AND environment=?", pid, environment)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name string
		var raw []byte
		if e = rows.Scan(&name, &raw); e != nil {
			return nil, e
		}
		b, e := unprotect(raw)
		if e != nil {
			return nil, e
		}
		out[name] = string(b)
	}
	return out, rows.Err()
}
func (a *App) githubCall(ctx context.Context, p Project, endpoint string, out any) error {
	token, _ := a.getSecret(p.ID, "GITHUB_TOKEN", "integration")
	req, e := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/"+p.Repository+endpoint, nil)
	if e != nil {
		return e
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "OneWorkspace-MyHosting")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(req)
	if e != nil {
		return errors.New("GitHub is unavailable; check the connection and retry")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub returned HTTP %d; check the repository, branch and fine-grained Contents:read permission", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
}
func (a *App) resolveSource(ctx context.Context, p Project, branch, sha string) (string, string, string, error) {
	if p.Repository == "" {
		return "", "", "", errors.New("Connect a GitHub repository or upload a ZIP")
	}
	if branch == "" {
		branch = p.Branch
	}
	if !validBranch.MatchString(branch) || strings.Contains(branch, "..") {
		return "", "", "", errors.New("Invalid branch")
	}
	if branch != p.Branch && !p.Previews {
		return "", "", "", errors.New("Preview branches are disabled for this project")
	}
	ref := branch
	if sha != "" {
		if !validSHA.MatchString(sha) {
			return "", "", "", errors.New("Select a full 40-character commit SHA")
		}
		ref = sha
	}
	var c struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"commit"`
	}
	if e := a.githubCall(ctx, p, "/commits/"+url.PathEscape(ref), &c); e != nil {
		return "", "", "", e
	}
	if !validSHA.MatchString(c.SHA) || sha != "" && c.SHA != sha {
		return "", "", "", errors.New("GitHub did not resolve the requested immutable commit")
	}
	if sha != "" {
		var compare struct {
			Status string `json:"status"`
		}
		if e := a.githubCall(ctx, p, "/compare/"+url.PathEscape(branch)+"..."+c.SHA, &compare); e != nil {
			return "", "", "", e
		}
		if compare.Status != "identical" && compare.Status != "behind" {
			return "", "", "", errors.New("The selected commit is not on the allowed branch history")
		}
	}
	msg := strings.SplitN(c.Commit.Message, "\n", 2)[0]
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return c.SHA, c.Commit.Author.Name, msg, nil
}
func (a *App) fetchSource(ctx context.Context, d *Deploy, report func(string)) error {
	if d.Source == "zip" {
		sum, e := hashFile(a.sourcePath(*d))
		if e != nil || sum != d.SourceHash {
			return errors.New("Uploaded source checksum does not match the queued deployment")
		}
		return nil
	}
	p := d.Snapshot
	report("Downloading immutable GitHub source " + d.Commit)
	token, _ := a.getSecret(p.ID, "GITHUB_TOKEN", "integration")
	dest := a.sourcePath(*d)
	if e := download(ctx, "https://api.github.com/repos/"+p.Repository+"/tarball/"+d.Commit, dest, token, 64<<20, report); e != nil {
		return e
	}
	sum, e := hashFile(dest)
	if e != nil {
		return e
	}
	d.SourceHash = sum
	return a.store.saveDeploy(*d)
}
func (a *App) sourcePath(d Deploy) string {
	ext := ".tar.gz"
	if d.Source == "zip" {
		ext = ".zip"
	}
	return filepath.Join(a.cfg.Data, "cache", "source-"+d.ID+ext)
}
func (a *App) githubAPI(w http.ResponseWriter, r *http.Request, s Session, p Project, tail []string) {
	if r.Method == "GET" {
		if len(tail) > 0 {
			var result any
			endpoint := ""
			switch tail[0] {
			case "branches":
				endpoint = "/branches?per_page=100"
			case "commits":
				branch := r.URL.Query().Get("branch")
				if branch == "" {
					branch = p.Branch
				}
				if !validBranch.MatchString(branch) {
					fail(w, 400, errors.New("Invalid branch"))
					return
				}
				endpoint = "/commits?per_page=30&sha=" + url.QueryEscape(branch)
			default:
				http.NotFound(w, r)
				return
			}
			if p.Repository == "" {
				fail(w, 400, errors.New("Connect a repository first"))
				return
			}
			if e := a.githubCall(r.Context(), p, endpoint, &result); e != nil {
				fail(w, 502, e)
				return
			}
			jsonOut(w, 200, result)
			return
		}
		token, _ := a.getSecret(p.ID, "GITHUB_TOKEN", "integration")
		hook, _ := a.getSecret(p.ID, "WEBHOOK_SECRET", "integration")
		jsonOut(w, 200, map[string]any{"repository": p.Repository, "privateCredentialSaved": token != "", "webhookConfigured": hook != "", "webhookURL": a.cfg.Origin + "/hooks/github/" + p.ID, "permission": "Fine-grained Contents: read for this repository only; webhooks are configured by its administrator"})
		return
	}
	if !requireAdmin(w, s) {
		return
	}
	if r.Method != "POST" {
		http.NotFound(w, r)
		return
	}
	var in struct {
		Token         string `json:"token"`
		RotateWebhook bool   `json:"rotateWebhook"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if len(in.Token) > 500 || strings.ContainsAny(in.Token, "\r\n\x00") {
		fail(w, 400, errors.New("Invalid GitHub credential"))
		return
	}
	if p.Repository == "" {
		fail(w, 400, errors.New("Set the repository in Build settings first"))
		return
	}
	if in.Token != "" {
		if e := a.putSecret(p.ID, "GITHUB_TOKEN", "integration", in.Token); e != nil {
			fail(w, 500, errors.New("Could not protect the GitHub credential"))
			return
		}
	}
	var repo struct {
		FullName string `json:"full_name"`
	}
	if e := a.githubCall(r.Context(), p, "", &repo); e != nil {
		fail(w, 400, e)
		return
	}
	hook, _ := a.getSecret(p.ID, "WEBHOOK_SECRET", "integration")
	show := ""
	if hook == "" || in.RotateWebhook {
		hook = id() + id()
		if e := a.putSecret(p.ID, "WEBHOOK_SECRET", "integration", hook); e != nil {
			fail(w, 500, errors.New("Could not protect the webhook secret"))
			return
		}
		show = hook
	}
	_ = a.store.audit(s.User.ID, p.ID, "github.connected", map[string]any{"repository": repo.FullName, "rotatedWebhook": in.RotateWebhook})
	jsonOut(w, 200, map[string]string{"repository": repo.FullName, "webhookURL": a.cfg.Origin + "/hooks/github/" + p.ID, "webhookSecret": show, "message": "GitHub repository access verified. Configure a JSON push webhook using this URL and the secret shown once."})
}
func (a *App) webhook(w http.ResponseWriter, r *http.Request, pid string) {
	if r.Method != "POST" || !validID.MatchString(pid) {
		http.NotFound(w, r)
		return
	}
	p, e := a.store.project(pid)
	secret, se := a.getSecret(pid, "WEBHOOK_SECRET", "integration")
	if e != nil || se != nil || secret == "" {
		fail(w, 401, errors.New("Webhook is not configured"))
		return
	}
	body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if e != nil {
		fail(w, 413, errors.New("Webhook payload is too large"))
		return
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(r.Header.Get("X-Hub-Signature-256"))) {
		fail(w, 401, errors.New("Webhook signature is invalid"))
		return
	}
	if r.Header.Get("X-GitHub-Event") == "ping" {
		jsonOut(w, 200, map[string]string{"state": "verified"})
		return
	}
	if r.Header.Get("X-GitHub-Event") != "push" {
		jsonOut(w, 202, map[string]string{"state": "ignored", "reason": "Only push events trigger deployment"})
		return
	}
	var event struct {
		Ref        string `json:"ref"`
		After      string `json:"after"`
		Deleted    bool   `json:"deleted"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if e = json.Unmarshal(body, &event); e != nil || !strings.EqualFold(event.Repository.FullName, p.Repository) {
		fail(w, 400, errors.New("Webhook repository does not match this project"))
		return
	}
	if event.Deleted || !strings.HasPrefix(event.Ref, "refs/heads/") {
		jsonOut(w, 202, map[string]string{"state": "ignored", "reason": "Deleted branches and tags are not deployed"})
		return
	}
	branch := strings.TrimPrefix(event.Ref, "refs/heads/")
	if branch != p.Branch && !p.Previews {
		jsonOut(w, 202, map[string]string{"state": "ignored", "reason": "Branch is not allowed"})
		return
	}
	environment := "production"
	if branch != p.Branch {
		environment = "preview"
	}
	d, e := a.queueGit(r.Context(), p, branch, event.After, environment, "webhook", digest(body), "github-webhook")
	if e != nil {
		fail(w, 400, e)
		return
	}
	jsonOut(w, 202, d)
}

var envName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,99}$`)

func (a *App) secretAPI(w http.ResponseWriter, r *http.Request, s Session, p Project) {
	if r.Method == "GET" {
		rows, e := a.store.db.Query("SELECT name,environment FROM secrets WHERE project_id=? AND environment!='integration' ORDER BY environment,name", p.ID)
		if e != nil {
			fail(w, 500, dbError(e))
			return
		}
		defer rows.Close()
		names := []map[string]string{}
		for rows.Next() {
			var n, env string
			_ = rows.Scan(&n, &env)
			names = append(names, map[string]string{"name": n, "environment": env, "value": "••••••••"})
		}
		jsonOut(w, 200, names)
		return
	}
	if !requireAdmin(w, s) {
		return
	}
	var in struct {
		Name        string `json:"name"`
		Environment string `json:"environment"`
		Value       string `json:"value"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	reserved := map[string]bool{"PATH": true, "SYSTEMROOT": true, "WINDIR": true, "TEMP": true, "TMP": true, "HOME": true, "USERPROFILE": true, "APPDATA": true, "LOCALAPPDATA": true, "PORT": true, "HOST": true, "HOSTNAME": true}
	if !envName.MatchString(in.Name) || reserved[in.Name] || strings.HasPrefix(in.Name, "WORKSPACE_") || (in.Environment != "production" && in.Environment != "preview") || len(in.Value) > 16384 || strings.ContainsRune(in.Value, 0) {
		fail(w, 400, errors.New("Use an uppercase environment name, production or preview, and a value up to 16 KiB; protected runtime variables cannot be overridden"))
		return
	}
	var e error
	if r.Method == "PUT" {
		e = a.putSecret(p.ID, in.Name, in.Environment, in.Value)
	} else if r.Method == "DELETE" {
		_, e = a.store.db.Exec("DELETE FROM secrets WHERE project_id=? AND name=? AND environment=?", p.ID, in.Name, in.Environment)
	} else {
		http.NotFound(w, r)
		return
	}
	if e != nil {
		fail(w, 500, errors.New("Could not update protected environment value"))
		return
	}
	_ = a.store.audit(s.User.ID, p.ID, "environment.updated", map[string]string{"name": in.Name, "environment": in.Environment, "method": r.Method})
	jsonOut(w, 200, map[string]bool{"ok": true})
}
