//go:build integration

package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BjornOnGit/lendcore/pkg/db"
	"github.com/BjornOnGit/lendcore/pkg/testutil"
	"github.com/BjornOnGit/lendcore/services/ledger-service/internal/store"
	"github.com/BjornOnGit/lendcore/services/ledger-service/migrations"
)

// The handler against the real repository and a migrated database.
func TestCreateAccountEndToEnd(t *testing.T) {
	dsn := testutil.NewDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	require.NoError(t, db.Migrate(ctx, dsn, migrations.FS, "."))
	pool, err := db.Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	h := NewAccounts(store.NewRepo(pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	body := `{"owner_id":"` + uuid.NewString() + `","type":"CUSTOMER_WALLET","currency":"NGN"}`

	first := postAccount(h, body)
	second := postAccount(h, body)

	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	assert.Equal(t, decode(t, first), decode(t, second), "the duplicate returns the very same account")
}
