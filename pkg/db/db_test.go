package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectBadDSN(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{"not a dsn", "this is not a dsn"},
		{"bad port", "postgres://user:s3cret@localhost:notaport/db"},
		{"bad sslmode", "postgres://user:s3cret@localhost/db?sslmode=bogus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := Connect(context.Background(), tt.dsn)

			assert.Nil(t, pool)
			require.Error(t, err)
			assert.ErrorContains(t, err, "db: parse dsn")
			assert.NotContains(t, err.Error(), "s3cret", "the password must not leak into errors")
		})
	}
}

func TestConnectUnreachableDatabase(t *testing.T) {
	// Port 1 on loopback refuses connections immediately: a valid DSN whose ping fails.
	pool, err := Connect(context.Background(), "postgres://user:s3cret@127.0.0.1:1/db?sslmode=disable")

	assert.Nil(t, pool)
	require.Error(t, err)
	assert.ErrorContains(t, err, "db: ping")
	assert.NotContains(t, err.Error(), "s3cret", "the password must not leak into errors")
}

func TestNewConfigAppliesPoolLimits(t *testing.T) {
	// A pool_max_conns in the DSN is deliberately overridden by our limits.
	cfg, err := newConfig("postgres://user:pw@localhost/db?pool_max_conns=99")

	require.NoError(t, err)
	assert.EqualValues(t, maxConns, cfg.MaxConns)
	assert.Equal(t, maxConnLifetime, cfg.MaxConnLifetime)
	assert.Equal(t, maxConnIdleTime, cfg.MaxConnIdleTime)
}
