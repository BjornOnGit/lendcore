package db

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validFS is a well-formed migrations directory.
func validFS() fstest.MapFS {
	return fstest.MapFS{
		"migrations/000001_init.up.sql":   {Data: []byte("SELECT 1;")},
		"migrations/000001_init.down.sql": {Data: []byte("SELECT 1;")},
	}
}

func TestMigrateMissingDir(t *testing.T) {
	err := Migrate(context.Background(), "postgres://user:pw@127.0.0.1:1/db", fstest.MapFS{}, "migrations")

	require.Error(t, err)
	assert.ErrorContains(t, err, "db: migrations source")
}

func TestMigrateBadDSN(t *testing.T) {
	err := Migrate(context.Background(), "postgres://user:s3cret@localhost:notaport/db", validFS(), "migrations")

	require.Error(t, err)
	assert.ErrorContains(t, err, "db: parse dsn")
	assert.NotContains(t, err.Error(), "s3cret", "the password must not leak into errors")
}

func TestMigrateUnreachableDatabase(t *testing.T) {
	// Port 1 on loopback refuses connections immediately: a valid DSN whose ping fails.
	err := Migrate(context.Background(), "postgres://user:s3cret@127.0.0.1:1/db?sslmode=disable", validFS(), "migrations")

	require.Error(t, err)
	assert.ErrorContains(t, err, "db: ping")
	assert.NotContains(t, err.Error(), "s3cret", "the password must not leak into errors")
}
