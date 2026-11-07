package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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
			if req.CatalogOnly {
				f.Kind = models.CandidateKindMedia
				f.Text = text
				out = append(out, f)
				continue
			}
			name, mimeType, err := a.primaryFile(ctx, d.Identifier, d.MediaType)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return nil, err
				}
				slog.Warn("archive item metadata unavailable", "identifier", d.Identifier, "error", err)
				continue
			}
			if name == "" {
				continue // no usable image or video file in the item
			}
			f.Kind = models.CandidateKindMedia
			f.Text = text
			f.MediaURL = a.base + "/download/" + url.PathEscape(d.Identifier) + "/" + escapePath(name)
			f.MimeType = mimeType
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
			return MapLicense("CC-" + strings.ToUpper(parts[0]) + "-" + parts[1])
		}
	}
	return ""
}

func jsonUnmarshal(b []byte, dst any) error { return json.Unmarshal(b, dst) }

type iaFiles struct {
	Result []struct {
		Name   string `json:"name"`
		Format string `json:"format"`
		Source string `json:"source"`
	} `json:"result"`
}

// iaExtMime is the set of file types we are willing to fetch from an archive item.
var iaExtMime = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".gif": "image/gif", ".tif": "image/tiff", ".tiff": "image/tiff",
	".mp4": "video/mp4", ".webm": "video/webm", ".mov": "video/quicktime", ".ogv": "video/ogg",
}

// primaryFile picks the file to download for an image or movies item: the original image, or
// the mp4 derivative for films (then webm, then the original). Thumbnails are never chosen.
// It returns "" when the item has no usable file.
func (a *InternetArchive) primaryFile(ctx context.Context, identifier, mediaType string) (name, mimeType string, err error) {
	var files iaFiles
	if _, err := a.client.GetJSON(ctx, a.base+"/metadata/"+url.PathEscape(identifier)+"/files", 4<<20, &files); err != nil {
		return "", "", err
	}
	best, bestScore := "", 0
	for _, f := range files.Result {
		lower := strings.ToLower(f.Name)
		format := strings.ToLower(f.Format)
		if strings.Contains(format, "thumb") || strings.Contains(format, "tile") || strings.Contains(lower, ".thumbs/") || strings.HasPrefix(lower, "__ia_") {
			continue
		}
		ext := strings.ToLower(pathExt(lower))
		mt, ok := iaExtMime[ext]
		if !ok {
			continue
		}
		isImage := strings.HasPrefix(mt, "image/")
		if (mediaType == "image") != isImage {
			continue
		}
		score := 1
		switch {
		case isImage && f.Source == "original":
			score = 3
		case !isImage && ext == ".mp4":
			score = 4
		case !isImage && ext == ".webm":
			score = 3
		case !isImage && f.Source == "original":
			score = 2
		}
		if score > bestScore {
			best, bestScore, mimeType = f.Name, score, mt
		}
	}
	return best, mimeType, nil
}

func pathExt(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 && !strings.Contains(name[i:], "/") {
		return name[i:]
	}
	return ""
}

// escapePath escapes each segment of a file name that may contain directories.
func escapePath(name string) string {
	parts := strings.Split(name, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}
