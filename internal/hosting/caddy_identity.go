package hosting

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

type caddyCredential struct{ Certificate, Key []byte }

func (a *App) caddyIdentity() (tls.Certificate, string, error) {
	path := filepath.Join(a.cfg.Data, "secrets", "caddy-control.dpapi")
	var saved caddyCredential
	raw, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			return tls.Certificate{}, "", e
		}
		serial, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if e != nil {
			return tls.Certificate{}, "", e
		}
		template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "My Hosting private configuration client"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(10, 0, 0), BasicConstraintsValid: true, IsCA: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
		der, e := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		if e != nil {
			return tls.Certificate{}, "", e
		}
		keyDER, e := x509.MarshalPKCS8PrivateKey(key)
		if e != nil {
			return tls.Certificate{}, "", e
		}
		saved = caddyCredential{Certificate: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), Key: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})}
		sealed, e := protect([]byte(encode(saved)))
		if e != nil {
			return tls.Certificate{}, "", e
		}
		if e = atomicFile(path, sealed); e != nil {
			return tls.Certificate{}, "", e
		}
	} else if e != nil {
		return tls.Certificate{}, "", e
	} else {
		plain, e := unprotect(raw)
		if e != nil {
			return tls.Certificate{}, "", e
		}
		if e = json.Unmarshal(plain, &saved); e != nil {
			return tls.Certificate{}, "", e
		}
	}
	cert, e := tls.X509KeyPair(saved.Certificate, saved.Key)
	if e != nil {
		return cert, "", e
	}
	return cert, base64.StdEncoding.EncodeToString(cert.Certificate[0]), nil
}
func (a *App) adminConfig() (map[string]any, error) {
	_, public, e := a.caddyIdentity()
	if e != nil {
		return nil, e
	}
	if a.caddyAdminPort == 0 {
		a.caddyAdminPort, e = freePort()
		if e != nil {
			return nil, e
		}
	}
	return map[string]any{"disabled": true, "identity": map[string]any{"identifiers": []string{"control.myhosting.localhost"}, "issuers": []any{map[string]any{"module": "internal", "ca": "hosting-control"}}}, "remote": map[string]any{"listen": fmt.Sprintf("127.0.0.1:%d", a.caddyAdminPort), "access_control": []any{map[string]any{"public_keys": []string{public}, "permissions": []any{map[string]any{"paths": []string{"/load"}, "methods": []string{"POST"}}}}}}}, nil
}
func (a *App) caddyTLS() (*tls.Config, error) {
	cert, _, e := a.caddyIdentity()
	if e != nil {
		return nil, e
	}
	root, e := os.ReadFile(filepath.Join(a.cfg.Data, "caddy", "state", "pki", "authorities", "hosting-control", "root.crt"))
	if e != nil {
		return nil, e
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(root) {
		return nil, errors.New("Private Caddy control CA is invalid")
	}
	return &tls.Config{RootCAs: pool, ServerName: "control.myhosting.localhost", Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, nil
}
