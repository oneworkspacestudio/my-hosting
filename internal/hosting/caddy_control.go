package hosting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) caddyLoad(ctx context.Context, body []byte) error {
	config, e := a.caddyTLS()
	if e != nil {
		return e
	}
	transport := &http.Transport{TLSClientConfig: config}

	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, e := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("https://127.0.0.1:%d/load", a.caddyAdminPort), bytes.NewReader(body))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	response, e := client.Do(req)
	if e != nil {
		return errors.New("The protected Caddy configuration connection failed")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("Caddy did not accept the configuration (HTTP %d); previous routes are retained", response.StatusCode)
	}
	_, e = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	return e
}
func (a *App) probeCaddy(ctx context.Context, body []byte) error {
	// Decode to one consistent JSON representation for both generated and saved configs.
	var config map[string]any
	if e := json.Unmarshal(body, &config); e != nil {
		return e
	}
	local := config["apps"].(map[string]any)["http"].(map[string]any)["servers"].(map[string]any)["local"].(map[string]any)
	route := local["routes"].([]any)[0].(map[string]any)
	revision := route["handle"].([]any)[0].(map[string]any)["response"].(map[string]any)["set"].(map[string]any)["X-MyHosting-Deploy"].([]any)[0].(string)
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return waitHealth(probeCtx, "http://"+local["listen"].([]any)[0].(string)+"/__myhosting_health", "control.myhosting.localhost", revision)
}
func (a *App) startCaddy(configPath string, environment []string) error {
	listener, e := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", a.settings().CaddyPort))
	if e != nil {
		return errors.New("The site port is already in use; choose a free port in Settings")
	}
	listener.Close()
	log, e := os.OpenFile(filepath.Join(a.cfg.Data, "logs", "caddy.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	run := exec.Command(filepath.Join(a.runtime.path("caddy"), "caddy.exe"), "run", "--config", configPath)
	run.Dir = a.cfg.Data
	run.Env = environment
	run.Stdout = log
	run.Stderr = log
	job, e := startProcess(run, "", 0, false)
	if e != nil {
		log.Close()
		return e
	}
	done := make(chan struct{})
	a.processMu.Lock()
	a.caddy = job
	a.caddyDone = done
	a.caddyError = ""
	a.processMu.Unlock()
	go func() {
		e := job.Wait()
		job.Close()
		log.Close()
		a.processMu.Lock()
		if a.caddy == job {
			a.caddy = nil
			if e != nil {
				a.caddyError = "Caddy stopped; inspect its process log and port permissions"
			}
		}
		a.processMu.Unlock()
		close(done)
	}()
	return nil
}
func (a *App) stopCaddy() error {
	a.processMu.Lock()
	job, done := a.caddy, a.caddyDone
	a.caddy = nil
	a.processMu.Unlock()
	if job == nil {
		return nil
	}
	job.Close()
	select {
	case <-done:
		return nil
	case <-time.After(10 * time.Second):
		return errors.New("Caddy did not stop within ten seconds")
	}
}
func (a *App) applyCaddyConfig(ctx context.Context, config map[string]any) error {
	body := []byte(encode(config))
	path := filepath.Join(a.cfg.Data, "caddy", "caddy.json")
	temp := filepath.Join(a.cfg.Data, "caddy", "validate-"+id()+".json")
	if e := os.WriteFile(temp, body, 0600); e != nil {
		return e
	}
	defer os.Remove(temp)
	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	check := exec.CommandContext(checkCtx, filepath.Join(a.runtime.path("caddy"), "caddy.exe"), "validate", "--config", temp)
	check.Env = []string{"SystemRoot=" + os.Getenv("SystemRoot"), "APPDATA=" + filepath.Join(a.cfg.Data, "caddy"), "LOCALAPPDATA=" + filepath.Join(a.cfg.Data, "caddy"), "TEMP=" + filepath.Join(a.cfg.Data, "cache")}
	if e := check.Run(); e != nil {
		return errors.New("Caddy rejected the generated configuration; previous routes are retained")
	}
	old, _ := os.ReadFile(path)
	if e := atomicFile(path, body); e != nil {
		return e
	}
	a.processMu.Lock()
	running := a.caddy != nil
	a.processMu.Unlock()
	var e error
	if running {
		e = a.caddyLoad(ctx, body)
	} else {
		e = a.startCaddy(path, check.Env)
	}
	if e == nil {
		e = a.probeCaddy(ctx, body)
	}
	if e != nil {
		if len(old) > 0 {
			restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer restoreCancel()
			fileErr := atomicFile(path, old)
			loadErr := a.caddyLoad(restoreCtx, old)
			if fileErr != nil || loadErr != nil || a.probeCaddy(restoreCtx, old) != nil {
				return fmt.Errorf("%w; previous routing could not be restored automatically: inspect Caddy logs", e)
			}
		} else {
			_ = a.stopCaddy()
			_ = os.Remove(path)
		}
		return e
	}
	return nil
}
func (a *App) reservedSiteDomain(host string) bool {
	s := a.settings()
	parent := s.PreviewDomain
	if parent != "" {
		if s.AddressStyle == "flat" {
			label := strings.TrimSuffix(host, "."+parent)
			if label != host && !strings.Contains(label, ".") && strings.HasSuffix(label, "-sites") {
				return true
			}
		} else if host == parent || strings.HasSuffix(host, "."+parent) {
			return true
		}
	}
	for _, raw := range []string{a.cfg.Origin, a.cfg.Issuer} {
		u, e := url.Parse(raw)
		if e == nil && strings.EqualFold(u.Hostname(), host) {
			return true
		}
	}
	return false
}
