package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Client is the one HTTP client every adapter uses: descriptive User-Agent, timeout,
// one retry on 429/5xx, and a minimum interval between requests to the same host.
type Client struct {
	UserAgent   string
	MinInterval time.Duration
	HTTP        *http.Client

	mu   sync.Mutex
	last map[string]time.Time
}

// NewHTTPClient builds a polite client. Archives throttle aggressively; one request per
// second per host is the default.
func NewHTTPClient(userAgent string) *Client {
	return &Client{UserAgent: userAgent, MinInterval: time.Second, HTTP: &http.Client{Timeout: 20 * time.Second}, last: map[string]time.Time{}}
}

func (c *Client) wait(ctx context.Context, host string) error {
	if c.MinInterval <= 0 {
		return nil
	}
	c.mu.Lock()
	next := c.last[host].Add(c.MinInterval)
	now := time.Now()
	if next.Before(now) {
		next = now
	}
	c.last[host] = next
	c.mu.Unlock()
	if d := time.Until(next); d > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
	return nil
}

// Get fetches url, retrying once on 429 or 5xx. Callers must close the body.
func (c *Client) Get(ctx context.Context, url string) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", c.UserAgent)
		req.Header.Set("Accept", "application/json, text/html;q=0.9, */*;q=0.5")
		if err := c.wait(ctx, req.URL.Host); err != nil {
			return nil, err
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			delay := retryAfter(resp)
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
			resp.Body.Close()
			lastErr = fmt.Errorf("sources: %s returned %d", req.URL.Host, resp.StatusCode)
			if attempt == 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
				}
			}
			continue
		}
		if resp.StatusCode >= 400 {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
			resp.Body.Close()
			return nil, fmt.Errorf("sources: %s returned %d", req.URL.Host, resp.StatusCode)
		}
		return resp, nil
	}
	return nil, lastErr
}

// GetJSON fetches and decodes a JSON document of at most maxBytes, returning the raw bytes too.
func (c *Client) GetJSON(ctx context.Context, url string, maxBytes int64, dst any) ([]byte, error) {
	resp, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return nil, fmt.Errorf("sources: decode %s: %w", url, err)
	}
	return raw, nil
}

func retryAfter(resp *http.Response) time.Duration {
	if s := resp.Header.Get("Retry-After"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 && n <= 30 {
			return time.Duration(n) * time.Second
		}
	}
	return 2 * time.Second
}
