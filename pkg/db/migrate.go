package db

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Migrate applies all pending migrations found in dir of fsys (typically an
// embed.FS) to the database at dsn. Files follow golang-migrate naming:
// 000001_name.up.sql / 000001_name.down.sql. It is idempotent: when the
// database is already up to date it returns nil. golang-migrate holds a
// Postgres advisory lock while running, so replicas migrating at once are
// serialized.
//
// ctx bounds connecting to the database; golang-migrate cannot interrupt a
// migration that is already running.
func Migrate(ctx context.Context, dsn string, fsys fs.FS, dir string) error {
	src, err := iofs.New(fsys, dir)
	if err != nil {
		return fmt.Errorf("db: migrations source: %w", err)
	}
	connCfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("db: parse dsn: %w", err)
	}
	sqlDB := stdlib.OpenDB(*connCfg)

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return fmt.Errorf("db: ping: %w", err)
	}
	driver, err := pgxmigrate.WithInstance(sqlDB, &pgxmigrate.Config{})
	if err != nil {
		_ = sqlDB.Close()
		return fmt.Errorf("db: migration driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	if err != nil {
		_ = driver.Close()
		return fmt.Errorf("db: migrate init: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("db: migrate up: %w", err)
	}
	return nil
}
