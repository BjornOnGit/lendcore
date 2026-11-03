CREATE TABLE accounts (
    id UUID PRIMARY KEY,
    owner_id UUID,
    type TEXT NOT NULL,            -- CUSTOMER_WALLET, LOAN_RECEIVABLE, FUNDING_POOL, INTEREST_INCOME
    currency CHAR(3) NOT NULL,
    balance BIGINT NOT NULL DEFAULT 0,
    allow_negative BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One account per owner and type. Accounts without an owner (system accounts
-- such as the funding pool) are exempt, so several may share a type.
CREATE UNIQUE INDEX accounts_owner_type_uniq ON accounts (owner_id, type) WHERE owner_id IS NOT NULL;