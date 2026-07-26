package pgclient

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/jackc/pgx/v5"
	pgxstdlib "github.com/jackc/pgx/v5/stdlib"

	"alga/ent"
	"alga/logger"
	"alga/trace"
)

type Client struct {
	Ent    *ent.Client
	Driver *entsql.Driver
	DB     *sql.DB
	DSN    string
}

func New(dsn string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	if t := trace.PGXTracer(); t != nil {
		cfg.Tracer = t
	}

	db := sql.OpenDB(pgxstdlib.GetConnector(*cfg))
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(30 * time.Second)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	logger.Info("connected to PostgreSQL", "component", "pgclient")

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := ent.NewClient(ent.Driver(drv))

	return &Client{
		Ent:    client,
		Driver: drv,
		DB:     db,
		DSN:    dsn,
	}, nil
}

func (c *Client) Close() {
	_ = c.Ent.Close()
	if c.DB != nil {
		_ = c.DB.Close()
	}
}

func ApplyMigrations(ctx context.Context, dsn string) error {
	logger.Info("applying database migrations", "component", "pgclient")

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer func() { _ = db.Close() }()

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := ent.NewClient(ent.Driver(drv))
	defer func() { _ = client.Close() }()

	if err := client.Schema.Create(ctx); err != nil {
		return fmt.Errorf("apply ent schema: %w", err)
	}
	if err := ApplyPgVector(ctx, db); err != nil {
		return fmt.Errorf("apply pgvector: %w", err)
	}
	if err := ApplyKnowledgeFTS(ctx, db); err != nil {
		return fmt.Errorf("apply knowledge fts: %w", err)
	}
	logger.Info("database migrations applied successfully", "component", "pgclient")
	return nil
}

func ApplyPgVector(ctx context.Context, db *sql.DB) error {
	logger.Debug("setting up pgvector extension", "component", "pgclient")

	_, err := db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS vector`)
	if err != nil {
		return fmt.Errorf("create vector extension: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		ALTER TABLE agent_memories ADD COLUMN IF NOT EXISTS vec vector(1536)
	`)
	if err != nil {
		return fmt.Errorf("add vec column: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_agent_memories_vec
		ON agent_memories USING hnsw (vec vector_cosine_ops)
	`)
	if err != nil {
		return fmt.Errorf("create hnsw index: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_agent_memories_fts
		ON agent_memories USING gin(to_tsvector('english', content))
	`)
	if err != nil {
		return fmt.Errorf("create fts index: %w", err)
	}
	return nil
}

// KnowledgeFTSExpression is the immutable, normalized full-text expression
// indexed on knowledge_notes (title + body + tags). It MUST stay byte-identical
// between the GIN expression index created in ApplyKnowledgeFTS and the WHERE
// clause emitted by the knowledge store text search; otherwise PostgreSQL cannot
// reuse the index and the search degrades to a sequential scan.
//
// The two-argument to_tsvector with a literal regconfig ('english') is
// IMMUTABLE, which is what makes it eligible for an expression index.
const KnowledgeFTSExpression = `to_tsvector('english', coalesce(title, '') || ' ' || coalesce(body_markdown, '') || ' ' || coalesce(tags::text, ''))`

// ApplyKnowledgeFTS creates the GIN full-text index over knowledge_notes. It
// mirrors the idx_agent_memories_fts pattern: an expression index maintained
// automatically by PostgreSQL on insert/update, entirely outside of Ent's schema
// awareness. Idempotent via IF NOT EXISTS, so existing deployments converge on
// the next migration run without a separate step.
func ApplyKnowledgeFTS(ctx context.Context, db *sql.DB) error {
	logger.Debug("setting up knowledge_notes full-text index", "component", "pgclient")

	_, err := db.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_notes_fts ON knowledge_notes USING gin(`+KnowledgeFTSExpression+`)`)
	if err != nil {
		return fmt.Errorf("create knowledge fts index: %w", err)
	}

	_, err = db.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_notes_tags ON knowledge_notes USING gin(tags jsonb_path_ops)`)
	if err != nil {
		return fmt.Errorf("create knowledge tags gin index: %w", err)
	}
	return nil
}
