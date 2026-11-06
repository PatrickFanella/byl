package httpapi

import (
	"context"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/testdb"
	"net/http"
	"testing"
)

func TestUploadContentTypeUsesFileBytes(t *testing.T) {
	if got := detectContentType("image/png", "misleading.png", []byte("\xff\xd8\xffjpeg")); got != "image/jpeg" {
		t.Fatalf("JPEG with misleading metadata detected as %q", got)
	}
	if got := detectContentType("image/png", "active.png", []byte("<html><script>alert(1)</script>")); got != "text/html" {
		t.Fatalf("HTML with misleading metadata detected as %q", got)
	}
}

func TestDocumentsDownloadAndPlaceCollections(t *testing.T) {
	env := newTestEnv(t)
	curator := env.signIn(t, "curator@example.com", models.AccountRoleCurator)
	asset := uploadMedia(t, env, curator, "record.pdf", "application/pdf", []byte("%PDF-1.4\narchive record"), map[string]string{
		"caption": "Depot inspection record", "credit": "Public archive", "license": "PD",
		"source_url": "https://archive.example.org/record", "license_url": "https://archive.example.org/rights",
	})
	if asset["kind"] != "document" || asset["external"] != false {
		t.Fatalf("document: %v", asset)
	}
	r := env.get(t, "/media/files/"+asset["storage_key"].(string), "")
	if r.status != 200 || r.header.Get("Content-Disposition") != "attachment" || r.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("document download: %d %v", r.status, r.header)
	}
	path := "/api/v1/admin/places/" + testdb.SeedPlaceStationID + "/media"
	if r := env.do(t, http.MethodPut, path, curator, models.MediaListRequest{MediaIDs: []string{asset["id"].(string)}}); r.status != 200 {
		t.Fatalf("attach: %d %s", r.status, r.raw)
	}
	public := env.get(t, "/api/v1/places/"+testdb.SeedPlaceStationID, "")
	if media := public.body["media"].([]any); len(media) != 1 || media[0].(map[string]any)["id"] != asset["id"] {
		t.Fatalf("public collection: %s", public.raw)
	}
	if r := env.do(t, http.MethodPatch, "/api/v1/admin/media/"+asset["id"].(string), curator, map[string]string{"credit": ""}); r.status != 409 {
		t.Fatalf("cannot remove published provenance: %d %s", r.status, r.raw)
	}
	if r := env.do(t, http.MethodPut, path, curator, models.MediaListRequest{MediaIDs: []string{asset["id"].(string), asset["id"].(string)}}); r.status != 422 {
		t.Fatalf("duplicate attachment: %d", r.status)
	}
}

func TestExternalVideoReferenceNeedsNoCopyingLicenseOrFetch(t *testing.T) {
	env := newTestEnv(t)
	curator := env.signIn(t, "curator@example.com", models.AccountRoleCurator)
	before, _ := env.store.ListJobs(context.Background(), models.JobStatusQueued, 100, 0)
	link := models.MediaLinkRequest{Kind: models.MediaKindVideo, PublicURL: "https://video.example.org/watch/123", Caption: "Documentary about the theatre", Credit: "Local historical society", SourceURL: "https://archive.example.org/videos"}
	r := env.do(t, http.MethodPost, "/api/v1/admin/media/links", curator, link)
	if r.status != 201 || r.body["external"] != true || r.body["license"] != "" || r.body["storage_key"] != nil {
		t.Fatalf("link: %d %s", r.status, r.raw)
	}
	after, _ := env.store.ListJobs(context.Background(), models.JobStatusQueued, 100, 0)
	if len(after) != len(before) {
		t.Fatal("external link must not enqueue a download")
	}
	path := "/api/v1/admin/moments/" + testdb.SeedMomentTalkiesID + "/media"
	if r := env.do(t, http.MethodPut, path, curator, models.MediaListRequest{MediaIDs: []string{r.body["id"].(string)}}); r.status != 200 {
		t.Fatalf("attach external video to published event: %d %s", r.status, r.raw)
	}
	for _, bad := range []string{"javascript:alert(1)", "https://user:secret@example.org/video"} {
		link.PublicURL = bad
		if r := env.do(t, http.MethodPost, "/api/v1/admin/media/links", curator, link); r.status != 422 {
			t.Fatalf("invalid link accepted: %d", r.status)
		}
	}
	if kind, _ := kindForContentType("video/mp4"); kind != "" {
		t.Fatal("video file uploads must be disabled")
	}
}
