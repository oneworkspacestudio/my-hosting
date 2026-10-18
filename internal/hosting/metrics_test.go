package hosting

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMetricsFlatPublishedHostDoesNotBlockHistoryConnection(t *testing.T) {
	a := testApp(t)
	s := a.settings()
	s.PreviewDomain = "example.test"
	s.AddressStyle = "flat"
	_ = a.store.setSetting("hosting", s)
	p := testProject(t, a)
	d, e := a.queued(p, "zip", p.Branch, "", "production", "build", "", "", "acceptance")
	if e != nil {
		t.Fatal(e)
	}
	d.State = "published"
	if e := a.store.saveDeploy(d); e != nil {
		t.Fatal(e)
	}
	host := siteHostForSettings(d, s)
	lines := encode(map[string]any{"status": 200, "request": map[string]string{"host": host}}) + "\n" + encode(map[string]any{"status": 502, "request": map[string]string{"host": host + ":443"}}) + "\n" + encode(map[string]any{"status": 200, "request": map[string]string{"host": "other.example.test"}}) + "\n"
	if e := os.WriteFile(filepath.Join(a.cfg.Data, "logs", "access.json"), []byte(lines), 0600); e != nil {
		t.Fatal(e)
	}
	u := testUser(t, a, "owner")
	c, csrf := testSession(t, a, u)
	w := request(t, a, "GET", "/api/projects/"+p.ID+"/metrics", nil, c, csrf)
	var metrics struct {
		Requests     int `json:"requests"`
		ServerErrors int `json:"serverErrors"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &metrics) != nil || metrics.Requests != 2 || metrics.ServerErrors != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
}
