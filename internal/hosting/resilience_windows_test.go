//go:build windows

package hosting

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeRepeatedExitUnhealthyAndManagerRestartRecovery(t *testing.T) {
	a := testApp(t)
	a.healthTestInterval, a.healthTestGrace, a.retryTestDelay = 100*time.Millisecond, time.Second, 50*time.Millisecond
	provisionTestRuntime(t, a, "caddy")
	provisionTestRuntime(t, a, "node24")
	port, err := freePort()
	if err != nil {
		t.Fatal(err)
	}
	s := a.settings()
	s.CaddyPort = port
	_ = a.store.setSetting("hosting", s)
	p := testProject(t, a)
	p.Adapter, p.Trusted, p.Start, p.HealthPath = "node", true, []string{"node", "server.js"}, "/healthz"
	_ = a.store.saveProject(p)
	source := `const http=require('node:http'),fs=require('node:fs'),path=require('node:path');let bad=false;
const marker=path.join(process.env.MYHOSTING_DATA_DIR,'fixture.txt');
http.createServer((q,r)=>{if(q.url==='/crash'){r.end('crashing');setTimeout(()=>process.exit(7),30);return}if(q.url==='/unhealthy'){bad=true;r.end('unhealthy');return}if(q.url==='/healthz'){r.writeHead(bad?503:200);r.end('health');return}r.end(fs.existsSync(marker)?fs.readFileSync(marker):'fixture')}).listen(Number(process.env.PORT),'127.0.0.1');`
	d := uploadFixture(t, a, p, "production", map[string]string{"server.js": source})
	if d.State != "published" {
		t.Fatal(d.State, d.Error)
	}
	marker := filepath.Join(a.cfg.Data, "projects", p.ID, "var", "fixture.txt")
	if err = os.WriteFile(marker, []byte("persistent synthetic app data"), 0600); err != nil {
		t.Fatal(err)
	}
	a.processMu.Lock()
	proc := a.processes[d.ID]
	a.processMu.Unlock()
	request := func(route string) {
		t.Helper()
		req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d%s", port, route), nil)
		req.Host = p.Slug + ".localhost"
		response, e := (&http.Client{Timeout: 3 * time.Second}).Do(req)
		if e != nil {
			t.Fatal(e)
		}
		response.Body.Close()
	}
	waitRestart := func(pid, count int) {
		t.Helper()
		end := time.Now().Add(12 * time.Second)
		for time.Now().Before(end) {
			proc.mu.Lock()
			restored := proc.pid != pid && proc.restarts >= count
			proc.mu.Unlock()
			if restored {
				if code, body := getSite(t, port, p.Slug+".localhost"); code == 200 && strings.Contains(body, "persistent synthetic") {
					return
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("Automatic recovery did not restore traffic", proc.summary())
	}
	for i := 1; i <= 6; i++ {
		proc.mu.Lock()
		pid := proc.pid
		proc.mu.Unlock()
		request("/crash")
		waitRestart(pid, i)
	}
	proc.mu.Lock()
	pid := proc.pid
	count := proc.restarts
	proc.mu.Unlock()
	request("/unhealthy")
	waitRestart(pid, count+1)
	// An explicit operator stop must survive reconciliation and restart.
	_ = a.store.setSetting("process:"+d.ID, "stopped")
	a.stopNode(d.ID)
	a.superviseRouting()
	a.processMu.Lock()
	stopped := a.processes[d.ID] == nil
	a.processMu.Unlock()
	if !stopped {
		t.Fatal("Manual stop was ignored")
	}
	_ = a.store.setSetting("process:"+d.ID, "running")
	a.superviseRouting()
	if code, body := getSite(t, port, p.Slug+".localhost"); code != 200 || !strings.Contains(body, "persistent synthetic") {
		t.Fatal(code, body)
	}
	// An unreadable environment for another published app must not prevent
	// healthy projects from returning after startup.
	broken := testProject(t, a)
	broken.Adapter, broken.Live = "node", id()
	_ = a.store.saveProject(broken)
	_ = a.store.saveDeploy(Deploy{ID: broken.Live, ProjectID: broken.ID, Environment: "production", State: "published", Snapshot: broken, Port: 1})
	// Close/reopen the actual app, store and router; restoration needs no deploy.
	a.Close()
	restarted, err := New(context.Background(), a.cfg, a.web)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restarted.Close)
	end := time.Now().Add(30 * time.Second)
	for time.Now().Before(end) {
		client := &http.Client{Timeout: time.Second}
		req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/", port), nil)
		req.Host = p.Slug + ".localhost"
		response, e := client.Do(req)
		if e == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("Published app did not return after a full manager restart")
}
