package sources

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"invisible-museum/backend/internal/models"
)

// LOC searches loc.gov. Photos, prints, and maps become media finds; other items become moment finds.
type LOC struct {
	client *Client
	base   string
}

// NewLOC builds the adapter; base defaults to https://www.loc.gov.
func NewLOC(client *Client, base string) *LOC {
	if base == "" {
		base = "https://www.loc.gov"
	}
	return &LOC{client: client, base: strings.TrimRight(base, "/")}
}

func (a *LOC) Name() string { return "loc" }

const locNoKnownRestrictions = "PD-US-LOC-no-known-restrictions"

type locResponse struct {
	Results []struct {
		ID             string  `json:"id"`
		Title          string  `json:"title"`
		Date           string  `json:"date"`
		Description    iaField `json:"description"`
		OriginalFormat iaField `json:"original_format"`
		ImageURL       iaField `json:"image_url"`
		Rights         string  `json:"rights"`
		URL            string  `json:"url"`
		Contributor    iaField `json:"contributor"`
	} `json:"results"`
}

func (a *LOC) Discover(ctx context.Context, req Request) ([]Found, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("sources: loc needs a query")
	}
	q := url.Values{}
	q.Set("q", req.Query)
	q.Set("fo", "json")
	q.Set("c", fmt.Sprint(req.limit()))
	var resp locResponse
	raw, err := a.client.GetJSON(ctx, a.base+"/search/?"+q.Encode(), 8<<20, &resp)
	if err != nil {
		return nil, err
	}
	var out []Found
	for _, item := range resp.Results {
		id := strings.Trim(item.ID, "/")
		id = id[strings.LastIndex(id, "/")+1:]
		if id == "" {
			continue
		}
		f := Found{ExternalID: "loc:" + id, Title: item.Title, URL: item.URL, Credit: item.Contributor.first(), Raw: raw}
		if f.URL == "" {
			f.URL = item.ID
		}
		if f.Credit == "" {
			f.Credit = "Library of Congress"
		}
		text := strings.TrimSpace(strings.Join(item.Description, "\n\n"))
		format := strings.ToLower(item.OriginalFormat.first())
		isImage := strings.Contains(format, "photo") || strings.Contains(format, "print") || strings.Contains(format, "map")
		if isImage && item.ImageURL.first() != "" {
			f.Kind = models.CandidateKindMedia
			f.MediaURL = strings.SplitN(item.ImageURL.first(), "#", 2)[0]
			f.MimeType = "image/jpeg"
			f.Text = text
			if strings.Contains(strings.ToLower(item.Rights), "no known restrictions") {
				f.License = locNoKnownRestrictions
				f.LicenseURL = f.URL
			}
		} else {
			f.Kind = models.CandidateKindMoment
			parts := []string{item.Title}
			if item.Date != "" {
				parts = append(parts, "Date: "+item.Date)
			}
			if text != "" {
				parts = append(parts, text)
			}
			f.Text = strings.Join(parts, "\n\n")
		}
		out = append(out, f)
	}
	return out, nil
}
