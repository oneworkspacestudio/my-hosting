package hosting

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

func (a *App) siteHost(d Deploy) string {
	s := a.settings()
	suffix := "localhost"
	if s.PreviewDomain != "" {
		suffix = s.PreviewDomain
	}
	name := d.Snapshot.Slug
	if d.Environment == "preview" {
		name = previewLabel(d.ID, name)
	}
	if s.AddressStyle == "flat" {
		name += "-sites"
	}
	return name + "." + suffix
}
func (a *App) deployURL(d Deploy) string {
	s := a.settings()
	if s.PreviewDomain != "" && s.Public {
		return "https://" + a.siteHost(d)
	}
	return fmt.Sprintf("http://%s:%d", d.Snapshot.SlugHostLocal(d), s.CaddyPort)
}
func (a *App) caddyState() map[string]any {
	a.processMu.Lock()
	defer a.processMu.Unlock()
	state := "not running"
	if a.caddy != nil {
		state = "running"
	}
	return map[string]any{"state": state, "error": a.caddyError, "adminAPI": "mutual TLS on loopback; unauthenticated admin disabled", "scope": "One owned instance; external Caddy instances are never adopted"}
}
func (a *App) route(d Deploy, hosts []string) map[string]any {
	handlers := []any{map[string]any{"handler": "headers", "response": map[string]any{"set": map[string][]string{"X-MyHosting-Deploy": {d.ID}, "X-Content-Type-Options": {"nosniff"}}}}}
	if d.Snapshot.Adapter == "static" {
		root := filepath.Join(a.releaseRoot(d), filepath.FromSlash(sourceBase(d.Snapshot)), filepath.FromSlash(d.Snapshot.Output))
		handlers = append(handlers, map[string]any{"handler": "vars", "root": root}, map[string]any{"handler": "file_server", "hide": []string{".git", ".env", ".env.*"}})
	} else {
		handlers = append(handlers, map[string]any{"handler": "reverse_proxy", "upstreams": []any{map[string]any{"dial": fmt.Sprintf("127.0.0.1:%d", d.Port)}}, "headers": map[string]any{"request": map[string]any{"set": map[string][]string{"X-Forwarded-Proto": {"{http.request.scheme}"}}}}})
	}
	return map[string]any{"match": []any{map[string]any{"host": hosts}}, "handle": handlers, "terminal": true}
}
func (a *App) routing(incoming *Deploy) (map[string]any, error) {
	s := a.settings()
	admin, e := a.adminConfig()
	if e != nil {
		return nil, e
	}
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
	revision := id()
	local := []any{map[string]any{"match": []any{map[string]any{"host": []string{"control.myhosting.localhost"}, "path": []string{"/__myhosting_health"}}}, "handle": []any{map[string]any{"handler": "headers", "response": map[string]any{"set": map[string][]string{"X-MyHosting-Deploy": {revision}}}}, map[string]any{"handler": "static_response", "status_code": 200, "body": "ready"}}, "terminal": true}}
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
	if s.Public && s.Transport == "tunnel" {
		local = append(local, public...)
	}
	local = append(local, map[string]any{"handle": []any{map[string]any{"handler": "static_response", "status_code": 404, "body": "No published My Hosting site matches this hostname."}}})
	servers := map[string]any{"local": map[string]any{"listen": []string{"127.0.0.1:" + strconv.Itoa(s.CaddyPort)}, "routes": local, "automatic_https": map[string]any{"disable": true}, "logs": map[string]any{"default_logger_name": "access"}}}
	if s.Public && s.Transport != "tunnel" && len(public) > 0 {
		servers["public"] = map[string]any{"listen": []string{":443"}, "routes": public, "tls_connection_policies": []any{map[string]any{}}, "logs": map[string]any{"default_logger_name": "access"}}
	}
	return map[string]any{"admin": admin, "storage": map[string]any{"module": "file_system", "root": filepath.Join(a.cfg.Data, "caddy", "state")}, "logging": map[string]any{"logs": map[string]any{"access": map[string]any{"include": []string{"http.log.access"}, "writer": map[string]any{"output": "file", "filename": filepath.Join(a.cfg.Data, "logs", "access.json"), "roll_size_mb": 10, "roll_keep": 3}}}}, "apps": map[string]any{"pki": map[string]any{"certificate_authorities": map[string]any{"hosting-control": map[string]any{"install_trust": false}}}, "http": map[string]any{"servers": servers}}}, nil
}
func (p Project) SlugHostLocal(d Deploy) string {
	host := p.Slug
	if d.Environment == "preview" {
		host = previewLabel(d.ID, host)
	}
	return host + ".localhost"
}
func previewLabel(deployID, slug string) string {
	if len(deployID) > 10 {
		deployID = deployID[:10]
	}
	if len(slug) > 37 {
		slug = slug[:37]
	}
	slug = strings.TrimRight(slug, "-")
	return "deploy-" + deployID + "--" + slug
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
	return a.applyCaddyConfig(ctx, config)
}

func routingRevision(config map[string]any) string {
	return config["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)["local"].(map[string]any)["routes"].([]any)[0].(map[string]any)["handle"].([]any)[0].(map[string]any)["response"].(map[string]any)["set"].(map[string][]string)["X-MyHosting-Deploy"][0]
}
