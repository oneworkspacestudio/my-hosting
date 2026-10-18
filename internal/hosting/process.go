package hosting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type managedProcess struct {
	mu                  sync.Mutex
	cancel              context.CancelFunc
	done                chan struct{}
	job                 *ProcessJob
	pid, port, restarts int
	state, lastError    string
	started             time.Time
}

func (p *managedProcess) summary() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return map[string]any{"pid": p.pid, "port": p.port, "state": p.state, "restarts": p.restarts, "error": p.lastError, "started": p.started.UTC().Format(time.RFC3339Nano), "limits": "Windows Job Object: per-project RAM limit, 25% CPU hard cap, 32 descendant processes. Disk quota is monitored during builds."}
}
func freePort() (int, error) {
	ln, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return 0, e
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port, nil
}
func (a *App) startNode(ctx context.Context, d *Deploy, env map[string]string, report func(string)) error {
	key := fmt.Sprint("node", d.Snapshot.NodeMajor)
	if e := a.runtime.provision(ctx, key, "", report); e != nil {
		return e
	}
	runtime := a.runtime.path(key)
	if e := allowRuntime(runtime, d.ProjectID); e != nil {
		return e
	}
	if e := allowProjectAncestors(a.cfg.Data, runtime, d.ProjectID); e != nil {
		return e
	}
	root := a.releaseRoot(*d)
	if e := freezeArtifact(root, d.ProjectID); e != nil {
		return e
	}
	cwd, e := insideRelease(root, sourceBase(d.Snapshot))
	if e != nil {
		return e
	}
	if e = allowProjectAncestors(a.cfg.Data, cwd, d.ProjectID); e != nil {
		return e
	}
	args := append([]string(nil), d.Snapshot.Start...)
	switch d.Snapshot.Adapter {
	case "next":
		cwd = filepath.Join(root, filepath.FromSlash(sourceBase(d.Snapshot)), ".next", "standalone")
		if _, e := insideRelease(root, pathFor(d.Snapshot, ".next/standalone/server.js")); e != nil {
			return errors.New("Next.js needs output:'standalone' and a successful build; .next/standalone/server.js is missing")
		}
		args = []string{"node", "server.js"}
	case "nuxt":
		if _, e := insideRelease(root, pathFor(d.Snapshot, ".output/server/index.mjs")); e != nil {
			return errors.New("Nuxt needs the Nitro node-server preset; .output/server/index.mjs is missing")
		}
		args = []string{"node", ".output/server/index.mjs"}
	}
	if len(args) < 2 {
		return errors.New("Configure a start command such as [\"node\",\"server.js\"] or [\"npm\",\"run\",\"start\"]")
	}
	if args[0] == "npm" {
		args = append([]string{filepath.Join(runtime, "node_modules", "npm", "bin", "npm-cli.js")}, args[1:]...)
	} else if args[0] == "node" {
		args = args[1:]
	} else {
		return errors.New("Only managed node/npm start commands are accepted")
	}
	variable := filepath.Join(a.cfg.Data, "projects", d.ProjectID, "var")
	if e := sandboxDir(variable, d.ProjectID); e != nil {
		return e
	}
	if e := os.MkdirAll(filepath.Join(variable, "tmp"), 0700); e != nil {
		return e
	}
	port, e := freePort()
	if e != nil {
		return e
	}
	d.Port = port
	values := []string{}
	for _, v := range env {
		if v != "" {
			values = append(values, v)
		}
	}
	logger := newRedactedLog(filepath.Join(a.cfg.Data, "logs", d.ID+"-process.log"), values)
	processCtx, cancel := context.WithCancel(a.ctx)
	p := &managedProcess{cancel: cancel, done: make(chan struct{}), port: port, state: "starting", started: time.Now()}
	started := make(chan error, 1)
	a.processMu.Lock()
	if old := a.processes[d.ID]; old != nil {
		old.cancel()
	}
	a.processes[d.ID] = p
	a.processMu.Unlock()
	did, pid := d.ID, d.ProjectID
	go func() {
		defer close(p.done)
		defer logger.Close()
		failures := 0
		backoff := time.Second
		first := true
		for {
			if processCtx.Err() != nil {
				return
			}
			cmd := exec.Command(filepath.Join(runtime, "node.exe"), args...)
			cmd.Dir = cwd
			cmd.Env = append(safeEnvironment(variable, runtime, port, env), "MYHOSTING_DATA_DIR="+variable)
			cmd.Stdout = logger
			cmd.Stderr = logger
			j, e := startProcess(cmd, pid, d.Snapshot.MemoryMB, true)
			if e != nil {
				if first {
					started <- e
				}
				p.mu.Lock()
				p.state = "failed"
				p.lastError = e.Error()
				p.mu.Unlock()
				return
			}
			p.mu.Lock()
			p.job = j
			p.pid = j.PID()
			p.state = "running"
			p.mu.Unlock()
			if first {
				started <- nil
				first = false
			}
			wait := make(chan error, 1)
			go func() { wait <- j.Wait() }()
			select {
			case <-processCtx.Done():
				j.Close()
				<-wait
				p.mu.Lock()
				p.state = "stopped"
				p.mu.Unlock()
				return
			case e = <-wait:
				j.Close()
			}
			failures++
			p.mu.Lock()
			p.restarts = failures
			p.state = "restarting"
			p.lastError = "Process exited; restarting with backoff"
			if e != nil {
				p.lastError = logger.Redact(e.Error())
			}
			p.mu.Unlock()
			_ = a.store.audit("supervisor", pid, "process.exited", map[string]any{"deploy": did, "attempt": failures})
			if failures >= 5 {
				p.mu.Lock()
				p.state = "crash-loop"
				p.lastError = "Stopped after five exits. Inspect process logs, fix the start command, then restart."
				p.mu.Unlock()
				return
			}
			select {
			case <-processCtx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
		}
	}()
	select {
	case e = <-started:
		if e != nil {
			cancel()
			<-p.done
			return e
		}
	case <-ctx.Done():
		cancel()
		<-p.done
		return ctx.Err()
	}
	healthCtx, healthCancel := context.WithTimeout(ctx, 45*time.Second)
	defer healthCancel()
	report(fmt.Sprintf("Waiting for Node health at 127.0.0.1:%d%s", port, d.Snapshot.HealthPath))
	if e = waitHealth(healthCtx, fmt.Sprintf("http://127.0.0.1:%d%s", port, d.Snapshot.HealthPath), "", ""); e != nil {
		a.stopNode(d.ID)
		return errors.New("Node did not pass its health check. Confirm PORT/HOST handling, start command, output files and process logs")
	}
	return nil
}
func (a *App) stopNode(did string) {
	a.processMu.Lock()
	p := a.processes[did]
	delete(a.processes, did)
	a.processMu.Unlock()
	if p != nil {
		p.cancel()
		select {
		case <-p.done:
		case <-time.After(8 * time.Second):
			p.mu.Lock()
			if p.job != nil {
				p.job.Close()
			}
			p.mu.Unlock()
		}
	}
}
func waitHealth(ctx context.Context, raw, host, deploy string) error {
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	t := time.NewTicker(300 * time.Millisecond)
	defer t.Stop()
	for {
		req, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
		if e != nil {
			return e
		}
		if host != "" {
			req.Host = host
		}
		resp, e := client.Do(req)
		if e == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 && (deploy == "" || resp.Header.Get("X-MyHosting-Deploy") == deploy) {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}
func (a *App) supervise() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
		}
		a.superviseRouting()
	}
}
func (a *App) superviseRouting() {
	if !a.jobsGate.TryRLock() {
		return
	}
	defer a.jobsGate.RUnlock()
	if a.runtime.ready("caddy") != nil {
		return
	}
	a.processMu.Lock()
	caddy := a.caddy
	a.processMu.Unlock()
	if caddy == nil {
		a.publishMu.Lock()
		defer a.publishMu.Unlock()
		_ = a.reloadRoutes(a.ctx, nil)
	}
}
func (a *App) restoreProcesses(ctx context.Context) error {
	rows, e := a.store.db.Query("SELECT detail FROM deployments WHERE state='published'")
	if e != nil {
		return e
	}
	var ds []Deploy
	for rows.Next() {
		var raw string
		var d Deploy
		_ = rows.Scan(&raw)
		if json.Unmarshal([]byte(raw), &d) == nil {
			ds = append(ds, d)
		}
	}
	rows.Close()
	for _, d := range ds {
		p, e := a.store.project(d.ProjectID)
		if e != nil {
			continue
		}
		if d.Environment != "preview" && d.ID != p.Live {
			continue
		}
		if d.Snapshot.Adapter == "static" {
			continue
		}
		var desired string
		_ = a.store.setting("process:"+d.ID, &desired)
		if desired == "stopped" {
			continue
		}
		a.processMu.Lock()
		existing := a.processes[d.ID]
		a.processMu.Unlock()
		if existing != nil {
			continue
		}
		env, e := a.deployEnv(d)
		if e != nil {
			return e
		}
		if e = a.startNode(ctx, &d, env, func(string) {}); e != nil {
			return e
		}
		if e = a.store.saveDeploy(d); e != nil {
			return e
		}
	}
	return nil
}
func (a *App) processAPI(w http.ResponseWriter, r *http.Request, s Session, p Project) {
	if strings.HasSuffix(r.URL.Path, "/process/logs") && r.Method == "GET" {
		if p.Live == "" {
			fail(w, 404, errors.New("No production release is running"))
			return
		}
		path := filepath.Join(a.cfg.Data, "logs", p.Live+"-process.log")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		f, e := os.Open(path)
		if e != nil {
			w.Write([]byte("No process output yet. Static sites do not run a Node process.\n"))
			return
		}
		defer f.Close()
		io.Copy(w, io.LimitReader(f, 10<<20))
		return
	}
	if r.Method == "GET" {
		a.processMu.Lock()
		proc := a.processes[p.Live]
		a.processMu.Unlock()
		if proc == nil {
			jsonOut(w, 200, map[string]string{"state": "stopped-or-static", "message": "Static sites have no Node process. A stopped server release can be started here."})
			return
		}
		jsonOut(w, 200, proc.summary())
		return
	}
	if r.Method != "POST" || rank(p.Role) < 2 {
		fail(w, 403, errors.New("Developer access is required"))
		return
	}
	var in struct {
		Action string `json:"action"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if p.Live == "" {
		fail(w, 409, errors.New("Publish a production release first"))
		return
	}
	d, e := a.store.deploy(p.Live)
	if e != nil || d.Snapshot.Adapter == "static" {
		fail(w, 409, errors.New("This project has no managed Node release"))
		return
	}
	if in.Action != "start" && in.Action != "stop" && in.Action != "restart" {
		fail(w, 400, errors.New("Choose start, stop or restart"))
		return
	}
	a.publishMu.Lock()
	defer a.publishMu.Unlock()
	if in.Action != "start" {
		a.stopNode(d.ID)
	}
	if in.Action == "stop" {
		_ = a.store.setSetting("process:"+d.ID, "stopped")
	} else {
		_ = a.store.setSetting("process:"+d.ID, "running")
	}
	if in.Action != "stop" {
		env, e := a.deployEnv(d)
		if e != nil {
			fail(w, 500, errors.New("Could not decrypt this release's environment"))
			return
		}
		if e = a.startNode(r.Context(), &d, env, func(string) {}); e != nil {
			fail(w, 409, e)
			return
		}
		_ = a.store.saveDeploy(d)
	}
	_ = a.reloadRoutes(r.Context(), nil)
	_ = a.store.audit(s.User.ID, p.ID, "process."+in.Action, map[string]string{"deploy": d.ID})
	jsonOut(w, 200, map[string]string{"state": in.Action + " requested"})
}
