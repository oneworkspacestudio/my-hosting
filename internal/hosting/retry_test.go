package hosting

import (
	"encoding/json"
	"os"
	"testing"
)

func TestRetryUsesOriginalEnvironmentAndRebuildUsesCurrent(t *testing.T) {
	a := testApp(t)
	p := testProject(t, a)
	_ = a.putSecret(p.ID, "API_KEY", "production", "original-synthetic-value")
	d, e := a.queued(p, "zip", p.Branch, "", "production", "build", "", "", "acceptance")
	if e != nil {
		t.Fatal(e)
	}
	_ = os.WriteFile(a.sourcePath(d), []byte("synthetic retained archive"), 0600)
	d.SourceHash = digest([]byte("synthetic retained archive"))
	d.State = "failed"
	_ = a.store.saveDeploy(d)
	_ = a.putSecret(p.ID, "API_KEY", "production", "updated-synthetic-value")
	u := testUser(t, a, "owner")
	cookie, csrf := testSession(t, a, u)
	for _, item := range []struct{ action, value string }{{"retry", "original-synthetic-value"}, {"rebuild", "updated-synthetic-value"}} {
		w := request(t, a, "POST", "/api/projects/"+p.ID+"/deployments/"+d.ID+"/"+item.action, map[string]any{}, cookie, csrf)
		var next Deploy
		_ = json.Unmarshal(w.Body.Bytes(), &next)
		if w.Code != 202 || next.State != "queued" {
			t.Fatal(item.action, w.Code, w.Body.String())
		}
		env, e := a.deployEnv(next)
		if e != nil || env["API_KEY"] != item.value {
			t.Fatal("Wrong environment snapshot", item.action, e)
		}
	}
}

func TestCancelledPreparationCannotBecomeQueued(t *testing.T) {
	a := testApp(t)
	p := testProject(t, a)
	d, e := a.queued(p, "zip", p.Branch, "", "production", "build", "", "", "acceptance", true)
	if e != nil {
		t.Fatal(e)
	}
	u := testUser(t, a, "owner")
	cookie, csrf := testSession(t, a, u)
	w := request(t, a, "POST", "/api/projects/"+p.ID+"/deployments/"+d.ID+"/cancel", map[string]any{}, cookie, csrf)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	_ = os.WriteFile(a.sourcePath(d), []byte("late upload completion"), 0600)
	got, e := a.finishPreparation(d)
	if e != nil || got.State != "cancelled" {
		t.Fatal("Preparation resurrected a cancelled job", got.State, e)
	}
	if _, e = os.Stat(a.sourcePath(d)); !os.IsNotExist(e) {
		t.Fatal("Cancelled upload source was retained", e)
	}
}
