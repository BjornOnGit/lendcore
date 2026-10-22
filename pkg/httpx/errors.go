package httpx

import (
	"errors"
	"net/http"
)

// Sentinel errors returned by domain and store code, usually wrapped with
// fmt.Errorf("...: %w", ErrX) to add context. MapError turns them into HTTP
// statuses and codes. Each code is the sentinel's name in SNAKE_CASE without
// the "Err" prefix.
var (
	ErrNotFound          = errors.New("not found")
	ErrValidation        = errors.New("validation failed")
	ErrConflict          = errors.New("conflict")
	ErrUnprocessable     = errors.New("unprocessable")
	ErrInsufficientFunds = errors.New("insufficient funds")
)

// MapError returns the HTTP status and error code for err, matching sentinels
// with errors.Is so wrapped errors work. Anything unrecognized, including a
// nil error, maps to 500 INTERNAL; callers should log such errors and send a
// generic message instead of err.Error().
func MapError(err error) (status int, code string) {
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, ErrValidation):
		return http.StatusBadRequest, "VALIDATION"
	case errors.Is(err, ErrConflict):
		return http.StatusConflict, "CONFLICT"
	case errors.Is(err, ErrUnprocessable):
		return http.StatusUnprocessableEntity, "UNPROCESSABLE"
	case errors.Is(err, ErrInsufficientFunds):
		return http.StatusUnprocessableEntity, "INSUFFICIENT_FUNDS"
	default:
		return http.StatusInternalServerError, "INTERNAL"
	}
}
