//go:build integration

package migrations_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BjornOnGit/lendcore/pkg/db"
	"github.com/BjornOnGit/lendcore/pkg/testutil"
	"github.com/BjornOnGit/lendcore/services/ledger-service/migrations"
)

// PostgreSQL error codes asserted below.
const (
	codeNotNull    = "23502"
	codeForeignKey = "23503"
	codeUnique     = "23505"
	codeCheck      = "23514"
)

func connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

// migrateDown rolls back every migration. db.Migrate only goes up, so the
// down direction is driven through golang-migrate directly.
func migrateDown(t *testing.T, dsn string) {
	t.Helper()
	src, err := iofs.New(migrations.FS, ".")
	require.NoError(t, err)
	cfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	driver, err := pgxmigrate.WithInstance(stdlib.OpenDB(*cfg), &pgxmigrate.Config{})
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	require.NoError(t, err)
	defer func() { _, _ = m.Close() }()
	require.NoError(t, m.Down())
}

func publicTables(t *testing.T, conn *pgx.Conn) []string {
	t.Helper()
	rows, err := conn.Query(context.Background(),
		"SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY 1")
	require.NoError(t, err)
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	return tables
}

// migratedDB returns a fresh database with all ledger migrations applied.
func migratedDB(t *testing.T) *pgx.Conn {
	t.Helper()
	dsn := testutil.NewDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	require.NoError(t, db.Migrate(ctx, dsn, migrations.FS, "."))
	return connect(t, dsn)
}

type column struct {
	Name, Type, Nullable, Default string
	MaxLen                        int
}

func tableColumns(t *testing.T, conn *pgx.Conn, table string) []column {
	t.Helper()
	rows, err := conn.Query(context.Background(), `
		SELECT column_name, data_type, is_nullable, COALESCE(column_default, ''), COALESCE(character_maximum_length, 0)
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1
		ORDER BY ordinal_position`, table)
	require.NoError(t, err)
	cols, err := pgx.CollectRows(rows, pgx.RowToStructByPos[column])
	require.NoError(t, err)
	return cols
}

// pgErrorOf asserts err is a PostgreSQL error and returns it.
func pgErrorOf(t *testing.T, err error) *pgconn.PgError {
	t.Helper()
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	return pgErr
}

func TestMigrationsUpDownUp(t *testing.T) {
	dsn := testutil.NewDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn := connect(t, dsn)
	wantTables := []string{"accounts", "entries", "schema_migrations", "transactions"}

	require.NoError(t, db.Migrate(ctx, dsn, migrations.FS, "."), "up")
	assert.ElementsMatch(t, wantTables, publicTables(t, conn))

	migrateDown(t, dsn)
	assert.Equal(t, []string{"schema_migrations"}, publicTables(t, conn), "down must remove every table it created")

	require.NoError(t, db.Migrate(ctx, dsn, migrations.FS, "."), "up again")
	assert.ElementsMatch(t, wantTables, publicTables(t, conn))
}

func TestAccountsColumns(t *testing.T) {
	got := tableColumns(t, migratedDB(t), "accounts")

	assert.Equal(t, []column{
		{"id", "uuid", "NO", "", 0},
		{"owner_id", "uuid", "YES", "", 0},
		{"type", "text", "NO", "", 0},
		{"currency", "character", "NO", "", 3},
		{"balance", "bigint", "NO", "0", 0},
		{"allow_negative", "boolean", "NO", "false", 0},
		{"created_at", "timestamp with time zone", "NO", "now()", 0},
	}, got)
}

func TestTransactionsColumns(t *testing.T) {
	got := tableColumns(t, migratedDB(t), "transactions")

	assert.Equal(t, []column{
		{"id", "uuid", "NO", "", 0},
		{"idempotency_key", "text", "NO", "", 0},
		{"request_hash", "text", "NO", "", 0},
		{"kind", "text", "NO", "", 0},
		{"reference", "text", "YES", "", 0},
		{"reversal_of", "uuid", "YES", "", 0},
		{"created_at", "timestamp with time zone", "NO", "now()", 0},
	}, got)
}

func TestEntriesColumns(t *testing.T) {
	got := tableColumns(t, migratedDB(t), "entries")

	assert.Equal(t, []column{
		{"id", "bigint", "NO", "nextval('entries_id_seq'::regclass)", 0},
		{"transaction_id", "uuid", "NO", "", 0},
		{"account_id", "uuid", "NO", "", 0},
		{"direction", "text", "NO", "", 0},
		{"amount", "bigint", "NO", "", 0},
	}, got)
}

