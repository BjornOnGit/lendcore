package httpx

import (
	"context"
	"net/http"
)

// Check is a named readiness probe, e.g. Check{"postgres", pool.Ping}.
type Check struct {
	Name string
	Fn   func(context.Context) error
}

type statusResponse struct {
	Status  string   `json:"status"`
	Failing []string `json:"failing,omitempty"`
}

// Healthz is the liveness handler: it always answers 200 {"status":"ok"}.
func Healthz(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

// Readyz returns the readiness handler. It runs every check with the request
// context (so a probe timeout cancels them) and answers 200 {"status":"ok"} if
// all pass, or 503 {"status":"unavailable","failing":[names...]} otherwise.
// Only check names are exposed, never error text.
func Readyz(checks ...Check) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var failing []string
		for _, c := range checks {
			if err := c.Fn(r.Context()); err != nil {
				failing = append(failing, c.Name)
			}
		}
		if len(failing) > 0 {
			WriteJSON(w, http.StatusServiceUnavailable, statusResponse{Status: "unavailable", Failing: failing})
			return
		}
		WriteJSON(w, http.StatusOK, statusResponse{Status: "ok"})
	}
}
