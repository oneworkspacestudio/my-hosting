package hosting

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (a *App) notify() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}
func (a *App) queued(p Project, source, branch, sha, environment, action, parent, key, actor string) (Deploy, error) {
	if environment != "production" && environment != "preview" {
		return Deploy{}, errors.New("Choose production or preview")
	}
	if environment == "preview" && !p.Previews {
		return Deploy{}, errors.New("Enable previews in Build settings first")
	}
	if action == "build" && environment == "production" && branch != "" && branch != p.Branch {
		return Deploy{}, errors.New("Production deployments must use the production branch")
	}
	if key != "" {
		var raw string
		if a.store.db.QueryRow("SELECT detail FROM deployments WHERE project_id=? AND idempotency=?", p.ID, key).Scan(&raw) == nil {
			var d Deploy
			_ = json.Unmarshal([]byte(raw), &d)
			return d, nil
		}
	}
	var count int
	_ = a.store.db.QueryRow("SELECT COUNT(*) FROM deployments WHERE state IN ('queued','building','publishing')").Scan(&count)
	if count >= 100 {
		return Deploy{}, errors.New("Deployment queue is full; wait for a job or cancel queued work")
	}
	d := Deploy{ID: id(), ProjectID: p.ID, State: "queued", Environment: environment, Source: source, Commit: sha, Branch: branch, Created: now(), Action: action, Parent: parent, Snapshot: p}
	d.Snapshot.Role = ""
	if source == "zip" && action == "build" {
		d.State = "uploading"
	}
	values, e := a.secretValues(p.ID, environment)
	if e != nil {
		return d, e
	}
	encrypted, e := protect([]byte(encode(values)))
	if e != nil {
		return d, e
	}
	tx, e := a.store.db.Begin()
	if e != nil {
		return d, e
	}
	defer tx.Rollback()
	var idem any
	if key != "" {
		idem = key
	}
	_, e = tx.Exec("INSERT INTO deployments(id,project_id,state,created,detail,idempotency) VALUES(?,?,?,?,?,?)", d.ID, p.ID, d.State, d.Created, encode(d), idem)
	if e != nil {
		return d, errors.New("The deployment was already queued or storage is unavailable")
	}
	_, e = tx.Exec("INSERT INTO deployment_env(deploy_id,value) VALUES(?,?)", d.ID, encrypted)
	if e != nil {
		return d, e
	}
	if e = tx.Commit(); e != nil {
		return d, e
	}
	_ = a.store.audit(actor, p.ID, "deploy.queued", map[string]string{"deploy": d.ID, "action": action, "environment": environment, "commit": sha})
	return d, nil
}
func (a *App) queueGit(ctx context.Context, p Project, branch, sha, environment, action, key, actor string) (Deploy, error) {
	if branch == "" {
		branch = p.Branch
	}
	resolved, author, message, e := a.resolveSource(ctx, p, branch, sha)
	if e != nil {
		return Deploy{}, e
	}
	d, e := a.queued(p, "github", branch, resolved, environment, action, "", key, actor)
	if e == nil && d.State == "queued" {
		d.Author = author
		d.Message = message
		e = a.store.saveDeploy(d)
		a.notify()
	}
	return d, e
}
func (a *App) recoverJobs() error {
	rows, e := a.store.db.Query("SELECT detail FROM deployments WHERE state IN ('building','publishing')")
	if e != nil {
		return e
	}
	var ds []Deploy
	for rows.Next() {
		var raw string
		var d Deploy
		_ = rows.Scan(&raw)
		if e = json.Unmarshal([]byte(raw), &d); e != nil {
			rows.Close()
			return e
		}
		ds = append(ds, d)
	}
	rows.Close()
	for _, d := range ds {
		d.State = "failed"
		d.Error = "The service stopped during this deployment. The previous published release is retained. Retry this exact source revision when ready."
		d.Finished = now()
		if e = a.store.saveDeploy(d); e != nil {
			return e
		}
	}
	return nil
}
func (a *App) scheduler() {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-a.wake:
		case <-t.C:
		}
		for {
			if a.ctx.Err() != nil {
				return
			}
			var raw string
			e := a.store.db.QueryRow("SELECT detail FROM deployments WHERE state='queued' ORDER BY created LIMIT 1").Scan(&raw)
			if e == sql.ErrNoRows {
				break
			}
			if e != nil {
				break
			}
			var d Deploy
			if json.Unmarshal([]byte(raw), &d) != nil {
				break
			}
			a.execute(d)
		}
	}
}
func (a *App) releaseRoot(d Deploy) string {
	artifact := d.Artifact
	if artifact == "" {
		artifact = d.ID
	}
	return filepath.Join(a.cfg.Data, "projects", d.ProjectID, "releases", artifact)
}
func (a *App) workspace(d Deploy) string {
	return filepath.Join(a.cfg.Data, "projects", d.ProjectID, "builds", d.ID)
}
func (a *App) deployEnv(d Deploy) (map[string]string, error) {
	var raw []byte
	if e := a.store.db.QueryRow("SELECT value FROM deployment_env WHERE deploy_id=?", d.ID).Scan(&raw); e != nil {
		return nil, e
	}
	plain, e := unprotect(raw)
	if e != nil {
		return nil, e
	}
	var values map[string]string
	e = json.Unmarshal(plain, &values)
	return values, e
}
func (a *App) execute(d Deploy) {
	a.jobsGate.RLock()
	defer a.jobsGate.RUnlock()
	ctx, cancel := context.WithTimeout(a.ctx, time.Duration(d.Snapshot.Timeout)*time.Second)
	a.jobsMu.Lock()
	a.active[d.ID] = cancel
	a.jobsMu.Unlock()
	defer func() { cancel(); a.jobsMu.Lock(); delete(a.active, d.ID); a.jobsMu.Unlock() }()
	fresh, e := a.store.deploy(d.ID)
	if e != nil || fresh.State != "queued" {
		return
	}
	d.State = "building"
	d.Started = now()
	if e = a.store.saveDeploy(d); e != nil {
		return
	}
	env, e := a.deployEnv(d)
	if e != nil {
		d.Error = "Could not decrypt the deployment's saved environment"
		d.State = "failed"
		d.Finished = now()
		_ = a.store.saveDeploy(d)
		return
	}
	secrets := []string{}
	for _, v := range env {
		if v != "" {
			secrets = append(secrets, v)
		}
	}
	logger := newRedactedLog(filepath.Join(a.cfg.Data, "logs", d.ID+".log"), secrets)
	defer logger.Close()
	report := func(s string) { fmt.Fprintln(logger, time.Now().UTC().Format("15:04:05"), s) }
	run := func() error {
		report("Starting " + d.Action + " deployment " + d.ID)
		if e = a.runtime.provision(ctx, "caddy", "", report); e != nil {
			return e
		}
		if d.Action == "rollback" || d.Action == "promote" || d.Action == "republish" {
			parent, e := a.store.deploy(d.Parent)
			if e != nil || parent.State != "published" {
				return errors.New("Only a successful retained release can be republished")
			}
			d.Artifact = parent.Artifact
			if d.Artifact == "" {
				d.Artifact = parent.ID
			}
			d.Snapshot = parent.Snapshot
			d.Bytes = parent.Bytes
			d.Commit = parent.Commit
			d.Branch = parent.Branch
			d.SourceHash = parent.SourceHash
			if _, e = os.Stat(a.releaseRoot(d)); e != nil {
				return errors.New("That release artifact is no longer available")
			}
		} else {
			if e = a.fetchSource(ctx, &d, report); e != nil {
				return e
			}
			work := a.workspace(d)
			if e = sandboxDir(work, d.ProjectID); e != nil {
				return e
			}
			if e = os.MkdirAll(filepath.Join(work, "tmp"), 0700); e != nil {
				return e
			}
			report("Extracting verified source (links and traversal paths are rejected)")
			if d.Source == "zip" {
				e = extractZIP(a.sourcePath(d), work, false)
			} else {
				e = extractTar(a.sourcePath(d), work)
			}
			if e != nil {
				return e
			}
			needsNode := d.Snapshot.Adapter != "static" || len(d.Snapshot.Install) > 0 || len(d.Snapshot.Build) > 0
			if needsNode {
				key := fmt.Sprint("node", d.Snapshot.NodeMajor)
				if e = a.runtime.provision(ctx, key, "", report); e != nil {
					return e
				}
				if e = allowRuntime(a.runtime.path(key), d.ProjectID); e != nil {
					return e
				}
				for _, step := range []struct {
					name string
					args []string
				}{{"Install dependencies", d.Snapshot.Install}, {"Build", d.Snapshot.Build}} {
					if len(step.args) == 0 {
						continue
					}
					report(step.name + ": " + encode(step.args))
					if e = a.runBuild(ctx, d, work, step.args, env, logger); e != nil {
						return fmt.Errorf("%s failed: %w", step.name, e)
					}
				}
			}
			if _, e = treeSize(work); e != nil {
				return e
			}
			if d.Snapshot.Adapter == "static" {
				output, e := insideRelease(work, d.Snapshot.Output)
				if e != nil {
					return fmt.Errorf("Build output %s is missing or unsafe", d.Snapshot.Output)
				}
				if _, e = os.Stat(filepath.Join(output, "index.html")); e != nil {
					return errors.New("Static output has no index.html; check the output directory or choose a server adapter for SSR")
				}
			}
			d.Bytes, e = treeSize(work)
			if e != nil {
				return e
			}
			if d.Bytes > int64(d.Snapshot.DiskMB)<<20 {
				return errors.New("Release exceeds its configured disk limit")
			}
			dest := a.releaseRoot(d)
			if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
				return e
			}
			if e = os.Rename(work, dest); e != nil {
				return e
			}
			if e = freezeArtifact(dest, d.ProjectID); e != nil {
				return e
			}
			d.Artifact = d.ID
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		report("Validating release and preparing isolated traffic switch")
		d.State = "publishing"
		if e = a.store.saveDeploy(d); e != nil {
			return e
		}
		if d.Snapshot.Adapter != "static" {
			if e = a.startNode(ctx, &d, env, report); e != nil {
				return e
			}
		}
		a.publishMu.Lock()
		defer a.publishMu.Unlock()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e = a.reloadRoutes(ctx, &d); e != nil {
			return e
		}
		d.State = "published"
		d.Finished = now()
		d.URL = a.deployURL(d)
		p, e := a.store.project(d.ProjectID)
		if e != nil {
			return e
		}
		previous := p.Live
		if d.Environment == "production" {
			p.Live = d.ID
			if e = a.store.saveProject(p); e != nil {
				return e
			}
		}
		if e = a.store.saveDeploy(d); e != nil {
			return e
		}
		_ = a.store.audit("scheduler", d.ProjectID, "deploy.published", map[string]string{"deploy": d.ID, "previous": previous, "action": d.Action, "commit": d.Commit})
		if previous != "" && previous != d.ID && d.Environment == "production" {
			a.stopNode(previous)
		}
		report("Published " + d.URL)
		return nil
	}
	e = run()
	if e != nil {
		a.stopNode(d.ID)
		d.State = "failed"
		d.Error = e.Error()
		if ctx.Err() != nil {
			d.State = "cancelled"
			d.Error = "Deployment cancelled or its time limit expired. The previous healthy release is retained."
		}
		d.Error = logger.Redact(d.Error)
		d.Finished = now()
		_ = a.store.saveDeploy(d)
		_ = a.store.audit("scheduler", d.ProjectID, "deploy."+d.State, map[string]string{"deploy": d.ID, "error": d.Error})
		report(d.Error)
	}
}
func (a *App) runBuild(ctx context.Context, d Deploy, work string, args []string, env map[string]string, out io.Writer) error {
	key := fmt.Sprint("node", d.Snapshot.NodeMajor)
	runtime := a.runtime.path(key)
	argv := append([]string(nil), args...)
	if argv[0] == "npm" {
		argv = append([]string{filepath.Join(runtime, "node_modules", "npm", "bin", "npm-cli.js")}, argv[1:]...)
	} else {
		argv = argv[1:]
	}
	cmd := exec.Command(filepath.Join(runtime, "node.exe"), argv...)
	cmd.Dir = work
	cmd.Env = safeEnvironment(work, runtime, 0, env)
	cmd.Env = append(cmd.Env, "NODE_ENV=development")
	cmd.Stdout = out
	cmd.Stderr = out
	j, e := startProcess(cmd, d.ProjectID, d.Snapshot.MemoryMB, true)
	if e != nil {
		return e
	}
	defer j.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case e = <-done:
			return e
		case <-ctx.Done():
			j.Close()
			<-done
			return ctx.Err()
		case <-timer.C:
			bytes, e := treeSize(work)
			if e != nil || bytes > int64(d.Snapshot.DiskMB)<<20 {
				j.Close()
				<-done
				return errors.New("Build exceeded disk limits or created an unsafe file/link")
			}
		}
	}
}

