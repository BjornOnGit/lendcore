CREATE TABLE transactions (
    id UUID PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE,
    request_hash TEXT NOT NULL,
    kind TEXT NOT NULL,            -- DISBURSEMENT, REPAYMENT, REVERSAL, PENALTY
    reference TEXT,
    reversal_of UUID REFERENCES transactions (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE entries (
    id BIGSERIAL PRIMARY KEY,
    transaction_id UUID NOT NULL REFERENCES transactions (id),
    account_id UUID NOT NULL REFERENCES accounts (id),
    direction TEXT NOT NULL CHECK (direction IN ('DEBIT', 'CREDIT')),
    amount BIGINT NOT NULL CHECK (amount > 0)
);

CREATE INDEX entries_account_idx ON entries (account_id, id);