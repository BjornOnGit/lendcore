// Package store is the ledger's persistence layer: the sqlc-generated queries
// (db.go, models.go, *.sql.go; regenerate with sqlc, never edit) and the Repo
// that wraps them in domain types.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BjornOnGit/lendcore/pkg/httpx"
	"github.com/BjornOnGit/lendcore/pkg/money"
	"github.com/BjornOnGit/lendcore/services/ledger-service/internal/domain"
)

// Repo is the ledger's persistence API. It wraps the generated queries and
// speaks in domain types: it assigns IDs, maps rows to domain values with UTC
// timestamps, and turns "no rows" into httpx.ErrNotFound.
type Repo struct {
	q *Queries
}

// NewRepo returns a Repo on db, which is usually a *pgxpool.Pool.
func NewRepo(db DBTX) *Repo {
	return &Repo{q: New(db)}
}

// NewAccount describes an account to create.
type NewAccount struct {
	OwnerID       *uuid.UUID // nil for a system account
	Type          string
	Currency      money.Currency
	AllowNegative bool
}

// CreateAccount creates the account and reports created=true, unless an
// account for the same owner and type already exists. In that case it returns
// the existing account unchanged (even if currency or allow_negative differ
// from the request) with created=false. This is safe under concurrency: of
// several simultaneous calls exactly one creates. Accounts without an owner
// are never merged, so each call creates a new one.
func (r *Repo) CreateAccount(ctx context.Context, in NewAccount) (acct domain.Account, created bool, err error) {
	row, err := r.q.CreateAccountIfAbsent(ctx, CreateAccountIfAbsentParams{
		ID:            uuid.New(),
		OwnerID:       in.OwnerID,
		Type:          in.Type,
		Currency:      string(in.Currency),
		AllowNegative: in.AllowNegative,
	})
	if err == nil {
		return mapAccount(row), true, nil
	}
	// No row back means the (owner, type) pair already exists, which can only
	// happen for an account that has an owner.
	if !errors.Is(err, pgx.ErrNoRows) || in.OwnerID == nil {
		return domain.Account{}, false, fmt.Errorf("store: create account: %w", err)
	}

	row, err = r.q.GetAccountByOwnerAndType(ctx, GetAccountByOwnerAndTypeParams{OwnerID: *in.OwnerID, Type: in.Type})
	if err != nil {
		return domain.Account{}, false, fmt.Errorf("store: load existing account: %w", err)
	}
	return mapAccount(row), false, nil
}

// GetAccount returns the account with the given ID, or an error wrapping
// httpx.ErrNotFound if there is none.
func (r *Repo) GetAccount(ctx context.Context, id uuid.UUID) (domain.Account, error) {
	row, err := r.q.GetAccount(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, fmt.Errorf("account %s: %w", id, httpx.ErrNotFound)
	}
	if err != nil {
		return domain.Account{}, fmt.Errorf("store: get account: %w", err)
	}
	return mapAccount(row), nil
}

// mapAccount converts a generated row to the domain type. pgx returns
// timestamps in the process's local time zone, so CreatedAt is normalized to UTC.
func mapAccount(a Account) domain.Account {
	return domain.Account{
		ID:            a.ID,
		OwnerID:       a.OwnerID,
		Type:          a.Type,
		Currency:      money.Currency(a.Currency),
		Balance:       money.Amount(a.Balance),
		AllowNegative: a.AllowNegative,
		CreatedAt:     a.CreatedAt.UTC(),
	}
}
