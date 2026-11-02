package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDSN(t *testing.T) {
	tests := []struct {
		name string
		base string
		db   string
		want string
	}{
		{"replaces the database", "postgres://u:p@127.0.0.1:5433/loans?sslmode=disable", "ledger",
			"postgres://u:p@127.0.0.1:5433/ledger?sslmode=disable"},
		{"adds a database when the base has none", "postgres://u:p@localhost:5432", "ledger",
			"postgres://u:p@localhost:5432/ledger"},
		{"keeps query options", "postgres://u:p@h/db?sslmode=disable&application_name=x", "other",
			"postgres://u:p@h/other?sslmode=disable&application_name=x"},
		{"accepts the postgresql scheme", "postgresql://u:p@h/db", "x", "postgresql://u:p@h/x"},
		{"keeps an escaped password intact", "postgres://u:p%40ss@h/db", "x", "postgres://u:p%40ss@h/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildDSN(tt.base, tt.db)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildDSNErrors(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		db      string
		wantErr string
	}{
		{"empty database name", "postgres://u:s3cret@h/db", "", "empty database name"},
		{"keyword/value DSN", "host=localhost user=u password=s3cret", "x", "must be a postgres:// URL"},
		{"wrong scheme", "mysql://u:s3cret@h/db", "x", "must be a postgres:// URL"},
		{"unparsable URL", "postgres://u:s3cret@h:notaport/db", "x", "not a valid URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildDSN(tt.base, tt.db)
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
			assert.NotContains(t, err.Error(), "s3cret", "the password must not leak into errors")
		})
	}
}
