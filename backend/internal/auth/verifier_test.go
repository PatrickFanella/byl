package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type fakeJWKS struct {
	key   *rsa.PrivateKey
	kid   string
	hits  atomic.Int32
	serve bool
}

func (f *fakeJWKS) handler(w http.ResponseWriter, _ *http.Request) {
	f.hits.Add(1)
	if !f.serve {
		w.WriteHeader(500)
		return
	}
	pub := f.key.PublicKey
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": f.kid, "use": "sig", "alg": "RS256",
		"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

func (f *fakeJWKS) sign(t *testing.T, claims jwt.MapClaims, kid string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(f.key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newFakeJWKS(t *testing.T) (*fakeJWKS, *httptest.Server) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeJWKS{key: key, kid: "k1", serve: true}
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	return f, srv
}

func baseClaims(aud string) jwt.MapClaims {
	return jwt.MapClaims{
		"iss": "https://issuer.example", "aud": aud, "sub": "subject-1", "email": "p@example.com", "email_verified": true, "name": "Pat",
		"iat": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	}
}

func TestVerifierAcceptsValidToken(t *testing.T) {
	f, srv := newFakeJWKS(t)
	v := NewJWKSVerifier(srv.URL, []string{"https://issuer.example"}, []string{"client-a", "client-b"}, time.Hour)
	id, err := v.Verify(context.Background(), f.sign(t, baseClaims("client-b"), "k1"))
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "subject-1" || id.Email != "p@example.com" || id.Name != "Pat" {
		t.Fatalf("identity %+v", id)
	}
	if _, err := v.Verify(context.Background(), f.sign(t, baseClaims("client-a"), "k1")); err != nil {
		t.Fatalf("second verify should reuse cached keys: %v", err)
	}
	if f.hits.Load() != 1 {
		t.Fatalf("expected 1 JWKS fetch, got %d", f.hits.Load())
	}
}

func TestVerifierRejects(t *testing.T) {
	f, srv := newFakeJWKS(t)
	v := NewJWKSVerifier(srv.URL, []string{"https://issuer.example"}, []string{"client-a"}, time.Hour)
	ctx := context.Background()

	if _, err := v.Verify(ctx, f.sign(t, baseClaims("someone-else"), "k1")); err == nil {
		t.Fatal("wrong audience accepted")
	}
	expired := baseClaims("client-a")
	expired["exp"] = time.Now().Add(-time.Minute).Unix()
	if _, err := v.Verify(ctx, f.sign(t, expired, "k1")); err == nil {
		t.Fatal("expired token accepted")
	}
	badIss := baseClaims("client-a")
	badIss["iss"] = "https://evil.example"
	if _, err := v.Verify(ctx, f.sign(t, badIss, "k1")); err == nil {
		t.Fatal("wrong issuer accepted")
	}
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, baseClaims("client-a"))
	tok.Header["kid"] = "k1"
	forged, _ := tok.SignedString(other)
	if _, err := v.Verify(ctx, forged); err == nil {
		t.Fatal("token signed by another key accepted")
	}
	if _, err := v.Verify(ctx, "not-a-jwt"); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestVerifierRefetchesOnUnknownKid(t *testing.T) {
	f, srv := newFakeJWKS(t)
	v := NewJWKSVerifier(srv.URL, []string{"https://issuer.example"}, []string{"client-a"}, time.Hour)
	ctx := context.Background()
	if _, err := v.Verify(ctx, f.sign(t, baseClaims("client-a"), "k1")); err != nil {
		t.Fatal(err)
	}
	f.kid = "k2"
	if _, err := v.Verify(ctx, f.sign(t, baseClaims("client-a"), "k2")); err != nil {
		t.Fatalf("rotated key should be fetched: %v", err)
	}
	if f.hits.Load() != 2 {
		t.Fatalf("expected 2 fetches, got %d", f.hits.Load())
	}
	if _, err := v.Verify(ctx, f.sign(t, baseClaims("client-a"), "k9")); err == nil {
		t.Fatal("unknown kid accepted")
	}
	for _, kid := range []string{"k10", "k11", "k12"} {
		if _, err := v.Verify(ctx, f.sign(t, baseClaims("client-a"), kid)); err == nil {
			t.Fatal("unknown kid accepted")
		}
	}
	if f.hits.Load() != 2 {
		t.Fatalf("unknown keys must not trigger repeated fetches, got %d", f.hits.Load())
	}
}

func TestAppleAndGoogleDefaults(t *testing.T) {
	a := NewApple([]string{"com.example.app"}).(*jwksVerifier)
	if a.jwksURL != "https://appleid.apple.com/auth/keys" || a.issuers[0] != "https://appleid.apple.com" {
		t.Fatalf("apple: %+v", a)
	}
	g := NewGoogle([]string{"x.apps.googleusercontent.com"}).(*jwksVerifier)
	if g.jwksURL != "https://www.googleapis.com/oauth2/v3/certs" || len(g.issuers) != 2 {
		t.Fatalf("google: %+v", g)
	}
}

func TestVerifierDropsUnverifiedEmail(t *testing.T) {
	f, srv := newFakeJWKS(t)
	v := NewJWKSVerifier(srv.URL, []string{"https://issuer.example"}, []string{"client-a"}, time.Hour)
	claims := baseClaims("client-a")
	claims["email_verified"] = false
	id, err := v.Verify(context.Background(), f.sign(t, claims, "k1"))
	if err != nil || id.Email != "" {
		t.Fatalf("unverified email must be dropped: %v %+v", err, id)
	}
	claims["email_verified"] = "true"
	id, _ = v.Verify(context.Background(), f.sign(t, claims, "k1"))
	if id.Email != "p@example.com" {
		t.Fatalf("string true (Apple style) must count as verified: %+v", id)
	}
	delete(claims, "email_verified")
	id, _ = v.Verify(context.Background(), f.sign(t, claims, "k1"))
	if id.Email != "" {
		t.Fatalf("missing email_verified must not be trusted: %+v", id)
	}
}
