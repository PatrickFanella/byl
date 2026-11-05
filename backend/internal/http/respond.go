package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxBodyBytes = 1 << 20

type errorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload != nil {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: message}})
}

func writeValidation(w http.ResponseWriter, fields map[string]string) {
	writeJSON(w, http.StatusUnprocessableEntity, errorEnvelope{Error: errorBody{
		Code:    "validation_failed",
		Message: "request failed validation",
		Fields:  fields,
	}})
}

// badRequest is returned by decodeJSON for malformed bodies.
type badRequest struct{ msg string }

func (b badRequest) Error() string { return b.msg }

func decodeJSON(r *http.Request, dst any) error {
	body := http.MaxBytesReader(nil, r.Body, maxBodyBytes)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var syntax *json.SyntaxError
		var typ *json.UnmarshalTypeError
		switch {
		case errors.Is(err, io.EOF):
			return badRequest{"request body is required"}
		case errors.As(err, &syntax):
			return badRequest{fmt.Sprintf("malformed JSON at byte %d", syntax.Offset)}
		case errors.As(err, &typ):
			return badRequest{fmt.Sprintf("field %q has the wrong type", typ.Field)}
		case strings.HasPrefix(err.Error(), "json: unknown field"):
			return badRequest{strings.TrimPrefix(err.Error(), "json: ")}
		default:
			return badRequest{"malformed JSON body"}
		}
	}
	if dec.More() {
		return badRequest{"request body must contain a single JSON object"}
	}
	return nil
}

// decodeOrFail decodes the body and writes a 400 on failure, returning false.
func decodeOrFail(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := decodeJSON(r, dst); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return false
	}
	return true
}
