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