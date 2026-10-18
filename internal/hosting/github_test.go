package hosting

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSignedWebhookQueueAndDeduplication(t *testing.T) {
	a := testApp(t)
	p := testProject(t, a)
	p.Repository = "fixture/project"
	p.Previews = false
	_ = a.store.saveProject(p)
	const secret = "synthetic-webhook-only"
	if e := a.putSecret(p.ID, "WEBHOOK_SECRET", "integration", secret); e != nil {
		t.Fatal(e)
	}
	send := func(branch, repo, sha, event string, valid bool) *httptest.ResponseRecorder {
		body := encode(map[string]any{"ref": "refs/heads/" + branch, "after": sha, "repository": map[string]string{"full_name": repo}})
		r := httptest.NewRequest("POST", "/hooks/github/"+p.ID, strings.NewReader(body))
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(body))
		signature := hex.EncodeToString(mac.Sum(nil))
		if !valid {
			signature = strings.Repeat("0", 64)
		}
		r.Header.Set("X-Hub-Signature-256", "sha256="+signature)
		r.Header.Set("X-GitHub-Event", event)
		w := httptest.NewRecorder()
		a.webhook(w, r, p.ID)
		return w
	}
	sha := strings.Repeat("a", 40)
	if w := send("main", p.Repository, sha, "push", false); w.Code != 401 {
		t.Fatal("Bad signature", w.Code)
	}
	if w := send("main", "other/repo", sha, "push", true); w.Code != 400 {
		t.Fatal("Wrong repository", w.Code)
	}
	if w := send("main", p.Repository, "bad-sha", "push", true); w.Code != 400 {
		t.Fatal("Invalid SHA", w.Code)
	}
	if w := send("feature", p.Repository, sha, "push", true); w.Code != 202 || !strings.Contains(w.Body.String(), "ignored") {
		t.Fatal("Branch allowlist", w.Body.String())
	}
	if w := send("main", p.Repository, sha, "ping", true); w.Code != 200 {
		t.Fatal("Ping", w.Code)
	}
	first := send("main", p.Repository, sha, "push", true)
	second := send("main", p.Repository, sha, "push", true)
	var d, repeat Deploy
	_ = json.Unmarshal(first.Body.Bytes(), &d)
	_ = json.Unmarshal(second.Body.Bytes(), &repeat)
	if first.Code != 202 || second.Code != 202 || !validID.MatchString(d.ID) || d.ID != repeat.ID || d.Commit != sha || d.State != "queued" || d.Environment != "production" {
		t.Fatal("Authenticated event not durably deduplicated", first.Body.String(), second.Body.String())
	}
	p.Previews = true
	_ = a.store.saveProject(p)
	w := send("feature", p.Repository, strings.Repeat("b", 40), "push", true)
	_ = json.Unmarshal(w.Body.Bytes(), &d)
	if w.Code != 202 || d.Environment != "preview" || d.Branch != "feature" {
		t.Fatal("Preview branch", w.Body.String())
	}
}

func TestDeploymentCancellationAndAuthorization(t *testing.T) {
	a := testApp(t)
	p := testProject(t, a)
	owner := testUser(t, a, "owner")
	c, csrf := testSession(t, a, owner)
	d, e := a.queued(p, "github", p.Branch, strings.Repeat("a", 40), "production", "build", "", "", "acceptance")
	if e != nil {
		t.Fatal(e)
	}
	path := "/api/projects/" + p.ID + "/deployments/" + d.ID
	if w := request(t, a, "POST", path+"/cancel", map[string]any{}, c, csrf); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	got, _ := a.store.deploy(d.ID)
	if got.State != "cancelled" {
		t.Fatal("Queue cancellation not persisted", got.State)
	}
	if w := request(t, a, "POST", path+"/cancel", map[string]any{}, c, csrf); w.Code != 409 {
		t.Fatal("Finished cancellation", w.Code)
	}
	viewer := testUser(t, a, "viewer")
	_, e = a.store.db.Exec("INSERT INTO members(project_id,user_id,role) VALUES(?,?,'viewer')", p.ID, viewer.ID)
	if e != nil {
		t.Fatal(e)
	}
	vc, vs := testSession(t, a, viewer)
	for _, action := range []string{"retry", "rebuild", "rollback", "promote", "republish", "cancel"} {
		if w := request(t, a, "POST", path+"/"+action, map[string]any{}, vc, vs); w.Code != 403 {
			t.Fatal("Viewer mutation allowed", action, w.Code)
		}
	}
}

func TestPreviewRepublishDoesNotPromoteImplicitly(t *testing.T) {
	a := testApp(t)
	p := testProject(t, a)
	d, e := a.queued(p, "zip", p.Branch, "", "preview", "build", "", "", "acceptance")
	if e != nil {
		t.Fatal(e)
	}
	d.State = "published"
	_ = a.store.saveDeploy(d)
	u := testUser(t, a, "owner")
	c, csrf := testSession(t, a, u)
	path := "/api/projects/" + p.ID + "/deployments/" + d.ID
	if w := request(t, a, "POST", path+"/rollback", map[string]any{}, c, csrf); w.Code != 400 {
		t.Fatal("Preview rollback was accepted", w.Code)
	}
	for _, item := range []struct{ action, environment string }{{"republish", "preview"}, {"promote", "production"}} {
		w := request(t, a, "POST", path+"/"+item.action, map[string]any{}, c, csrf)
		var result Deploy
		_ = json.Unmarshal(w.Body.Bytes(), &result)
		if w.Code != 202 || result.Environment != item.environment {
			t.Fatal(item.action, w.Code, w.Body.String())
		}
	}
}
