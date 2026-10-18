package hosting

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

func TestOIDCCodePKCENonceOwnerAndRejection(t *testing.T) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	signer, e := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "synthetic-key"))
	if e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"owner", "member", "unverified-email", "wrong-nonce", "wrong-audience", "wrong-issuer", "expired"} {
		t.Run(mode, func(t *testing.T) {
			a := testApp(t)
			var provider *httptest.Server
			nonce, challenge := "", ""
			provider = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/.well-known/openid-configuration":
					_ = json.NewEncoder(w).Encode(map[string]any{"issuer": provider.URL, "authorization_endpoint": provider.URL + "/authorize", "token_endpoint": provider.URL + "/token", "jwks_uri": provider.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
				case "/keys":
					_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "synthetic-key", Algorithm: "RS256", Use: "sig"}}})
				case "/token":
					_ = r.ParseForm()
					sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
					if challenge == "" || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
						http.Error(w, "PKCE mismatch", 400)
						return
					}
					sub, issuer, aud, tokenNonce, verified, expires := "suite-owner", provider.URL, "fixture-client", nonce, true, time.Now().Add(time.Hour).Unix()
					switch mode {
					case "member":
						sub = "other-member"
					case "unverified-email":
						verified = false
					case "wrong-nonce":
						tokenNonce = "wrong"
					case "wrong-audience":
						aud = "other-app"
					case "wrong-issuer":
						issuer = "https://untrusted.example.test"
					case "expired":
						expires = time.Now().Add(-time.Minute).Unix()
					}
					raw, e := jwt.Signed(signer).Claims(map[string]any{"iss": issuer, "aud": aud, "sub": sub, "exp": expires, "iat": time.Now().Add(-time.Second).Unix(), "nonce": tokenNonce, "name": "Synthetic member", "email": "owner@example.test", "email_verified": verified}).Serialize()
					if e != nil {
						t.Error(e)
						http.Error(w, "signing", 500)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "synthetic-access", "token_type": "Bearer", "expires_in": 3600, "id_token": raw})
				default:
					http.NotFound(w, r)
				}
			}))
			defer provider.Close()
			a.cfg.Issuer = provider.URL
			a.cfg.ClientID = "fixture-client"
			a.cfg.ClientSecret = "fixture-secret"
			a.cfg.Owner = "suite-owner"
			login := httptest.NewRecorder()
			a.Handler().ServeHTTP(login, httptest.NewRequest("GET", "/auth/login", nil))
			if login.Code != 302 {
				t.Fatal(login.Code, login.Body.String())
			}
			authorize, e := url.Parse(login.Header().Get("Location"))
			if e != nil {
				t.Fatal(e)
			}
			nonce = authorize.Query().Get("nonce")
			challenge = authorize.Query().Get("code_challenge")
			if nonce == "" || challenge == "" || authorize.Query().Get("code_challenge_method") != "S256" {
				t.Fatal("Nonce/PKCE missing")
			}
			callback := httptest.NewRequest("GET", "/auth/callback?code=synthetic-code&state="+url.QueryEscape(authorize.Query().Get("state")), nil)
			for _, cookie := range login.Result().Cookies() {
				callback.AddCookie(cookie)
			}
			response := httptest.NewRecorder()
			a.Handler().ServeHTTP(response, callback)
			success := mode == "owner" || mode == "member" || mode == "unverified-email"
			if !success {
				if response.Code != 401 {
					t.Fatal("Invalid identity accepted", response.Code)
				}
				return
			}
			if response.Code != 303 {
				t.Fatal(response.Code, response.Body.String())
			}
			sessionRequest := httptest.NewRequest("GET", "/api/me", nil)
			for _, cookie := range response.Result().Cookies() {
				sessionRequest.AddCookie(cookie)
			}
			session, ok := a.session(sessionRequest)
			if !ok {
				t.Fatal("OIDC session missing")
			}
			expected := "owner"
			if mode == "member" {
				expected = "viewer"
			}
			if session.User.Role != expected {
				t.Fatal("Email-based privilege escalation", session.User.Role)
			}
			if mode == "unverified-email" && session.User.Email != "" {
				t.Fatal("Unverified email trusted")
			}
			replay := httptest.NewRecorder()
			a.Handler().ServeHTTP(replay, callback)
			if replay.Code != 401 {
				t.Fatal("Callback replay accepted")
			}
			logout := httptest.NewRequest("POST", "/auth/logout", nil)
			logout.Header.Set("Origin", a.cfg.Origin)
			for _, cookie := range sessionRequest.Cookies() {
				logout.AddCookie(cookie)
			}
			a.Handler().ServeHTTP(httptest.NewRecorder(), logout)
			if _, ok = a.session(sessionRequest); ok {
				t.Fatal("Signed-out session remained valid")
			}
		})
	}
}
