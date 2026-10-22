package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type payload struct {
	Name   string `json:"name"`
	Amount int64  `json:"amount"`
}

func newReq(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusCreated, payload{Name: "a", Amount: 5})

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"name":"a","amount":5}`, rec.Body.String())
}

func TestWriteJSONMarshalFailureIs500(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusOK, make(chan int)) // channels cannot be marshaled

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.JSONEq(t, `{"error":{"code":"INTERNAL","message":"internal error"}}`, rec.Body.String())
}

func TestWriteErrorShape(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, http.StatusBadRequest, "BAD_THING", "bad thing happened")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"error":{"code":"BAD_THING","message":"bad thing happened"}}`, rec.Body.String())
}

func TestReadJSON(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    payload
		wantErr string // substring; empty means success
	}{
		{"success", `{"name":"a","amount":5}`, payload{"a", 5}, ""},
		{"trailing whitespace is fine", "{\"name\":\"a\"}\n  ", payload{Name: "a"}, ""},
		{"unknown field", `{"name":"a","bogus":1}`, payload{}, `unknown field "bogus"`},
		{"malformed", `{"name":`, payload{}, "invalid JSON body"},
		{"empty body", ``, payload{}, "invalid JSON body"},
		{"second value", `{"name":"a"}{"name":"b"}`, payload{}, "unexpected data after JSON value"},
		{"trailing garbage", `{"name":"a"}}`, payload{}, "invalid JSON body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got payload
			err := ReadJSON(newReq(tt.body), &got)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// bodyOfSize returns a valid JSON object exactly n bytes long.
func bodyOfSize(n int) string {
	const prefix, suffix = `{"name":"`, `"}`
	return prefix + strings.Repeat("a", n-len(prefix)-len(suffix)) + suffix
}

func TestReadJSONBodyLimit(t *testing.T) {
	t.Run("exactly 1 MB is accepted", func(t *testing.T) {
		var got payload
		require.NoError(t, ReadJSON(newReq(bodyOfSize(1<<20)), &got))
		assert.Len(t, got.Name, 1<<20-len(`{"name":""}`))
	})
	t.Run("over 1 MB is rejected", func(t *testing.T) {
		var got payload
		err := ReadJSON(newReq(bodyOfSize(1<<20+1)), &got)
		var tooLarge *http.MaxBytesError
		assert.True(t, errors.As(err, &tooLarge), "want *http.MaxBytesError, got %v", err)
	})
}
