// Package db provides the Postgres connection pool helper shared by services.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxConns        = 10
	maxConnLifetime = 30 * time.Minute
	maxConnIdleTime = 5 * time.Minute
	pingTimeout     = 10 * time.Second
)

// newConfig parses dsn and applies the pool limits. It does not connect.
// The limits override any pool_* parameters in the DSN.
func newConfig(dsn string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("db: parse dsn: %w", err)
	}
	cfg.MaxConns = maxConns
	cfg.MaxConnLifetime = maxConnLifetime
	cfg.MaxConnIdleTime = maxConnIdleTime
	return cfg, nil
}

// Connect opens a pgx pool for dsn with sensible limits (10 connections max,
// 30m max lifetime, 5m max idle) and verifies it with a ping, which is
// bounded to 10s so an unreachable database cannot hang startup. On any
// failure no pool is left open. Callers own the returned pool and must Close it.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := newConfig(dsn)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: create pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return pool, nil
}
