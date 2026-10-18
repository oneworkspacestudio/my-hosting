//go:build windows

package hosting

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNativeCancellationKillsDescendantsAndKeepsLiveSite(t *testing.T) {
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
	live := uploadFixture(t, a, p, "production", map[string]string{"index.html": "Keep this live release"})
	if live.State != "published" {
		t.Fatal(live.State, live.Error)
	}
	p, _ = a.store.project(p.ID)
	p.Trusted = true
	p.Build = []string{"node", "build.js"}
	p.Timeout = 60
	_ = a.store.saveProject(p)
	script := `const fs=require('node:fs'),cp=require('node:child_process'),path=require('node:path');const child=cp.spawn(process.execPath,['-e','setInterval(()=>{},1000)'],{stdio:'ignore'});fs.writeFileSync(path.join(process.env.HOME,'child.pid'),String(child.pid));console.log('Restricted parent and descendant started');setInterval(()=>{},1000);`
	d := queueFixture(t, a, p, "production", map[string]string{"build.js": script, "index.html": "Must not replace live"})
	done := make(chan struct{})
	go func() { defer close(done); a.execute(d) }()
	marker := filepath.Join(a.cfg.Data, "projects", p.ID, "var", "child.pid")
	deadline := time.Now().Add(15 * time.Second)
	var raw []byte
	for time.Now().Before(deadline) {
		raw, e = os.ReadFile(marker)
		if e == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if e != nil {
		t.Fatal("Restricted child did not start", e)
	}
	childID, e := strconv.Atoi(strings.TrimSpace(string(raw)))
	if e != nil {
		t.Fatal(e)
	}
	u := testUser(t, a, "owner")
	cookie, csrf := testSession(t, a, u)
	w := request(t, a, "POST", "/api/projects/"+p.ID+"/deployments/"+d.ID+"/cancel", map[string]any{}, cookie, csrf)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Cancelled build kept running")
	}
	got, _ := a.store.deploy(d.ID)
	if got.State != "cancelled" {
		t.Fatal("Accepted cancellation published or failed instead", got.State, got.Error)
	}
	h, e := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(childID))
	if e == nil {
		defer windows.CloseHandle(h)
		state, waitErr := windows.WaitForSingleObject(h, 0)
		if waitErr != nil || state != windows.WAIT_OBJECT_0 {
			t.Fatal("A descendant survived cancellation", childID, state, waitErr)
		}
	} else if e != windows.ERROR_INVALID_PARAMETER && e != windows.ERROR_NOT_FOUND {
		t.Fatal("Cannot confirm descendant shutdown", e)
	}
	if code, body := getSite(t, port, p.Slug+".localhost"); code != 200 || body != "Keep this live release" {
		t.Fatal("Cancellation changed live traffic", code, body)
	}
}
