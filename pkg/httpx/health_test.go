package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func pass(context.Context) error { return nil }

func fail(msg string) func(context.Context) error {
	return func(context.Context) error { return errors.New(msg) }
}

func getReadyz(h http.HandlerFunc) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	return rec
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	Healthz(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestReadyzAllPass(t *testing.T) {
	rec := getReadyz(Readyz(Check{"db", pass}, Check{"ledger", pass}))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestReadyzNoChecks(t *testing.T) {
	rec := getReadyz(Readyz())

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestReadyzOneFails(t *testing.T) {
	rec := getReadyz(Readyz(Check{"db", fail("dial tcp: password=hunter2")}, Check{"ledger", pass}))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.JSONEq(t, `{"status":"unavailable","failing":["db"]}`, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "hunter2", "error text must not be exposed")
}

func TestReadyzRunsAllChecksAndKeepsOrder(t *testing.T) {
	var ran []string
	record := func(name string, err error) Check {
		return Check{name, func(context.Context) error { ran = append(ran, name); return err }}
	}
	rec := getReadyz(Readyz(
		record("db", errors.New("down")),
		record("cache", nil),
		record("ledger", errors.New("down")),
	))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.JSONEq(t, `{"status":"unavailable","failing":["db","ledger"]}`, rec.Body.String())
	assert.Equal(t, []string{"db", "cache", "ledger"}, ran, "a failing check must not stop later ones")
}

func TestReadyzPassesRequestContextToChecks(t *testing.T) {
	type key struct{}
	var got any
	h := Readyz(Check{"ctx", func(ctx context.Context) error { got = ctx.Value(key{}); return nil }})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	h(httptest.NewRecorder(), req.WithContext(context.WithValue(req.Context(), key{}, "from-request")))

	assert.Equal(t, "from-request", got)
}
