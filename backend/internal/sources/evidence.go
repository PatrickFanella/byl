package sources

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"invisible-museum/backend/internal/models"
)

const evidenceMaxBytes = 2 << 20
const evidenceMaxRunes = 20000

var errEvidenceRedirect = errors.New("unsafe or excessive evidence redirect")

// NormalizeEvidenceURL rejects credentials, non-web schemes and nonstandard ports.
// It removes fragments from source identity but retains them separately as references.
func NormalizeEvidenceURL(raw string) (normalized, reference, key string, err error) {
	if len(raw) > 2000 {
		return "", "", "", errors.New("source URL exceeds 2000 bytes")
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", "", "", errors.New("source URL must be HTTP or HTTPS without credentials")
	}
	standardPort := "80"
	if u.Scheme == "https" {
		standardPort = "443"
	}
	if port := u.Port(); port != "" && port != standardPort {
		return "", "", "", errors.New("source URL must use the standard HTTP or HTTPS port")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" || len(host) > 253 || strings.Contains(host, "%") {
		return "", "", "", errors.New("invalid source host")
	}
	if ip := net.ParseIP(host); ip != nil && !publicIP(ip) {
		return "", "", "", ErrPrivateHost
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	u.Host = host
	reference = u.Fragment
	u.Fragment = ""
	u.RawFragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	normalized = u.String()
	digest := sha256.Sum256([]byte(normalized))
	key = hex.EncodeToString(digest[:])
	return normalized, reference, key, nil
}

// EvidenceRetriever is the bounded interface shared by durable jobs and later research adapters.
type EvidenceRetriever interface {
	RetrieveEvidence(context.Context, string, string) (models.EvidenceResult, error)
}

// RetrieveEvidence makes at most three HTTP requests (two redirects), with no
// automatic retry and a total 20-second deadline. Private access is never enabled,
// even when this client allows trusted private media/archive URLs elsewhere.
func (c *Client) RetrieveEvidence(ctx context.Context, raw, kind string) (models.EvidenceResult, error) {
	result := models.EvidenceResult{Outcome: "blocked", RetrievedAt: time.Now().UTC(), Untrusted: true, MediaReuse: "unresolved", Passages: []models.EvidencePassage{}}
	page, _, _, err := NormalizeEvidenceURL(raw)
	if err != nil {
		return result, nil
	}
	u, _ := url.Parse(page)
	result.FinalURL = page
	result.Publisher = u.Hostname()
	if kind == "search_snippet" {
		result.Outcome = "search_snippet"
		return result, nil
	}
	if kind != "page" && kind != "catalog" {
		return result, nil
	}
	limited, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	hc := *c.HTTP
	if hc.Timeout <= 0 || hc.Timeout > 20*time.Second {
		hc.Timeout = 20 * time.Second
	}
	hc.Jar = nil
	hc.Transport = publicOnlyTransport(c.HTTP.Transport)
	// Tests supply a guarded transport with deterministic DNS/dial behavior.
	if c.evidenceTransport != nil {
		hc.Transport = c.evidenceTransport
	}
	if transport, ok := hc.Transport.(*http.Transport); ok {
		defer transport.CloseIdleConnections()
	}
	hc.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errEvidenceRedirect
		}
		if _, _, _, err := NormalizeEvidenceURL(next.URL.String()); err != nil {
			return errEvidenceRedirect
		}
		if via[len(via)-1].URL.Scheme == "https" && next.URL.Scheme != "https" {
			return errEvidenceRedirect
		}
		next.Header.Del("Referer")
		return c.wait(limited, next.URL.Host)
	}
	if err := c.wait(limited, u.Host); err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		result.Outcome = "timeout"
		return result, nil
	}
	req, err := http.NewRequestWithContext(limited, http.MethodGet, page, nil)
	if err != nil {
		return result, nil
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "text/html, text/plain;q=0.9")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := hc.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		switch {
		case errors.Is(err, ErrPrivateHost) || errors.Is(err, errEvidenceRedirect):
			result.Outcome = "blocked"
		case limited.Err() != nil:
			result.Outcome = "timeout"
		default:
			var timeout net.Error
			if errors.As(err, &timeout) && timeout.Timeout() {
				result.Outcome = "timeout"
			} else {
				result.Outcome = "unavailable"
			}
		}
		return result, nil
	}
	defer func() { _ = resp.Body.Close() }()
	result.FinalURL = resp.Request.URL.String()
	result.Publisher = resp.Request.URL.Hostname()
	result.HTTPStatus = resp.StatusCode
	result.ContentType, _, _ = mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK {
		result.Outcome = "inaccessible"
		return result, nil
	}
	if resp.ContentLength > evidenceMaxBytes {
		result.Outcome = "too_large"
		return result, nil
	}
	if result.ContentType != "text/html" && result.ContentType != "text/plain" {
		result.Outcome = "unsupported_format"
		return result, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, evidenceMaxBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		var timeout net.Error
		if limited.Err() != nil || (errors.As(err, &timeout) && timeout.Timeout()) {
			result.Outcome = "timeout"
		} else {
			result.Outcome = "unavailable"
		}
		return result, nil
	}
	if len(body) > evidenceMaxBytes {
		result.Outcome = "too_large"
		return result, nil
	}
	if !utf8.Valid(body) {
		result.Outcome = "unsupported_encoding"
		return result, nil
	}
	digest := sha256.Sum256(body)
	result.ContentSHA256 = hex.EncodeToString(digest[:])
	text := string(body)
	if result.ContentType == "text/html" {
		result.Title, text = ExtractArticle(text)
	}
	if result.Title == "" {
		result.Title = result.Publisher
	}
	result.Title = string([]rune(result.Title)[:min(len([]rune(result.Title)), 500)])
	if len(strings.Fields(text)) < 20 {
		result.Outcome = "no_readable_text"
		return result, nil
	}
	remaining := evidenceMaxRunes
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		runes := []rune(line)
		if len(runes) > remaining {
			runes = runes[:remaining]
			result.Truncated = true
		}
		result.Passages = append(result.Passages, models.EvidencePassage{Reference: fmt.Sprintf("paragraph:%d", len(result.Passages)+1), Text: string(runes)})
		remaining -= len(runes)
		if remaining == 0 || len(result.Passages) == 100 {
			result.Truncated = true
			break
		}
	}
	result.Outcome = "inspected_page"
	if kind == "catalog" {
		result.Outcome = "catalog_only"
	}
	return result, nil
}
