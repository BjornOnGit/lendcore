package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BjornOnGit/lendcore/pkg/logx"
)

// serve sends one GET through RequestID and returns the ID echoed in the
// response header and the ID the handler saw in its context. With no hdr
// argument the request has no X-Request-ID header; with one, it is set to hdr[0].
func serve(hdr ...string) (respID, ctxID string) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = logx.RequestID(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if len(hdr) > 0 {
		req.Header.Set(RequestIDHeader, hdr[0])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Header().Get(RequestIDHeader), seen
}

func TestRequestIDGeneratedWhenMissing(t *testing.T) {
	respID, ctxID := serve()

	assert.Equal(t, respID, ctxID, "context and response header must agree")
	id, err := uuid.Parse(respID)
	require.NoError(t, err, "generated ID should be a UUID, got %q", respID)
	assert.Equal(t, uuid.Version(4), id.Version())
}

func TestRequestIDGeneratedIDsDiffer(t *testing.T) {
	a, _ := serve()
	b, _ := serve()
	assert.NotEqual(t, a, b)
}

func TestRequestIDPreservedWhenProvided(t *testing.T) {
	respID, ctxID := serve("abc-123")

	assert.Equal(t, "abc-123", respID)
	assert.Equal(t, "abc-123", ctxID)
}

func TestRequestIDMaxLengthAccepted(t *testing.T) {
	id := strings.Repeat("a", maxRequestIDLen)
	respID, ctxID := serve(id)

	assert.Equal(t, id, respID)
	assert.Equal(t, id, ctxID)
}

func TestRequestIDReplacedWhenInvalid(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{"empty", ""},
		{"too long", strings.Repeat("a", maxRequestIDLen+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respID, ctxID := serve(tt.header)

			assert.NotEqual(t, tt.header, respID)
			assert.Equal(t, respID, ctxID)
			_, err := uuid.Parse(respID)
			assert.NoError(t, err, "replacement should be a UUID, got %q", respID)
		})
	}
}
