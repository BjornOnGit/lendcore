//go:build integration

package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testDSN returns the DSN of a disposable Postgres, or skips the test.
// TEST_DATABASE_URL is deliberately separate from DATABASE_URL so tests never
// run against a database a service is configured for.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; start the compose Postgres (T2.1) and set it")
	}
	return dsn
}

func TestConnectIntegration(t *testing.T) {
	dsn := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	require.NoError(t, pool.Ping(ctx))
	var one int
	require.NoError(t, pool.QueryRow(ctx, "SELECT 1").Scan(&one))
	assert.Equal(t, 1, one)
	assert.EqualValues(t, maxConns, pool.Config().MaxConns)
}
