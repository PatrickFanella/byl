package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
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
	// AllowPrivateHosts lets GetPublic reach loopback, private, link-local and ULA addresses.
	// Off by default; turn it on only for development or a trusted LAN archive.
	AllowPrivateHosts bool
	evidenceTransport http.RoundTripper

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
	return c.do(ctx, c.HTTP, url)
}

// GetWithRedirectCheck is Get with a per-call redirect policy. The shared client is copied,
// never modified, so concurrent callers keep the default policy.
func (c *Client) GetWithRedirectCheck(ctx context.Context, url string, check func(*http.Request, []*http.Request) error) (*http.Response, error) {
	hc := *c.HTTP
	hc.CheckRedirect = check
	return c.do(ctx, &hc, url)
}

// ErrPrivateHost reports a URL that resolves to a non-public address.
var ErrPrivateHost = errors.New("sources: host resolves to a private address")

// GetPublic is Get for URLs supplied by users: every connection, including those made after a
// redirect, is dialled only when the host resolves to public addresses (unless
// AllowPrivateHosts is set), which closes the SSRF path through a crafted media_url.
func (c *Client) GetPublic(ctx context.Context, url string) (*http.Response, error) {
	if c.AllowPrivateHosts {
		return c.Get(ctx, url)
	}
	hc := *c.HTTP
	hc.Transport = publicOnlyTransport(c.HTTP.Transport)
	hc.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if r.URL.Scheme != "http" && r.URL.Scheme != "https" {
			return fmt.Errorf("redirect to %s scheme refused", r.URL.Scheme)
		}
		return nil
	}
	return c.do(ctx, &hc, url)
}

// publicOnlyTransport clones base (or the default transport) with a dialer that resolves the
// host itself and refuses non-public addresses, so DNS answers are checked at connect time.
func publicOnlyTransport(base http.RoundTripper) http.RoundTripper {
	return publicTransportWithResolver(base, net.DefaultResolver.LookupIPAddr, (&net.Dialer{Timeout: 10 * time.Second}).DialContext)
}

func publicTransportWithResolver(base http.RoundTripper, lookup func(context.Context, string) ([]net.IPAddr, error), dial func(context.Context, string, string) (net.Conn, error)) http.RoundTripper {
	var t *http.Transport
	if bt, ok := base.(*http.Transport); ok && bt != nil {
		t = bt.Clone()
	} else {
		t = http.DefaultTransport.(*http.Transport).Clone()
	}
	// Dial targets directly: through a proxy only the proxy's address would be checked.
	t.Proxy = nil
	t.DialTLSContext = nil
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := lookup(ctx, host)
		if err != nil {
			return nil, err
		}
		var lastErr error = fmt.Errorf("%w: %s", ErrPrivateHost, host)
		for _, ip := range ips {
			if !publicIP(ip.IP) {
				continue
			}
			conn, err := dial(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
	return t
}

// Exclude special-use and transition ranges as well as private and local addresses.
var blockedPublicRanges = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}
var globalIPv6Range = netip.MustParsePrefix("2000::/3")

// publicIP accepts ordinary globally routable addresses, without IPv6 translation tunnels.
func publicIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if addr.Is6() && !globalIPv6Range.Contains(addr) {
		return false
	}
	for _, prefix := range blockedPublicRanges {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

func (c *Client) do(ctx context.Context, hc *http.Client, url string) (*http.Response, error) {
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
		resp, err := hc.Do(req)
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
