package httpapi

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"

	"invisible-museum/backend/internal/blob"
	"invisible-museum/backend/internal/models"
)

const maxUploadBytes = 64 << 20

func kindForContentType(ct string) (models.MediaKind, string) {
	switch {
	case strings.HasPrefix(ct, "image/"):
		return models.MediaKindImage, "images"
	case ct == "application/pdf" || ct == "text/plain":
		return models.MediaKindDocument, "documents"
	case strings.HasPrefix(ct, "audio/"):
		return models.MediaKindAudio, "audio"
	}
	return "", ""
}

func (d Deps) adminUploadMedia(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "expected a multipart form with a file field (max 64 MiB)")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil {
		writeValidation(w, map[string]string{"file": "is required"})
		return
	}
	defer file.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	head = head[:n]
	contentType := detectContentType(header.Header.Get("Content-Type"), header.Filename, head)
	kind, prefix := kindForContentType(contentType)
	if kind == "" || strings.Contains(contentType, "svg") || strings.Contains(contentType, "xml") || strings.Contains(contentType, "html") {
		writeValidation(w, map[string]string{"file": "must be a photo, PDF, plain-text document or audio file; add videos as external links"})
		return
	}
	extensions, _ := mime.ExtensionsByType(contentType)
	if len(extensions) == 0 {
		writeValidation(w, map[string]string{"file": "has an unsupported media type"})
		return
	}
	name := strings.TrimSuffix(path.Base(header.Filename), path.Ext(header.Filename)) + extensions[0]
	key := blob.KeyFor(prefix, name)
	body := io.MultiReader(bytes.NewReader(head), file)
	if err := d.Blob.Put(r.Context(), key, contentType, body, header.Size); err != nil {
		d.fail(w, err)
		return
	}
	asset, err := d.Store.CreateMedia(r.Context(), models.MediaAsset{
		Kind: kind, StorageKey: key, PublicURL: d.Blob.PublicURL(key), MimeType: contentType,
		Caption: r.FormValue("caption"), Credit: r.FormValue("credit"), License: r.FormValue("license"),
		LicenseURL: r.FormValue("license_url"), SourceURL: r.FormValue("source_url"),
	})
	if err != nil {
		_ = d.Blob.Delete(r.Context(), key)
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, asset)
}

func (d Deps) adminListMedia(w http.ResponseWriter, r *http.Request) {
	assets, err := d.Store.ListMedia(r.Context(), queryInt(r, "limit", 50), queryOffset(r))
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: assets})
}

func (d Deps) adminUpdateMedia(w http.ResponseWriter, r *http.Request) {
	var req models.MediaMetadataRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	id := chi.URLParam(r, "id")
	current, err := d.Store.GetMedia(r.Context(), id)
	if err != nil {
		d.fail(w, err)
		return
	}
	for _, field := range []struct {
		value  *string
		target *string
	}{
		{req.Caption, &current.Caption}, {req.Credit, &current.Credit}, {req.License, &current.License},
		{req.LicenseURL, &current.LicenseURL}, {req.SourceURL, &current.SourceURL},
	} {
		if field.value != nil {
			*field.target = *field.value
		}
	}
	if issue := current.PublicationIssue(); issue != "" {
		moments, err := d.Store.PublishedMomentsUsingMedia(r.Context(), id)
		if err != nil {
			d.fail(w, err)
			return
		}
		places, err := d.Store.PublishedPlacesUsingMedia(r.Context(), id)
		if err != nil {
			d.fail(w, err)
			return
		}
		if moments+places > 0 {
			writeError(w, http.StatusConflict, "conflict", "asset is attached to published content: "+issue)
			return
		}
	}
	asset, err := d.Store.UpdateMediaMetadata(r.Context(), id, req)
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, asset)
}

// detectContentType uses the file signature rather than untrusted metadata.
func detectContentType(_, _ string, head []byte) string {
	// Neither a declared MIME type nor a filename can override the file signature.
	return strings.Split(http.DetectContentType(head), ";")[0]
}

func (d Deps) adminCreateMediaLink(w http.ResponseWriter, r *http.Request) {
	var req models.MediaLinkRequest
	if !decodeOrFail(w, r, &req) {
		return
	}
	if fields := req.Validate(); len(fields) > 0 {
		writeValidation(w, fields)
		return
	}
	asset, err := d.Store.CreateMedia(r.Context(), models.MediaAsset{
		Kind: req.Kind, External: true, PublicURL: req.PublicURL, Caption: req.Caption, Credit: req.Credit, SourceURL: req.SourceURL,
	})
	if err != nil {
		d.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, asset)
}
