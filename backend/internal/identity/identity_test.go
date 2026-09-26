package identity

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/coreos/go-oidc/v3/oidc"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSignedCloudflareIdentity(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "test-key", "n": enc(key.N.Bytes()), "e": enc(big.NewInt(int64(key.E)).Bytes())}}})
	}))
	defer server.Close()
	issuer := "https://test.cloudflareaccess.com"
	auth := &Auth{origin: "https://storyforge.example", admins: map[string]bool{"dad@example.test": true}, verifier: oidc.NewVerifier(issuer, oidc.NewRemoteKeySet(context.Background(), server.URL), &oidc.Config{ClientID: "storyforge-aud", SupportedSigningAlgs: []string{"RS256"}})}
	sign := func(claims map[string]any) string {
		head := enc([]byte(`{"alg":"RS256","kid":"test-key"}`))
		payload, _ := json.Marshal(claims)
		message := head + "." + enc(payload)
		digest := sha256.Sum256([]byte(message))
		sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		return message + "." + enc(sig)
	}
	for _, tc := range []struct {
		name, iss, aud, email string
		expiry                int64
		valid                 bool
	}{
		{"valid admin", issuer, "storyforge-aud", "dad@example.test", time.Now().Add(time.Hour).Unix(), true},
		{"wrong issuer", "https://other.cloudflareaccess.com", "storyforge-aud", "dad@example.test", time.Now().Add(time.Hour).Unix(), false},
		{"wrong application", issuer, "other-aud", "dad@example.test", time.Now().Add(time.Hour).Unix(), false},
		{"expired", issuer, "storyforge-aud", "dad@example.test", time.Now().Add(-time.Hour).Unix(), false},
		{"service identity", issuer, "storyforge-aud", "", time.Now().Add(time.Hour).Unix(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/session", nil)
			r.Header.Set("Cf-Access-Jwt-Assertion", sign(map[string]any{"iss": tc.iss, "aud": []string{tc.aud}, "email": tc.email, "sub": "person", "exp": tc.expiry, "iat": time.Now().Add(-time.Minute).Unix()}))
			u, err := auth.Identify(r)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if tc.valid && (!u.Admin || u.Email != tc.email) {
				t.Fatalf("bad user %+v", u)
			}
		})
	}
	r := httptest.NewRequest("GET", "/api/session", nil)
	r.Header.Set("Cf-Access-Authenticated-User-Email", "dad@example.test")
	if _, err = auth.Identify(r); err == nil {
		t.Fatal("trusted unsigned email header")
	}
	token := sign(map[string]any{"iss": issuer, "aud": []string{"storyforge-aud"}, "email": "dad@example.test", "exp": time.Now().Add(time.Hour).Unix()})
	parts := strings.Split(token, ".")
	parts[1] = enc([]byte(`{"email":"intruder@example.test"}`))
	r.Header.Set("Cf-Access-Jwt-Assertion", strings.Join(parts, "."))
	if _, err = auth.Identify(r); err == nil {
		t.Fatal("accepted tampered JWT")
	}
}

func TestOriginAndConfiguration(t *testing.T) {
	a := &Auth{origin: "https://storyforge.example"}
	for _, tc := range []struct {
		origin, site string
		want         bool
	}{{"https://storyforge.example", "same-origin", true}, {"https://evil.example", "cross-site", false}, {"", "cross-site", false}} {
		r := httptest.NewRequest("POST", "/api/projects", nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Sec-Fetch-Site", tc.site)
		if a.SameOrigin(r) != tc.want {
			t.Fatal("origin policy")
		}
	}
	t.Setenv("STORYFORGE_PUBLIC_ORIGIN", "https://storyforge.example")
	t.Setenv("STORYFORGE_ADMIN_EMAILS", "dad@example.test")
	t.Setenv("CF_ACCESS_TEAM_DOMAIN", "https://test.cloudflareaccess.com")
	t.Setenv("CF_ACCESS_AUD", "app")
	t.Setenv("STORYFORGE_AUTH_MODE", "")
	if _, err := FromEnv(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STORYFORGE_AUTH_MODE", "development")
	t.Setenv("STORYFORGE_DEV_AUTH_TOKEN", strings.Repeat("a", 32))
	if _, err := FromEnv(); err == nil {
		t.Fatal("development mode accepted public origin")
	}
}
