package hosting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var cfEndpoint = "https://api.cloudflare.com/client/v4"

type siteDNS struct {
	ID, Type, Name, Content string
	Proxied                 bool
}

func (a *App) siteCF(ctx context.Context, method, path string, body, out any) error {
	var buffer bytes.Buffer
	if body != nil {
		if e := json.NewEncoder(&buffer).Encode(body); e != nil {
			return e
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, cfEndpoint+path, &buffer)
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.SiteCFToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, e := client.Do(req)
	if e != nil {
		return errors.New("Cannot connect to Cloudflare; verify the connection in Workspace")
	}
	defer response.Body.Close()
	var result struct {
		Success bool            `json:"success"`
		Result  json.RawMessage `json:"result"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&result) != nil || response.StatusCode >= 300 || !result.Success {
		return fmt.Errorf("Cloudflare refused site setup (HTTP %d). Reconnect Cloudflare in Workspace with DNS Edit and Tunnel Edit permissions", response.StatusCode)
	}
	if out != nil {
		return json.Unmarshal(result.Result, out)
	}
	return nil
}

// Register exact, first-level site hostnames. Never install a zone-wide wildcard
// ingress or replace existing DNS/routes belonging to another service.
func (a *App) ensureHostedRoute(ctx context.Context, d Deploy) error {
	s := a.settings()
	if !s.Public || s.Transport != "tunnel" || a.cfg.SiteCFToken == "" {
		return nil
	}
	if s.AddressStyle != "flat" {
		return errors.New("Automatic free tunnel publishing uses flat site addresses; nested addresses need separate edge certificate coverage")
	}
	if !validID.MatchString(a.cfg.SiteCFAccount) || !validID.MatchString(a.cfg.SiteCFZone) {
		return errors.New("Workspace Cloudflare account/zone needs reconnecting")
	}
	tunnel := a.cfg.SiteCFTunnel
	if len(tunnel) != 36 || strings.Trim(tunnel, "abcdefABCDEF0123456789-") != "" {
		return errors.New("Workspace tunnel identifier is invalid")
	}
	unlock, e := acquireTunnelLock(ctx, a.cfg.SiteCFAccount, tunnel)
	if e != nil {
		return e
	}
	defer unlock()
	var zone struct {
		Name    string
		Account struct{ ID string }
	}
	if e := a.siteCF(ctx, "GET", "/zones/"+a.cfg.SiteCFZone, nil, &zone); e != nil {
		return e
	}
	if zone.Name != s.PreviewDomain || zone.Account.ID != a.cfg.SiteCFAccount {
		return errors.New("Site parent domain must match the connected Cloudflare zone")
	}
	host := a.siteHost(d)
	label := strings.TrimSuffix(host, "."+zone.Name)
	if strings.Contains(label, ".") || !strings.HasSuffix(label, "-sites") || len(label) > 63 {
		return errors.New("Site hostname is outside the connected first-level sites namespace")
	}
	dnsPath := "/zones/" + a.cfg.SiteCFZone + "/dns_records"
	var records []siteDNS
	if e := a.siteCF(ctx, "GET", dnsPath+"?name="+url.QueryEscape(host), nil, &records); e != nil {
		return e
	}
	target := tunnel + ".cfargotunnel.com"
	if len(records) > 0 && (len(records) != 1 || records[0].Type != "CNAME" || !records[0].Proxied || !strings.EqualFold(strings.TrimSuffix(records[0].Content, "."), target)) {
		return errors.New("This site's hostname is already used by another DNS record. Choose another site name; existing DNS was preserved")
	}
	path := "/accounts/" + a.cfg.SiteCFAccount + "/cfd_tunnel/" + tunnel + "/configurations"
	var remote struct{ Config map[string]any }
	if e := a.siteCF(ctx, "GET", path, nil, &remote); e != nil {
		return e
	}
	routes, ok := remote.Config["ingress"].([]any)
	if !ok || len(routes) == 0 {
		return errors.New("Workspace tunnel has no valid ingress configuration")
	}
	service := fmt.Sprintf("http://127.0.0.1:%d", s.CaddyPort)
	found := false
	for _, entry := range routes {
		rule, ok := entry.(map[string]any)
		if !ok {
			return errors.New("Workspace tunnel route format needs repair")
		}
		if rule["hostname"] == host {
			if rule["service"] != service || (rule["path"] != nil && rule["path"] != "") {
				return errors.New("The site address already routes to a different service; its route was preserved")
			}
			found = true
		}
	}
	if !found {
		last, ok := routes[len(routes)-1].(map[string]any)
		if !ok || (last["hostname"] != nil && last["hostname"] != "") {
			return errors.New("Workspace tunnel needs a final catch-all rule")
		}
		route := map[string]any{"hostname": host, "service": service, "originRequest": map[string]any{"httpHostHeader": host}}
		next := append([]any{}, routes[:len(routes)-1]...)
		next = append(next, route, last)
		remote.Config["ingress"] = next
		if e := a.siteCF(ctx, "PUT", path, map[string]any{"config": remote.Config}, nil); e != nil {
			return e
		}
	}
	if len(records) == 0 {
		if e := a.siteCF(ctx, "POST", dnsPath, map[string]any{"type": "CNAME", "name": host, "content": target, "proxied": true, "ttl": 1}, nil); e != nil {
			return e
		}
	}
	_ = a.store.audit("publisher", d.ProjectID, "site.route", map[string]string{"hostname": host, "transport": "cloudflare-tunnel"})
	return nil
}
