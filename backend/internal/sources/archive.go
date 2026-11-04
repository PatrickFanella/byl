package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"invisible-museum/backend/internal/models"
)

// InternetArchive searches archive.org for texts, images, and films matching a query.
type InternetArchive struct {
	client *Client
	base   string
}

// NewInternetArchive builds the adapter; base defaults to https://archive.org.
func NewInternetArchive(client *Client, base string) *InternetArchive {
	if base == "" {
		base = "https://archive.org"
	}
	return &InternetArchive{client: client, base: strings.TrimRight(base, "/")}
}

func (a *InternetArchive) Name() string { return "internet_archive" }

// iaField tolerates IA returning a string or a list of strings.
type iaField []string

func (f *iaField) UnmarshalJSON(b []byte) error {
	var one string
	if err := jsonUnmarshal(b, &one); err == nil {
		*f = iaField{one}
		return nil
	}
	var many []string
	if err := jsonUnmarshal(b, &many); err != nil {
		*f = nil
		return nil
	}
	*f = many
	return nil
}

func (f iaField) first() string {
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

type iaResponse struct {
	Response struct {
		Docs []struct {
			Identifier  string  `json:"identifier"`
			Title       iaField `json:"title"`
			Description iaField `json:"description"`
			Date        iaField `json:"date"`
			LicenseURL  iaField `json:"licenseurl"`
			Creator     iaField `json:"creator"`
			MediaType   string  `json:"mediatype"`
		} `json:"docs"`
	} `json:"response"`
}

func (a *InternetArchive) Discover(ctx context.Context, req Request) ([]Found, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("sources: internet_archive needs a query")
	}
	q := url.Values{}
	q.Set("q", fmt.Sprintf("(%s) AND mediatype:(texts OR image OR movies)", req.Query))
	for _, f := range []string{"identifier", "title", "description", "date", "licenseurl", "creator", "mediatype"} {
		q.Add("fl[]", f)
	}
	q.Set("rows", fmt.Sprint(req.limit()))
	q.Set("output", "json")
	var resp iaResponse
	raw, err := a.client.GetJSON(ctx, a.base+"/advancedsearch.php?"+q.Encode(), 8<<20, &resp)
	if err != nil {
		return nil, err
	}
	var out []Found
	for _, d := range resp.Response.Docs {
		if d.Identifier == "" {
			continue
		}
		credit := d.Creator.first()
		if credit == "" {
			credit = "Internet Archive"
		}
		text := strings.TrimSpace(stripHTML(strings.Join(d.Description, "\n\n")))
		f := Found{
			ExternalID: "ia:" + d.Identifier, Title: d.Title.first(), URL: a.base + "/details/" + url.PathEscape(d.Identifier),
			Credit: credit, LicenseURL: d.LicenseURL.first(), License: licenseFromURL(d.LicenseURL.first()), Raw: raw,
		}
		if f.Title == "" {
			f.Title = d.Identifier
		}
		switch d.MediaType {
		case "texts":
			f.Kind = models.CandidateKindMoment
			parts := []string{f.Title}
			if dt := d.Date.first(); dt != "" {
				parts = append(parts, "Date: "+dt)
			}
			if text != "" {
				parts = append(parts, text)
			}
			f.Text = strings.Join(parts, "\n\n")
		case "image", "movies":
			f.Kind = models.CandidateKindMedia
			f.Text = text
			f.MediaURL = a.base + "/download/" + url.PathEscape(d.Identifier)
			if d.MediaType == "movies" {
				f.MimeType = "video/mp4"
			} else {
				f.MimeType = "image/jpeg"
			}
		default:
			continue
		}
		out = append(out, f)
	}
	return out, nil
}

// licenseFromURL maps common licence URLs to identifiers.
func licenseFromURL(u string) string {
	u = strings.ToLower(u)
	switch {
	case u == "":
		return ""
	case strings.Contains(u, "publicdomain/zero"):
		return "CC0-1.0"
	case strings.Contains(u, "publicdomain/mark"):
		return "PD"
	case strings.Contains(u, "creativecommons.org/licenses/"):
		rest := strings.TrimPrefix(u[strings.Index(u, "/licenses/")+len("/licenses/"):], "")
		parts := strings.Split(strings.Trim(rest, "/"), "/")
		if len(parts) >= 2 {
			return "CC-" + strings.ToUpper(parts[0]) + "-" + parts[1]
		}
	}
	return ""
}

func jsonUnmarshal(b []byte, dst any) error { return json.Unmarshal(b, dst) }
