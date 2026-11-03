//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BjornOnGit/lendcore/pkg/db"
	"github.com/BjornOnGit/lendcore/pkg/testutil"
	"github.com/BjornOnGit/lendcore/services/ledger-service/migrations"
)

// migratedQueries returns Queries on a fresh database with the ledger migrations applied.
func migratedQueries(t *testing.T) *Queries {
	t.Helper()
	dsn := testutil.NewDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	require.NoError(t, db.Migrate(ctx, dsn, migrations.FS, "."))
	pool, err := db.Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return New(pool)
}

func TestAccountQueries(t *testing.T) {
	q := migratedQueries(t)
	ctx := context.Background()

	owner := uuid.New()
	wallet, err := q.CreateAccount(ctx, CreateAccountParams{
		ID: uuid.New(), OwnerID: &owner, Type: "CUSTOMER_WALLET", Currency: "NGN",
	})
	require.NoError(t, err)

	t.Run("CreateAccount returns the stored row with defaults applied", func(t *testing.T) {
		require.NotNil(t, wallet.OwnerID)
		assert.Equal(t, owner, *wallet.OwnerID)
		assert.Equal(t, "CUSTOMER_WALLET", wallet.Type)
		assert.Equal(t, "NGN", wallet.Currency)
		assert.Zero(t, wallet.Balance)
		assert.False(t, wallet.AllowNegative)
		assert.WithinDuration(t, time.Now(), wallet.CreatedAt, time.Minute)
	})

	t.Run("an account without an owner has a nil OwnerID", func(t *testing.T) {
		fundingPool, err := q.CreateAccount(ctx, CreateAccountParams{
			ID: uuid.New(), Type: "FUNDING_POOL", Currency: "NGN", AllowNegative: true,
		})
		require.NoError(t, err)
		assert.Nil(t, fundingPool.OwnerID)
		assert.True(t, fundingPool.AllowNegative)

		got, err := q.GetAccount(ctx, fundingPool.ID)
		require.NoError(t, err)
		assert.Nil(t, got.OwnerID)
	})

	t.Run("GetAccount", func(t *testing.T) {
		got, err := q.GetAccount(ctx, wallet.ID)
		require.NoError(t, err)
		assert.Equal(t, wallet, got)

		_, err = q.GetAccount(ctx, uuid.New())
		assert.ErrorIs(t, err, pgx.ErrNoRows)
	})

	t.Run("GetAccountByOwnerAndType", func(t *testing.T) {
		got, err := q.GetAccountByOwnerAndType(ctx, GetAccountByOwnerAndTypeParams{OwnerID: owner, Type: "CUSTOMER_WALLET"})
		require.NoError(t, err)
		assert.Equal(t, wallet.ID, got.ID)

		_, err = q.GetAccountByOwnerAndType(ctx, GetAccountByOwnerAndTypeParams{OwnerID: owner, Type: "LOAN_RECEIVABLE"})
		assert.ErrorIs(t, err, pgx.ErrNoRows, "same owner, other type")

		_, err = q.GetAccountByOwnerAndType(ctx, GetAccountByOwnerAndTypeParams{OwnerID: uuid.New(), Type: "CUSTOMER_WALLET"})
		assert.ErrorIs(t, err, pgx.ErrNoRows, "other owner, same type")
	})

	t.Run("ListAccountsByType returns only that type, ordered by created_at then id", func(t *testing.T) {
		base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		// create inserts an account of the listed type, then pins its created_at.
		create := func(id uuid.UUID, createdAt time.Time) uuid.UUID {
			owner := uuid.New()
			_, err := q.CreateAccount(ctx, CreateAccountParams{ID: id, OwnerID: &owner, Type: "LOAN_RECEIVABLE", Currency: "NGN"})
			require.NoError(t, err)
			_, err = q.db.Exec(ctx, "UPDATE accounts SET created_at = $1 WHERE id = $2", createdAt, id)
			require.NoError(t, err)
			return id
		}
		lowID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
		highID := uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")

		// Inserted out of order on purpose: neither insertion order, nor id order,
		// nor reverse time order equals the expected result.
		late := create(uuid.New(), base.Add(3*time.Hour))
		tiedHigh := create(highID, base.Add(2*time.Hour))
		early := create(uuid.New(), base.Add(1*time.Hour))
		tiedLow := create(lowID, base.Add(2*time.Hour)) // same created_at as tiedHigh: id breaks the tie

		// A different type must not show up.
		other := uuid.New()
		_, err := q.CreateAccount(ctx, CreateAccountParams{ID: uuid.New(), OwnerID: &other, Type: "CUSTOMER_WALLET", Currency: "NGN"})
		require.NoError(t, err)

		got, err := q.ListAccountsByType(ctx, "LOAN_RECEIVABLE")
		require.NoError(t, err)

		var gotIDs []uuid.UUID
		for _, a := range got {
			gotIDs = append(gotIDs, a.ID)
		}
		assert.Equal(t, []uuid.UUID{early, tiedLow, tiedHigh, late}, gotIDs)
	})

	t.Run("ListAccountsByType with no matches returns an empty, non-nil slice", func(t *testing.T) {
		got, err := q.ListAccountsByType(ctx, "INTEREST_INCOME")
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})
}
