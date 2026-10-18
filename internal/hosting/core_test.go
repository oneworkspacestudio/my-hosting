package hosting

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func testApp(t *testing.T) *App {
	t.Helper()
	parent := "C:/Projects/.cache/myhosting-acceptance-20261018"
	if e := os.MkdirAll(parent, 0700); e != nil {
		t.Fatal(e)
	}
	root, e := os.MkdirTemp(parent, "unit-")
	if e != nil {
		t.Fatal(e)
	}
	if e = prepareData(root); e != nil {
		t.Fatal(e)
	}
	s, e := openStore(root)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a := &App{cfg: Config{Data: root, Listen: "127.0.0.1:19007", Origin: "http://127.0.0.1:19007"}, store: s, web: fstest.MapFS{"index.html": {Data: []byte("<title>Hosting fixture</title>")}}, ctx: ctx, cancel: cancel, flows: map[string]loginFlow{}, attempts: map[string]attempt{}, active: map[string]context.CancelFunc{}, wake: make(chan struct{}, 1), processes: map[string]*managedProcess{}}
	a.runtime = &runtimeManager{root: filepath.Join(root, "runtimes"), status: map[string]string{}}
	t.Cleanup(func() {
		a.Close()
		if filepath.Dir(root) != filepath.FromSlash(parent) {
			t.Fatal("Unexpected test cleanup path")
		}
		if e = os.RemoveAll(root); e != nil {
			t.Error(e)
		}
	})
	return a
}
func testProject(t *testing.T, a *App) Project {
	t.Helper()
	p := Project{ID: id(), Name: "Fixture", Slug: "fixture-" + id()[:8], Branch: "main", Adapter: "static", Output: ".", Previews: true}
	if e := p.validate(); e != nil {
		t.Fatal(e)
	}
	if e := a.store.saveProject(p); e != nil {
		t.Fatal(e)
	}
	return p
}
func testUser(t *testing.T, a *App, role string) User {
	t.Helper()
	u := User{ID: id(), Issuer: "https://accounts.example.test", Subject: id(), Name: "Synthetic tester", Role: role}
	_, e := a.store.db.Exec("INSERT INTO users(id,issuer,subject,name,email,role) VALUES(?,?,?,?,?,?)", u.ID, u.Issuer, u.Subject, u.Name, "", u.Role)
	if e != nil {
		t.Fatal(e)
	}
	return u
}
func testSession(t *testing.T, a *App, u User) (*http.Cookie, string) {
	t.Helper()
	w := httptest.NewRecorder()
	if e := a.createSession(w, u, time.Now().Add(time.Hour)); e != nil {
		t.Fatal(e)
	}
	c := w.Result().Cookies()[0]
	r := httptest.NewRequest("GET", "http://127.0.0.1:19007/api/me", nil)
	r.AddCookie(c)
	s, ok := a.session(r)
	if !ok {
		t.Fatal("Fixture session unavailable")
	}
	return c, s.CSRF
}
func request(t *testing.T, a *App, method, path string, body any, c *http.Cookie, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	var in io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		in = bytes.NewReader(b)
	}
	r := httptest.NewRequest(method, "http://127.0.0.1:19007"+path, in)
	r.Header.Set("Origin", a.cfg.Origin)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-CSRF-Token", csrf)
	if c != nil {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}