func TestAccountsBehavior(t *testing.T) {
	conn := migratedDB(t)
	ctx := context.Background()

	// insert adds an NGN account; owner "" means no owner (a system account).
	insert := func(owner, typ string) error {
		var ownerArg any
		if owner != "" {
			ownerArg = owner
		}
		_, err := conn.Exec(ctx,
			"INSERT INTO accounts (id, owner_id, type, currency) VALUES ($1::uuid, $2::uuid, $3, 'NGN')",
			uuid.NewString(), ownerArg, typ)
		return err
	}

	t.Run("defaults", func(t *testing.T) {
		id := uuid.NewString()
		_, err := conn.Exec(ctx, "INSERT INTO accounts (id, type, currency) VALUES ($1::uuid, 'FUNDING_POOL', 'NGN')", id)
		require.NoError(t, err)

		var balance int64
		var allowNegative, hasCreatedAt bool
		require.NoError(t, conn.QueryRow(ctx,
			"SELECT balance, allow_negative, created_at IS NOT NULL FROM accounts WHERE id = $1::uuid", id).
			Scan(&balance, &allowNegative, &hasCreatedAt))
		assert.Zero(t, balance)
		assert.False(t, allowNegative)
		assert.True(t, hasCreatedAt)
	})

	t.Run("one account per owner and type", func(t *testing.T) {
		ownerA, ownerB := uuid.NewString(), uuid.NewString()
		require.NoError(t, insert(ownerA, "CUSTOMER_WALLET"))

		pgErr := pgErrorOf(t, insert(ownerA, "CUSTOMER_WALLET"))
		assert.Equal(t, codeUnique, pgErr.Code)
		assert.Equal(t, "accounts_owner_type_uniq", pgErr.ConstraintName)

		assert.NoError(t, insert(ownerA, "LOAN_RECEIVABLE"), "same owner, different type is allowed")
		assert.NoError(t, insert(ownerB, "CUSTOMER_WALLET"), "different owner, same type is allowed")
	})

	t.Run("system accounts without an owner are exempt", func(t *testing.T) {
		assert.NoError(t, insert("", "INTEREST_INCOME"))
		assert.NoError(t, insert("", "INTEREST_INCOME"))
	})
}

// execer is satisfied by *pgx.Conn and pgx.Tx.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func insertTransaction(ctx context.Context, e execer, id, key string, reversalOf any) error {
	_, err := e.Exec(ctx,
		"INSERT INTO transactions (id, idempotency_key, request_hash, kind, reversal_of) VALUES ($1::uuid, $2, 'hash', 'DISBURSEMENT', $3::uuid)",
		id, key, reversalOf)
	return err
}

func insertAccount(ctx context.Context, e execer, id string) error {
	_, err := e.Exec(ctx, "INSERT INTO accounts (id, type, currency) VALUES ($1::uuid, 'FUNDING_POOL', 'NGN')", id)
	return err
}

func insertEntry(ctx context.Context, e execer, txnID, accountID, direction string, amount int64) error {
	_, err := e.Exec(ctx,
		"INSERT INTO entries (transaction_id, account_id, direction, amount) VALUES ($1::uuid, $2::uuid, $3, $4)",
		txnID, accountID, direction, amount)
	return err
}

