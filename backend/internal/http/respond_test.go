package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteErrorEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusNotFound, "not_found", "tour not found")
	if rec.Code != 404 {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "not_found" || body.Error.Message != "tour not found" {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestWriteValidation(t *testing.T) {
	rec := httptest.NewRecorder()
	writeValidation(rec, map[string]string{"title": "is required"})
	if rec.Code != 422 {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		Error struct {
			Code   string            `json:"code"`
			Fields map[string]string `json:"fields"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Error.Code != "validation_failed" || body.Error.Fields["title"] != "is required" {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"title":"x","bogus":1}`))
	var dst struct {
		Title string `json:"title"`
	}
	if err := decodeJSON(req, &dst); err == nil {
		t.Fatal("expected error for unknown field")
	}
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"title":"x"}`))
	if err := decodeJSON(req, &dst); err != nil || dst.Title != "x" {
		t.Fatalf("expected ok, got %v %+v", err, dst)
	}
}