type redactedLog struct {
	mu      sync.Mutex
	file    *os.File
	values  []string
	pending string
	tail    int
	bytes   int
	closed  bool
}

func newRedactedLog(path string, values []string) *redactedLog {
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	tail := 0
	for _, v := range values {
		if len(v) > tail {
			tail = len(v)
		}
	}
	return &redactedLog{file: f, values: values, tail: tail}
}
func (l *redactedLog) Redact(s string) string {
	for _, v := range l.values {
		if v != "" {
			s = strings.ReplaceAll(s, v, "[redacted]")
		}
	}
	return s
}
func (l *redactedLog) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := len(b)
	if l.closed {
		return n, nil
	}
	l.pending += string(b)
	if len(l.pending) > l.tail {
		cut := len(l.pending) - l.tail
		if l.tail > 0 {
			for _, v := range l.values {
				if v == "" {
					continue
				}
				for start := 0; start < len(l.pending); {
					pos := strings.Index(l.pending[start:], v)
					if pos < 0 {
						break
					}
					pos += start
					if pos < cut && pos+len(v) > cut {
						cut = pos
					}
					start = pos + 1
				}
			}
		}
		if cut > 0 {
			l.write(l.Redact(l.pending[:cut]))
			l.pending = l.pending[cut:]
		}
	}
	return n, nil
}
func (l *redactedLog) write(s string) {
	if l.file != nil && l.bytes < 10<<20 {
		if len(s)+l.bytes > 10<<20 {
			s = s[:(10<<20)-l.bytes] + "\nLog limit reached; remaining output discarded.\n"
		}
		_, _ = l.file.WriteString(s)
		l.bytes += len(s)
	}
}
func (l *redactedLog) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.closed {
		l.write(l.Redact(l.pending))
		l.pending = ""
		l.closed = true
		if l.file != nil {
			l.file.Close()
		}
	}
}
func (a *App) deployAPI(w http.ResponseWriter, r *http.Request, s Session, p Project, tail []string) {
	if len(tail) == 0 && r.Method == "GET" {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if offset < 0 {
			offset = 0
		}
		ds, e := a.store.history(p.ID, 30, offset)
		if e != nil {
			fail(w, 500, dbError(e))
			return
		}
		jsonOut(w, 200, map[string]any{"deployments": ds, "offset": offset, "pageSize": 30})
		return
	}
	if len(tail) == 0 && r.Method == "POST" {
		if rank(p.Role) < 2 {
			fail(w, 403, errors.New("Developer access is required to deploy"))
			return
		}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/zip") {
			d, e := a.queued(p, "zip", p.Branch, "", environmentQuery(r), "build", "", "", s.User.ID)
			if e != nil {
				fail(w, 400, e)
				return
			}
			dest := a.sourcePath(d)
			r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
			f, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e == nil {
				_, e = io.Copy(f, r.Body)
				f.Close()
			}
			if e != nil {
				d.State = "failed"
				d.Error = "ZIP upload failed or exceeded 64 MiB"
				d.Finished = now()
				_ = a.store.saveDeploy(d)
				fail(w, 400, errors.New(d.Error))
				return
			}
			d.SourceHash, e = hashFile(dest)
			d.State = "queued"
			if e != nil {
				fail(w, 500, dbError(e))
				return
			}
			_ = a.store.saveDeploy(d)
			a.notify()
			jsonOut(w, 202, d)
			return
		}
		var in struct {
			Branch      string `json:"branch"`
			Commit      string `json:"commit"`
			Environment string `json:"environment"`
			ClearCache  bool   `json:"clearCache"`
		}
		if !readJSON(w, r, &in) {
			return
		}
		if in.Environment == "" {
			in.Environment = "production"
		}
		key := r.Header.Get("Idempotency-Key")
		if len(key) > 120 {
			fail(w, 400, errors.New("Idempotency key is too long"))
			return
		}
		if in.ClearCache {
			if !requireAdmin(w, s) {
				return
			}
			if e := a.clearBuildCache(p.ID); e != nil {
				fail(w, 409, e)
				return
			}
		}
		d, e := a.queueGit(r.Context(), p, in.Branch, in.Commit, in.Environment, "build", key, s.User.ID)
		if e != nil {
			fail(w, 400, e)
			return
		}
		jsonOut(w, 202, d)
		return
	}
	if len(tail) < 1 || !validID.MatchString(tail[0]) {
		http.NotFound(w, r)
		return
	}
	d, e := a.store.deploy(tail[0])
	if e != nil || d.ProjectID != p.ID {
		fail(w, 404, errors.New("Deployment not found"))
		return
	}
	if len(tail) == 1 && r.Method == "GET" {
		jsonOut(w, 200, d)
		return
	}
	if len(tail) != 2 {
		http.NotFound(w, r)
		return
	}
	action := tail[1]
	if r.Method == "GET" && (action == "logs" || action == "events") {
		a.logAPI(w, r, s, d, action)
		return
	}
	if r.Method != "POST" || rank(p.Role) < 2 {
		fail(w, 403, errors.New("Developer access is required for this action"))
		return
	}
	if action == "cancel" {
		a.jobsMu.Lock()
		cancel := a.active[d.ID]
		if cancel != nil {
			cancel()
		}
		a.jobsMu.Unlock()
		if d.State == "queued" {
			d.State = "cancelled"
			d.Finished = now()
			d.Error = "Cancelled before the build started"
			_ = a.store.saveDeploy(d)
		} else if cancel == nil {
			fail(w, 409, errors.New("This deployment has already finished"))
			return
		}
		_ = a.store.audit(s.User.ID, p.ID, "deploy.cancel", map[string]string{"deploy": d.ID})
		jsonOut(w, 202, map[string]string{"state": "cancelling"})
		return
	}
	if action == "rollback" || action == "promote" || action == "republish" {
		if action == "promote" && d.Environment != "preview" {
			fail(w, 400, errors.New("Only a preview release can be promoted"))
			return
		}
		if action == "rollback" && rank(p.Role) < 3 {
			fail(w, 403, errors.New("An administrator must approve production rollback"))
			return
		}
		if d.State != "published" {
			fail(w, 409, errors.New("Choose a successful retained release"))
			return
		}
		next, e := a.queued(d.Snapshot, d.Source, d.Branch, d.Commit, "production", action, d.ID, "", s.User.ID)
		if e != nil {
			fail(w, 400, e)
			return
		}
		a.notify()
		jsonOut(w, 202, next)
		return
	}
	if action == "retry" || action == "rebuild" {
		if action == "retry" && d.State != "failed" && d.State != "cancelled" {
			fail(w, 400, errors.New("Retry applies to failed or cancelled deployments"))
			return
		}
		snapshot := d.Snapshot
		if action == "rebuild" {
			snapshot = p
		}
		next, e := a.queued(snapshot, d.Source, d.Branch, d.Commit, d.Environment, "build", d.ID, "", s.User.ID)
		if e != nil {
			fail(w, 400, e)
			return
		}
		if d.Source == "zip" {
			in, e := os.Open(a.sourcePath(d))
			if e != nil {
				fail(w, 409, errors.New("The source ZIP is no longer retained; upload it again"))
				return
			}
			out, e := os.Create(a.sourcePath(next))
			if e != nil {
				in.Close()
				fail(w, 500, dbError(e))
				return
			}
			_, e = io.Copy(out, in)
			out.Close()
			in.Close()
			if e != nil {
				fail(w, 500, dbError(e))
				return
			}
			next.SourceHash = d.SourceHash
			next.State = "queued"
			_ = a.store.saveDeploy(next)
		}
		a.notify()
		jsonOut(w, 202, next)
		return
	}
	http.NotFound(w, r)
}
func environmentQuery(r *http.Request) string {
	if r.URL.Query().Get("environment") == "preview" {
		return "preview"
	}
	return "production"
}
func (a *App) clearBuildCache(pid string) error {
	a.jobsMu.Lock()
	busy := len(a.active) > 0
	a.jobsMu.Unlock()
	if busy {
		return errors.New("Wait for running builds before clearing cache")
	}
	path := filepath.Join(a.cfg.Data, "projects", pid, "var", ".npm-cache")
	if _, e := os.Stat(path); os.IsNotExist(e) {
		return nil
	}
	if _, e := treeSize(path); e != nil {
		return e
	}
	return os.RemoveAll(path)
}
func (a *App) logAPI(w http.ResponseWriter, r *http.Request, s Session, d Deploy, action string) {
	path := filepath.Join(a.cfg.Data, "logs", d.ID+".log")
	if action == "logs" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=deploy-"+d.ID+".log")
		w.Header().Set("Cache-Control", "no-store")
		f, e := os.Open(path)
		if e != nil {
			w.Write([]byte("No log output has been written yet.\n"))
			return
		}
		defer f.Close()
		io.Copy(w, f)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, errors.New("Streaming is unavailable"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	offset := int64(0)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		if _, ok := a.session(r); !ok {
			return
		}
		current, e := a.store.deploy(d.ID)
		if e != nil {
			return
		}
		chunk := ""
		if f, e := os.Open(path); e == nil {
			_, _ = f.Seek(offset, 0)
			b, _ := io.ReadAll(io.LimitReader(f, 256<<10))
			offset += int64(len(b))
			chunk = string(b)
			f.Close()
		}
		fmt.Fprintf(w, "data: %s\n\n", encode(map[string]any{"deploy": current, "text": chunk, "offset": offset}))
		flusher.Flush()
		if current.State != "queued" && current.State != "building" && current.State != "publishing" {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-a.ctx.Done():
			return
		case <-tick.C:
		}
	}
}
