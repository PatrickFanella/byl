package sources

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"invisible-museum/backend/internal/models"
)

const (
	urlMaxBytes = 2 << 20
	urlMaxText  = 20000
)

// URL turns configured local archive pages into moment finds with the page text.
type URL struct {
	client *Client
	urls   []string

	mu    sync.Mutex
	skips []string
}

// NewURL builds the adapter over a fixed list of pages.
func NewURL(client *Client, urls []string) *URL {
	return &URL{client: client, urls: urls}
}

func (a *URL) Name() string { return "url" }

// LastSkips reports pages the last Discover could not use, with reasons.
func (a *URL) LastSkips() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.skips...)
}

func (a *URL) Discover(ctx context.Context, req Request) ([]Found, error) {
	var out []Found
	var skips []string
	query := strings.ToLower(strings.TrimSpace(req.Query))
	for _, page := range a.urls {
		page = strings.TrimSpace(page)
		if page == "" {
			continue
		}
		f, err := a.fetch(ctx, page)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return out, err
			}
			skips = append(skips, page+": "+err.Error())
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(f.Text), query) && !strings.Contains(strings.ToLower(f.Title), query) {
			continue
		}
		out = append(out, f)
	}
	a.mu.Lock()
	a.skips = skips
	a.mu.Unlock()
	return out, nil
}

func (a *URL) fetch(ctx context.Context, page string) (Found, error) {
	parsed, err := url.Parse(page)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return Found{}, fmt.Errorf("not an http(s) url")
	}
	resp, err := a.client.GetWithRedirectCheck(ctx, page, func(r *http.Request, _ []*http.Request) error {
		if r.URL.Host != parsed.Host {
			return fmt.Errorf("redirects off-site to %s", r.URL.Host)
		}
		return nil
	})
	if err != nil {
		return Found{}, err
	}
	defer resp.Body.Close()
	if resp.ContentLength > urlMaxBytes {
		return Found{}, fmt.Errorf("page larger than %d bytes", urlMaxBytes)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, urlMaxBytes+1))
	if err != nil {
		return Found{}, err
	}
	if len(body) > urlMaxBytes {
		return Found{}, fmt.Errorf("page larger than %d bytes", urlMaxBytes)
	}
	title, text := ExtractArticle(string(body))
	if len(strings.Fields(text)) < 20 {
		return Found{}, fmt.Errorf("no readable article text")
	}
	if len(text) > urlMaxText {
		text = text[:urlMaxText]
	}
	sum := sha256.Sum256([]byte(page))
	if title == "" {
		title = parsed.Host + parsed.Path
	}
	return Found{
		Kind: models.CandidateKindMoment, ExternalID: "url:" + hex.EncodeToString(sum[:]),
		Title: title, Text: text, URL: page, Credit: parsed.Host,
	}, nil
}
