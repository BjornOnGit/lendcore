//go:build integration

package store

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BjornOnGit/lendcore/pkg/httpx"
	"github.com/BjornOnGit/lendcore/pkg/money"
	"github.com/BjornOnGit/lendcore/services/ledger-service/internal/domain"
)

func TestRepoCreateAccount(t *testing.T) {
	pool := migratedPool(t)
	repo := NewRepo(pool)
	ctx := context.Background()

	t.Run("creates a new account", func(t *testing.T) {
		owner := uuid.New()

		acct, created, err := repo.CreateAccount(ctx, NewAccount{OwnerID: &owner, Type: "CUSTOMER_WALLET", Currency: money.NGN})

		require.NoError(t, err)
		assert.True(t, created)
		assert.NotEqual(t, uuid.Nil, acct.ID)
		require.NotNil(t, acct.OwnerID)
		assert.Equal(t, owner, *acct.OwnerID)
		assert.Equal(t, "CUSTOMER_WALLET", acct.Type)
		assert.Equal(t, money.NGN, acct.Currency)
		assert.Zero(t, acct.Balance)
		assert.False(t, acct.AllowNegative)
		assert.WithinDuration(t, time.Now(), acct.CreatedAt, time.Minute)
	})

	t.Run("stores the requested currency, type and allow_negative", func(t *testing.T) {
		owner := uuid.New()

		acct, _, err := repo.CreateAccount(ctx, NewAccount{OwnerID: &owner, Type: "LOAN_RECEIVABLE", Currency: "USD", AllowNegative: true})
		require.NoError(t, err)
		got, err := repo.GetAccount(ctx, acct.ID)
		require.NoError(t, err)

		for _, a := range []domain.Account{acct, got} {
			assert.Equal(t, money.Currency("USD"), a.Currency)
			assert.Equal(t, "LOAN_RECEIVABLE", a.Type)
			assert.True(t, a.AllowNegative)
		}
	})

	t.Run("creating twice with the same owner and type returns the same account", func(t *testing.T) {
		owner := uuid.New()
		in := NewAccount{OwnerID: &owner, Type: "CUSTOMER_WALLET", Currency: money.NGN}

		first, createdFirst, err := repo.CreateAccount(ctx, in)
		require.NoError(t, err)
		second, createdSecond, err := repo.CreateAccount(ctx, in)
		require.NoError(t, err)

		assert.True(t, createdFirst)
		assert.False(t, createdSecond, "the second call finds the existing account")
		assert.Equal(t, first.ID, second.ID)
		assert.Equal(t, first, second)
	})

	t.Run("a different type or a different owner is a different account", func(t *testing.T) {
		ownerA, ownerB := uuid.New(), uuid.New()
		wallet, _, err := repo.CreateAccount(ctx, NewAccount{OwnerID: &ownerA, Type: "CUSTOMER_WALLET", Currency: money.NGN})
		require.NoError(t, err)

		otherType, created, err := repo.CreateAccount(ctx, NewAccount{OwnerID: &ownerA, Type: "LOAN_RECEIVABLE", Currency: money.NGN})
		require.NoError(t, err)
		assert.True(t, created)
		assert.NotEqual(t, wallet.ID, otherType.ID)

		otherOwner, created, err := repo.CreateAccount(ctx, NewAccount{OwnerID: &ownerB, Type: "CUSTOMER_WALLET", Currency: money.NGN})
		require.NoError(t, err)
		assert.True(t, created)
		assert.NotEqual(t, wallet.ID, otherOwner.ID)
	})

	t.Run("an existing account is returned as stored even if the request differs", func(t *testing.T) {
		owner := uuid.New()
		first, _, err := repo.CreateAccount(ctx, NewAccount{OwnerID: &owner, Type: "CUSTOMER_WALLET", Currency: money.NGN})
		require.NoError(t, err)

		again, created, err := repo.CreateAccount(ctx, NewAccount{OwnerID: &owner, Type: "CUSTOMER_WALLET", Currency: "USD", AllowNegative: true})

		require.NoError(t, err)
		assert.False(t, created)
		assert.Equal(t, first, again, "the stored account wins; the request is not applied")
	})

	t.Run("accounts without an owner are never merged", func(t *testing.T) {
		in := NewAccount{Type: "FUNDING_POOL", Currency: money.NGN, AllowNegative: true}

		first, createdFirst, err := repo.CreateAccount(ctx, in)
		require.NoError(t, err)
		second, createdSecond, err := repo.CreateAccount(ctx, in)
		require.NoError(t, err)

		assert.True(t, createdFirst)
		assert.True(t, createdSecond)
		assert.NotEqual(t, first.ID, second.ID)
		assert.Nil(t, first.OwnerID)
		assert.True(t, first.AllowNegative)
	})

	t.Run("concurrent creates for the same owner and type yield exactly one account", func(t *testing.T) {
		owner := uuid.New()
		const callers = 20
		type result struct {
			acct    uuid.UUID
			created bool
			err     error
		}
		results := make([]result, callers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range callers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				acct, created, err := repo.CreateAccount(ctx, NewAccount{OwnerID: &owner, Type: "CUSTOMER_WALLET", Currency: money.NGN})
				results[i] = result{acct.ID, created, err}
			}()
		}
		close(start)
		wg.Wait()

		createdCount := 0
		for _, r := range results {
			require.NoError(t, r.err)
			assert.Equal(t, results[0].acct, r.acct, "every caller sees the same account")
			if r.created {
				createdCount++
			}
		}
		assert.Equal(t, 1, createdCount, "exactly one caller creates it")

		var rows int
		require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM accounts WHERE owner_id = $1", owner).Scan(&rows))
		assert.Equal(t, 1, rows)
	})
}

