//go:build windows

package hosting

import (
	"os"
	"strings"
	"testing"
)

func TestNativeRestartReappliesRuntimeReadAccess(t *testing.T) {
	a := testApp(t)
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
	d := uploadFixture(t, a, p, "production", map[string]string{"server.js": `require('node:http').createServer((q,r)=>r.end('restored Node runtime')).listen(Number(process.env.PORT),'127.0.0.1');`})
	if d.State != "published" {
		t.Fatal(d.State, d.Error)
	}
	a.stopNode(d.ID)
	_ = a.stopCaddy()
	if e = prepareData(a.cfg.Data); e != nil {
		t.Fatal(e)
	}
	if e = a.restoreProcesses(a.ctx); e != nil {
		t.Fatal("Runtime startup after protected-file reset", e)
	}
	if e = a.reloadRoutes(a.ctx, nil); e != nil {
		t.Fatal(e)
	}
	if code, body := getSite(t, port, p.Slug+".localhost"); code != 200 || !strings.Contains(body, "restored Node") {
		t.Fatal(code, body)
	}
	p = testProject(t, a)
	p.Trusted = true
	p.Build = []string{"npm", "--version"}
	_ = a.store.saveProject(p)
	d = uploadFixture(t, a, p, "production", map[string]string{"index.html": "npm is readable after restart"})
	if d.State != "published" {
		raw, _ := os.ReadFile(a.cfg.Data + "/logs/" + d.ID + ".log")
		t.Fatal(d.State, d.Error, string(raw))
	}
}
