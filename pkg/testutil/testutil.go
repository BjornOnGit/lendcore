// Package testutil holds helpers for integration tests that need a real Postgres.
//
// The test server is chosen explicitly with TEST_DATABASE_URL, a postgres:// URL
// whose host, port, credentials and options are reused; only the database name
// is swapped per call. There is deliberately no default: guessing localhost:5432
// could silently hit an unrelated Postgres on the developer's machine.
package testutil

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const envDSN = "TEST_DATABASE_URL"

// buildDSN returns baseURL with its database name replaced by dbName. Errors
// never include baseURL, because it contains a password.
func buildDSN(baseURL, dbName string) (string, error) {
	if dbName == "" {
		return "", errors.New("testutil: empty database name")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", errors.New("testutil: " + envDSN + " is not a valid URL")
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", errors.New("testutil: " + envDSN + " must be a postgres:// URL")
	}
	u.Path = "/" + dbName
	u.RawPath = ""
	return u.String(), nil
}

// DSN returns a DSN for database dbName on the test server. The test is
// skipped if TEST_DATABASE_URL is unset and fails if it is malformed.
func DSN(t testing.TB, dbName string) string {
	t.Helper()
	base := os.Getenv(envDSN)
	if base == "" {
		t.Skip(envDSN + " not set; point it at the compose Postgres, " +
			"e.g. postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable")
	}
	dsn, err := buildDSN(base, dbName)
	if err != nil {
		t.Fatal(err)
	}
	return dsn
}

// NewDatabase creates an empty database with a unique name on the test server
// and returns its DSN. The database is dropped when the test (and its
// subtests) finish, so tests never share state or touch real data. A test
// process that is killed mid-run can leave its database behind; `make
// reset-db` clears those.
func NewDatabase(t testing.TB) string {
	t.Helper()
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, DSN(t, "postgres"))
	if err != nil {
		t.Fatalf("testutil: connect to test server: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	name := "lendcore_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
		t.Fatalf("testutil: create database: %v", err)
	}
	t.Cleanup(func() {
		// FORCE disconnects any connection the test forgot to close.
		if _, err := admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)"); err != nil {
			t.Errorf("testutil: drop database %s: %v", name, err)
		}
	})
	return DSN(t, name)
}
