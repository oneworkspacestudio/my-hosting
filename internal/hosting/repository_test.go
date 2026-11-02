package hosting

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryInputForms(t *testing.T) {
	base := "https://workspace.example.test/git"
	for _, raw := range []string{"fidget-clicker-creator", "owner/fidget-clicker-creator", base + "/owner/fidget-clicker-creator", base + "/owner/fidget-clicker-creator.git", base + "/owner/fidget-clicker-creator.git/", "git@workspace.example.test:owner/fidget-clicker-creator.git"} {
		loc, e := parseRepository(raw, base)
		if e != nil || loc.Provider != "mygit" || !strings.HasSuffix(loc.Name, "fidget-clicker-creator") {
			t.Fatalf("%s: %#v %v", raw, loc, e)
		}
	}
	for _, raw := range []string{"https://github.com/owner/repo", "https://github.com/owner/repo.git", "git@github.com:owner/repo.git"} {
		loc, e := parseRepository(raw, base)
		if e != nil || loc.Provider != "github" || loc.Name != "owner/repo" {
			t.Fatal(raw, loc, e)
		}
	}
	for _, raw := range []string{"", "../repo", "owner/../repo", "https://user:pass@workspace.example.test/git/o/r", "https://workspace.example.test/git/o/r?token=secret", "https://127.0.0.1/o/r", "https://169.254.169.254/o/r", "http://example.com/o/r", "https://workspace.example.test/git/%2e%2e/repo", "https://workspace.example.test/git//o/r"} {
		if _, e := parseRepository(raw, base); e == nil {
			t.Fatal("unsafe input accepted", raw)
		}
	}
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "169.254.169.254", "100.64.0.1", "::1", "fc00::1"} {
		if publicRepositoryIP(net.ParseIP(raw)) {
			t.Fatal(raw)
		}
	}
}
func TestMyGitSourceLifecycleAndAccountBinding(t *testing.T) {
	a := testApp(t)
	u := testUser(t, a, "owner")
	a.cfg.Issuer = u.Issuer
	a.cfg.GitBase = "https://workspace.example.test/git"
	a.cfg.GitBridgeToken = "synthetic-connector"
	sha := strings.Repeat("a", 40)
	denied := false
	calls := 0
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	text := []byte("<!doctype html><title>Synthetic My Git site</title>")
	_ = tw.WriteHeader(&tar.Header{Name: "repo/index.html", Mode: 0600, Size: int64(len(text))})
	_, _ = tw.Write(text)
	_ = tw.Close()
	_ = gz.Close()
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var in gitBridgeCall
		if json.NewDecoder(r.Body).Decode(&in) != nil || r.Header.Get("Authorization") != "Bearer synthetic-connector" || in.Issuer != u.Issuer || in.Subject != u.Subject {
			t.Error("account or capability missing")
			http.Error(w, "denied", 403)
			return
		}
		if denied {
			http.Error(w, "Repository not found or read access is not granted in My Git", 404)
			return
		}
		if in.Operation != "resolve" && in.RepositoryID != 7 {
			t.Error("immutable repository id missing")
		}
		switch in.Operation {
		case "resolve":
			json.NewEncoder(w).Encode(repositoryInfo{Provider: "mygit", Repository: a.cfg.GitBase + "/synthetic/fidget-clicker-creator", FullName: "synthetic/fidget-clicker-creator", ID: 7, DefaultBranch: "trunk", Private: true})
		case "commit":
			fmt.Fprintf(w, `{"sha":%q,"commit":{"message":"Synthetic site","author":{"name":"Synthetic"}}}`, sha)
		case "contents":
			if in.File == "package.json" {
				http.Error(w, "not found", 404)
			} else {
				json.NewEncoder(w).Encode(map[string]string{"encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte("{}"))})
			}
		case "archive":
			if in.Ref != sha {
				t.Error("archive is not immutable")
			}
			w.Write(archive.Bytes())
		default:
			http.Error(w, "unsupported", 400)
		}
	}))
	defer bridge.Close()
	a.cfg.GitBridgeURL = bridge.URL + "/_integration/hosting/git"
	p := testProject(t, a)
	p.Repository = "fidget-clicker-creator"
	p.Branch = ""
	p.SourceActor = "forged"
	p.SourceRepoID = 999
	if e := a.prepareProjectRepository(context.Background(), &p, u, nil); e != nil {
		t.Fatal(e)
	}
	if p.SourceActor != u.ID || p.SourceRepoID != 7 || p.Branch != "trunk" || p.SourceProvider != "mygit" {
		t.Fatalf("unsafe source binding %#v", p)
	}
	if e := p.validate(); e != nil {
		t.Fatal(e)
	}
	if e := a.store.saveProject(p); e != nil {
		t.Fatal(e)
	}
	detected, e := a.detectGitHub(context.Background(), p)
	if e != nil || detected["output"] != "." {
		t.Fatal(detected, e)
	}
	commit, _, _, e := a.resolveSource(context.Background(), p, "", "")
	if e != nil || commit != sha {
		t.Fatal(commit, e)
	}
	out := filepath.Join(a.cfg.Data, "source-test.tar.gz")
	if e = a.gitArchive(context.Background(), p, sha, out, func(string) {}); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(out)
	if e != nil || !bytes.Equal(b, archive.Bytes()) {
		t.Fatal("archive corrupted", e)
	}
	denied = true
	if _, _, _, e = a.resolveSource(context.Background(), p, "", ""); e == nil {
		t.Fatal("revoked access still deploys")
	}
	denied = false
	_, _ = a.store.db.Exec("UPDATE users SET role='disabled' WHERE id=?", u.ID)
	before := calls
	if _, _, _, e = a.resolveSource(context.Background(), p, "", ""); e == nil || calls != before {
		t.Fatal("disabled actor used broker")
	}
	// Existing GitHub projects retain their provider when edited under Workspace.
	old := p
	old.Repository = "owner/legacy"
	old.SourceProvider = ""
	old.SourceActor = ""
	old.SourceRepoID = 0
	updated := old
	updated.SourceProvider = "mygit"
	if e = a.prepareProjectRepository(context.Background(), &updated, u, &old); e != nil || updated.SourceProvider != "" {
		t.Fatal("legacy source reinterpreted", e)
	}
}
func TestExternalRepositoryTokenIsBoundToSource(t *testing.T) {
	a := testApp(t)
	p := testProject(t, a)
	p.SourceProvider = "gitea"
	p.Repository = "https://git.example.test/alice/site"
	if e := a.putSecret(p.ID, "REPOSITORY_TOKEN_SCOPE", "integration", p.Repository); e != nil {
		t.Fatal(e)
	}
	if e := a.putSecret(p.ID, "REPOSITORY_TOKEN", "integration", "synthetic-read-token"); e != nil {
		t.Fatal(e)
	}
	if a.repositoryCredential(p) != "synthetic-read-token" {
		t.Fatal("credential unavailable")
	}
	p.Repository = "https://different.example.test/alice/site"
	if a.repositoryCredential(p) != "" {
		t.Fatal("credential leaked to changed repository")
	}
}

