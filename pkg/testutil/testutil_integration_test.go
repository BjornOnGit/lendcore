//go:build integration

package testutil

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dbNameOf(t *testing.T, dsn string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	return strings.TrimPrefix(u.Path, "/")
}

func connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

func databaseExists(t *testing.T, name string) bool {
	t.Helper()
	var n int
	err := connect(t, DSN(t, "postgres")).
		QueryRow(context.Background(), "SELECT count(*) FROM pg_database WHERE datname = $1", name).Scan(&n)
	require.NoError(t, err)
	return n == 1
}

func TestDSNPointsAtRequestedDatabase(t *testing.T) {
	var current string
	err := connect(t, DSN(t, "postgres")).QueryRow(context.Background(), "SELECT current_database()").Scan(&current)

	require.NoError(t, err)
	assert.Equal(t, "postgres", current)
}

func TestNewDatabaseIsQueryableAndDroppedAfterTest(t *testing.T) {
	var name string
	t.Run("use the database", func(t *testing.T) {
		dsn := NewDatabase(t)
		name = dbNameOf(t, dsn)

		var current string
		require.NoError(t, connect(t, dsn).QueryRow(context.Background(), "SELECT current_database()").Scan(&current))
		assert.Equal(t, name, current)
		assert.True(t, databaseExists(t, name), "database exists while the test runs")
	})

	// The subtest's cleanups have run by now.
	assert.False(t, databaseExists(t, name), "database must be dropped when the test finishes")
}

func TestNewDatabaseIsUniqueAndIsolated(t *testing.T) {
	a, b := NewDatabase(t), NewDatabase(t)
	require.NotEqual(t, a, b)

	ctx := context.Background()
	_, err := connect(t, a).Exec(ctx, "CREATE TABLE only_in_a (id int)")
	require.NoError(t, err)

	var inA, inB bool
	require.NoError(t, connect(t, a).QueryRow(ctx, "SELECT to_regclass('public.only_in_a') IS NOT NULL").Scan(&inA))
	require.NoError(t, connect(t, b).QueryRow(ctx, "SELECT to_regclass('public.only_in_a') IS NOT NULL").Scan(&inB))
	assert.True(t, inA)
	assert.False(t, inB, "tables must not leak between test databases")
}
