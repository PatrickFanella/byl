package sources

import (
	"context"
	"errors"
	"net/http"
	"time"
)

type discoveryBudgetKey struct{}
type discoveryPageHostsKey struct{}
type discoveryBudget func(context.Context) error

// ErrDiscoveryBudget stops external work while allowing successful earlier results to be retained.
var ErrDiscoveryBudget = errors.New("discovery request allowance exhausted")

type discoveryTransport struct {
	base    http.RoundTripper
	reserve discoveryBudget
}

func (t discoveryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if _, _, _, err := NormalizeEvidenceURL(req.URL.String()); err != nil {
		return nil, err
	}
	if hosts, ok := req.Context().Value(discoveryPageHostsKey{}).(map[string]bool); ok && !hosts[req.URL.Hostname()] {
		return nil, errEvidenceRedirect
	}
	if err := t.reserve(req.Context()); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

func withDiscoveryTransport(ctx context.Context, base http.RoundTripper) http.RoundTripper {
	if reserve, ok := ctx.Value(discoveryBudgetKey{}).(discoveryBudget); ok {
		return discoveryTransport{base: base, reserve: reserve}
	}
	return base
}

func (c *Client) discoveryHTTP(ctx context.Context, original *http.Client) *http.Client {
	hc := *original
	hc.Jar = nil
	if hc.Timeout <= 0 || hc.Timeout > 20*time.Second {
		hc.Timeout = 20 * time.Second
	}
	base := publicOnlyTransport(original.Transport)
	if c.evidenceTransport != nil {
		base = c.evidenceTransport // deterministic guarded DNS in tests
	}
	hc.Transport = withDiscoveryTransport(ctx, base)
	check := original.CheckRedirect
	hc.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 3 || (via[len(via)-1].URL.Scheme == "https" && next.URL.Scheme != "https") {
			return errEvidenceRedirect
		}
		next.Header.Del("Referer")
		if check != nil {
			if err := check(next, via); err != nil {
				return err
			}
		}
		return c.wait(ctx, next.URL.Host)
	}
	return &hc
}
