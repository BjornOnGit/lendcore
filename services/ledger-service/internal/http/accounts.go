// Package httpapi holds the ledger service's HTTP handlers.
package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/BjornOnGit/lendcore/pkg/httpx"
	"github.com/BjornOnGit/lendcore/pkg/logx"
	"github.com/BjornOnGit/lendcore/pkg/money"
	"github.com/BjornOnGit/lendcore/services/ledger-service/internal/domain"
	"github.com/BjornOnGit/lendcore/services/ledger-service/internal/store"
)

// AccountStore is what the account handlers need from persistence.
type AccountStore interface {
	CreateAccount(ctx context.Context, in store.NewAccount) (acct domain.Account, created bool, err error)
}

// Accounts serves the /v1/accounts endpoints.
type Accounts struct {
	store AccountStore
	log   *slog.Logger
}

// NewAccounts returns the account handlers.
func NewAccounts(store AccountStore, log *slog.Logger) *Accounts {
	return &Accounts{store: store, log: log}
}

type createAccountRequest struct {
	OwnerID       *string `json:"owner_id"` // optional; omit for system accounts
	Type          string  `json:"type"`
	Currency      string  `json:"currency"`
	AllowNegative bool    `json:"allow_negative"`
}

type accountResponse struct {
	ID            uuid.UUID  `json:"id"`
	OwnerID       *uuid.UUID `json:"owner_id"` // null for system accounts
	Type          string     `json:"type"`
	Currency      string     `json:"currency"`
	Balance       int64      `json:"balance"` // minor units
	AllowNegative bool       `json:"allow_negative"`
	CreatedAt     time.Time  `json:"created_at"` // UTC
}

func toAccountResponse(a domain.Account) accountResponse {
	return accountResponse{
		ID: a.ID, OwnerID: a.OwnerID, Type: a.Type, Currency: string(a.Currency),
		Balance: int64(a.Balance), AllowNegative: a.AllowNegative, CreatedAt: a.CreatedAt,
	}
}

// Create handles POST /v1/accounts. It answers 201 with the new account, or
// 200 with the existing one when the owner already has an account of that type.
func (h *Accounts) Create(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		h.fail(w, r, fmt.Errorf("%w: %v", httpx.ErrValidation, err))
		return
	}
	if err := domain.ValidateNewAccount(req.Type, money.Currency(req.Currency)); err != nil {
		h.fail(w, r, err)
		return
	}
	in := store.NewAccount{Type: req.Type, Currency: money.Currency(req.Currency), AllowNegative: req.AllowNegative}
	if req.OwnerID != nil {
		owner, err := uuid.Parse(*req.OwnerID)
		if err != nil {
			h.fail(w, r, fmt.Errorf("%w: owner_id must be a UUID, got %q", httpx.ErrValidation, *req.OwnerID))
			return
		}
		in.OwnerID = &owner
	}

	acct, created, err := h.store.CreateAccount(r.Context(), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.WriteJSON(w, status, toAccountResponse(acct))
}

// fail writes err in the standard error shape. Unexpected errors are logged
// in full and reported to the client only as a generic 500.
func (h *Accounts) fail(w http.ResponseWriter, r *http.Request, err error) {
	status, code := httpx.MapError(err)
	message := err.Error()
	if status == http.StatusInternalServerError {
		h.log.ErrorContext(r.Context(), "request failed",
			"method", r.Method, "path", r.URL.Path, "request_id", logx.RequestID(r.Context()), "error", err)
		message = "internal error"
	}
	httpx.WriteError(w, status, code, message)
}
