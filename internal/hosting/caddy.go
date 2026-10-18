package hosting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

func (a *App) siteHost(d Deploy) string {
	s := a.settings()
	suffix := "localhost"
	if s.PreviewDomain != "" {
		suffix = s.PreviewDomain
	}
	name := d.Snapshot.Slug
	if d.Environment == "preview" {
		name = d.ID[:12] + "--" + name
	}
	return name + "." + suffix
}
func (a *App) deployURL(d Deploy) string {
	s := a.settings()
	if s.PreviewDomain != "" && s.Public {
		return "https://" + a.siteHost(d)
	}
	return fmt.Sprintf("http://%s:%d", a.siteHost(d), s.CaddyPort)
}
func (a *App) caddyState() map[string]any {
	a.processMu.Lock()
	defer a.processMu.Unlock()
	state := "not running"
	if a.caddy != nil {
		state = "running"
	}
	return map[string]any{"state": state, "error": a.caddyError, "adminAPI": "disabled", "scope": "One owned instance; external Caddy instances are never adopted"}
}
func (a *App) route(d Deploy, hosts []string) map[string]any {
	handlers := []any{map[string]any{"handler": "headers", "response": map[string]any{"set": map[string][]string{"X-MyHosting-Deploy": {d.ID}, "X-Content-Type-Options": {"nosniff"}}}}}
	if d.Snapshot.Adapter == "static" {
		root := filepath.Join(a.releaseRoot(d), filepath.FromSlash(d.Snapshot.Output))
		handlers = append(handlers, map[string]any{"handler": "vars", "root": root}, map[string]any{"handler": "file_server", "hide": []string{".git", ".env", ".env.*"}})
	} else {
		handlers = append(handlers, map[string]any{"handler": "reverse_proxy", "upstreams": []any{map[string]any{"dial": fmt.Sprintf("127.0.0.1:%d", d.Port)}}, "headers": map[string]any{"request": map[string]any{"set": map[string][]string{"X-Forwarded-Proto": {"{http.request.scheme}"}}}}})
	}
	return map[string]any{"match": []any{map[string]any{"host": hosts}}, "handle": handlers, "terminal": true}
}
func (a *App) routing(incoming *Deploy) (map[string]any, error) {
	s := a.settings()
	rows, e := a.store.db.Query("SELECT detail FROM deployments WHERE state='published'")
	if e != nil {
		return nil, e
	}
	var ds []Deploy
	for rows.Next() {
		var raw string
		var d Deploy
		if e = rows.Scan(&raw); e != nil {
			rows.Close()
			return nil, e
		}
		if e = json.Unmarshal([]byte(raw), &d); e != nil {
			rows.Close()
			return nil, e
		}
		ds = append(ds, d)
	}
	rows.Close()
	local := []any{}
	public := []any{}
	for _, d := range ds {
		p, e := a.store.project(d.ProjectID)
		if e != nil {
			continue
		}
		if d.Environment == "production" && (d.ID != p.Live || (incoming != nil && incoming.ProjectID == p.ID && incoming.Environment == "production")) {
			continue
		}
		if incoming != nil && incoming.ID == d.ID {
			continue
		}
		local = append(local, a.route(d, []string{d.Snapshot.SlugHostLocal(d)}))
		if s.Public && s.PreviewDomain != "" {
			public = append(public, a.route(d, []string{a.siteHost(d)}))
		}
		if d.Environment == "production" && s.Public {
			hosts, e := a.verifiedDomains(p.ID)
			if e != nil {
				return nil, e
			}
			if len(hosts) > 0 {
				public = append(public, a.route(d, hosts))
			}
		}
	}
	if incoming != nil {
		d := *incoming
		local = append(local, a.route(d, []string{d.Snapshot.SlugHostLocal(d)}))
		if s.Public && s.PreviewDomain != "" {
			public = append(public, a.route(d, []string{a.siteHost(d)}))
		}
		if d.Environment == "production" && s.Public {
			hosts, e := a.verifiedDomains(d.ProjectID)
			if e != nil {
				return nil, e
			}
			if len(hosts) > 0 {
				public = append(public, a.route(d, hosts))
			}
		}
	}
	local = append(local, map[string]any{"handle": []any{map[string]any{"handler": "static_response", "status_code": 404, "body": "No published My Hosting site matches this hostname."}}})
	servers := map[string]any{"local": map[string]any{"listen": []string{"127.0.0.1:" + strconv.Itoa(s.CaddyPort)}, "routes": local, "automatic_https": map[string]any{"disable": true}, "logs": map[string]any{"default_logger_name": "access"}}}
	if s.Public && len(public) > 0 {
		servers["public"] = map[string]any{"listen": []string{":443"}, "routes": public, "tls_connection_policies": []any{map[string]any{}}, "logs": map[string]any{"default_logger_name": "access"}}
	}
	return map[string]any{"admin": map[string]any{"disabled": true}, "storage": map[string]any{"module": "file_system", "root": filepath.Join(a.cfg.Data, "caddy", "state")}, "logging": map[string]any{"logs": map[string]any{"access": map[string]any{"include": []string{"http.log.access"}, "writer": map[string]any{"output": "file", "filename": filepath.Join(a.cfg.Data, "logs", "access.json"), "roll_size_mb": 10, "roll_keep": 3}}}}, "apps": map[string]any{"http": map[string]any{"servers": servers}}}, nil
}
func (p Project) SlugHostLocal(d Deploy) string {
	host := p.Slug
	if d.Environment == "preview" {
		host = d.ID[:12] + "--" + host
	}
	return host + ".localhost"
}
func (a *App) reloadRoutes(ctx context.Context, incoming *Deploy) error {
	if e := a.runtime.ready("caddy"); e != nil {
		return e
	}
	if e := a.restoreProcesses(ctx); e != nil {
		return e
	}
	config, e := a.routing(incoming)
	if e != nil {
		return e
	}
	body := []byte(encode(config))
	path := filepath.Join(a.cfg.Data, "caddy", "caddy.json")
	temp := filepath.Join(a.cfg.Data, "caddy", "validate-"+id()+".json")
	if e = os.WriteFile(temp, body, 0600); e != nil {
		return e
	}
	defer os.Remove(temp)
	runtime := a.runtime.path("caddy")
	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(checkCtx, filepath.Join(runtime, "caddy.exe"), "validate", "--config", temp)
	cmd.Env = []string{"SystemRoot=" + os.Getenv("SystemRoot"), "APPDATA=" + filepath.Join(a.cfg.Data, "caddy"), "LOCALAPPDATA=" + filepath.Join(a.cfg.Data, "caddy"), "TEMP=" + filepath.Join(a.cfg.Data, "cache")}
	if e = cmd.Run(); e != nil {
		return errors.New("Caddy rejected the generated routing configuration; the previous config is retained")
	}
	old, _ := os.ReadFile(path)
	if e = atomicFile(path, body); e != nil {
		return e
	}
	a.processMu.Lock()
	running := a.caddy != nil
	a.processMu.Unlock()
	if !running {
		ln, e := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", a.settings().CaddyPort))
		if e != nil {
			_ = atomicFile(path, old)
			return errors.New("Site port is in use by another application. Choose a free port in Settings; external servers are never adopted")
		}
		ln.Close()
		log, e := os.OpenFile(filepath.Join(a.cfg.Data, "logs", "caddy.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		run := exec.Command(filepath.Join(runtime, "caddy.exe"), "run", "--config", path, "--watch")
		run.Env = cmd.Env
		run.Stdout = log
		run.Stderr = log
		j, e := startProcess(run, "", 0, false)
		if e != nil {
			log.Close()
			return e
		}
		a.processMu.Lock()
		a.caddy = j
		a.caddyError = ""
		a.processMu.Unlock()
		go func() {
			e := run.Wait()
			j.Close()
			log.Close()
			a.processMu.Lock()
			if a.caddy == j {
				a.caddy = nil
				if e != nil {
					a.caddyError = "Caddy stopped; check its process log and port permissions"
				}
			}
			a.processMu.Unlock()
		}()
	}
	if incoming != nil {
		probeCtx, probeCancel := context.WithTimeout(ctx, 20*time.Second)
		defer probeCancel()
		host := incoming.Snapshot.SlugHostLocal(*incoming)
		e = waitHealth(probeCtx, fmt.Sprintf("http://127.0.0.1:%d%s", a.settings().CaddyPort, incoming.Snapshot.HealthPath), host, incoming.ID)
		if e != nil {
			if len(old) > 0 {
				_ = atomicFile(path, old)
			}
			return errors.New("The new route did not pass its health probe. Previous routes were restored; inspect Caddy and process logs")
		}
	}
	return nil
}