func TestProjectValidation(t *testing.T) {
	p := Project{Name: "Site", Slug: "test-site", Adapter: "static", Output: "."}
	if e := p.validate(); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{"../outside", "C:/windows", "folder\\escape", "/absolute", "a/../b"} {
		q := p
		q.Output = bad
		if q.validate() == nil {
			t.Errorf("accepted output %q", bad)
		}
	}
	p.Adapter = "node"
	if p.validate() == nil {
		t.Fatal("executable project was not explicitly trusted")
	}
	p.Trusted = true
	p.Start = []string{"powershell", "-c", "anything"}
	if p.validate() == nil {
		t.Fatal("accepted an unrestricted executable")
	}
	p.Start = []string{"node", "server.js"}
	if e := p.validate(); e != nil {
		t.Fatal(e)
	}
}
func TestArchiveRejection(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "C:ads", "ok/../../escape", "dir\\escape", "CON.txt", "a. ", "dir/./file"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			file := filepath.Join(root, "input.zip")
			f, _ := os.Create(file)
			z := zip.NewWriter(f)
			w, _ := z.Create(name)
			w.Write([]byte("bad"))
			z.Close()
			f.Close()
			if extractZIP(file, filepath.Join(root, "out"), false) == nil {
				t.Fatal("unsafe archive was accepted")
			}
		})
	}
	root := t.TempDir()
	f, _ := os.Create(filepath.Join(root, "case.zip"))
	z := zip.NewWriter(f)
	for _, name := range []string{"Index.html", "index.HTML"} {
		w, _ := z.Create(name)
		w.Write([]byte("test"))
	}
	z.Close()
	f.Close()
	if extractZIP(filepath.Join(root, "case.zip"), filepath.Join(root, "out"), false) == nil {
		t.Fatal("case-insensitive duplicate accepted")
	}
}
func TestRedactionAcrossEveryBoundary(t *testing.T) {
	values := []string{"abaa", "super-secret-value", "short"}
	text := "before abaa middle super-secret-value after short end\n"
	for n := 1; n <= len(text); n++ {
		file := filepath.Join(t.TempDir(), "log")
		l := newRedactedLog(file, values)
		for start := 0; start < len(text); start += n {
			end := start + n
			if end > len(text) {
				end = len(text)
			}
			l.Write([]byte(text[start:end]))
		}
		l.Close()
		raw, _ := os.ReadFile(file)
		for _, v := range values {
			if bytes.Contains(raw, []byte(v)) {
				t.Fatalf("secret leaked at write size %d: %s", n, raw)
			}
		}
		if string(raw) != "before [redacted] middle [redacted] after [redacted] end\n" {
			t.Fatalf("incorrect redaction at size %d: %q", n, raw)
		}
	}
}
func TestEncryptedBackupFraming(t *testing.T) {
	var encrypted bytes.Buffer
	w, e := encryptStream(&encrypted, "fixture-password-only")
	if e != nil {
		t.Fatal(e)
	}
	source := bytes.Repeat([]byte("synthetic backup data"), 10000)
	if _, e = w.Write(source); e != nil {
		t.Fatal(e)
	}
	if e = w.Close(); e != nil {
		t.Fatal(e)
	}
	var restored bytes.Buffer
	if e = decryptStream(bytes.NewReader(encrypted.Bytes()), &restored, "fixture-password-only"); e != nil || !bytes.Equal(source, restored.Bytes()) {
		t.Fatalf("restore failed: %v", e)
	}
	if decryptStream(bytes.NewReader(encrypted.Bytes()), io.Discard, "wrong-password") == nil {
		t.Fatal("wrong password accepted")
	}
	for _, cut := range []int{10, 50, encrypted.Len() - 1} {
		if decryptStream(bytes.NewReader(encrypted.Bytes()[:cut]), io.Discard, "fixture-password-only") == nil {
			t.Fatalf("truncation at %d accepted", cut)
		}
	}
	bad := append([]byte(nil), encrypted.Bytes()...)
	bad[len(bad)/2] ^= 1
	if decryptStream(bytes.NewReader(bad), io.Discard, "fixture-password-only") == nil {
		t.Fatal("corrupted backup accepted")
	}
}
func TestAuthPermissionAndCSRF(t *testing.T) {
	a := testApp(t)
	p := testProject(t, a)
	owner := testUser(t, a, "owner")
	viewer := testUser(t, a, "viewer")
	oc, csrf := testSession(t, a, owner)
	vc, _ := testSession(t, a, viewer)
	if w := request(t, a, "GET", "/api/projects", nil, nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request(t, a, "GET", "/api/projects/"+p.ID, nil, vc, ""); w.Code != 404 {
		t.Fatal("cross-project read", w.Code)
	}
	if w := request(t, a, "GET", "/api/projects/"+p.ID, nil, oc, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request(t, a, "POST", "/api/projects", p, oc, ""); w.Code != 403 {
		t.Fatal("missing CSRF accepted", w.Code)
	}
	if w := request(t, a, "PUT", "/api/projects/"+p.ID+"/secrets", map[string]string{"name": "API_TOKEN", "environment": "production", "value": "synthetic-value-only"}, oc, csrf); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := request(t, a, "GET", "/api/projects/"+p.ID+"/secrets", nil, oc, "")
	if strings.Contains(w.Body.String(), "synthetic-value-only") {
		t.Fatal("secret returned to browser")
	}
	_, e := a.store.db.Exec("INSERT INTO members(project_id,user_id,role) VALUES(?,?,'viewer')", p.ID, viewer.ID)
	if e != nil {
		t.Fatal(e)
	}
	if w := request(t, a, "GET", "/api/projects/"+p.ID, nil, vc, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	_, _ = a.store.db.Exec("UPDATE users SET role='disabled' WHERE id=?", viewer.ID)
	if w := request(t, a, "GET", "/api/projects", nil, vc, ""); w.Code != 401 {
		t.Fatal("disabled member kept access", w.Code)
	}
}
func TestQueueSnapshotAndRecovery(t *testing.T) {
	a := testApp(t)
	p := testProject(t, a)
	if e := a.putSecret(p.ID, "API_TOKEN", "production", "original-value"); e != nil {
		t.Fatal(e)
	}
	d, e := a.queued(p, "github", "main", strings.Repeat("a", 40), "production", "build", "", "request-1", "fixture")
	if e != nil {
		t.Fatal(e)
	}
	same, e := a.queued(p, "github", "main", strings.Repeat("a", 40), "production", "build", "", "request-1", "fixture")
	if e != nil || same.ID != d.ID {
		t.Fatal("idempotency failed", e)
	}
	_ = a.putSecret(p.ID, "API_TOKEN", "production", "changed-value")
	values, e := a.deployEnv(d)
	if e != nil || values["API_TOKEN"] != "original-value" {
		t.Fatal("deployment environment was not immutable", e)
	}
	d.State = "building"
	_ = a.store.saveDeploy(d)
	if e = a.recoverJobs(); e != nil {
		t.Fatal(e)
	}
	got, _ := a.store.deploy(d.ID)
	if got.State != "failed" || !strings.Contains(got.Error, "service stopped") {
		t.Fatal("interrupted state did not recover")
	}
	if _, e = a.store.db.Exec("UPDATE audit SET action='altered'"); e == nil {
		t.Fatal("audit was mutable")
	}
}
func TestRestrictedProcessCannotReadOrModifyHostData(t *testing.T) {
	a := testApp(t)
	p := testProject(t, a)
	other := testProject(t, a)
	own := filepath.Join(a.cfg.Data, "projects", p.ID, "builds", "fixture")
	foreign := filepath.Join(a.cfg.Data, "projects", other.ID, "builds", "fixture")
	if e := sandboxDir(own, p.ID); e != nil {
		t.Fatal(e)
	}
	if e := sandboxDir(foreign, other.ID); e != nil {
		t.Fatal(e)
	}
	private := filepath.Join(a.cfg.Data, "secrets", "private.txt")
	foreignFile := filepath.Join(foreign, "other.txt")
	os.WriteFile(private, []byte("synthetic-private"), 0600)
	os.WriteFile(foreignFile, []byte("synthetic-other"), 0600)
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", private, foreignFile, filepath.Join(own, "result.txt"))
	cmd.Dir = own
	cmd.Env = append(safeEnvironment(own, filepath.Dir(os.Args[0]), 0, nil), "MYHOSTING_HELPER=1")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	j, e := startProcess(cmd, p.ID, 256, true)
	if e != nil {
		t.Fatal("restricted worker failed to start:", e)
	}
	defer j.Close()
	if e = cmd.Wait(); e != nil {
		t.Fatalf("restricted worker probe: %v: %s", e, out.String())
	}
	raw, e := os.ReadFile(filepath.Join(own, "result.txt"))
	if e != nil || string(raw) != "isolated" {
		t.Fatal("worker could not write its own folder", e)
	}
	raw, _ = os.ReadFile(private)
	if string(raw) != "synthetic-private" {
		t.Fatal("private file changed")
	}
}
func TestHelperProcess(t *testing.T) {
	if os.Getenv("MYHOSTING_HELPER") != "1" {
		return
	}
	var args []string
	for i, x := range os.Args {
		if x == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	if len(args) != 3 {
		os.Exit(9)
	}
	for _, p := range args[:2] {
		if _, e := os.ReadFile(p); e == nil {
			os.Stderr.WriteString("private/cross-project read allowed")
			os.Exit(10)
		}
		if e := os.WriteFile(p, []byte("changed"), 0600); e == nil {
			os.Stderr.WriteString("private/cross-project write allowed")
			os.Exit(11)
		}
		if e := setDACL(p, privateACL()); e == nil {
			os.Stderr.WriteString("worker could change protected ACL")
			os.Exit(12)
		}
	}
	if e := os.WriteFile(args[2], []byte("isolated"), 0600); e != nil {
		os.Stderr.WriteString(e.Error())
		os.Exit(13)
	}
	os.Exit(0)
}
