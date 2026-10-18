package hosting

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type cleanupResult struct {
	Files   int   `json:"files"`
	Bytes   int64 `json:"bytes"`
	Expired int   `json:"expired"`
}

// Cleanup is explicit. Keep the configured newest deployments, all active
// preview routes, live production and any artifacts/sources they reference.
// Metadata, environment snapshots, audit entries, mutable app data and backups
// are never removed by retention.
func (a *App) cleanupStorage() (cleanupResult, error) {
	var result cleanupResult
	if !a.jobsGate.TryLock() {
		return result, errors.New("Wait for deployments, runtime setup or backups before cleaning storage")
	}
	defer a.jobsGate.Unlock()
	a.publishMu.Lock()
	defer a.publishMu.Unlock()
	retain := a.settings().Retain
	if retain < 2 || retain > 100 {
		return result, errors.New("Choose a retention target between 2 and 100 before cleanup")
	}
	projects, e := a.store.projects(User{Role: "owner"})
	if e != nil {
		return result, e
	}
	for _, p := range projects {
		if !validID.MatchString(p.ID) {
			return result, errors.New("Invalid project storage identifier")
		}
		deploys, e := a.store.history(p.ID, 100001, 0)
		if e != nil {
			return result, e
		}
		if len(deploys) > 100000 {
			return result, errors.New("Project history needs administrator maintenance before cleanup")
		}
		byID := map[string]Deploy{}
		keepRecords := map[string]bool{}
		keepArtifacts := map[string]bool{}
		keepSources := map[string]bool{}
		for i, d := range deploys {
			if !validID.MatchString(d.ID) || d.ProjectID != p.ID {
				return result, errors.New("Invalid deployment storage identifier")
			}
			byID[d.ID] = d
			if i < retain || d.ID == p.Live || (d.Environment == "preview" && d.State == "published") || d.State == "queued" || d.State == "preparing" || d.State == "uploading" || d.State == "building" || d.State == "publishing" {
				keepRecords[d.ID] = true
			}
		}
		for did := range keepRecords {
			d := byID[did]
			artifact := d.Artifact
			if artifact == "" {
				artifact = d.ID
			}
			keepArtifacts[artifact] = true
			seen := map[string]bool{}
			for validID.MatchString(d.ID) && !seen[d.ID] {
				seen[d.ID] = true
				keepSources[d.ID] = true
				parent, ok := byID[d.Parent]
				if !ok {
					break
				}
				d = parent
			}
		}
		removedArtifacts := map[string]bool{}
		for _, d := range deploys {
			if keepRecords[d.ID] {
				continue
			}
			artifact := d.Artifact
			if artifact == "" {
				artifact = d.ID
			}
			if !validID.MatchString(artifact) {
				return result, errors.New("Invalid retained artifact identifier")
			}
			if !keepArtifacts[artifact] && !removedArtifacts[artifact] {
				if e = a.removeRetentionPath(filepath.Join(a.cfg.Data, "projects", p.ID, "releases", artifact), &result); e != nil {
					return result, e
				}
				removedArtifacts[artifact] = true
			}
			if e = a.removeRetentionPath(a.workspace(d), &result); e != nil {
				return result, e
			}
			if !keepSources[d.ID] {
				if e = a.removeRetentionPath(a.sourcePath(d), &result); e != nil {
					return result, e
				}
				for _, suffix := range []string{".log", "-process.log"} {
					if e = a.removeRetentionPath(filepath.Join(a.cfg.Data, "logs", d.ID+suffix), &result); e != nil {
						return result, e
					}
				}
			}
			if d.State == "published" && !keepArtifacts[artifact] {
				d.State = "expired"
				d.URL = ""
				d.Error = "Runtime artifact removed by the configured retention policy. Deployment metadata and audit history remain; rebuild or upload source to deploy again."
				if e = a.store.saveDeploy(d); e != nil {
					return result, e
				}
				result.Expired++
			}
		}
	}
	return result, nil
}

func (a *App) removeRetentionPath(path string, result *cleanupResult) error {
	rel, e := filepath.Rel(a.cfg.Data, path)
	if e != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("Cleanup path escaped private app storage")
	}
	// Restrict callers to immutable deployment storage and logs. Never allow a
	// future caller to pass secrets, mutable var folders, the database or backups.
	parts := strings.Split(filepath.ToSlash(rel), "/")
	allowed := false
	if len(parts) == 4 && parts[0] == "projects" && validID.MatchString(parts[1]) {
		if parts[2] == "releases" || parts[2] == "builds" {
			allowed = validID.MatchString(parts[3])
		}
		if parts[2] == "sources" {
			name := strings.TrimPrefix(parts[3], "source-")
			name = strings.TrimSuffix(strings.TrimSuffix(name, ".tar.gz"), ".zip")
			allowed = strings.HasPrefix(parts[3], "source-") && validID.MatchString(name)
		}
	}
	if len(parts) == 2 && parts[0] == "logs" && strings.HasSuffix(parts[1], ".log") {
		allowed = validID.MatchString(strings.TrimSuffix(strings.TrimSuffix(parts[1], ".log"), "-process"))
	}
	if !allowed {
		return errors.New("Cleanup path is not disposable deployment storage")
	}
	for probe := path; ; probe = filepath.Dir(probe) {
		entry, e := os.Lstat(probe)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if e == nil && entry.Mode()&os.ModeSymlink != 0 {
			return errors.New("Cleanup refused a linked ancestor")
		}
		if probe == filepath.Clean(a.cfg.Data) {
			break
		}
		if filepath.Dir(probe) == probe {
			return errors.New("Cleanup ancestor escaped its data root")
		}
	}
	info, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Cleanup refused a linked path")
	}
	var size int64
	if info.IsDir() {
		size, e = treeSize(path)
		if e != nil {
			return e
		}
	} else {
		size = info.Size()
	}
	files := 0
	if e = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !entry.IsDir() {
			files++
		}
		return nil
	}); e != nil {
		return e
	}
	if e = os.RemoveAll(path); e != nil {
		return e
	}
	result.Bytes += size
	result.Files += files
	return nil
}

func (a *App) cleanupAPI(w http.ResponseWriter, r *http.Request, s Session) {
	if !requireAdmin(w, s) {
		return
	}
	if r.Method != "POST" {
		http.NotFound(w, r)
		return
	}
	result, e := a.cleanupStorage()
	if e != nil {
		fail(w, 409, e)
		return
	}
	_ = a.store.audit(s.User.ID, "", "storage.cleanup", result)
	jsonOut(w, 200, result)
}
