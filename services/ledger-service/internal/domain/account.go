package domain

import (
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/BjornOnGit/lendcore/pkg/money"
)

// Account is a ledger account: a balance in one currency.
type Account struct {
	ID            uuid.UUID
	OwnerID       *uuid.UUID // nil for system accounts such as the funding pool
	Type          string     // CUSTOMER_WALLET, LOAN_RECEIVABLE, FUNDING_POOL or INTEREST_INCOME
	Currency      money.Currency
	Balance       money.Amount // cached; the entries are the source of truth
	AllowNegative bool
	CreatedAt     time.Time // always UTC
}

// The kinds of ledger account.
const (
	AccountCustomerWallet = "CUSTOMER_WALLET"
	AccountFundingPool    = "FUNDING_POOL"
	AccountInterestIncome = "INTEREST_INCOME"
	AccountLoanReceivable = "LOAN_RECEIVABLE"
)

// AccountTypes lists the allowed account types, sorted.
var AccountTypes = []string{AccountCustomerWallet, AccountFundingPool, AccountInterestIncome, AccountLoanReceivable}

// ValidateNewAccount checks that accountType is one of AccountTypes and that
// currency is a 3-letter uppercase code. Failures wrap httpx.ErrValidation.
func ValidateNewAccount(accountType string, currency money.Currency) error {
	if !slices.Contains(AccountTypes, accountType) {
		return invalid("type must be one of %s, got %q", strings.Join(AccountTypes, ", "), accountType)
	}
	if len(currency) != 3 || strings.Trim(string(currency), "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		return invalid("currency must be a 3-letter uppercase code such as NGN, got %q", currency)
	}
	return nil
}
