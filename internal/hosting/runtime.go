package hosting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RuntimePin struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Strip   bool   `json:"strip"`
	Exe     string `json:"executable"`
}

var runtimePins = map[string]RuntimePin{
	"caddy":  {ID: "caddy", Version: "2.11.7", URL: "https://github.com/caddyserver/caddy/releases/download/v2.11.7/caddy_2.11.7_windows_amd64.zip", SHA256: "0a1edc0b799512051c57071ce0e798d3f2cf67dc98171366f1d2b326072e3b06", Exe: "caddy.exe"},
	"node22": {ID: "node22", Version: "22.23.3", URL: "https://nodejs.org/dist/v22.23.3/node-v22.23.3-win-x64.zip", SHA256: "2b0ff57b049cda1bbcea2240eec20467018713c1efe1f7360c2681859b90ed71", Strip: true, Exe: "node.exe"},
	"node24": {ID: "node24", Version: "24.22.0", URL: "https://nodejs.org/dist/v24.22.0/node-v24.22.0-win-x64.zip", SHA256: "f9683a19877e12a9b167305c8db71040d812876359de21f0d2e97c3f321f80c3", Strip: true, Exe: "node.exe"},
}

type runtimeReceipt struct {
	Pin    RuntimePin        `json:"pin"`
	Hashes map[string]string `json:"hashes"`
}
type runtimeManager struct {
	mu       sync.Mutex
	gateOnce sync.Once
	gate     chan struct{}
	root     string
	status   map[string]string
}

