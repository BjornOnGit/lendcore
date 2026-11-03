package migrations_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BjornOnGit/lendcore/services/ledger-service/migrations"
)

func TestEveryUpMigrationHasADown(t *testing.T) {
	ups, err := fs.Glob(migrations.FS, "*.up.sql")
	require.NoError(t, err)
	require.NotEmpty(t, ups, "no migrations embedded")

	for _, up := range ups {
		down := strings.TrimSuffix(up, ".up.sql") + ".down.sql"
		_, err := fs.Stat(migrations.FS, down)
		assert.NoError(t, err, "%s has no matching down migration", up)
	}
}
