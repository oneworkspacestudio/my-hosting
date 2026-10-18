package hosting

import (
	"archive/zip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/crypto/scrypt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const backupMagic = "MHBACKUP1\n"

type encryptedWriter struct {
	out     io.Writer
	aead    cipher.AEAD
	prefix  []byte
	counter uint32
	pending []byte
}

func encryptStream(out io.Writer, password string) (*encryptedWriter, error) {
	salt := make([]byte, 16)
	prefix := make([]byte, 8)
	if _, e := rand.Read(salt); e != nil {
		return nil, e
	}
	if _, e := rand.Read(prefix); e != nil {
		return nil, e
	}
	key, e := scrypt.Key([]byte(password), salt, 32768, 8, 1, 32)
	if e != nil {
		return nil, e
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	aead, e := cipher.NewGCM(block)
	if e != nil {
		return nil, e
	}
	header := append(append([]byte(backupMagic), salt...), prefix...)
	if _, e = out.Write(header); e != nil {
		return nil, e
	}
	return &encryptedWriter{out: out, aead: aead, prefix: prefix}, nil
}
func (w *encryptedWriter) chunk(b []byte) error {
	nonce := make([]byte, 12)
	copy(nonce, w.prefix)
	binary.BigEndian.PutUint32(nonce[8:], w.counter)
	w.counter++
	ciphertext := w.aead.Seal(nil, nonce, b, []byte(backupMagic))
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(ciphertext)))
	if _, e := w.out.Write(size[:]); e != nil {
		return e
	}
	_, e := w.out.Write(ciphertext)
	return e
}
func (w *encryptedWriter) Write(b []byte) (int, error) {
	n := len(b)
	w.pending = append(w.pending, b...)
	for len(w.pending) >= 64<<10 {
		if e := w.chunk(w.pending[:64<<10]); e != nil {
			return 0, e
		}
		w.pending = w.pending[64<<10:]
	}
	return n, nil
}
func (w *encryptedWriter) Close() error {
	if len(w.pending) > 0 {
		if e := w.chunk(w.pending); e != nil {
			return e
		}
	}
	w.pending = nil
	return w.chunk(nil)
}
func decryptStream(in io.Reader, out io.Writer, password string) error {
	header := make([]byte, len(backupMagic)+24)
	if _, e := io.ReadFull(in, header); e != nil {
		return errors.New("Backup header is incomplete")
	}
	if string(header[:len(backupMagic)]) != backupMagic {
		return errors.New("Not a My Hosting encrypted backup")
	}
	key, e := scrypt.Key([]byte(password), header[len(backupMagic):len(backupMagic)+16], 32768, 8, 1, 32)
	if e != nil {
		return e
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return e
	}
	aead, e := cipher.NewGCM(block)
	if e != nil {
		return e
	}
	var counter uint32
	var total int64
	for {
		var size [4]byte
		if _, e = io.ReadFull(in, size[:]); e != nil {
			return errors.New("Backup is truncated; no restore was committed")
		}
		n := binary.BigEndian.Uint32(size[:])
		if n < 16 || n > (64<<10)+16 {
			return errors.New("Invalid encrypted backup frame")
		}
		b := make([]byte, n)
		if _, e = io.ReadFull(in, b); e != nil {
			return errors.New("Backup is truncated")
		}
		nonce := make([]byte, 12)
		copy(nonce, header[len(backupMagic)+16:])
		binary.BigEndian.PutUint32(nonce[8:], counter)
		counter++
		plain, e := aead.Open(nil, nonce, b, []byte(backupMagic))
		if e != nil {
			return errors.New("Wrong password or damaged backup; authentication failed")
		}
		if len(plain) == 0 {
			var extra [1]byte
			if n, e := in.Read(extra[:]); n != 0 || e != io.EOF {
				return errors.New("Unexpected bytes after the final authenticated backup record")
			}
			return nil
		}
		total += int64(len(plain))
		if total > 10<<30 {
			return errors.New("Backup exceeds the 10 GiB restore limit")
		}
		if _, e = out.Write(plain); e != nil {
			return e
		}
	}
}
func (a *App) backupAPI(w http.ResponseWriter, r *http.Request, s Session) {
	if !requireAdmin(w, s) {
		return
	}
	if r.Method != "POST" {
		fail(w, 405, errors.New("Use Export backup with an encryption password"))
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if len(in.Password) < 8 || len(in.Password) > 1024 {
		fail(w, 400, errors.New("Use a backup password of at least eight characters and keep it separately"))
		return
	}
	if !a.jobsGate.TryLock() {
		fail(w, 409, errors.New("Wait for the active deployment before exporting a consistent backup"))
		return
	}
	defer a.jobsGate.Unlock()
	a.publishMu.Lock()
	defer a.publishMu.Unlock()
	a.processMu.Lock()
	ids := []string{}
	for did := range a.processes {
		ids = append(ids, did)
	}
	a.processMu.Unlock()
	for _, did := range ids {
		a.stopNode(did)
	}
	if e := a.stopCaddy(); e != nil {
		fail(w, 500, e)
		return
	}
	defer func() { _ = a.restoreProcesses(a.ctx); _ = a.reloadRoutes(a.ctx, nil) }()
	path := filepath.Join(a.cfg.Data, "backups", "myhosting-"+id()+".mhbackup")
	if e := a.makeBackup(path, in.Password); e != nil {
		os.Remove(path)
		fail(w, 500, e)
		return
	}
	_ = a.store.audit(s.User.ID, "", "backup.exported", map[string]string{"format": "MHBACKUP1", "credentials": "encrypted and portable; password required"})
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(path))
	w.Header().Set("Cache-Control", "no-store")
	f, e := os.Open(path)
	if e != nil {
		fail(w, 500, dbError(e))
		return
	}
	defer f.Close()
	io.Copy(w, f)
}
func (a *App) makeBackup(dest, password string) error {
	snapshot := filepath.Join(a.cfg.Data, "backups", "snapshot-"+id()+".db")
	defer os.Remove(snapshot)
	if _, e := a.store.db.Exec("VACUUM INTO ?", snapshot); e != nil {
		return errors.New("Could not create a consistent SQLite snapshot")
	}
	db, e := sql.Open("sqlite", snapshot)
	if e != nil {
		return e
	}
	_, e = db.Exec("DELETE FROM sessions")
	if e == nil {
		_, e = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	}
	db.Close()
	if e != nil {
		return e
	}
	out, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer out.Close()
	encrypted, e := encryptStream(out, password)
	if e != nil {
		return e
	}
	z := zip.NewWriter(encrypted)
	add := func(src, name string) error {
		entry, e := z.Create(name)
		if e != nil {
			return e
		}
		f, e := os.Open(src)
		if e != nil {
			return e
		}
		defer f.Close()
		_, e = io.Copy(entry, f)
		return e
	}
	if e = add(snapshot, "data/myhosting.db"); e != nil {
		return e
	}
	for _, area := range []string{"projects", "logs", "caddy"} {
		root := filepath.Join(a.cfg.Data, area)
		e = filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.Type()&os.ModeSymlink != 0 {
				return errors.New("Backup rejected a linked file/directory")
			}
			rel, e := filepath.Rel(root, p)
			if e != nil {
				return e
			}
			if d.IsDir() {
				if strings.HasPrefix(rel, "builds") || d.Name() == ".npm-cache" || d.Name() == "builds" {
					return filepath.SkipDir
				}
				return nil
			}
			return add(p, area+"/"+filepath.ToSlash(rel))
		})
		if e != nil {
			return e
		}
	}
	rows, e := a.store.db.Query("SELECT project_id,name,environment,value FROM secrets")
	if e != nil {
		return e
	}
	secrets := []map[string]string{}
	for rows.Next() {
		var pid, name, env string
		var raw []byte
		if e = rows.Scan(&pid, &name, &env, &raw); e != nil {
			rows.Close()
			return e
		}
		plain, e := unprotect(raw)
		if e != nil {
			rows.Close()
			return e
		}
		secrets = append(secrets, map[string]string{"project": pid, "name": name, "environment": env, "value": string(plain)})
	}
	rows.Close()
	entry, e := z.Create("portable-secrets.json")
	if e != nil {
		return e
	}
	if e = json.NewEncoder(entry).Encode(secrets); e != nil {
		return e
	}
	envRows, e := a.store.db.Query("SELECT deploy_id,value FROM deployment_env")
	if e != nil {
		return e
	}
	portableEnvs := []map[string]string{}
	for envRows.Next() {
		var did string
		var raw []byte
		if e = envRows.Scan(&did, &raw); e != nil {
			envRows.Close()
			return e
		}
		plain, e := unprotect(raw)
		if e != nil {
			envRows.Close()
			return e
		}
		portableEnvs = append(portableEnvs, map[string]string{"deploy": did, "value": string(plain)})
	}
	envRows.Close()
	envEntry, e := z.Create("portable-deployment-env.json")
	if e != nil {
		return e
	}
	if e = json.NewEncoder(envEntry).Encode(portableEnvs); e != nil {
		return e
	}
	entry, e = z.Create("backup-manifest.json")
	if e != nil {
		return e
	}
	if e = json.NewEncoder(entry).Encode(map[string]any{"schema": 1, "version": Version, "created": now(), "origin": a.cfg.Origin, "sessions": "excluded", "runtimes": "re-provision verified official pins"}); e != nil {
		return e
	}
	if e = z.Close(); e != nil {
		return e
	}
	if e = encrypted.Close(); e != nil {
		return e
	}
	return out.Sync()
}
func Restore(source, dest, password string) error {
	if _, e := os.Stat(dest); !os.IsNotExist(e) {
		return errors.New("Restore requires a new, empty private-data path; existing installations are never overwritten")
	}
	if e := prepareData(dest); e != nil {
		return e
	}
	temp := filepath.Join(dest, "backups", "restore.zip")
	defer os.Remove(temp)
	in, e := os.Open(source)
	if e != nil {
		return e
	}
	out, e := os.Create(temp)
	if e != nil {
		in.Close()
		return e
	}
	e = decryptStream(in, out, password)
	out.Close()
	in.Close()
	if e != nil {
		return e
	}
	stage := filepath.Join(dest, "backups", "restore-stage")
	if e = os.Mkdir(stage, 0700); e != nil {
		return e
	}
	defer os.RemoveAll(stage)
	if e = extractZIPLimit(temp, stage, false, 10<<30); e != nil {
		return e
	}
	manifest, e := os.ReadFile(filepath.Join(stage, "backup-manifest.json"))
	if e != nil {
		return errors.New("Backup manifest is missing")
	}
	var m struct {
		Schema   int    `json:"schema"`
		Version  string `json:"version"`
		Created  string `json:"created"`
		Origin   string `json:"origin"`
		Sessions string `json:"sessions"`
		Runtimes string `json:"runtimes"`
	}
	if e = json.Unmarshal(manifest, &m); e != nil || m.Schema != 1 || m.Version != Version {
		return errors.New("Restore using the matching My Hosting version before upgrading")
	}
	var secrets []map[string]string
	raw, e := os.ReadFile(filepath.Join(stage, "portable-secrets.json"))
	if e != nil {
		return e
	}
	if e = json.Unmarshal(raw, &secrets); e != nil {
		return e
	}
	db, e := sql.Open("sqlite", filepath.Join(stage, "data", "myhosting.db"))
	if e != nil {
		return e
	}
	for _, s := range secrets {
		encrypted, e := protect([]byte(s["value"]))
		if e != nil {
			db.Close()
			return e
		}
		if _, e = db.Exec("UPDATE secrets SET value=? WHERE project_id=? AND name=? AND environment=?", encrypted, s["project"], s["name"], s["environment"]); e != nil {
			db.Close()
			return e
		}
	}
	envRaw, e := os.ReadFile(filepath.Join(stage, "portable-deployment-env.json"))
	if e != nil {
		db.Close()
		return e
	}
	var envs []map[string]string
	if e = json.Unmarshal(envRaw, &envs); e != nil {
		db.Close()
		return e
	}
	for _, env := range envs {
		encrypted, e := protect([]byte(env["value"]))
		if e != nil {
			db.Close()
			return e
		}
		if _, e = db.Exec("UPDATE deployment_env SET value=? WHERE deploy_id=?", encrypted, env["deploy"]); e != nil {
			db.Close()
			return e
		}
	}
	var settingsRaw string
	if e = db.QueryRow("SELECT value FROM settings WHERE name='hosting'").Scan(&settingsRaw); e == nil {
		var settings Settings
		if json.Unmarshal([]byte(settingsRaw), &settings) == nil {
			settings.Public = false
			settings.PreviewDomain = ""
			_, e = db.Exec("UPDATE settings SET value=? WHERE name='hosting'", encode(settings))
			if e != nil {
				db.Close()
				return e
			}
		}
	}
	_, e = db.Exec("DELETE FROM sessions; PRAGMA wal_checkpoint(TRUNCATE)")
	db.Close()
	if e != nil {
		return e
	}
	for _, area := range []string{"data", "projects", "logs", "caddy"} {
		src := filepath.Join(stage, area)
		if _, e = os.Stat(src); os.IsNotExist(e) {
			continue
		}
		target := filepath.Join(dest, area)
		if lenList, e := os.ReadDir(target); e != nil || len(lenList) > 0 {
			return fmt.Errorf("Restore target %s is not empty", area)
		}
		if e = os.Remove(target); e != nil {
			return e
		}
		if e = os.Rename(src, target); e != nil {
			return e
		}
	}
	return atomicFile(filepath.Join(dest, "restore-result.json"), []byte(encode(map[string]any{"state": "restored", "version": Version, "sourceOrigin": m.Origin, "next": "Configure this host's OIDC client/callback and domain routing. Re-provision runtimes; old browser sessions were discarded."})))
}