func hashFile(p string) (string, error) {
	f, e := os.Open(p)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	_, e = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), e
}
func (m *runtimeManager) path(key string) string { return filepath.Join(m.root, key) }
func (m *runtimeManager) ready(key string) error {
	pin, ok := runtimePins[key]
	if !ok {
		return errors.New("Unknown runtime")
	}
	raw, e := os.ReadFile(filepath.Join(m.path(key), "verified.json"))
	if e != nil {
		return errors.New("Runtime is not installed; provision it in Settings")
	}
	var receipt runtimeReceipt
	if e = decode(raw, &receipt); e != nil || receipt.Pin.SHA256 != pin.SHA256 {
		return errors.New("Runtime verification record is invalid")
	}
	for _, name := range []string{pin.Exe, "node_modules/npm/bin/npm-cli.js"} {
		if key == "caddy" && name != pin.Exe {
			continue
		}
		sum, e := hashFile(filepath.Join(m.path(key), filepath.FromSlash(name)))
		if e != nil || receipt.Hashes[name] != sum {
			return errors.New("Runtime checksum changed; re-provision the verified official archive")
		}
	}
	return nil
}
func (m *runtimeManager) provision(ctx context.Context, key, offline string, report func(string), replace ...bool) error {
	repair := len(replace) > 0 && replace[0]
	m.gateOnce.Do(func() { m.gate = make(chan struct{}, 1) })
	select {
	case m.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-m.gate }()
	pin, ok := runtimePins[key]
	if !ok {
		return errors.New("Unknown runtime")
	}
	if m.ready(key) == nil && !repair {
		return nil
	}
	if report == nil {
		report = func(string) {}
	}
	originalReport := report
	report = func(message string) { m.mu.Lock(); m.status[key] = message; m.mu.Unlock(); originalReport(message) }
	report("Preparing " + key + " " + pin.Version)
	m.mu.Lock()
	m.status[key] = "downloading"
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.ready(key) == nil {
			m.status[key] = "ready"
		} else {
			m.status[key] = "needs attention"
		}
	}()
	zipPath := filepath.Join(m.root, "download-"+id()+".zip")
	defer os.Remove(zipPath)
	if offline == "" {
		if e := download(ctx, pin.URL, zipPath, "", 128<<20, report); e != nil {
			return e
		}
	} else {
		in, e := os.Open(offline)
		if e != nil {
			return e
		}
		out, e := os.Create(zipPath)
		if e != nil {
			in.Close()
			return e
		}
		_, e = io.Copy(out, io.LimitReader(in, 128<<20+1))
		out.Close()
		in.Close()
		if e != nil {
			return e
		}
	}
	h, e := hashFile(zipPath)
	if e != nil || h != pin.SHA256 {
		return errors.New("Official runtime archive checksum does not match the pinned release")
	}
	report("Verifying and extracting " + key)
	stage := filepath.Join(m.root, "stage-"+id())
	if e = os.Mkdir(stage, 0700); e != nil {
		return e
	}
	defer os.RemoveAll(stage)
	if e = extractZIP(zipPath, stage, pin.Strip); e != nil {
		return e
	}
	receipt := runtimeReceipt{Pin: pin, Hashes: map[string]string{}}
	for _, name := range []string{pin.Exe, "node_modules/npm/bin/npm-cli.js"} {
		if key == "caddy" && name != pin.Exe {
			continue
		}
		sum, e := hashFile(filepath.Join(stage, filepath.FromSlash(name)))
		if e != nil {
			return e
		}
		receipt.Hashes[name] = sum
	}
	if e = os.WriteFile(filepath.Join(stage, "verified.json"), []byte(encode(receipt)), 0600); e != nil {
		return e
	}
	dest := m.path(key)
	previous := ""
	if _, e = os.Stat(dest); e == nil {
		if !repair {
			return errors.New("An incomplete runtime directory exists. Choose Repair runtime in Settings to replace it with a verified archive")
		}
		if _, e = treeSize(dest); e != nil {
			return errors.New("The runtime folder contains an unsafe link; inspect it before repair")
		}
		previous = filepath.Join(m.root, "previous-"+key+"-"+id())
		if e = os.Rename(dest, previous); e != nil {
			return errors.New("Cannot replace the runtime. Stop other programs using it and check Windows permissions, then retry repair")
		}
	}
	if e = os.Rename(stage, dest); e != nil {
		if previous != "" {
			if restoreErr := os.Rename(previous, dest); restoreErr != nil {
				return fmt.Errorf("Runtime replacement failed and recovery needs attention: %w", restoreErr)
			}
		}
		return e
	}
	if previous != "" {
		if e = os.RemoveAll(previous); e != nil {
			report("Runtime is ready; previous runtime cleanup needs attention: " + previous)
		}
	}
	report(key + " is ready")
	return nil
}
func download(ctx context.Context, raw, dest, token string, max int64, report func(string)) error {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil {
		return errors.New("Only HTTPS provider downloads are permitted")
	}
	allowed := func(host string) bool {
		return host == "api.github.com" || host == "github.com" || host == "codeload.github.com" || host == "release-assets.githubusercontent.com" || host == "objects.githubusercontent.com" || host == "nodejs.org"
	}
	if !allowed(u.Hostname()) {
		return errors.New("Download provider is not allowed")
	}
	client := &http.Client{Timeout: 15 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || req.URL.Scheme != "https" || !allowed(req.URL.Hostname()) {
			return errors.New("Unexpected download redirect")
		}
		if req.URL.Hostname() != "api.github.com" {
			req.Header.Del("Authorization")
		}
		return nil
	}}
	r, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if e != nil {
		return e
	}
	r.Header.Set("User-Agent", "OneWorkspace-MyHosting/"+Version)
	if token != "" && u.Hostname() == "api.github.com" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	resp, e := client.Do(r)
	if e != nil {
		return errors.New("Download failed; check internet access and provider availability")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Provider download returned HTTP %d; check repository access and credentials", resp.StatusCode)
	}
	if resp.ContentLength > max {
		return errors.New("Download exceeds the size limit")
	}
	f, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	var n int64
	buf := make([]byte, 1<<20)
	next := int64(8 << 20)
	for {
		count, re := resp.Body.Read(buf)
		if count > 0 {
			n += int64(count)
			if n > max {
				return errors.New("Download exceeded its size limit")
			}
			if _, e = f.Write(buf[:count]); e != nil {
				return e
			}
			if report != nil && n >= next {
				total := "unknown total"
				if resp.ContentLength > 0 {
					total = strconv.FormatInt(resp.ContentLength>>20, 10) + " MiB"
				}
				report(fmt.Sprintf("Downloaded %d MiB / %s", n>>20, total))
				next += 8 << 20
			}
		}
		if re == io.EOF {
			break
		}
		if re != nil {
			return re
		}
	}
	return f.Sync()
}
func (m *runtimeManager) summary() []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []map[string]any{}
	for _, key := range []string{"caddy", "node22", "node24"} {
		pin := runtimePins[key]
		state := "not installed"
		if m.status[key] != "" && m.status[key] != "ready" {
			state = m.status[key]
		} else if m.ready(key) == nil {
			state = "ready"
		}
		out = append(out, map[string]any{"id": key, "version": pin.Version, "state": state, "source": pin.URL, "sha256": pin.SHA256})
	}
	return out
}
func decode(b []byte, v any) error { return jsonDecode(strings.NewReader(string(b)), v) }
