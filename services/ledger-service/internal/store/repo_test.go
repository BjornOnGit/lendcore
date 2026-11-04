package store

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/BjornOnGit/lendcore/pkg/money"
	"github.com/BjornOnGit/lendcore/services/ledger-service/internal/domain"
)

func TestMapAccount(t *testing.T) {
	wat := time.FixedZone("WAT", 3600) // UTC+1, like Lagos
	id, owner := uuid.New(), uuid.New()
	createdLocal := time.Date(2026, 11, 3, 12, 7, 7, 0, wat) // 11:07:07 UTC

	t.Run("copies every field and converts to domain types", func(t *testing.T) {
		got := mapAccount(Account{
			ID: id, OwnerID: &owner, Type: "CUSTOMER_WALLET", Currency: "NGN",
			Balance: 12345, AllowNegative: true, CreatedAt: createdLocal,
		})

		assert.Equal(t, domain.Account{
			ID: id, OwnerID: &owner, Type: "CUSTOMER_WALLET", Currency: money.NGN,
			Balance: 12345, AllowNegative: true, CreatedAt: createdLocal.UTC(),
		}, got)
	})

	t.Run("CreatedAt is normalized to UTC and keeps the same instant", func(t *testing.T) {
		got := mapAccount(Account{CreatedAt: createdLocal})

		assert.Equal(t, time.UTC, got.CreatedAt.Location())
		assert.True(t, got.CreatedAt.Equal(createdLocal))
		assert.Equal(t, 11, got.CreatedAt.Hour())
	})

	t.Run("a missing owner stays nil", func(t *testing.T) {
		assert.Nil(t, mapAccount(Account{}).OwnerID)
	})
}
