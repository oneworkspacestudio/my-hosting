package hosting

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeRetentionKeepsLiveRollbackPreviewAndBackup(t *testing.T) {
	a := testApp(t)
	provisionTestRuntime(t, a, "caddy")
	port, e := freePort()
	if e != nil {
		t.Fatal(e)
	}
	s := a.settings()
	s.CaddyPort = port
	s.Retain = 2
	_ = a.store.setSetting("hosting", s)
	p := testProject(t, a)
	first := uploadFixture(t, a, p, "production", map[string]string{"index.html": "Original release"})
	old := uploadFixture(t, a, p, "production", map[string]string{"index.html": "Unused old release"})
	preview := uploadFixture(t, a, p, "preview", map[string]string{"index.html": "Active preview"})
	latest := uploadFixture(t, a, p, "production", map[string]string{"index.html": "Latest build"})
	for _, d := range []Deploy{first, old, preview, latest} {
		if d.State != "published" {
			t.Fatal(d.State, d.Error)
		}
	}
	rollback, e := a.queued(p, "zip", p.Branch, "", "production", "rollback", first.ID, "", "acceptance")
	if e != nil {
		t.Fatal(e)
	}
	a.execute(rollback)
	rollback, _ = a.store.deploy(rollback.ID)
	if rollback.State != "published" {
		t.Fatal(rollback.Error)
	}
	backup := filepath.Join(a.cfg.Data, "backups", "keep.mhbackup")
	_ = os.WriteFile(backup, []byte("synthetic backup sentinel"), 0600)
	variable := filepath.Join(a.cfg.Data, "projects", p.ID, "var")
	_ = os.MkdirAll(variable, 0700)
	_ = os.WriteFile(filepath.Join(variable, "keep.txt"), []byte("user app data"), 0600)
	result, e := a.cleanupStorage()
	if e != nil {
		t.Fatal(e)
	}
	if result.Expired != 1 || result.Bytes == 0 {
		t.Fatal("Unexpected retention result", result)
	}
	if _, e = os.Stat(a.releaseRoot(old)); !os.IsNotExist(e) {
		t.Fatal("Unused runtime artifact remains", e)
	}
	for _, path := range []string{a.releaseRoot(first), a.releaseRoot(preview), a.releaseRoot(latest), a.sourceForRead(rollback), backup, filepath.Join(variable, "keep.txt")} {
		if _, e = os.Stat(path); e != nil {
			t.Fatal("Protected retention path removed", path, e)
		}
	}
	pruned, _ := a.store.deploy(old.ID)
	if pruned.State != "expired" {
		t.Fatal("Removed artifact was still advertised as published", pruned.State)
	}
	if code, body := getSite(t, port, p.Slug+".localhost"); code != 200 || body != "Original release" {
		t.Fatal(code, body)
	}
	if code, body := getSite(t, port, preview.Snapshot.SlugHostLocal(preview)); code != 200 || body != "Active preview" {
		t.Fatal(code, body)
	}
	if e = a.removeRetentionPath(backup, &result); e == nil {
		t.Fatal("Cleanup accepted a backup path")
	}
	if e = a.removeRetentionPath(filepath.Join(a.cfg.Data, "logs", "caddy.log"), &result); e == nil {
		t.Fatal("Cleanup accepted control-plane logs")
	}
}

func TestMaintenanceRejectsNewQueueWork(t *testing.T) {
	a := testApp(t)
	p := testProject(t, a)
	a.jobsGate.Lock()
	defer a.jobsGate.Unlock()
	if _, e := a.queued(p, "zip", p.Branch, "", "production", "build", "", "", "acceptance"); e == nil {
		t.Fatal("A job entered maintenance storage")
	}
}
