package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BjornOnGit/lendcore/pkg/money"
	"github.com/BjornOnGit/lendcore/services/ledger-service/internal/domain"
	"github.com/BjornOnGit/lendcore/services/ledger-service/internal/store"
)

// fakeStore mimics the repository's contract: accounts with an owner are
// unique per (owner, type); accounts without an owner are always new.
type fakeStore struct {
	accounts map[string]domain.Account
	calls    []store.NewAccount
	err      error
}

func (f *fakeStore) CreateAccount(_ context.Context, in store.NewAccount) (domain.Account, bool, error) {
	f.calls = append(f.calls, in)
	if f.err != nil {
		return domain.Account{}, false, f.err
	}
	key := ""
	if in.OwnerID != nil {
		key = in.OwnerID.String() + "/" + in.Type
		if existing, ok := f.accounts[key]; ok {
			return existing, false, nil
		}
	}
	acct := domain.Account{
		ID: uuid.New(), OwnerID: in.OwnerID, Type: in.Type, Currency: in.Currency,
		AllowNegative: in.AllowNegative, CreatedAt: time.Date(2026, 11, 3, 11, 7, 7, 0, time.UTC),
	}
	if f.accounts == nil {
		f.accounts = map[string]domain.Account{}
	}
	if key != "" {
		f.accounts[key] = acct
	}
	return acct, true, nil
}

func newHandler(f *fakeStore) *Accounts {
	return NewAccounts(f, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func postAccount(h *Accounts, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/v1/accounts", strings.NewReader(body)))
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &m), rec.Body.String())
	return m
}

func TestCreateAccountValid(t *testing.T) {
	f := &fakeStore{}
	owner := uuid.New()

	rec := postAccount(newHandler(f), `{"owner_id":"`+owner.String()+`","type":"CUSTOMER_WALLET","currency":"NGN"}`)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	body := decode(t, rec)
	_, err := uuid.Parse(body["id"].(string))
	assert.NoError(t, err)
	assert.Equal(t, owner.String(), body["owner_id"])
	assert.Equal(t, "CUSTOMER_WALLET", body["type"])
	assert.Equal(t, "NGN", body["currency"])
	assert.EqualValues(t, 0, body["balance"])
	assert.Equal(t, false, body["allow_negative"])
	assert.Equal(t, "2026-11-03T11:07:07Z", body["created_at"])
	require.Len(t, f.calls, 1)
	assert.Equal(t, store.NewAccount{OwnerID: &owner, Type: "CUSTOMER_WALLET", Currency: money.NGN}, f.calls[0])
}

func TestCreateAccountDuplicateReturns200WithSameID(t *testing.T) {
	h := newHandler(&fakeStore{})
	body := `{"owner_id":"` + uuid.NewString() + `","type":"CUSTOMER_WALLET","currency":"NGN"}`

	first := postAccount(h, body)
	second := postAccount(h, body)

	assert.Equal(t, http.StatusCreated, first.Code)
	assert.Equal(t, http.StatusOK, second.Code)
	assert.Equal(t, decode(t, first)["id"], decode(t, second)["id"])
}

func TestCreateAccountWithoutOwner(t *testing.T) {
	f := &fakeStore{}

	rec := postAccount(newHandler(f), `{"type":"FUNDING_POOL","currency":"NGN","allow_negative":true}`)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	body := decode(t, rec)
	assert.Nil(t, body["owner_id"])
	assert.Equal(t, true, body["allow_negative"])
	require.Len(t, f.calls, 1)
	assert.Nil(t, f.calls[0].OwnerID)
	assert.True(t, f.calls[0].AllowNegative)

	again := postAccount(newHandler(f), `{"type":"FUNDING_POOL","currency":"NGN","allow_negative":true}`)
	assert.Equal(t, http.StatusCreated, again.Code, "ownerless accounts are never merged")
}

func TestCreateAccountInvalid(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		reason string
	}{
		{"unknown type", `{"type":"WALLET","currency":"NGN"}`, `type must be one of CUSTOMER_WALLET, FUNDING_POOL, INTEREST_INCOME, LOAN_RECEIVABLE, got "WALLET"`},
		{"missing type", `{"currency":"NGN"}`, "type must be one of"},
		{"missing currency", `{"type":"CUSTOMER_WALLET"}`, "currency must be a 3-letter uppercase code"},
		{"lowercase currency", `{"type":"CUSTOMER_WALLET","currency":"ngn"}`, `got "ngn"`},
		{"owner_id is not a UUID", `{"owner_id":"abc","type":"CUSTOMER_WALLET","currency":"NGN"}`, `owner_id must be a UUID, got "abc"`},
		{"malformed JSON", `{"type":`, "invalid JSON body"},
		{"empty body", ``, "invalid JSON body"},
		{"unknown field", `{"type":"CUSTOMER_WALLET","currency":"NGN","balance":5}`, `unknown field "balance"`},
		{"wrong field type", `{"type":5,"currency":"NGN"}`, "invalid JSON body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeStore{}

			rec := postAccount(newHandler(f), tt.body)

			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			errBody := decode(t, rec)["error"].(map[string]any)
			assert.Equal(t, "VALIDATION", errBody["code"])
			assert.Contains(t, errBody["message"], tt.reason)
			assert.Empty(t, f.calls, "nothing is stored for an invalid request")
		})
	}
}

func TestCreateAccountStoreFailureIs500AndNotLeaked(t *testing.T) {
	var logs bytes.Buffer
	h := NewAccounts(&fakeStore{err: errors.New("connection refused: password=hunter2")}, slog.New(slog.NewTextHandler(&logs, nil)))

	rec := postAccount(h, `{"type":"CUSTOMER_WALLET","currency":"NGN"}`)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	errBody := decode(t, rec)["error"].(map[string]any)
	assert.Equal(t, "INTERNAL", errBody["code"])
	assert.Equal(t, "internal error", errBody["message"])
	assert.NotContains(t, rec.Body.String(), "hunter2")
	assert.Contains(t, logs.String(), "hunter2", "the real error is logged for operators")
}
