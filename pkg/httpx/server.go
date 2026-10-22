package httpx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// shutdownTimeout is how long in-flight requests get to finish after the
// context is cancelled; it fits inside Kubernetes' default 30s grace period.
const shutdownTimeout = 10 * time.Second

// newServer builds an http.Server with conservative timeouts, so slow or
// stalled clients cannot hold connections open forever, and with server-level
// errors routed through logger instead of the standard library's plain log.
func newServer(handler http.Handler, logger *slog.Logger) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
}

// Run serves handler on addr until ctx is cancelled, then shuts down
// gracefully: it stops accepting connections and waits up to 10s for
// in-flight requests. It returns nil after a clean shutdown, or the error
// that stopped the server (listen failure, serve failure, shutdown timeout).
func Run(ctx context.Context, addr string, handler http.Handler, logger *slog.Logger) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	return serveListener(ctx, ln, handler, logger)
}

// serveListener is Run on an existing listener, which lets tests bind port 0 and
// learn the address.
func serveListener(ctx context.Context, ln net.Listener, handler http.Handler, logger *slog.Logger) error {
	srv := newServer(handler, logger)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	logger.Info("http server listening", "addr", ln.Addr().String())

	select {
	case err := <-errCh:
		return fmt.Errorf("serve: %w", err) // stopped without a shutdown request
	case <-ctx.Done():
	}

	logger.Info("http server shutting down")
	// ctx is already cancelled, so the shutdown deadline starts from a fresh context.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close() // drop whatever is still connected
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	logger.Info("http server stopped")
	return nil
}
