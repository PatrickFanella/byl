package sources

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"

	"invisible-museum/backend/internal/models"
)

// Commons finds Wikimedia Commons files near a point and carries their licence metadata.
type Commons struct {
	client  *Client
	apiBase string
}

// NewCommons builds the adapter; apiBase defaults to commons.wikimedia.org.
func NewCommons(client *Client, apiBase string) *Commons {
	if apiBase == "" {
		apiBase = "https://commons.wikimedia.org/w/api.php"
	}
	return &Commons{client: client, apiBase: apiBase}
}

func (c *Commons) Name() string { return "wikimedia_commons" }

type commonsGeo struct {
	Query struct {
		Geosearch []struct {
			PageID int     `json:"pageid"`
			Title  string  `json:"title"`
			Lat    float64 `json:"lat"`
			Lon    float64 `json:"lon"`
		} `json:"geosearch"`
	} `json:"query"`
}

type commonsInfo struct {
	Query struct {
		Pages map[string]struct {
			PageID    int    `json:"pageid"`
			Title     string `json:"title"`
			ImageInfo []struct {
				URL            string               `json:"url"`
				DescriptionURL string               `json:"descriptionurl"`
				Mime           string               `json:"mime"`
				Width          int                  `json:"width"`
				Height         int                  `json:"height"`
				ExtMetadata    map[string]metaValue `json:"extmetadata"`
			} `json:"imageinfo"`
		} `json:"pages"`
	} `json:"query"`
}

func (c *Commons) Discover(ctx context.Context, req Request) ([]Found, error) {
	q := url.Values{}
	q.Set("action", "query")
	q.Set("list", "geosearch")
	q.Set("gscoord", fmt.Sprintf("%f|%f", req.Center.Lat, req.Center.Lng))
	q.Set("gsradius", fmt.Sprintf("%d", int(min(req.radius(), 10000))))
	q.Set("gsnamespace", "6")
	q.Set("gslimit", fmt.Sprintf("%d", req.limit()))
	q.Set("format", "json")
	var geo commonsGeo
	if _, err := c.client.GetJSON(ctx, c.apiBase+"?"+q.Encode(), 2<<20, &geo); err != nil {
		return nil, err
	}
	if len(geo.Query.Geosearch) == 0 {
		return nil, nil
	}
	coords := map[int][2]float64{}
	ids := make([]string, 0, len(geo.Query.Geosearch))
	for i, hit := range geo.Query.Geosearch {
		if i >= req.limit() {
			break
		}
		ids = append(ids, fmt.Sprint(hit.PageID))
		coords[hit.PageID] = [2]float64{hit.Lat, hit.Lon}
	}
	var out []Found
	// imageinfo accepts up to 50 ids per call.
	for start := 0; start < len(ids); start += 50 {
		end := min(start+50, len(ids))
		iq := url.Values{}
		iq.Set("action", "query")
		iq.Set("pageids", strings.Join(ids[start:end], "|"))
		iq.Set("prop", "imageinfo")
		iq.Set("iiprop", "url|extmetadata|mime|size")
		iq.Set("format", "json")
		var info commonsInfo
		raw, err := c.client.GetJSON(ctx, c.apiBase+"?"+iq.Encode(), 8<<20, &info)
		if err != nil {
			return out, err
		}
		for _, p := range info.Query.Pages {
			if len(p.ImageInfo) == 0 {
				continue
			}
			ii := p.ImageInfo[0]
			meta := func(k string) string { return strings.TrimSpace(ii.ExtMetadata[k].String()) }
			f := Found{
				Kind: models.CandidateKindMedia, ExternalID: fmt.Sprintf("commons:%d", p.PageID),
				Title: strings.TrimPrefix(p.Title, "File:"), Text: stripHTML(meta("ImageDescription")),
				URL: ii.DescriptionURL, MediaURL: ii.URL, MimeType: ii.Mime, Width: ii.Width, Height: ii.Height,
				License: MapLicense(meta("LicenseShortName")), LicenseURL: meta("LicenseUrl"),
				Credit: stripHTML(meta("Artist")), Raw: raw,
			}
			if f.URL == "" {
				f.URL = "https://commons.wikimedia.org/wiki/" + url.PathEscape(p.Title)
			}
			if f.Credit == "" {
				f.Credit = stripHTML(meta("Credit"))
			}
			if xy, ok := coords[p.PageID]; ok {
				f.Lat, f.Lng = f64(xy[0]), f64(xy[1])
			}
			out = append(out, f)
		}
	}
	return out, nil
}

// metaValue tolerates Commons returning numbers or strings for extmetadata values.
type metaValue struct {
	Value any `json:"value"`
}

func (m metaValue) String() string {
	switch v := m.Value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

var tagRe = regexp.MustCompile(`<[^>]*>`)
var spaceRe = regexp.MustCompile(`\s+`)

// stripHTML removes tags and collapses whitespace; good enough for captions and credits.
func stripHTML(s string) string {
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}
