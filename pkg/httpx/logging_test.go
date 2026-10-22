package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeLogs(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	trimmed := strings.TrimSpace(buf.String())
	if trimmed == "" {
		return nil
	}
	var out []map[string]any
	for _, line := range strings.Split(trimmed, "\n") {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m), "log line is not valid JSON: %q", line)
		out = append(out, m)
	}
	return out
}

// runLogged sends one GET (with X-Request-ID "rid-1") through RequestID and
// Logging and returns the response and the decoded log lines.
func runLogged(t *testing.T, handler http.HandlerFunc, target string) (*httptest.ResponseRecorder, []map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	h := RequestID(Logging(slog.New(slog.NewJSONHandler(&buf, nil)))(handler))
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set(RequestIDHeader, "rid-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, decodeLogs(t, &buf)
}

func TestLoggingLogsRequest(t *testing.T) {
	rec, logs := runLogged(t, func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusCreated, map[string]string{"ok": "yes"})
	}, "/loans?token=secret")

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.JSONEq(t, `{"ok":"yes"}`, rec.Body.String())

	require.Len(t, logs, 1)
	assert.Equal(t, "request", logs[0]["msg"])
	assert.Equal(t, "GET", logs[0]["method"])
	assert.Equal(t, "/loans", logs[0]["path"], "query string must not be logged")
	assert.EqualValues(t, http.StatusCreated, logs[0]["status"])
	assert.Equal(t, "rid-1", logs[0]["request_id"])
	assert.Contains(t, logs[0], "duration_ms")
}

func TestLoggingDefaultStatusIs200(t *testing.T) {
	_, logs := runLogged(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hi"))
	}, "/x")

	require.Len(t, logs, 1)
	assert.EqualValues(t, http.StatusOK, logs[0]["status"])
}

func TestLoggingRecoversPanic(t *testing.T) {
	rec, logs := runLogged(t, func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}, "/x")

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"error":{"code":"INTERNAL","message":"internal error"}}`, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "boom", "panic details must not reach the client")

	require.Len(t, logs, 2)
	assert.Equal(t, "panic recovered", logs[0]["msg"])
	assert.Equal(t, "ERROR", logs[0]["level"])
	assert.Equal(t, "boom", logs[0]["panic"])
	assert.NotEmpty(t, logs[0]["stack"])
	assert.Equal(t, "rid-1", logs[0]["request_id"])
	assert.Equal(t, "request", logs[1]["msg"])
	assert.EqualValues(t, http.StatusInternalServerError, logs[1]["status"])
}

func TestLoggingPanicAfterResponseStarted(t *testing.T) {
	rec, logs := runLogged(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("partial"))
		panic("late")
	}, "/x")

	assert.Equal(t, http.StatusAccepted, rec.Code, "status already sent cannot change")
	assert.Equal(t, "partial", rec.Body.String(), "no error JSON appended to a started response")

	require.Len(t, logs, 2)
	assert.Equal(t, "late", logs[0]["panic"])
	assert.EqualValues(t, http.StatusAccepted, logs[1]["status"])
}

func TestLoggingRepanicsErrAbortHandler(t *testing.T) {
	var buf bytes.Buffer
	h := Logging(slog.New(slog.NewJSONHandler(&buf, nil)))(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))

	assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	})

	logs := decodeLogs(t, &buf)
	require.Len(t, logs, 1, "aborted request is logged but not reported as a panic")
	assert.Equal(t, "request", logs[0]["msg"])
}
