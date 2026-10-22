//go:build integration

package db

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tempDatabase creates an empty, uniquely named database on the server that
// adminDSN (a postgres:// URL) points at, and returns a DSN for it. The
// database is dropped when the test ends, so migrations never touch shared data.
func tempDatabase(t *testing.T, adminDSN string) string {
	t.Helper()
	u, err := url.Parse(adminDSN)
	require.NoError(t, err, "TEST_DATABASE_URL must be a postgres:// URL")

	ctx := context.Background()
	admin, err := Connect(ctx, adminDSN)
	require.NoError(t, err)
	t.Cleanup(admin.Close)

	name := "lendcore_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ident := pgx.Identifier{name}.Sanitize()
	_, err = admin.Exec(ctx, "CREATE DATABASE "+ident)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)")
		assert.NoError(t, err)
	})

	u.Path = "/" + name
	return u.String()
}

func TestMigrateIntegration(t *testing.T) {
	dsn := tempDatabase(t, testDSN(t))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fsys := fstest.MapFS{
		// Two statements in one file, to prove multi-statement migrations work.
		"migrations/000001_create_probe.up.sql": {Data: []byte(
			"CREATE TABLE migrate_probe (id integer PRIMARY KEY);\n" +
				"CREATE INDEX migrate_probe_id_idx ON migrate_probe (id);")},
		"migrations/000001_create_probe.down.sql": {Data: []byte("DROP TABLE migrate_probe;")},
	}

	require.NoError(t, Migrate(ctx, dsn, fsys, "migrations"), "first run applies the migration")
	require.NoError(t, Migrate(ctx, dsn, fsys, "migrations"), "second run is a no-op, not an error")

	pool, err := Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	schemaVersion := func() (version int, dirty bool) {
		require.NoError(t, pool.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty))
		return version, dirty
	}
	version, dirty := schemaVersion()
	assert.Equal(t, 1, version)
	assert.False(t, dirty)

	var tableExists bool
	require.NoError(t, pool.QueryRow(ctx, "SELECT to_regclass('public.migrate_probe') IS NOT NULL").Scan(&tableExists))
	assert.True(t, tableExists)

	// A new migration added later is applied on top of the existing ones.
	fsys["migrations/000002_add_note.up.sql"] = &fstest.MapFile{Data: []byte("ALTER TABLE migrate_probe ADD COLUMN note text;")}
	fsys["migrations/000002_add_note.down.sql"] = &fstest.MapFile{Data: []byte("ALTER TABLE migrate_probe DROP COLUMN note;")}
	require.NoError(t, Migrate(ctx, dsn, fsys, "migrations"))

	version, dirty = schemaVersion()
	assert.Equal(t, 2, version)
	assert.False(t, dirty)
}
