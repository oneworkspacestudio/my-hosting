package hosting

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSitesNamingAndTransport(t *testing.T) {
	a := testApp(t)
	a.cfg.SiteDomain = "sites.m8e.co.uk"
	a.cfg.SiteTransport = "tunnel"
	p := testProject(t, a)
	p.Slug = "my-site"
	d := Deploy{ID: id(), Snapshot: p, Environment: "production"}
	if got := a.deployURL(d); got != "http://my-site.localhost:9080" {
		t.Fatal(got)
	}
	s := a.settings()
	s.Public = true
	if e := a.store.setSetting("hosting", s); e != nil {
		t.Fatal(e)
	}
	if got := a.deployURL(d); got != "https://my-site.sites.m8e.co.uk" {
		t.Fatal(got)
	}
	s.PreviewDomain = "m8e.co.uk"
	s.AddressStyle = "flat"
	_ = a.store.setSetting("hosting", s)
	if got := a.deployURL(d); got != "https://my-site-sites.m8e.co.uk" {
		t.Fatal(got)
	}
	d.Environment = "preview"
	d.Snapshot.Slug = "a" + strings.Repeat("b", 47)
	label := strings.Split(a.siteHost(d), ".")[0]
	if len(label) > 63 || !strings.HasPrefix(label, "deploy-") {
		t.Fatal(label)
	}
	if !a.reservedSiteDomain("other-sites.m8e.co.uk") || !a.reservedSiteDomain("127.0.0.1") {
		t.Fatal("Reserved host reassignment allowed")
	}
	p.Slug = "bad-"
	if p.validate() == nil {
		t.Fatal("Trailing hyphen accepted")
	}
}
func provisionTestRuntime(t *testing.T, a *App, key string) {
	t.Helper()
	cache := os.Getenv("MYHOSTING_RUNTIME_CACHE")
	if cache == "" {
		t.Skip("Set MYHOSTING_RUNTIME_CACHE to verified official runtime archives for native acceptance")
	}
	if e := a.runtime.provision(context.Background(), key, filepath.Join(cache, key+".zip"), func(s string) { t.Log(s) }); e != nil {
		t.Fatal(e)
	}
}
func uploadFixture(t *testing.T, a *App, p Project, env string, files map[string]string) Deploy {
	t.Helper()
	d, e := a.queued(p, "zip", p.Branch, "", env, "build", "", "", "acceptance")
	if e != nil {
		t.Fatal(e)
	}
	var buffer bytes.Buffer
	z := zip.NewWriter(&buffer)
	for name, body := range files {
		w, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = io.WriteString(w, body); e != nil {
			t.Fatal(e)
		}
	}
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(a.sourcePath(d), buffer.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	d.SourceHash = digest(buffer.Bytes())
	d.State = "queued"
	if e = a.store.saveDeploy(d); e != nil {
		t.Fatal(e)
	}
	a.execute(d)
	result, e := a.store.deploy(d.ID)
	if e != nil {
		t.Fatal(e)
	}
	return result
}
func getSite(t *testing.T, port int, host string) (int, string) {
	t.Helper()
	r, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/", port), nil)
	r.Host = host
	client := &http.Client{Timeout: 5 * time.Second}
	resp, e := client.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}
func TestNativeStaticPublicationRollbackAndBackup(t *testing.T) {
	a := testApp(t)
	defer func() {
		if t.Failed() {
			b, _ := os.ReadFile(filepath.Join(a.cfg.Data, "logs", "caddy.log"))
			t.Log(string(b))
		}
	}()
	provisionTestRuntime(t, a, "caddy")
	port, e := freePort()
	if e != nil {
		t.Fatal(e)
	}
	s := a.settings()
	s.CaddyPort = port
	s.PreviewDomain = "sites.m8e.co.uk"
	s.Transport = "tunnel"
	s.Public = true
	if e = a.store.setSetting("hosting", s); e != nil {
		t.Fatal(e)
	}
	p := testProject(t, a)
	first := uploadFixture(t, a, p, "production", map[string]string{"index.html": "<h1>First real release</h1>"})
	if first.State != "published" {
		t.Fatal(first.State, first.Error)
	}
	if code, body := getSite(t, port, p.Slug+".sites.m8e.co.uk"); code != 200 || !strings.Contains(body, "First real release") {
		t.Fatal(code, body)
	}
	preview := uploadFixture(t, a, p, "preview", map[string]string{"index.html": "<h1>Real deploy preview</h1>"})
	if preview.State != "published" {
		t.Fatal(preview.State, preview.Error)
	}
	if code, body := getSite(t, port, a.siteHost(preview)); code != 200 || !strings.Contains(body, "Real deploy preview") {
		t.Fatal(code, body)
	}
	second := uploadFixture(t, a, p, "production", map[string]string{"index.html": "<h1>Second real release</h1>"})
	if second.State != "published" {
		t.Fatal(second.State, second.Error)
	}
	if _, body := getSite(t, port, p.Slug+".sites.m8e.co.uk"); !strings.Contains(body, "Second real release") {
		t.Fatal(body)
	}
	current, e := a.store.project(p.ID)
	if e != nil {
		t.Fatal(e)
	}
	rollback, e := a.queued(current, "zip", p.Branch, "", "production", "rollback", first.ID, "", "acceptance")
	if e != nil {
		t.Fatal(e)
	}
	a.execute(rollback)
	rollback, e = a.store.deploy(rollback.ID)
	if e != nil || rollback.State != "published" {
		t.Fatal(rollback.State, rollback.Error, e)
	}
	if _, body := getSite(t, port, p.Slug+".sites.m8e.co.uk"); !strings.Contains(body, "First real release") {
		t.Fatal(body)
	}
	if code, _ := getSite(t, port, "unknown.sites.m8e.co.uk"); code != 404 {
		t.Fatal("Unknown hostname was served", code)
	}
	if e = a.stopCaddy(); e != nil {
		t.Fatal(e)
	}
	backup := filepath.Join(a.cfg.Data, "backups", "acceptance.mhbackup")
	if e = a.putSecret(p.ID, "API_KEY", "production", "synthetic-portable-secret"); e != nil {
		t.Fatal(e)
	}
	if e = a.makeBackup(backup, "synthetic-backup-password"); e != nil {
		t.Fatal(e)
	}
	restoreRoot := filepath.Join(filepath.Dir(a.cfg.Data), "restore-"+id())
	defer os.RemoveAll(restoreRoot)
	if e = Restore(backup, restoreRoot, "synthetic-backup-password"); e != nil {
		t.Fatal(e)
	}
	restored, e := openStore(restoreRoot)
	if e != nil {
		t.Fatal(e)
	}
	defer restored.db.Close()
	var secret []byte
	if e = restored.db.QueryRow("SELECT value FROM secrets WHERE project_id=? AND name='API_KEY'", p.ID).Scan(&secret); e != nil {
		t.Fatal(e)
	}
	plain, e := unprotect(secret)
	if e != nil || string(plain) != "synthetic-portable-secret" {
		t.Fatal("Restored secret differs", e)
	}
	var stored string
	restored.db.QueryRow("SELECT value FROM settings WHERE name='hosting'").Scan(&stored)
	var restoredSettings Settings
	if json.Unmarshal([]byte(stored), &restoredSettings) != nil || restoredSettings.Public {
		t.Fatal("Restore exposed public listeners")
	}
	if _, e = os.Stat(filepath.Join(restoreRoot, "projects", p.ID, "sources", "source-"+first.ID+".zip")); e != nil {
		t.Fatal("Retained source missing", e)
	}
}

func TestNativeNodeAndControlIsolation(t *testing.T) {
	a := testApp(t)
	defer func() {
		if t.Failed() {
			entries, _ := os.ReadDir(filepath.Join(a.cfg.Data, "logs"))
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".log") {
					raw, _ := os.ReadFile(filepath.Join(a.cfg.Data, "logs", entry.Name()))
					t.Log(entry.Name(), string(raw))
				}
			}
		}
	}()
	provisionTestRuntime(t, a, "caddy")
	provisionTestRuntime(t, a, "node24")
	port, e := freePort()
	if e != nil {
		t.Fatal(e)
	}
	s := a.settings()
	s.CaddyPort = port
	_ = a.store.setSetting("hosting", s)
	p := testProject(t, a)
	p.Adapter = "node"
	p.Trusted = true
	p.Start = []string{"node", "server.js"}
	_ = a.store.saveProject(p)
	other := testProject(t, a)
	otherDir := filepath.Join(a.cfg.Data, "projects", other.ID, "var")
	if e = sandboxDir(otherDir, other.ID); e != nil {
		t.Fatal(e)
	}
	otherFile := filepath.Join(otherDir, "private.txt")
	_ = os.WriteFile(otherFile, []byte("synthetic-other-project"), 0600)
	private := filepath.Join(a.cfg.Data, "secrets", "private.txt")
	_ = os.WriteFile(private, []byte("synthetic-control-secret"), 0600)
	source := `const fs=require('node:fs'),http=require('node:http');
 let denied=0;for(const p of [` + encode(private) + `,` + encode(otherFile) + `]){try{fs.readFileSync(p)}catch(e){denied++}}
 try{fs.writeFileSync(__filename,'tampered')}catch(e){denied++}
 fs.writeFileSync(process.env.MYHOSTING_DATA_DIR+'/own.txt','allowed');
 if(denied!==3)throw Error('Private or immutable data access was allowed');
 http.createServer((req,res)=>res.end('Real Node publication; isolated='+denied)).listen(Number(process.env.PORT),'127.0.0.1');`
	d := uploadFixture(t, a, p, "production", map[string]string{"server.js": source})
	if d.State != "published" {
		t.Fatal(d.State, d.Error)
	}
	if code, body := getSite(t, port, p.Slug+".localhost"); code != 200 || !strings.Contains(body, "isolated=3") {
		t.Fatal(code, body)
	}
	config, e := a.caddyTLS()
	if e != nil {
		t.Fatal(e)
	}
	config.Certificates = nil
	transport := &http.Transport{TLSClientConfig: config}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	r, e := client.Post(fmt.Sprintf("https://127.0.0.1:%d/load", a.caddyAdminPort), "application/json", strings.NewReader("{}"))
	if e == nil {
		r.Body.Close()
		if r.StatusCode < 400 {
			t.Fatal("Unauthenticated Caddy control allowed")
		}
	}
	a.stopNode(d.ID)
	a.processMu.Lock()
	running := a.processes[d.ID]
	a.processMu.Unlock()
	if running != nil {
		t.Fatal("Node stop did not remove supervised process")
	}
	if e = a.restoreProcesses(a.ctx); e != nil {
		t.Fatal(e)
	}
	if e = a.reloadRoutes(a.ctx, nil); e != nil {
		t.Fatal(e)
	}
	if _, body := getSite(t, port, p.Slug+".localhost"); !strings.Contains(body, "Real Node publication") {
		t.Fatal(body)
	}
	build := testProject(t, a)
	build.Trusted = true
	build.Build = []string{"node", "build.js"}
	build.Output = "dist"
	_ = a.store.saveProject(build)
	built := uploadFixture(t, a, build, "production", map[string]string{"build.js": `const fs=require('node:fs');fs.mkdirSync('dist');fs.writeFileSync('dist/index.html','Actual restricted build output');`})
	if built.State != "published" {
		t.Fatal(built.State, built.Error)
	}
	if code, body := getSite(t, port, build.Slug+".localhost"); code != 200 || !strings.Contains(body, "Actual restricted build output") {
		t.Fatal(code, body)
	}
}
