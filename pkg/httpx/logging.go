package httpx

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/BjornOnGit/lendcore/pkg/logx"
)

// statusRecorder remembers the status code the handler wrote.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status, s.wrote = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.status, s.wrote = http.StatusOK, true
	}
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer (Flush, etc.).
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Logging returns middleware that logs one "request" line per request (method,
// path without query string, status, duration_ms, request_id) after the
// handler returns. A panicking handler is recovered: the panic and its stack
// are logged at error level and, if the response has not started, the client
// gets a 500 in the standard error shape without the panic details.
// http.ErrAbortHandler is logged as a normal request and then re-panicked, as
// net/http expects. Put RequestID outside Logging so the ID is available.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			requestID := logx.RequestID(r.Context())

			defer func() {
				p := recover()
				if p != nil && p != http.ErrAbortHandler {
					logger.Error("panic recovered",
						"panic", fmt.Sprint(p),
						"stack", string(debug.Stack()),
						"request_id", requestID)
					if !rec.wrote {
						WriteError(rec, http.StatusInternalServerError, "INTERNAL", "internal error")
					}
				}
				logger.Info("request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", rec.status,
					"duration_ms", float64(time.Since(start).Microseconds())/1000,
					"request_id", requestID)
				if p == http.ErrAbortHandler {
					panic(p)
				}
			}()

			next.ServeHTTP(rec, r)
		})
	}
}
