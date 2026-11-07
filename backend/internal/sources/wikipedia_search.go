package sources

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	"invisible-museum/backend/internal/models"
)

// WikipediaSearch discovers links through MediaWiki full-text search. Snippets are never page evidence.
type WikipediaSearch struct {
	client *Client
	base   string
}

// NewWikipediaSearch accepts an alternate API endpoint for fixture tests.
func NewWikipediaSearch(client *Client, base string) *WikipediaSearch {
	if base == "" {
		base = "https://en.wikipedia.org/w/api.php"
	}
	return &WikipediaSearch{client: client, base: base}
}

func (a *WikipediaSearch) Name() string { return "wikipedia_search" }

func (a *WikipediaSearch) Discover(ctx context.Context, req Request) ([]Found, error) {
	q := url.Values{"action": {"query"}, "format": {"json"}, "list": {"search"}, "srnamespace": {"0"}, "srsearch": {req.Query}, "srlimit": {strconv.Itoa(req.limit())}}
	var resp struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
		Query struct {
			Search []struct {
				PageID  int    `json:"pageid"`
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
			} `json:"search"`
		} `json:"query"`
	}
	if _, err := a.client.GetJSON(ctx, a.base+"?"+q.Encode(), 2<<20, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, errors.New("Wikipedia search unavailable")
	}
	out := []Found{}
	for _, hit := range resp.Query.Search {
		if hit.PageID <= 0 || hit.Title == "" {
			continue
		}
		out = append(out, Found{Kind: models.CandidateKindMoment, ExternalID: "wikipedia:" + strconv.Itoa(hit.PageID), Title: hit.Title, Text: stripHTML(hit.Snippet), URL: "https://en.wikipedia.org/?curid=" + strconv.Itoa(hit.PageID), Credit: "Wikipedia"})
	}
	return out, nil
}
