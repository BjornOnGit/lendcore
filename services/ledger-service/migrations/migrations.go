// Package migrations embeds the ledger database migrations (golang-migrate
// naming: 000001_name.up.sql / 000001_name.down.sql).
package migrations

import "embed"

// FS holds the SQL migration files at its root; pass "." as the directory to db.Migrate.
//
//go:embed *.sql
var FS embed.FS
