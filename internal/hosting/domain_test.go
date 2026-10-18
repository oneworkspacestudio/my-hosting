package hosting

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNativeCanonicalRoutingPreservesPathsAndPreviewOrigins(t *testing.T) {
	a := testApp(t)
	provisionTestRuntime(t, a, "caddy")
	port, e := freePort()
	if e != nil {
		t.Fatal(e)
	}
	s := a.settings()
	s.CaddyPort = port
	s.Public = true
	s.Transport = "tunnel"
	s.AddressStyle = "flat"
	s.PreviewDomain = "example.test"
	_ = a.store.setSetting("hosting", s)
	p := testProject(t, a)
	d := uploadFixture(t, a, p, "production", map[string]string{"index.html": "Canonical site output"})
	if d.State != "published" {
		t.Fatal(d.State, d.Error)
	}
	if e = a.saveDomain(Domain{Host: "canonical.example.test", ProjectID: p.ID, Verified: true, HTTPS: "certificate-valid", Canonical: true}); e != nil {
		t.Fatal(e)
	}
	if e = a.reloadRoutes(a.ctx, nil); e != nil {
		t.Fatal(e)
	}
	r, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/hello?from=alias", port), nil)
	r.Host = a.siteHost(d)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, e := client.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != 308 || response.Header.Get("Location") != "https://canonical.example.test/hello?from=alias" || response.Header.Get("X-MyHosting-Deploy") != d.ID {
		t.Fatal("Canonical redirect did not preserve request/deploy", response.StatusCode, response.Header)
	}
	if code, body := getSite(t, port, "canonical.example.test"); code != 200 || body != "Canonical site output" {
		t.Fatal(code, body)
	}
	if code, _ := getSite(t, port, p.Slug+".localhost"); code != 200 {
		t.Fatal("Local preview was redirected", code)
	}
	preview := uploadFixture(t, a, p, "preview", map[string]string{"index.html": "Independent deploy preview"})
	if preview.State != "published" {
		t.Fatal(preview.State, preview.Error)
	}
	if code, body := getSite(t, port, a.siteHost(preview)); code != 200 || body != "Independent deploy preview" {
		t.Fatal(code, body)
	}
}

func TestPublicNodeRoutesTellAppsHTTPSIsInUse(t *testing.T) {
	a := testApp(t)
	p := testProject(t, a)
	p.Adapter = "node"
	d := Deploy{ID: id(), ProjectID: p.ID, Snapshot: p, Environment: "production", Port: 12345}
	s := a.settings()
	s.PreviewDomain = "example.test"
	routes, e := a.publicProjectRoutes(d, s)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(encode(routes), `"X-Forwarded-Proto":["https"]`) {
		t.Fatal("Tunnel proxy advertised a cleartext public origin")
	}
}

func TestUnverifiedCanonicalDomainIsRejected(t *testing.T) {
	a := testApp(t)
	p := testProject(t, a)
	if e := a.saveDomain(Domain{Host: "custom.example.test", ProjectID: p.ID}); e != nil {
		t.Fatal(e)
	}
	u := testUser(t, a, "owner")
	c, csrf := testSession(t, a, u)
	w := request(t, a, "POST", "/api/projects/"+p.ID+"/domains/custom.example.test/canonical", map[string]bool{"enabled": true}, c, csrf)
	if w.Code != 409 {
		t.Fatal("Unverified canonical target accepted", w.Code, w.Body.String())
	}
}
