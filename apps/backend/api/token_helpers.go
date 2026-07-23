package api

import (
	"time"

	"alga/store"
)

// serializeWebhookTokenOpts renders a webhook token row. includeToken must be
// true only on create (the plaintext token is shown once); list responses must
// pass includeToken=false so persisted hashes never leak.
func serializeWebhookTokenOpts(t store.WebhookTokenRecord, showExpired, includeToken bool) map[string]any {
	row := map[string]any{
		"id":         t.ID.String(),
		"name":       t.Name,
		"created_at": t.CreatedAt,
		"last_used":  t.LastUsedAt,
		"revoked":    t.Revoked,
	}
	if includeToken {
		row["token"] = t.Token
	}
	if t.ExpiresAt != nil && !t.ExpiresAt.IsZero() {
		row["expires_at"] = t.ExpiresAt.UTC().Format(time.RFC3339)
		if showExpired {
			row["expired"] = time.Now().After(*t.ExpiresAt)
		}
	}
	return row
}

func serializeWebhookToken(t store.WebhookTokenRecord, showExpired bool) map[string]any {
	return serializeWebhookTokenOpts(t, showExpired, true)
}

// serializeWebhookTokenSummary is the list-response shape: it omits Token so
// GET /webhook-tokens never exposes persisted token material.
func serializeWebhookTokenSummary(t store.WebhookTokenRecord, showExpired bool) map[string]any {
	return serializeWebhookTokenOpts(t, showExpired, false)
}