func TestRepoGetAccount(t *testing.T) {
	repo := NewRepo(migratedPool(t))
	ctx := context.Background()

	t.Run("returns the account that was created", func(t *testing.T) {
		owner := uuid.New()
		created, _, err := repo.CreateAccount(ctx, NewAccount{OwnerID: &owner, Type: "CUSTOMER_WALLET", Currency: money.NGN})
		require.NoError(t, err)

		got, err := repo.GetAccount(ctx, created.ID)

		require.NoError(t, err)
		assert.Equal(t, created, got)
	})

	t.Run("an unknown ID is ErrNotFound", func(t *testing.T) {
		id := uuid.New()

		_, err := repo.GetAccount(ctx, id)

		require.ErrorIs(t, err, httpx.ErrNotFound)
		assert.ErrorContains(t, err, id.String())
		status, code := httpx.MapError(err)
		assert.Equal(t, http.StatusNotFound, status)
		assert.Equal(t, "NOT_FOUND", code)
	})
}

// pgx decodes timestamptz into the process's local time zone; the repository
// must hand out UTC regardless. Setting time.Local makes that observable on any machine.
func TestRepoReturnsUTCTimestamps(t *testing.T) {
	wat := time.FixedZone("WAT", 3600) // UTC+1, like Lagos
	original := time.Local
	time.Local = wat
	t.Cleanup(func() { time.Local = original }) // registered first, so it runs after the pool is closed

	pool := migratedPool(t)
	repo := NewRepo(pool)
	ctx := context.Background()
	owner := uuid.New()

	created, _, err := repo.CreateAccount(ctx, NewAccount{OwnerID: &owner, Type: "CUSTOMER_WALLET", Currency: money.NGN})
	require.NoError(t, err)
	got, err := repo.GetAccount(ctx, created.ID)
	require.NoError(t, err)

	raw, err := New(pool).GetAccount(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, wat, raw.CreatedAt.Location(), "precondition: the generated query returns local time")
	assert.Equal(t, time.UTC, created.CreatedAt.Location())
	assert.Equal(t, time.UTC, got.CreatedAt.Location())
	assert.True(t, got.CreatedAt.Equal(raw.CreatedAt), "same instant, different zone")
}
