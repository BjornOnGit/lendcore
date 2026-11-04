package domain

import (
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
