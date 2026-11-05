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
	case strings.HasPrefix(ct, "video/"):
		return models.MediaKindVideo, "videos"
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
	if kind == "" {
		writeValidation(w, map[string]string{"file": "must be an image, video, or audio file"})
		return
	}
	key := blob.KeyFor(prefix, header.Filename)
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
	if req.License != nil && strings.TrimSpace(*req.License) == "" {
		n, err := d.Store.PublishedMomentsUsingMedia(r.Context(), id)
		if err != nil {
			d.fail(w, err)
			return
		}
		if n > 0 {
			writeError(w, http.StatusConflict, "conflict", "asset is attached to a published moment; unpublish it before removing the license")
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

// detectContentType trusts a specific declared type, otherwise sniffs the bytes, then the extension.
func detectContentType(declared, filename string, head []byte) string {
	declared = strings.ToLower(strings.TrimSpace(strings.Split(declared, ";")[0]))
	if declared != "" && declared != "application/octet-stream" {
		return declared
	}
	if sniffed := http.DetectContentType(head); sniffed != "application/octet-stream" && !strings.HasPrefix(sniffed, "text/") {
		return strings.Split(sniffed, ";")[0]
	}
	if byExt := mime.TypeByExtension(strings.ToLower(path.Ext(filename))); byExt != "" {
		return strings.Split(byExt, ";")[0]
	}
	return "application/octet-stream"
}
