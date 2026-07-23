package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"alga/config"
	"alga/ent"
	entschema "alga/ent/schema"
	"alga/pgclient"

	"github.com/jackc/pgx/v5/pgconn"
)

func handleQueryErr[T any](err error, entity string) (T, error) {
	var zero T
	if ent.IsNotFound(err) {
		return zero, nil
	}
	return zero, fmt.Errorf("failed to query %s: %w", entity, err)
}

func rollbackTx(tx *ent.Tx) {
	_ = tx.Rollback()
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
	client *ent.Client
}

// pgctx derives a store-query context from the caller's context with a bounded
// timeout. The caller's cancellation, deadline, and trace are preserved. A nil
// ctx falls back to context.Background() so legacy callers don't panic.
func pgctx(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, 5*time.Second)
}

// pgIsDuplicateKey reports whether err is a PostgreSQL unique-violation
// (SQLSTATE 23505). It checks the typed *pgconn.PgError first, then falls back
// to a string match for wrapped or non-PG errors (e.g. errors surfaced through
// Ent, middleware, or pooled connection wrappers).
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

func NewPostgresTriageResultStore(cli *pgclient.Client) TriageResultStore {
	return newPGTriageResultStore(cli.Ent)
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

func routeConditionsToSchema(in []config.RouteCondition) []entschema.RouteCondition {
	out := make([]entschema.RouteCondition, len(in))
	for i, c := range in {
		out[i] = entschema.RouteCondition{
			Source:   c.Source,
			Field:    c.Field,
			Operator: c.Operator,
			Value:    c.Value,
		}
	}
	return out
}

func routeConditionsFromSchema(in []entschema.RouteCondition) []config.RouteCondition {
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
