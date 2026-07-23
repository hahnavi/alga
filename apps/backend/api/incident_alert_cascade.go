package api

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"alga/logger"
	"alga/sse"
	"alga/store"
)

// CascadeActor identifies who triggered the incident transition that owns the
// cascade. Type is "user" (HTTP path) or "agent" (agent tool path). IP and
// UserAgent are captured on the HTTP path so cascade audit entries carry the
// caller's provenance; they stay empty on the agent path, matching the
// existing convention for agent audit entries.
type CascadeActor struct {
	ID        uuid.UUID
	Type      string
	Name      string
	IP        string
	UserAgent string
}

// CascadeSummary is the operator-facing roll-up attached to incident
// resolve/close responses.
type CascadeSummary struct {
	Resolved int `json:"resolved"`
	Skipped  int `json:"skipped"`
	Failed   int `json:"failed"`
}

// incidentResolveResponse wraps the updated incident with the cascade roll-up.
// Used as the resolve/close HTTP response body.
type incidentResolveResponse struct {
	Incident *store.IncidentRecord `json:"incident"`
	Cascade  CascadeSummary        `json:"cascade"`
}

// cascadeAlertStore is the narrow subset of store.Store the runner needs.
// Both pgAlertStore (store.Store) and test fakes satisfy it.
type cascadeAlertStore interface {
	ResolveAlertsByIncident(ctx context.Context, incidentNumber int64, actor *store.EventActor) (store.AlertCascadeResult, error)
}

// cascadeSSEPublisher is the narrow subset of *sse.DualPublisher the runner
// needs. Both Server.ssePublisher and AgentToolExecutor.ssePublisher satisfy it.
type cascadeSSEPublisher interface {
	Publish(event sse.Event)
}

// runAlertCascade resolves linked firing alerts for an incident and emits the
// per-alert audit + SSE side effects. Best-effort: the incident transition has
// already succeeded before this is called. Returns the structured result so the
// caller can surface resolved/skipped/failed counts.
func runAlertCascade(
	ctx context.Context,
	alertStore cascadeAlertStore,
	auditStore store.AuditStore,
	ssePublisher cascadeSSEPublisher,
	incidentNumber int64,
	actor CascadeActor,
) store.AlertCascadeResult {
	if alertStore == nil {
		return store.AlertCascadeResult{}
	}

	result, err := alertStore.ResolveAlertsByIncident(ctx, incidentNumber, &store.EventActor{
		UserID:      actor.ID.String(),
		Username:    actor.Name,
		DisplayName: actor.Name,
		Source:      "incident_cascade",
	})
	if err != nil {
		logger.WarnCtx(ctx, "incident alert cascade store call failed",
			"incident_number", incidentNumber, "error", err)
		return result
	}

	for _, rec := range result.Resolved {
		if auditStore != nil {
			auditStore.Log(store.AuditAlertAutoResolved, &actor.ID, actor.Name, actor.IP, actor.UserAgent, true, map[string]any{
				"incident_number": incidentNumber,
				"cascade":         true,
				"alert_number":    rec.AlertNumber,
				"fingerprint":     rec.Fingerprint,
			})
		}
		if ssePublisher != nil {
			ssePublisher.Publish(sse.Event{
				Type: "alert_updated",
				Data: rec,
			})
		}
	}

	if ssePublisher != nil && len(result.Failed) > 0 {
		ssePublisher.Publish(sse.Event{
			Type: "incident_alert_cascade_partial",
			Data: map[string]any{
				"incident_number": incidentNumber,
				"resolved":        len(result.Resolved),
				"failed":          len(result.Failed),
			},
		})
	}

	for _, ref := range result.Failed {
		logger.WarnCtx(ctx, "incident alert cascade failed to resolve alert",
			"incident_number", incidentNumber,
			"alert_number", ref.AlertNumber, "fingerprint", ref.Fingerprint)
	}

	return result
}

// cascadeSummary converts the store result into the response roll-up.
func cascadeSummary(r store.AlertCascadeResult) CascadeSummary {
	return CascadeSummary{
		Resolved: len(r.Resolved),
		Skipped:  len(r.Skipped),
		Failed:   len(r.Failed),
	}
}

// cascadeActorFromRequest builds the CascadeActor for an HTTP-driven incident
// transition from the authenticated user in the request context.
func cascadeActorFromRequest(r *http.Request) CascadeActor {
	actor := CascadeActor{
		Type:      "user",
		IP:        r.RemoteAddr,
		UserAgent: r.UserAgent(),
	}
	if user := userFromContext(r.Context()); user != nil {
		actor.ID = user.ID
		actor.Name = user.Email
	}
	return actor
}

// cascadePublisherFromDual adapts the server's concrete *sse.DualPublisher to
// the runner's narrow interface, returning a nil interface when the publisher
// is unset so the runner's nil guard works (a typed nil pointer would otherwise
// satisfy the non-nil interface check).
func cascadePublisherFromDual(dp *sse.DualPublisher) cascadeSSEPublisher {
	if dp == nil {
		return nil
	}
	return dp
}
