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

func publicFunctions(t *testing.T, conn *pgx.Conn) []string {
	t.Helper()
	rows, err := conn.Query(context.Background(), `
		SELECT p.proname FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'public' ORDER BY 1`)
	require.NoError(t, err)
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	return names
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
	assert.Equal(t, []string{"check_entries_balanced"}, publicFunctions(t, conn))

	migrateDown(t, dsn)
	assert.Equal(t, []string{"schema_migrations"}, publicTables(t, conn), "down must remove every table it created")
	assert.Empty(t, publicFunctions(t, conn), "down must remove every function it created")

	require.NoError(t, db.Migrate(ctx, dsn, migrations.FS, "."), "up again")
	assert.ElementsMatch(t, wantTables, publicTables(t, conn))
	assert.Equal(t, []string{"check_entries_balanced"}, publicFunctions(t, conn))
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

	t.Run("indexes exist", func(t *testing.T) {
		wantIndexes := map[string]string{
			"entries_account_idx":     "(account_id, id)",
			"entries_transaction_idx": "(transaction_id)",
		}
		for index, want := range wantIndexes {
			var def string
			require.NoError(t, conn.QueryRow(ctx,
				"SELECT indexdef FROM pg_indexes WHERE tablename = 'entries' AND indexname = $1", index).Scan(&def), index)
			assert.Contains(t, def, want, index)
		}
	})
}

func seedTransaction(t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, insertTransaction(context.Background(), conn, id, uuid.NewString(), nil))
	return id
}

func seedAccount(t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, insertAccount(context.Background(), conn, id))
	return id
}

func entryCount(t *testing.T, conn *pgx.Conn, txnID string) int {
	t.Helper()
	var n int
	require.NoError(t, conn.QueryRow(context.Background(),
		"SELECT count(*) FROM entries WHERE transaction_id = $1::uuid", txnID).Scan(&n))
	return n
}

// seedBalancedPair commits one debit and one credit of amount for a new
// transaction and returns the transaction id and the two entry ids.
func seedBalancedPair(t *testing.T, conn *pgx.Conn, amount int64) (txnID string, debitID, creditID int64) {
	t.Helper()
	ctx := context.Background()
	txnID = seedTransaction(t, conn)
	debitAccount, creditAccount := seedAccount(t, conn), seedAccount(t, conn)

	tx, err := conn.Begin(ctx)
	require.NoError(t, err)
	insert := func(account, direction string) (id int64) {
		require.NoError(t, tx.QueryRow(ctx,
			"INSERT INTO entries (transaction_id, account_id, direction, amount) VALUES ($1::uuid, $2::uuid, $3, $4) RETURNING id",
			txnID, account, direction, amount).Scan(&id))
		return id
	}
	debitID, creditID = insert(debitAccount, "DEBIT"), insert(creditAccount, "CREDIT")
	require.NoError(t, tx.Commit(ctx))
	return txnID, debitID, creditID
}

type entrySpec struct {
	direction string
	amount    int64
}

