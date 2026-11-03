// Package domain holds the ledger's business rules. It is pure Go: no
// database and no HTTP.
package domain

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/BjornOnGit/lendcore/pkg/httpx"
	"github.com/BjornOnGit/lendcore/pkg/money"
)

// Direction says which side of the books an entry is on.
type Direction string

// The two sides of a double-entry posting.
const (
	Debit  Direction = "DEBIT"
	Credit Direction = "CREDIT"
)

// Entry is one line of a transaction, against one account.
type Entry struct {
	AccountID uuid.UUID
	Direction Direction
	Amount    money.Amount // minor units; must be positive
	Currency  money.Currency
}

// TransactionRequest is an unvalidated request to post a transaction.
type TransactionRequest struct {
	IdempotencyKey string
	Entries        []Entry
}

// Transaction is a validated request: a balanced set of entries in a single currency.
type Transaction struct {
	IdempotencyKey string
	Currency       money.Currency
	Entries        []Entry
}

// NewTransaction validates req and returns the Transaction. It checks, in
// this order: the idempotency key is present, there are at least 2 entries,
// every entry has a positive amount, a valid direction and a currency, all
// entries share one currency, and total debits equal total credits. The first
// problem found is returned as an error wrapping httpx.ErrValidation with a
// reason the caller can show to a client. The returned Transaction does not
// alias req.Entries.
func NewTransaction(req TransactionRequest) (Transaction, error) {
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return Transaction{}, invalid("idempotency key is required")
	}
	if len(req.Entries) < 2 {
		return Transaction{}, invalid("at least 2 entries are required, got %d", len(req.Entries))
	}

	currency := req.Entries[0].Currency
	for i, e := range req.Entries {
		switch {
		case e.Amount <= 0:
			return Transaction{}, invalid("entries[%d]: amount must be positive, got %d", i, e.Amount)
		case e.Direction != Debit && e.Direction != Credit:
			return Transaction{}, invalid("entries[%d]: direction must be DEBIT or CREDIT, got %q", i, e.Direction)
		case e.Currency == "":
			return Transaction{}, invalid("entries[%d]: currency is required", i)
		case e.Currency != currency:
			return Transaction{}, invalid("entries mix currencies: %s and %s", currency, e.Currency)
		}
	}

	debits, credits := money.Money{Currency: currency}, money.Money{Currency: currency}
	for _, e := range req.Entries {
		amount := money.Money{Amount: e.Amount, Currency: currency}
		var err error
		if e.Direction == Debit {
			debits, err = debits.Add(amount)
		} else {
			credits, err = credits.Add(amount)
		}
		if err != nil {
			return Transaction{}, invalid("entry amounts are too large to add up")
		}
	}
	if debits != credits {
		return Transaction{}, invalid("debits (%d) do not equal credits (%d)", debits.Amount, credits.Amount)
	}

	return Transaction{
		IdempotencyKey: req.IdempotencyKey,
		Currency:       currency,
		Entries:        slices.Clone(req.Entries),
	}, nil
}

// invalid builds an httpx.ErrValidation error carrying a client-safe reason.
func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", httpx.ErrValidation, fmt.Sprintf(format, args...))
}
