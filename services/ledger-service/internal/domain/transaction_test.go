package domain

import (
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BjornOnGit/lendcore/pkg/httpx"
	"github.com/BjornOnGit/lendcore/pkg/money"
)

var (
	acctA = uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	acctB = uuid.MustParse("00000000-0000-0000-0000-00000000000b")
	acctC = uuid.MustParse("00000000-0000-0000-0000-00000000000c")
)

func entry(account uuid.UUID, dir Direction, amount money.Amount) Entry {
	return Entry{AccountID: account, Direction: dir, Amount: amount, Currency: money.NGN}
}

func TestNewTransactionValid(t *testing.T) {
	tests := []struct {
		name    string
		entries []Entry
	}{
		{"one debit, one credit", []Entry{entry(acctA, Debit, 100), entry(acctB, Credit, 100)}},
		{"one debit split across two credits", []Entry{entry(acctA, Debit, 100), entry(acctB, Credit, 60), entry(acctC, Credit, 40)}},
		{"two debits, one credit", []Entry{entry(acctA, Debit, 70), entry(acctB, Debit, 30), entry(acctC, Credit, 100)}},
		{"smallest amounts", []Entry{entry(acctA, Debit, 1), entry(acctB, Credit, 1)}},
		{"largest amounts", []Entry{entry(acctA, Debit, math.MaxInt64), entry(acctB, Credit, math.MaxInt64)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txn, err := NewTransaction(TransactionRequest{IdempotencyKey: "key-1", Entries: tt.entries})

			require.NoError(t, err)
			assert.Equal(t, "key-1", txn.IdempotencyKey)
			assert.Equal(t, money.NGN, txn.Currency)
			assert.Equal(t, tt.entries, txn.Entries)
		})
	}
}

func TestNewTransactionDoesNotAliasRequestEntries(t *testing.T) {
	entries := []Entry{entry(acctA, Debit, 100), entry(acctB, Credit, 100)}
	txn, err := NewTransaction(TransactionRequest{IdempotencyKey: "key-1", Entries: entries})
	require.NoError(t, err)

	entries[0].Amount = 999

	assert.Equal(t, money.Amount(100), txn.Entries[0].Amount, "later changes to the request must not reach the validated transaction")
}

func TestNewTransactionInvalid(t *testing.T) {
	usd := func(e Entry) Entry { e.Currency = "USD"; return e }
	noCurrency := func(e Entry) Entry { e.Currency = ""; return e }
	pair := []Entry{entry(acctA, Debit, 100), entry(acctB, Credit, 100)}

	tests := []struct {
		name    string
		key     string
		entries []Entry
		reason  string // substring of the error message
	}{
		{"missing idempotency key", "", pair, "idempotency key is required"},
		{"blank idempotency key", "   ", pair, "idempotency key is required"},
		{"no entries", "k", nil, "at least 2 entries are required, got 0"},
		{"one entry", "k", pair[:1], "at least 2 entries are required, got 1"},
		{"zero amount", "k", []Entry{entry(acctA, Debit, 100), entry(acctB, Credit, 0)}, "entries[1]: amount must be positive, got 0"},
		{"negative amount", "k", []Entry{entry(acctA, Debit, -5), entry(acctB, Credit, -5)}, "entries[0]: amount must be positive, got -5"},
		{"unknown direction", "k", []Entry{entry(acctA, "SIDEWAYS", 100), entry(acctB, Credit, 100)}, `entries[0]: direction must be DEBIT or CREDIT, got "SIDEWAYS"`},
		{"empty direction", "k", []Entry{entry(acctA, Debit, 100), entry(acctB, "", 100)}, `entries[1]: direction must be DEBIT or CREDIT, got ""`},
		{"missing currency", "k", []Entry{entry(acctA, Debit, 100), noCurrency(entry(acctB, Credit, 100))}, "entries[1]: currency is required"},
		{"missing currency on the first entry", "k", []Entry{noCurrency(entry(acctA, Debit, 100)), entry(acctB, Credit, 100)}, "entries[0]: currency is required"},
		{"mixed currencies", "k", []Entry{entry(acctA, Debit, 100), usd(entry(acctB, Credit, 100))}, "entries mix currencies: NGN and USD"},
		{"credits less than debits", "k", []Entry{entry(acctA, Debit, 100), entry(acctB, Credit, 90)}, "debits (100) do not equal credits (90)"},
		{"credits more than debits", "k", []Entry{entry(acctA, Debit, 100), entry(acctB, Credit, 110)}, "debits (100) do not equal credits (110)"},
		{"debits only", "k", []Entry{entry(acctA, Debit, 100), entry(acctB, Debit, 100)}, "debits (200) do not equal credits (0)"},
		{"credits only", "k", []Entry{entry(acctA, Credit, 100), entry(acctB, Credit, 100)}, "debits (0) do not equal credits (200)"},
		{"amounts overflow when added", "k", []Entry{entry(acctA, Debit, math.MaxInt64), entry(acctB, Debit, 1), entry(acctC, Credit, 1)}, "too large to add up"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txn, err := NewTransaction(TransactionRequest{IdempotencyKey: tt.key, Entries: tt.entries})

			require.Error(t, err)
			assert.ErrorIs(t, err, httpx.ErrValidation)
			assert.ErrorContains(t, err, tt.reason)
			assert.Zero(t, txn, "no transaction is returned on failure")
		})
	}
}

func TestNewTransactionReportsTheFirstProblem(t *testing.T) {
	tests := []struct {
		name    string
		req     TransactionRequest
		reason  string
		because string
	}{
		{"key before entries", TransactionRequest{}, "idempotency key", "the key is checked first"},
		{"entry count before entry contents",
			TransactionRequest{IdempotencyKey: "k", Entries: []Entry{entry(acctA, Debit, 0)}},
			"at least 2 entries", "too few entries is reported before a bad amount"},
		{"entry problems before balance",
			TransactionRequest{IdempotencyKey: "k", Entries: []Entry{entry(acctA, Debit, 100), entry(acctB, Credit, 0)}},
			"amount must be positive", "a bad amount is reported before the totals are compared"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTransaction(tt.req)

			assert.ErrorContains(t, err, tt.reason, tt.because)
		})
	}
}
