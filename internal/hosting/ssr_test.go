package hosting

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeSSRFrameworkBuilds(t *testing.T) {
	if os.Getenv("MYHOSTING_SSR") != "1" {
		t.Skip("Set MYHOSTING_SSR=1 for pinned Next/Nuxt network-dependent build acceptance")
	}
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
	for _, adapter := range []string{"next", "nuxt"} {
		t.Run(adapter, func(t *testing.T) {
			p := testProject(t, a)
			p.Adapter = adapter
			p.Trusted = true
			p.Install = []string{"npm", "ci", "--include=dev", "--ignore-scripts", "--audit=false", "--fund=false"}
			p.Build = []string{"npm", "run", "build"}
			p.MemoryMB = 3072
			p.DiskMB = 4096
			p.Timeout = 1800
			if e := p.validate(); e != nil {
				t.Fatal(e)
			}
			_ = a.store.saveProject(p)
			source := map[string]string{}
			example := filepath.Join("..", "..", "examples", adapter)
			e := filepath.WalkDir(example, func(path string, d fs.DirEntry, e error) error {
				if e != nil {
					return e
				}
				if d.IsDir() {
					if d.Name() == "node_modules" {
						return filepath.SkipDir
					}
					return nil
				}
				name, e := filepath.Rel(example, path)
				if e != nil {
					return e
				}
				raw, e := os.ReadFile(path)
				if e != nil {
					return e
				}
				source[filepath.ToSlash(name)] = string(raw)
				return nil
			})
			if e != nil {
				t.Fatal(e)
			}
			if source["package-lock.json"] == "" {
				t.Fatal("Pinned framework lock missing")
			}
			d := uploadFixture(t, a, p, "production", source)
			if d.State != "published" {
				raw, _ := os.ReadFile(filepath.Join(a.cfg.Data, "logs", d.ID+".log"))
				t.Fatal(d.Error, string(raw))
			}
			if _, e := os.Stat(filepath.Join(a.releaseRoot(d), "node_modules")); !os.IsNotExist(e) {
				t.Fatal("Build-only dependencies were retained at the release root", e)
			}
			if d.Bytes <= 0 || d.Bytes > 300<<20 {
				t.Fatal("Unexpected framework runtime artifact size", d.Bytes)
			}
			code, body := getSite(t, port, p.Slug+".localhost")
			expected := "Actual Next.js SSR"
			if adapter == "nuxt" {
				expected = "Actual Nuxt SSR"
			}
			if code != 200 || !strings.Contains(body, expected) || !strings.Contains(body, "Rendered on the Windows server") {
				t.Fatal(code, body)
			}
			t.Log("Real framework built and served via Caddy", adapter, d.ID)
		})
	}
}
