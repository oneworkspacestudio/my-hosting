package hosting

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Fresh names can remain cached as an old wildcard/NXDOMAIN in Windows or a
// router. A public resolver and an IP-pinned dial avoid that false failure.
// TLS still verifies the real hostname and the ordinary public certificate chain.
func publicSiteAddresses(ctx context.Context, host string) ([]net.IP, error) {
	endpoint := "https://cloudflare-dns.com/dns-query?type=A&name=" + url.QueryEscape(host)
	req, e := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Accept", "application/dns-json")
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	defer response.Body.Close()
	var answer struct {
		Status int
		Answer []struct {
			Type int
			Data string
		}
	}
	if response.StatusCode != 200 || json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&answer) != nil || answer.Status != 0 {
		return nil, errors.New("Public DNS is still propagating")
	}
	out := []net.IP{}
	for _, record := range answer.Answer {
		ip := net.ParseIP(record.Data)
		if record.Type == 1 && ip != nil && publicAddress(ip) {
			out = append(out, ip)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("Public DNS has no usable site address yet")
	}
	return out, nil
}
func waitPublicSite(ctx context.Context, raw, deployment string, report func(string)) error {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Host != u.Hostname() {
		return errors.New("Public probe requires an HTTPS hostname without credentials or a port")
	}
	nextReport := time.Time{}
	var last error
	for ctx.Err() == nil {
		addresses, e := publicSiteAddresses(ctx, u.Hostname())
		if e == nil {
			for _, ip := range addresses {
				target := net.JoinHostPort(ip.String(), "443")
				transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname()}, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
					return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, target)
				}}
				client := &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
				req, _ := http.NewRequestWithContext(ctx, "GET", raw, nil)
				response, requestErr := client.Do(req)
				if requestErr == nil {
					_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
					response.Body.Close()
					if response.StatusCode >= 200 && response.StatusCode < 400 && response.Header.Get("X-MyHosting-Deploy") == deployment {
						transport.CloseIdleConnections()
						return nil
					}
					last = fmt.Errorf("Public edge returned HTTP %d before the expected release was visible", response.StatusCode)
				} else {
					last = errors.New("Public HTTPS connection/certificate is not ready")
				}
				transport.CloseIdleConnections()
			}
		} else {
			last = e
		}
		if report != nil && time.Now().After(nextReport) {
			message := "Waiting for DNS propagation, the tunnel route and a verified public HTTPS response"
			if last != nil {
				message += " · " + strings.TrimSpace(last.Error())
			}
			report(message)
			nextReport = time.Now().Add(10 * time.Second)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return ctx.Err()
}
