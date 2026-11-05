package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BjornOnGit/lendcore/pkg/httpx"
	"github.com/BjornOnGit/lendcore/pkg/money"
)

func TestValidateNewAccount(t *testing.T) {
	for _, typ := range AccountTypes {
		assert.NoError(t, ValidateNewAccount(typ, money.NGN), typ)
	}
	assert.NoError(t, ValidateNewAccount(AccountFundingPool, "USD"))

	tests := []struct {
		name     string
		typ      string
		currency money.Currency
		reason   string
	}{
		{"unknown type", "WALLET", "NGN", `type must be one of CUSTOMER_WALLET, FUNDING_POOL, INTEREST_INCOME, LOAN_RECEIVABLE, got "WALLET"`},
		{"empty type", "", "NGN", "type must be one of"},
		{"lowercase type", "customer_wallet", "NGN", "type must be one of"},
		{"empty currency", AccountCustomerWallet, "", "currency must be a 3-letter uppercase code"},
		{"lowercase currency", AccountCustomerWallet, "ngn", `got "ngn"`},
		{"too short", AccountCustomerWallet, "NG", "currency must be"},
		{"too long", AccountCustomerWallet, "NGNN", "currency must be"},
		{"digits", AccountCustomerWallet, "N6N", "currency must be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNewAccount(tt.typ, tt.currency)

			require.Error(t, err)
			assert.ErrorIs(t, err, httpx.ErrValidation)
			assert.ErrorContains(t, err, tt.reason)
		})
	}
}