func TestEntriesBalanceTrigger(t *testing.T) {
	conn := migratedDB(t)
	ctx := context.Background()

	// post inserts the entries for a new transaction inside one database
	// transaction and returns the transaction id and the error from COMMIT.
	// The inserts themselves must succeed: balance is only checked at commit.
	post := func(t *testing.T, specs ...entrySpec) (string, error) {
		t.Helper()
		accounts := make([]string, len(specs))
		for i := range specs {
			accounts[i] = seedAccount(t, conn)
		}
		txnID := seedTransaction(t, conn)

		tx, err := conn.Begin(ctx)
		require.NoError(t, err)
		for i, s := range specs {
			require.NoError(t, insertEntry(ctx, tx, txnID, accounts[i], s.direction, s.amount),
				"the insert must succeed; balance is only checked at commit")
		}
		return txnID, tx.Commit(ctx)
	}

	assertUnbalanced := func(t *testing.T, err error) {
		t.Helper()
		pgErr := pgErrorOf(t, err)
		assert.Equal(t, codeCheck, pgErr.Code)
		assert.Equal(t, "entries_balanced", pgErr.ConstraintName)
		assert.Contains(t, pgErr.Message, "unbalanced")
	}

	t.Run("balanced entries commit", func(t *testing.T) {
		txnID, err := post(t, entrySpec{"DEBIT", 100}, entrySpec{"CREDIT", 100})

		require.NoError(t, err)
		assert.Equal(t, 2, entryCount(t, conn, txnID))
	})

	t.Run("one debit split across two credits commits", func(t *testing.T) {
		txnID, err := post(t, entrySpec{"DEBIT", 100}, entrySpec{"CREDIT", 60}, entrySpec{"CREDIT", 40})

		require.NoError(t, err)
		assert.Equal(t, 3, entryCount(t, conn, txnID))
	})

	t.Run("unbalanced entries fail at commit and leave nothing behind", func(t *testing.T) {
		txnID, err := post(t, entrySpec{"DEBIT", 100}, entrySpec{"CREDIT", 90})

		assertUnbalanced(t, err)
		assert.Contains(t, pgErrorOf(t, err).Message, "debits 100 <> credits 90")
		assert.Zero(t, entryCount(t, conn, txnID), "the failed commit must roll the entries back")
	})

	t.Run("credits exceeding debits fail at commit", func(t *testing.T) {
		txnID, err := post(t, entrySpec{"DEBIT", 100}, entrySpec{"CREDIT", 110})

		assertUnbalanced(t, err)
		assert.Contains(t, pgErrorOf(t, err).Message, "debits 100 <> credits 110")
		assert.Zero(t, entryCount(t, conn, txnID))
	})

	t.Run("a lone entry fails at commit", func(t *testing.T) {
		txnID, err := post(t, entrySpec{"DEBIT", 100})

		assertUnbalanced(t, err)
		assert.Zero(t, entryCount(t, conn, txnID))
	})

	t.Run("changing an amount so it no longer balances fails", func(t *testing.T) {
		txnID, debitID, _ := seedBalancedPair(t, conn, 100)

		_, err := conn.Exec(ctx, "UPDATE entries SET amount = amount + 1 WHERE id = $1", debitID)

		assertUnbalanced(t, err)
		var amount int64
		require.NoError(t, conn.QueryRow(ctx, "SELECT amount FROM entries WHERE id = $1", debitID).Scan(&amount))
		assert.EqualValues(t, 100, amount, "the failed update must not stick")
		assert.Equal(t, 2, entryCount(t, conn, txnID))
	})

	t.Run("changing both sides together stays balanced", func(t *testing.T) {
		txnID, debitID, creditID := seedBalancedPair(t, conn, 100)

		tx, err := conn.Begin(ctx)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, "UPDATE entries SET amount = 150 WHERE id IN ($1, $2)", debitID, creditID)
		require.NoError(t, err)

		require.NoError(t, tx.Commit(ctx))
		assert.Equal(t, 2, entryCount(t, conn, txnID))
	})

	t.Run("an entry moved out of a transaction unbalances it even if the destination stays balanced", func(t *testing.T) {
		txnA, debitA, _ := seedBalancedPair(t, conn, 100)
		txnB, _, _ := seedBalancedPair(t, conn, 50)
		offsetAccount := seedAccount(t, conn)

		tx, err := conn.Begin(ctx)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, "UPDATE entries SET transaction_id = $1::uuid WHERE id = $2", txnB, debitA)
		require.NoError(t, err)
		// B now has debits 150 against credits 50; an extra credit of 100 rebalances it,
		// so only A, the transaction the entry left, is wrong at commit.
		require.NoError(t, insertEntry(ctx, tx, txnB, offsetAccount, "CREDIT", 100))

		err = tx.Commit(ctx)

		assertUnbalanced(t, err)
		assert.Contains(t, pgErrorOf(t, err).Message, txnA, "the error must name the transaction that became unbalanced")
		assert.Equal(t, 2, entryCount(t, conn, txnA))
		assert.Equal(t, 2, entryCount(t, conn, txnB))
	})

	t.Run("an entry moved into a transaction unbalances it even if the source stays balanced", func(t *testing.T) {
		txnA, debitA, _ := seedBalancedPair(t, conn, 100)
		txnB, _, _ := seedBalancedPair(t, conn, 50)
		offsetAccount := seedAccount(t, conn)

		tx, err := conn.Begin(ctx)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, "UPDATE entries SET transaction_id = $1::uuid WHERE id = $2", txnB, debitA)
		require.NoError(t, err)
		// A is left with only its credit of 100; an extra debit of 100 rebalances it,
		// so only B, the transaction the entry joined, is wrong at commit.
		require.NoError(t, insertEntry(ctx, tx, txnA, offsetAccount, "DEBIT", 100))

		err = tx.Commit(ctx)

		assertUnbalanced(t, err)
		assert.Contains(t, pgErrorOf(t, err).Message, txnB, "the error must name the transaction that became unbalanced")
		assert.Equal(t, 2, entryCount(t, conn, txnA))
		assert.Equal(t, 2, entryCount(t, conn, txnB))
	})

	t.Run("deleting one entry fails", func(t *testing.T) {
		txnID, _, creditID := seedBalancedPair(t, conn, 100)

		_, err := conn.Exec(ctx, "DELETE FROM entries WHERE id = $1", creditID)

		assertUnbalanced(t, err)
		assert.Equal(t, 2, entryCount(t, conn, txnID), "the failed delete must not stick")
	})

	t.Run("moving an entry to another transaction fails", func(t *testing.T) {
		txnA, debitA, _ := seedBalancedPair(t, conn, 100)
		txnB, _, _ := seedBalancedPair(t, conn, 50)

		_, err := conn.Exec(ctx, "UPDATE entries SET transaction_id = $1::uuid WHERE id = $2", txnB, debitA)

		assertUnbalanced(t, err)
		assert.Equal(t, 2, entryCount(t, conn, txnA))
		assert.Equal(t, 2, entryCount(t, conn, txnB))
	})
}
