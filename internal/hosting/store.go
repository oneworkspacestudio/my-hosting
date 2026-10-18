package hosting

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	db      *sql.DB
	auditMu sync.Mutex
}

func openStore(root string) (*Store, error) {
	d, e := sql.Open("sqlite", filepath.Join(root, "data", "myhosting.db"))
	if e != nil {
		return nil, e
	}
	d.SetMaxOpenConns(1)
	schema := `PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=10000;
 CREATE TABLE IF NOT EXISTS schema_version(version INTEGER NOT NULL); INSERT INTO schema_version SELECT 1 WHERE NOT EXISTS(SELECT 1 FROM schema_version);
 CREATE TABLE IF NOT EXISTS users(id TEXT PRIMARY KEY,issuer TEXT NOT NULL,subject TEXT NOT NULL,name TEXT NOT NULL,email TEXT NOT NULL,role TEXT NOT NULL,password BLOB,UNIQUE(issuer,subject));
 CREATE TABLE IF NOT EXISTS sessions(id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id),csrf TEXT NOT NULL,expires TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS projects(id TEXT PRIMARY KEY,slug TEXT UNIQUE NOT NULL,config TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS members(project_id TEXT NOT NULL REFERENCES projects(id),user_id TEXT NOT NULL REFERENCES users(id),role TEXT NOT NULL,PRIMARY KEY(project_id,user_id));
 CREATE TABLE IF NOT EXISTS deployments(id TEXT PRIMARY KEY,project_id TEXT NOT NULL REFERENCES projects(id),state TEXT NOT NULL,created TEXT NOT NULL,detail TEXT NOT NULL,idempotency TEXT,UNIQUE(project_id,idempotency));
 CREATE INDEX IF NOT EXISTS deploy_history ON deployments(project_id,created DESC); CREATE INDEX IF NOT EXISTS job_state ON deployments(state,created);
 CREATE TABLE IF NOT EXISTS deployment_env(deploy_id TEXT PRIMARY KEY REFERENCES deployments(id),value BLOB NOT NULL);
 CREATE TABLE IF NOT EXISTS domains(host TEXT PRIMARY KEY,project_id TEXT NOT NULL REFERENCES projects(id),detail TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS secrets(project_id TEXT NOT NULL,name TEXT NOT NULL,environment TEXT NOT NULL,value BLOB NOT NULL,PRIMARY KEY(project_id,name,environment));
 CREATE TABLE IF NOT EXISTS settings(name TEXT PRIMARY KEY,value TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS audit(seq INTEGER PRIMARY KEY AUTOINCREMENT,at TEXT NOT NULL,actor TEXT NOT NULL,project_id TEXT NOT NULL,action TEXT NOT NULL,detail TEXT NOT NULL,previous TEXT NOT NULL,hash TEXT NOT NULL);
 CREATE TRIGGER IF NOT EXISTS audit_no_update BEFORE UPDATE ON audit BEGIN SELECT RAISE(ABORT,'audit is append-only'); END;
 CREATE TRIGGER IF NOT EXISTS audit_no_delete BEFORE DELETE ON audit BEGIN SELECT RAISE(ABORT,'audit is append-only'); END;`
	if _, e = d.Exec(schema); e != nil {
		d.Close()
		return nil, e
	}
	var version int
	if e = d.QueryRow("SELECT version FROM schema_version").Scan(&version); e != nil || version != 1 {
		d.Close()
		return nil, errors.New("Unsupported database schema; restore the matching application version")
	}
	return &Store{db: d}, nil
}
func (s *Store) project(pid string) (Project, error) {
	var p Project
	var raw string
	e := s.db.QueryRow("SELECT config FROM projects WHERE id=?", pid).Scan(&raw)
	if e == nil {
		e = json.Unmarshal([]byte(raw), &p)
	}
	return p, e
}
func (s *Store) saveProject(p Project) error {
	_, e := s.db.Exec("INSERT INTO projects(id,slug,config) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET slug=excluded.slug,config=excluded.config", p.ID, p.Slug, encode(p))
	return e
}
func (s *Store) deploy(did string) (Deploy, error) {
	var d Deploy
	var raw string
	e := s.db.QueryRow("SELECT detail FROM deployments WHERE id=?", did).Scan(&raw)
	if e == nil {
		e = json.Unmarshal([]byte(raw), &d)
	}
	return d, e
}
func (s *Store) saveDeploy(d Deploy) error {
	_, e := s.db.Exec("UPDATE deployments SET state=?,detail=? WHERE id=?", d.State, encode(d), d.ID)
	return e
}
func (s *Store) audit(actor, pid, action string, detail any) error {
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	var prev string
	e := s.db.QueryRow("SELECT hash FROM audit ORDER BY seq DESC LIMIT 1").Scan(&prev)
	if e != nil && e != sql.ErrNoRows {
		return e
	}
	at := now()
	body := encode(detail)
	h := digest([]byte(encode([]string{prev, at, actor, pid, action, body})))
	_, e = s.db.Exec("INSERT INTO audit(at,actor,project_id,action,detail,previous,hash) VALUES(?,?,?,?,?,?,?)", at, actor, pid, action, body, prev, h)
	return e
}
func (s *Store) setting(key string, out any) error {
	var raw string
	e := s.db.QueryRow("SELECT value FROM settings WHERE name=?", key).Scan(&raw)
	if e == nil {
		e = json.Unmarshal([]byte(raw), out)
	}
	return e
}
func (s *Store) setSetting(key string, v any) error {
	_, e := s.db.Exec("INSERT INTO settings(name,value) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET value=excluded.value", key, encode(v))
	return e
}
func (s *Store) role(u User, pid string) string {
	if u.Role == "owner" || u.Role == "admin" {
		return u.Role
	}
	var role string
	_ = s.db.QueryRow("SELECT role FROM members WHERE project_id=? AND user_id=?", pid, u.ID).Scan(&role)
	return role
}
func (s *Store) projects(u User) ([]Project, error) {
	rows, e := s.db.Query("SELECT config FROM projects ORDER BY slug")
	if e != nil {
		return nil, e
	}
	var all []Project
	for rows.Next() {
		var raw string
		var p Project
		if e = rows.Scan(&raw); e != nil {
			rows.Close()
			return nil, e
		}
		if e = json.Unmarshal([]byte(raw), &p); e != nil {
			rows.Close()
			return nil, e
		}
		all = append(all, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	out := []Project{}
	for _, p := range all {
		p.Role = s.role(u, p.ID)
		if p.Role != "" {
			out = append(out, p)
		}
	}
	return out, nil
}
func (s *Store) history(pid string, limit, offset int) ([]Deploy, error) {
	rows, e := s.db.Query("SELECT detail FROM deployments WHERE project_id=? ORDER BY created DESC LIMIT ? OFFSET ?", pid, limit, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Deploy{}
	for rows.Next() {
		var raw string
		var d Deploy
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(raw), &d); e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func initDirs(root string) error {
	for _, name := range []string{"data", "projects", "logs", "cache", "secrets", "caddy", "runtimes", "backups"} {
		if e := os.MkdirAll(filepath.Join(root, name), 0700); e != nil {
			return e
		}
	}
	return nil
}
func dbError(e error) error {
	if e == sql.ErrNoRows {
		return errors.New("Item not found")
	}
	return fmt.Errorf("Storage operation failed: %w", e)
}
