package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"alga/logger"
	"alga/trace"
)

type Client struct {
	DB  *bun.DB
	DSN string
}

// PoolConfig tunes the database/sql connection pool. Non-positive fields are
// replaced with DefaultPoolConfig() values at connection time, so a partially
// populated config is always safe.
type PoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxIdleTime time.Duration
	ConnMaxLifetime time.Duration
}

// DefaultPoolConfig returns production-safe connection pool defaults.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxIdleTime: 5 * time.Minute,
		ConnMaxLifetime: 30 * time.Minute,
	}
}

func (p PoolConfig) withDefaults() PoolConfig {
	d := DefaultPoolConfig()
	if p.MaxOpenConns <= 0 {
		p.MaxOpenConns = d.MaxOpenConns
	}
	if p.MaxIdleConns <= 0 {
		p.MaxIdleConns = d.MaxIdleConns
	}
	if p.ConnMaxIdleTime <= 0 {
		p.ConnMaxIdleTime = d.ConnMaxIdleTime
	}
	if p.ConnMaxLifetime <= 0 {
		p.ConnMaxLifetime = d.ConnMaxLifetime
	}
	// database/sql requires idle <= open; clamp to keep the pool coherent.
	if p.MaxIdleConns > p.MaxOpenConns {
		p.MaxIdleConns = p.MaxOpenConns
	}
	return p
}

// New connects to PostgreSQL using DefaultPoolConfig(). Suitable for one-shot
// CLI commands and tests; long-lived services should use NewWithPool.
func New(dsn string) (*Client, error) {
	return NewWithPool(dsn, DefaultPoolConfig())
}

// NewWithPool connects to PostgreSQL with an explicit connection pool
// configuration. Zero-valued pool fields fall back to DefaultPoolConfig().
func NewWithPool(dsn string, pool PoolConfig) (*Client, error) {
	pool = pool.withDefaults()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	connCfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	if t := trace.PGXTracer(); t != nil {
		connCfg.Tracer = t
	}

	sqldb := stdlib.OpenDB(*connCfg)
	sqldb.SetMaxOpenConns(pool.MaxOpenConns)
	sqldb.SetMaxIdleConns(pool.MaxIdleConns)
	sqldb.SetConnMaxIdleTime(pool.ConnMaxIdleTime)
	sqldb.SetConnMaxLifetime(pool.ConnMaxLifetime)

	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	logger.Info("connected to PostgreSQL", "component", "db",
		"max_open_conns", pool.MaxOpenConns,
		"max_idle_conns", pool.MaxIdleConns,
		"conn_max_idle_time", pool.ConnMaxIdleTime.String(),
		"conn_max_lifetime", pool.ConnMaxLifetime.String())

	bunDB := bun.NewDB(sqldb, pgdialect.New())

	return &Client{DB: bunDB, DSN: dsn}, nil
}

func (c *Client) Close() {
	if c.DB != nil {
		_ = c.DB.Close()
	}
}

func OpenSQLDB(dsn string) (*sql.DB, error) {
	connCfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	return stdlib.OpenDB(*connCfg), nil
}
