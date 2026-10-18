package hosting

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloudflareSiteSetupPreservesOtherRoutes(t *testing.T) {
	a := testApp(t)
	s := a.settings()
	s.Public = true
	s.Transport = "tunnel"
	s.AddressStyle = "flat"
	s.PreviewDomain = "example.test"
	_ = a.store.setSetting("hosting", s)
	a.cfg.SiteCFAccount = strings.Repeat("a", 32)
	a.cfg.SiteCFZone = strings.Repeat("b", 32)
	a.cfg.SiteCFTunnel = "00000000-0000-0000-0000-000000000001"
	a.cfg.SiteCFToken = "synthetic-test-token"
	p := testProject(t, a)
	d := Deploy{ID: id(), ProjectID: p.ID, Snapshot: p, Environment: "production"}
	original := []any{map[string]any{"hostname": "workspace.example.test", "service": "http://127.0.0.1:8990"}, map[string]any{"service": "http_status:404"}}
	config := map[string]any{"ingress": original, "warp-routing": map[string]any{"enabled": true}}
	conflict := false
	updates, creates := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-test-token" {
			t.Error("Wrong provider authentication")
		}
		var out any
		switch {
		case r.Method == "GET" && r.URL.Path == "/zones/"+a.cfg.SiteCFZone:
			out = map[string]any{"name": "example.test", "account": map[string]string{"id": a.cfg.SiteCFAccount}}
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/dns_records"):
			if conflict {
				out = []any{map[string]any{"type": "A", "name": a.siteHost(d), "content": "192.0.2.1", "proxied": false}}
			} else {
				out = []any{}
			}
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/configurations"):
			out = map[string]any{"config": config}
		case r.Method == "PUT" && strings.HasSuffix(r.URL.Path, "/configurations"):
			var body struct{ Config map[string]any }
			_ = json.NewDecoder(r.Body).Decode(&body)
			config = body.Config
			updates++
			out = map[string]bool{"ok": true}
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/dns_records"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["name"] != a.siteHost(d) || body["type"] != "CNAME" || body["proxied"] != true {
				t.Error("Unexpected site DNS mutation")
			}
			creates++
			out = map[string]string{"id": "synthetic-record"}
		default:
			t.Error("Unexpected provider request", r.Method, r.URL.Path)
			http.Error(w, "unexpected", 500)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": out})
	}))
	defer server.Close()
	old := cfEndpoint
	cfEndpoint = server.URL
	defer func() { cfEndpoint = old }()
	if e := a.ensureHostedRoute(a.ctx, d); e != nil {
		t.Fatal(e)
	}
	routes := config["ingress"].([]any)
	if len(routes) != 3 || encode(routes[0]) != encode(original[0]) || encode(routes[2]) != encode(original[1]) || config["warp-routing"] == nil {
		t.Fatal("Existing tunnel configuration was replaced")
	}
	site := routes[1].(map[string]any)
	if site["hostname"] != a.siteHost(d) || !strings.HasSuffix(site["hostname"].(string), "-sites.example.test") {
		t.Fatal("Wrong sites route", site)
	}
	conflict = true
	if e := a.ensureHostedRoute(a.ctx, d); e == nil {
		t.Fatal("Conflicting DNS was overwritten")
	}
	if updates != 1 || creates != 1 {
		t.Fatal("Unexpected extra mutations", updates, creates)
	}
}
