package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"invisible-museum/backend/internal/models"
)

// Wikipedia finds articles near a point. Each article yields a place find (first
// paragraph) and, when the article has text, a moment find (full extract).
type Wikipedia struct {
	client  *Client
	apiBase string
}

// NewWikipedia builds the adapter; apiBase defaults to English Wikipedia.
func NewWikipedia(client *Client, apiBase string) *Wikipedia {
	if apiBase == "" {
		apiBase = "https://en.wikipedia.org/w/api.php"
	}
	return &Wikipedia{client: client, apiBase: apiBase}
}

func (w *Wikipedia) Name() string { return "wikipedia" }

type wikiGeo struct {
	Query struct {
		Geosearch []struct {
			PageID int     `json:"pageid"`
			Title  string  `json:"title"`
			Lat    float64 `json:"lat"`
			Lon    float64 `json:"lon"`
		} `json:"geosearch"`
	} `json:"query"`
}

type wikiPage struct {
	PageID      int    `json:"pageid"`
	Title       string `json:"title"`
	Extract     string `json:"extract"`
	FullURL     string `json:"fullurl"`
	Coordinates []struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"coordinates"`
	PageProps struct {
		WikibaseItem string `json:"wikibase_item"`
	} `json:"pageprops"`
	Original struct {
		Source string `json:"source"`
	} `json:"original"`
}

type wikiPages struct {
	Query struct {
		Pages map[string]wikiPage `json:"pages"`
	} `json:"query"`
}

const wikipediaLicenseURL = "https://creativecommons.org/licenses/by-sa/4.0/"

func (w *Wikipedia) Discover(ctx context.Context, req Request) ([]Found, error) {
	q := url.Values{}
	q.Set("action", "query")
	q.Set("list", "geosearch")
	q.Set("gscoord", fmt.Sprintf("%f|%f", req.Center.Lat, req.Center.Lng))
	q.Set("gsradius", fmt.Sprintf("%d", int(min(req.radius(), 10000))))
	q.Set("gslimit", fmt.Sprintf("%d", req.limit()))
	q.Set("format", "json")
	var geo wikiGeo
	if _, err := w.client.GetJSON(ctx, w.apiBase+"?"+q.Encode(), 2<<20, &geo); err != nil {
		return nil, err
	}

	var out []Found
	for i, hit := range geo.Query.Geosearch {
		if i >= req.limit() {
			break
		}
		page, raw, err := w.page(ctx, hit.PageID)
		if err != nil {
			return out, err
		}
		lat, lon := hit.Lat, hit.Lon
		if len(page.Coordinates) > 0 {
			lat, lon = page.Coordinates[0].Lat, page.Coordinates[0].Lon
		}
		title := page.Title
		if title == "" {
			title = hit.Title
		}
		pageURL := page.FullURL
		if pageURL == "" {
			pageURL = "https://en.wikipedia.org/?curid=" + fmt.Sprint(hit.PageID)
		}
		extract := strings.TrimSpace(page.Extract)
		base := Found{
			Title: title, URL: pageURL, Lat: f64(lat), Lng: f64(lon),
			License: "CC-BY-SA-4.0", LicenseURL: wikipediaLicenseURL, Credit: "Wikipedia contributors",
			MediaURL: page.Original.Source, Raw: raw,
		}
		place := base
		place.Kind = models.CandidateKindPlace
		place.ExternalID = fmt.Sprintf("wikipedia:%d:place", hit.PageID)
		place.Text = firstParagraph(extract)
		out = append(out, place)
		if extract != "" {
			moment := base
			moment.Kind = models.CandidateKindMoment
			moment.ExternalID = fmt.Sprintf("wikipedia:%d:moment", hit.PageID)
			moment.Text = extract
			out = append(out, moment)
		}
	}
	return out, nil
}

// page fetches one article's full plain-text extract; the API returns full extracts for one page per request.
func (w *Wikipedia) page(ctx context.Context, pageID int) (wikiPage, json.RawMessage, error) {
	q := url.Values{}
	q.Set("action", "query")
	q.Set("pageids", fmt.Sprint(pageID))
	q.Set("prop", "extracts|coordinates|pageprops|pageimages|info")
	q.Set("explaintext", "1")
	q.Set("exsectionformat", "plain")
	q.Set("inprop", "url")
	q.Set("piprop", "original")
	q.Set("format", "json")
	var pages wikiPages
	raw, err := w.client.GetJSON(ctx, w.apiBase+"?"+q.Encode(), 4<<20, &pages)
	if err != nil {
		return wikiPage{}, nil, err
	}
	for _, p := range pages.Query.Pages {
		return p, raw, nil
	}
	return wikiPage{}, raw, nil
}

func firstParagraph(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