func TestTransactionsBehavior(t *testing.T) {
	conn := migratedDB(t)
	ctx := context.Background()

	t.Run("defaults", func(t *testing.T) {
		id := uuid.NewString()
		require.NoError(t, insertTransaction(ctx, conn, id, uuid.NewString(), nil))

		var reference, reversalOf *string
		var hasCreatedAt bool
		require.NoError(t, conn.QueryRow(ctx,
			"SELECT reference, reversal_of::text, created_at IS NOT NULL FROM transactions WHERE id = $1::uuid", id).
			Scan(&reference, &reversalOf, &hasCreatedAt))
		assert.Nil(t, reference)
		assert.Nil(t, reversalOf)
		assert.True(t, hasCreatedAt)
	})

	t.Run("idempotency key is unique", func(t *testing.T) {
		key := uuid.NewString()
		require.NoError(t, insertTransaction(ctx, conn, uuid.NewString(), key, nil))

		pgErr := pgErrorOf(t, insertTransaction(ctx, conn, uuid.NewString(), key, nil))
		assert.Equal(t, codeUnique, pgErr.Code)
		assert.Equal(t, "transactions_idempotency_key_key", pgErr.ConstraintName)
	})

	t.Run("request hash is required", func(t *testing.T) {
		_, err := conn.Exec(ctx,
			"INSERT INTO transactions (id, idempotency_key, kind) VALUES ($1::uuid, $2, 'REPAYMENT')",
			uuid.NewString(), uuid.NewString())

		pgErr := pgErrorOf(t, err)
		assert.Equal(t, codeNotNull, pgErr.Code)
		assert.Equal(t, "request_hash", pgErr.ColumnName)
	})

	t.Run("reversal_of must reference an existing transaction", func(t *testing.T) {
		pgErr := pgErrorOf(t, insertTransaction(ctx, conn, uuid.NewString(), uuid.NewString(), uuid.NewString()))
		assert.Equal(t, codeForeignKey, pgErr.Code)
		assert.Equal(t, "transactions_reversal_of_fkey", pgErr.ConstraintName)

		original := uuid.NewString()
		require.NoError(t, insertTransaction(ctx, conn, original, uuid.NewString(), nil))
		assert.NoError(t, insertTransaction(ctx, conn, uuid.NewString(), uuid.NewString(), original))
	})
}

func TestEntriesBehavior(t *testing.T) {
	conn := migratedDB(t)
	ctx := context.Background()

	txnID, accountID := uuid.NewString(), uuid.NewString()
	require.NoError(t, insertTransaction(ctx, conn, txnID, uuid.NewString(), nil))
	require.NoError(t, insertAccount(ctx, conn, accountID))

	t.Run("check constraints reject invalid entries", func(t *testing.T) {
		tests := []struct {
			name       string
			direction  string
			amount     int64
			constraint string
		}{
			{"zero amount", "DEBIT", 0, "entries_amount_check"},
			{"negative amount", "CREDIT", -5, "entries_amount_check"},
			{"unknown direction", "SIDEWAYS", 100, "entries_direction_check"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				pgErr := pgErrorOf(t, insertEntry(ctx, conn, txnID, accountID, tt.direction, tt.amount))
				assert.Equal(t, codeCheck, pgErr.Code)
				assert.Equal(t, tt.constraint, pgErr.ConstraintName)
			})
		}
	})

	t.Run("foreign keys are enforced", func(t *testing.T) {
		pgErr := pgErrorOf(t, insertEntry(ctx, conn, uuid.NewString(), accountID, "DEBIT", 100))
		assert.Equal(t, codeForeignKey, pgErr.Code)
		assert.Equal(t, "entries_transaction_id_fkey", pgErr.ConstraintName)

		pgErr = pgErrorOf(t, insertEntry(ctx, conn, txnID, uuid.NewString(), "DEBIT", 100))
		assert.Equal(t, codeForeignKey, pgErr.Code)
		assert.Equal(t, "entries_account_id_fkey", pgErr.ConstraintName)
	})

	t.Run("a balanced pair is accepted", func(t *testing.T) {
		pairTxn, creditAccount := uuid.NewString(), uuid.NewString()
		require.NoError(t, insertTransaction(ctx, conn, pairTxn, uuid.NewString(), nil))
		require.NoError(t, insertAccount(ctx, conn, creditAccount))

		// Inserted in one database transaction, so this keeps passing once a
		// balanced-transaction trigger checks debits against credits at commit.
		tx, err := conn.Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, insertEntry(ctx, tx, pairTxn, accountID, "DEBIT", 100))
		require.NoError(t, insertEntry(ctx, tx, pairTxn, creditAccount, "CREDIT", 100))
		require.NoError(t, tx.Commit(ctx))

		var n int
		require.NoError(t, conn.QueryRow(ctx, "SELECT count(*) FROM entries WHERE transaction_id = $1::uuid", pairTxn).Scan(&n))
		assert.Equal(t, 2, n)
	})

	t.Run("account index covers (account_id, id)", func(t *testing.T) {
		var def string
		require.NoError(t, conn.QueryRow(ctx,
			"SELECT indexdef FROM pg_indexes WHERE tablename = 'entries' AND indexname = 'entries_account_idx'").Scan(&def))
		assert.Contains(t, def, "(account_id, id)")
	})
}