func TestManagedLocalGitURLAndWorkerIsolation(t *testing.T) {
	a := testApp(t)
	p := testProject(t, a)
	p.Repository = "http://127.0.0.1:8990/git/alice/site"
	p.SourceProvider = "mygit"
	if e := p.validate(); e != nil {
		t.Fatal(e)
	}
	t.Setenv("MYHOSTING_GIT_BRIDGE_TOKEN", "synthetic-capability")
	t.Setenv("WORKSPACE_OIDC_CLIENT_SECRET", "synthetic-secret")
	for _, entry := range safeEnvironment(a.cfg.Data, "C:/synthetic-node", 9000, map[string]string{}) {
		if strings.Contains(entry, "synthetic-capability") || strings.Contains(entry, "synthetic-secret") {
			t.Fatal("integration authority inherited by build worker")
		}
	}
}

// A live Workspace broker + native Git fixture is supplied by the suite's
// TestHostingGitNativePrivateRepository orchestration, never an owner service.
func TestMyGitNativeBridgeDeployment(t *testing.T) {
	bridge := os.Getenv("HOSTING_ACCEPTANCE_BRIDGE")
	if bridge == "" {
		t.Skip("Set synthetic broker fixture for native end-to-end deployment")
	}
	a := testApp(t)
	u := testUser(t, a, "owner")
	u.Subject = "subject-alice"
	_, _ = a.store.db.Exec("UPDATE users SET subject=? WHERE id=?", u.Subject, u.ID)
	a.cfg.Issuer = u.Issuer
	a.cfg.GitBase = "https://workspace.example.test/git"
	a.cfg.GitBridgeURL = bridge
	a.cfg.GitBridgeToken = os.Getenv("HOSTING_ACCEPTANCE_CAPABILITY")
	provisionTestRuntime(t, a, "caddy")
	port, e := freePort()
	if e != nil {
		t.Fatal(e)
	}
	settings := a.settings()
	settings.CaddyPort = port
	if e = a.store.setSetting("hosting", settings); e != nil {
		t.Fatal(e)
	}
	p := testProject(t, a)
	p.Repository = "fidget-clicker-creator"
	p.Branch = ""
	p.Trusted = true
	if e = a.prepareProjectRepository(context.Background(), &p, u, nil); e != nil {
		t.Fatal(e)
	}
	if e = p.validate(); e != nil {
		t.Fatal(e)
	}
	detected, e := a.detectGitHub(context.Background(), p)
	if e != nil {
		t.Fatal(detected, e)
	}
	p.Output = "."
	p.Install = []string{}
	p.Build = []string{}
	if os.Getenv("HOSTING_ACCEPTANCE_NODE") == "1" {
		if detected["output"] != "dist" {
			t.Fatal("Node build detection", detected)
		}
		provisionTestRuntime(t, a, "node24")
		p.Output = "dist"
		p.Install = detected["install"].([]string)
		p.Build = detected["build"].([]string)
	}
	if e = a.store.saveProject(p); e != nil {
		t.Fatal(e)
	}
	d, e := a.queueGit(context.Background(), p, "", "", "production", "build", "", u.ID)
	if e != nil {
		t.Fatal(e)
	}
	a.execute(d)
	d, e = a.store.deploy(d.ID)
	if e != nil || d.State != "published" {
		b, _ := os.ReadFile(filepath.Join(a.cfg.Data, "logs", d.ID+".log"))
		t.Fatal(d.State, d.Error, e, string(b))
	}
	if code, body := getSite(t, port, a.siteHost(d)); code != 200 || !strings.Contains(body, "Native My Git Hosting acceptance") {
		t.Fatal(code, body)
	}
	if !validSHA.MatchString(d.Commit) || d.SourceHash == "" {
		t.Fatal("immutable source proof missing")
	}
}
