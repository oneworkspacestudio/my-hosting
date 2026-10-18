package hosting

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifiedRuntimeRepairPreservesFilesUntilArchivePasses(t *testing.T) {
	a := testApp(t)
	provisionTestRuntime(t, a, "caddy")
	exe := filepath.Join(a.runtime.path("caddy"), "caddy.exe")
	if e := os.WriteFile(exe, []byte("synthetic damaged runtime"), 0600); e != nil {
		t.Fatal(e)
	}
	if a.runtime.ready("caddy") == nil {
		t.Fatal("Damaged binary was accepted")
	}
	bad := filepath.Join(a.cfg.Data, "cache", "invalid-runtime.zip")
	_ = os.WriteFile(bad, []byte("invalid archive"), 0600)
	if e := a.runtime.provision(context.Background(), "caddy", bad, nil, true); e == nil {
		t.Fatal("Invalid replacement accepted")
	}
	raw, _ := os.ReadFile(exe)
	if string(raw) != "synthetic damaged runtime" {
		t.Fatal("Runtime was changed before replacement verification")
	}
	good := filepath.Join(os.Getenv("MYHOSTING_RUNTIME_CACHE"), "caddy.zip")
	if e := a.runtime.provision(context.Background(), "caddy", good, nil, true); e != nil {
		t.Fatal(e)
	}
	if e := a.runtime.ready("caddy"); e != nil {
		t.Fatal("Verified repair did not restore runtime", e)
	}
	entries, _ := os.ReadDir(a.runtime.root)
	if len(entries) != 1 || entries[0].Name() != "caddy" {
		t.Fatal("Temporary runtime copies were left behind", entries)
	}
}
