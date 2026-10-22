package httpx

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/BjornOnGit/lendcore/pkg/logx"
)

// RequestIDHeader carries the request ID on requests and responses.
const RequestIDHeader = "X-Request-ID"

// maxRequestIDLen caps client-supplied IDs so they cannot bloat every log line.
const maxRequestIDLen = 128

// RequestID is middleware that reuses the incoming X-Request-ID header or
// generates a UUIDv4, stores the ID in the request context (read it with
// logx.RequestID), and echoes it in the response header. An empty or
// over-long incoming value is replaced by a generated one.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if id == "" || len(id) > maxRequestIDLen {
			id = uuid.NewString()
		}
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(logx.WithRequestID(r.Context(), id)))
	})
}
