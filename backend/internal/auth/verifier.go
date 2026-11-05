// Package auth verifies third-party identity tokens and manages API sessions.
package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Identity is what a verified token says about the person.
type Identity struct {
	Subject string
	Email   string
	Name    string
}

// Verifier checks a provider token and returns the identity inside it.
type Verifier interface {
	Verify(ctx context.Context, token string) (Identity, error)
}

// ErrInvalidToken is returned for any token that does not verify.
var ErrInvalidToken = errors.New("invalid identity token")

type jwksVerifier struct {
	jwksURL   string
	issuers   []string
	audiences []string
	cacheTTL  time.Duration
	client    *http.Client

	mu                 sync.Mutex
	keys               map[string]*rsa.PublicKey
	fetchedAt          time.Time
	unknownRefreshedAt time.Time
}

// NewJWKSVerifier verifies RS256 tokens against a JWKS endpoint.
func NewJWKSVerifier(jwksURL string, issuers, audiences []string, cacheTTL time.Duration) Verifier {
	return &jwksVerifier{jwksURL: jwksURL, issuers: issuers, audiences: audiences, cacheTTL: cacheTTL,
		client: &http.Client{Timeout: 10 * time.Second}, keys: map[string]*rsa.PublicKey{}}
}

// NewApple verifies Sign in with Apple identity tokens.
func NewApple(audiences []string) Verifier {
	return NewJWKSVerifier("https://appleid.apple.com/auth/keys", []string{"https://appleid.apple.com"}, audiences, time.Hour)
}

// NewGoogle verifies Google ID tokens.
func NewGoogle(audiences []string) Verifier {
	return NewJWKSVerifier("https://www.googleapis.com/oauth2/v3/certs", []string{"https://accounts.google.com", "accounts.google.com"}, audiences, time.Hour)
}

func (v *jwksVerifier) Verify(ctx context.Context, raw string) (Identity, error) {
	if len(v.audiences) == 0 {
		return Identity{}, fmt.Errorf("%w: provider has no configured client ids", ErrInvalidToken)
	}
	claims := jwt.MapClaims{}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}), jwt.WithExpirationRequired(), jwt.WithLeeway(30*time.Second))
	_, err := parser.ParseWithClaims(raw, claims, func(tok *jwt.Token) (any, error) {
		kid, _ := tok.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("token has no kid")
		}
		return v.key(ctx, kid)
	})
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	iss, _ := claims["iss"].(string)
	if !contains(v.issuers, iss) {
		return Identity{}, fmt.Errorf("%w: unexpected issuer %q", ErrInvalidToken, iss)
	}
	if !v.audienceOK(claims) {
		return Identity{}, fmt.Errorf("%w: unexpected audience", ErrInvalidToken)
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return Identity{}, fmt.Errorf("%w: missing subject", ErrInvalidToken)
	}
	email, _ := claims["email"].(string)
	if !emailVerified(claims["email_verified"]) {
		email = ""
	}
	name, _ := claims["name"].(string)
	return Identity{Subject: sub, Email: email, Name: name}, nil
}

func (v *jwksVerifier) audienceOK(claims jwt.MapClaims) bool {
	switch aud := claims["aud"].(type) {
	case string:
		return contains(v.audiences, aud)
	case []any:
		for _, a := range aud {
			if s, ok := a.(string); ok && contains(v.audiences, s) {
				return true
			}
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// key returns the public key for kid, refetching the JWKS when the kid is unknown or the cache is stale.
func (v *jwksVerifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if k, ok := v.keys[kid]; ok && time.Since(v.fetchedAt) < v.cacheTTL {
		return k, nil
	}
	if _, known := v.keys[kid]; !known && !v.fetchedAt.IsZero() {
		if time.Since(v.unknownRefreshedAt) < 30*time.Second {
			return nil, fmt.Errorf("unknown key id %q", kid)
		}
		v.unknownRefreshedAt = time.Now()
	}
	if err := v.refresh(ctx); err != nil {
		return nil, err
	}
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	v.unknownRefreshedAt = time.Now()
	return nil, fmt.Errorf("unknown key id %q", kid)
}

func (v *jwksVerifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch jwks: status %d", resp.StatusCode)
	}
	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("decode jwks: %w", err)
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if !strings.EqualFold(k.Kty, "RSA") {
			continue
		}
		n, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		e, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	v.keys = keys
	v.fetchedAt = time.Now()
	return nil
}

// emailVerified accepts Google's boolean and Apple's "true" string; anything else is unverified.
func emailVerified(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(x, "true")
	}
	return false
}
