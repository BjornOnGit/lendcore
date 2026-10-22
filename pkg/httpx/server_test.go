package httpx

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testLogger = slog.New(slog.NewTextHandler(io.Discard, nil))
	testClient = &http.Client{
		Timeout:   3 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
)

// fetch performs a GET and returns the status and body.
func fetch(url string) (int, string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return 0, "", err
	}
	resp, err := testClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	body, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	return resp.StatusCode, string(body), errors.Join(readErr, closeErr)
}

// startServer runs serveListener on a random local port. It returns the base URL, a
// function that cancels the server's context, and a channel carrying the result.
func startServer(t *testing.T, h http.Handler) (baseURL string, stop context.CancelFunc, done <-chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch := make(chan error, 1)
	go func() { ch <- serveListener(ctx, ln, h, testLogger) }()
	return "http://" + ln.Addr().String(), cancel, ch
}

func waitDone(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("serveListener did not return in time")
		return nil
	}
}

func TestServeServesThenShutsDownCleanly(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", Healthz)
	baseURL, stop, done := startServer(t, mux)

	status, body, err := fetch(baseURL + "/healthz")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.JSONEq(t, `{"status":"ok"}`, body)

	stop()
	assert.NoError(t, waitDone(t, done))

	_, _, err = fetch(baseURL + "/healthz")
	assert.Error(t, err, "listener must be closed after shutdown")
}

func TestServeShutdownWaitsForInflightRequest(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	baseURL, stop, done := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusAccepted)
	}))

	type result struct {
		status int
		err    error
	}
	resCh := make(chan result, 1)
	go func() {
		status, _, err := fetch(baseURL)
		resCh <- result{status, err}
	}()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("handler never started")
	}
	stop()

	select {
	case err := <-done:
		t.Fatalf("serveListener returned (%v) while a request was still in flight", err)
	case <-time.After(150 * time.Millisecond):
	}

	close(release)
	res := <-resCh
	require.NoError(t, res.err)
	assert.Equal(t, http.StatusAccepted, res.status, "in-flight request must complete")
	assert.NoError(t, waitDone(t, done))
}

func TestNewServerHasTimeouts(t *testing.T) {
	srv := newServer(http.NotFoundHandler(), testLogger)

	assert.Positive(t, srv.ReadHeaderTimeout)
	assert.Positive(t, srv.ReadTimeout)
	assert.Positive(t, srv.WriteTimeout)
	assert.Positive(t, srv.IdleTimeout)
}

func TestRunReturnsListenError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	// The address is already taken, so Run must fail to listen and return.
	err = Run(context.Background(), ln.Addr().String(), http.NotFoundHandler(), testLogger)
	assert.ErrorContains(t, err, "listen")
}
