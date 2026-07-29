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

func New(dsn string) (*Client, error) {
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
	sqldb.SetMaxOpenConns(4)
	sqldb.SetMaxIdleConns(1)
	sqldb.SetConnMaxIdleTime(30 * time.Second)

	if err := sqldb.PingContext(ctx); err != nil {
		sqldb.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	logger.Info("connected to PostgreSQL", "component", "db")

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
