-- name: CreateAccount :one
INSERT INTO accounts (id, owner_id, type, currency, allow_negative)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetAccount :one
SELECT * FROM accounts WHERE id = $1;

-- name: GetAccountByOwnerAndType :one
SELECT * FROM accounts
WHERE owner_id = sqlc.arg(owner_id)::uuid AND type = sqlc.arg(type);

-- name: ListAccountsByType :many
SELECT * FROM accounts WHERE type = $1 ORDER BY created_at, id;

-- name: CreateAccountIfAbsent :one
-- Inserts the account unless one already exists for this owner and type. On a
-- conflict nothing changes and no row is returned (pgx.ErrNoRows). Accounts
-- without an owner never conflict.
INSERT INTO accounts (id, owner_id, type, currency, allow_negative)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (owner_id, type) WHERE owner_id IS NOT NULL DO NOTHING
RETURNING *;