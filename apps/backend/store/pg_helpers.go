package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"alga/config"
	"alga/db/models"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uptrace/bun"
)

func handleQueryErr[T any](err error, entity string) (T, error) {
	var zero T
	if errors.Is(err, sql.ErrNoRows) {
		return zero, nil
	}
	return zero, fmt.Errorf("failed to query %s: %w", entity, err)
}

func isNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func maskSuffix(s string) string {
	if len(s) <= 4 {
		return "••••"
	}
	return "••••" + s[len(s)-4:]
}

func generateTokenBase64(prefix string, byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}

type pgStoreBase struct {
	db *bun.DB
}

func pgctx(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, 5*time.Second)
}

func pgIsDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return true
	}
	return containsDuplicateKey(err)
}

func containsDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "23505") ||
		strings.Contains(msg, "unique constraint")
}

func IsDuplicateKey(err error) bool {
	return pgIsDuplicateKey(err)
}

// pgIsForeignKeyViolation reports SQLSTATE 23503 (FK RESTRICT/NO ACTION).
func pgIsForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return true
	}
	return false
}

// IsForeignKeyViolation reports whether err is a PostgreSQL foreign-key
// violation (e.g. deleting a row another table still references).
func IsForeignKeyViolation(err error) bool {
	return pgIsForeignKeyViolation(err)
}

func extractLimitSkip(filter map[string]any, defaultLimit int) (limit, skip int) {
	limit = defaultLimit
	if lv, ok := filter["$limit"]; ok {
		switch v := lv.(type) {
		case float64:
			limit = int(v)
		case int64:
			limit = int(v)
		case int:
			limit = v
		}
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	limit = min(limit, 500)
	if sv, ok := filter["$skip"]; ok {
		switch v := sv.(type) {
		case float64:
			skip = int(v)
		case int64:
			skip = int(v)
		case int:
			skip = v
		}
	}
	skip = max(skip, 0)
	return
}

func parseSortFromFilter(filter map[string]any, defaultField string, defaultDesc bool) (field string, desc bool) {
	raw, ok := filter["$sort"].(string)
	if !ok || raw == "" {
		return defaultField, defaultDesc
	}

	d := strings.HasPrefix(raw, "-")
	f := strings.TrimPrefix(raw, "-")
	f = strings.TrimPrefix(f, "+")
	if f == "" {
		return defaultField, defaultDesc
	}

	allowed := map[string]bool{
		"created_at": true, "updated_at": true, "alert_number": true,
		"severity": true, "status": true, "name": true, "fingerprint": true,
		"provider": true, "channel": true, "started_at": true, "ended_at": true,
		"investigation_id": true, "title": true,
	}
	if !allowed[f] {
		return defaultField, defaultDesc
	}
	return f, d
}

func routeConditionsToModels(in []config.RouteCondition) []models.RouteCondition {
	out := make([]models.RouteCondition, len(in))
	for i, c := range in {
		out[i] = models.RouteCondition{
			Source:   c.Source,
			Field:    c.Field,
			Operator: c.Operator,
			Value:    c.Value,
		}
	}
	return out
}

func routeConditionsFromModels(in []models.RouteCondition) []config.RouteCondition {
	out := make([]config.RouteCondition, len(in))
	for i, c := range in {
		out[i] = config.RouteCondition{
			Source:   c.Source,
			Field:    c.Field,
			Operator: c.Operator,
			Value:    c.Value,
		}
	}
	return out
}

// nextSeq draws the next value from a native Postgres sequence. Sequences are
// gap-tolerant (a rolled-back transaction still consumes its value) which is
// acceptable: alert/incident/triage numbers must be unique and monotonic, not
// gapless. This also supports reserve-then-create flows where a number is
// allocated before the owning row is inserted.
func nextSeq(ctx context.Context, db bun.IDB, seq string) (int64, error) {
	var n int64
	if err := db.NewRaw("SELECT nextval(?::regclass)", seq).Scan(ctx, &n); err != nil {
		return 0, fmt.Errorf("nextval %s: %w", seq, err)
	}
	return n, nil
}

// retentionDeleteBatch caps each DELETE in DeleteOlderThan-style purges so a
// large backlogged window releases locks between batches instead of holding a
// single long transaction (DT-E3).
const retentionDeleteBatch = 5000

// deleteOlderThanBatched hard-deletes rows of model older than cutoff in
// bounded batches, returning the total removed. col is the caller-supplied
// timestamp column (a compile-time constant at each call site, never user
// input). The batch loop stops on the first not-full batch.
func deleteOlderThanBatched[T any](ctx context.Context, db bun.IDB, col string, cutoff time.Time) (int64, error) {
	var total int64
	for {
		sub := db.NewSelect().Model((*T)(nil)).Column("id").Where(col+" < ?", cutoff).Limit(retentionDeleteBatch)
		res, err := db.NewDelete().Model((*T)(nil)).Where("id IN (?)", sub).Exec(ctx)
		if err != nil {
			return total, fmt.Errorf("retention delete on %s: %w", col, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("retention delete rows affected: %w", err)
		}
		total += n
		if n < retentionDeleteBatch {
			return total, nil
		}
	}
}
