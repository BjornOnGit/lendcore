// Package httpx holds shared HTTP helpers: JSON I/O and the standard error shape.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// maxBodyBytes is the largest request body ReadJSON accepts (1 MB).
const maxBodyBytes = 1 << 20

// errorBody is the standard error shape: {"error": {"code": "...", "message": "..."}}.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteJSON writes v as a JSON response with the given status. The body is
// marshaled before anything is written, so a marshal failure becomes a clean
// 500 instead of a truncated 2xx.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		// errorBody always marshals, so this cannot recurse.
		WriteError(w, http.StatusInternalServerError, "INTERNAL", "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

// WriteError writes the standard error shape with the given status, code
// (SNAKE_CASE) and human-readable message.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

// ReadJSON decodes the request body into v. It rejects unknown fields,
// trailing data after the JSON value, and bodies over 1 MB; an oversized body
// yields an error wrapping *http.MaxBytesError (use errors.As to detect it).
func ReadJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	// The body must hold exactly one JSON value: the next token must be EOF.
	_, err := dec.Token()
	if err == nil {
		return errors.New("invalid JSON body: unexpected data after JSON value")
	}
	if !errors.Is(err, io.EOF) {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}
