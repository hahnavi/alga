package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	alga "github.com/alga/agent-sdk-go"
)

// AlgaClient is the subset of the SDK client surface used by the tools. It is
// satisfied directly by *alga.AlgaClient (no adapter needed); tests may
// substitute a fake.
type AlgaClient interface {
	ListAlerts(ctx context.Context, params map[string]string) (*alga.AlertListResponse, error)
	GetAlert(ctx context.Context, fingerprint string) (*alga.Alert, error)
	ListInvestigations(ctx context.Context, params map[string]string) (*alga.InvestigationListResponse, error)
	GetInvestigation(ctx context.Context, id string) (*alga.Investigation, error)
	PostUpdate(ctx context.Context, id, updateType, message string) (*alga.Investigation, error)
	SendMessage(ctx context.Context, chatID, text string, mentions []string) (*alga.SendMessageResponse, error)
	SendCommand(ctx context.Context, chatID string, cmd alga.InvestigationCommand) (*alga.CommandResponse, error)
	ListKnowledge(ctx context.Context, params map[string]string) (*alga.KnowledgeListResponse, error)
	CreateKnowledge(ctx context.Context, params map[string]any) (*alga.KnowledgeNote, error)
	ListMemories(ctx context.Context, params map[string]string) (*alga.MemoryListResponse, error)
	CreateMemory(ctx context.Context, params map[string]any) (*alga.Memory, error)
	DeleteMemory(ctx context.Context, id string) error
	GetIncident(ctx context.Context, id string) (*alga.Incident, error)
	AddIncidentTimeline(ctx context.Context, id, message, eventType string) error
	ListServices(ctx context.Context) ([]alga.Service, error)
	WhoIsOnCall(ctx context.Context) (map[string]any, error)
	ListIncidentTasks(ctx context.Context, incidentNumber int64) ([]alga.CoordinationTask, error)
}

// *alga.AlgaClient satisfies AlgaClient at compile time. If the SDK ever
// drifts from this interface, the build breaks here rather than at every
// call site.
var _ AlgaClient = (*alga.AlgaClient)(nil)

// RegisterAlgaTools registers all Alga platform tools against the registry.
// client may be nil; in that case all Alga tools are skipped (the agent runs
// with shell + web search only).
func RegisterAlgaTools(reg *Registry, client AlgaClient) {
	if client == nil {
		return
	}
	for _, t := range buildAlgaTools(client) {
		reg.Register(t)
	}
}

// --- ID resolution helpers ---

// chatIDFromCtx resolves the chat id for SendCommand tools. When inside an
// Alga thread, use the investigation chat id; otherwise require the user to
// provide one via the "chat_id" argument.
func chatIDFromCtx(ctx context.Context, args map[string]string) (string, error) {
	if v := args["chat_id"]; v != "" {
		return v, nil
	}
	if cc, ok := CallContextFrom(ctx); ok && cc.ChatID != "" {
		return cc.ChatID, nil
	}
	return "", errors.New("chat_id is required (not running inside an Alga thread)")
}

// invIDFromCtx resolves an investigation id from Alga context, falling back to
// the "investigation_id" argument. This implements the ID resolution policy
// from SPEC §6.1.
func invIDFromCtx(ctx context.Context, args map[string]string) (string, error) {
	if v := args["investigation_id"]; v != "" {
		return v, nil
	}
	if cc, ok := CallContextFrom(ctx); ok && cc.AlgaInvestigationID != "" {
		return cc.AlgaInvestigationID, nil
	}
	return "", errors.New("investigation_id is required (provide it explicitly, e.g. inv_<id>)")
}

// incidentIDFromCtx resolves an incident id from Alga context, falling back to
// the explicit argument.
func incidentIDFromCtx(ctx context.Context, args map[string]string) (string, error) {
	if v := args["incident_id"]; v != "" {
		return v, nil
	}
	if cc, ok := CallContextFrom(ctx); ok && cc.AlgaIncidentID != "" {
		return cc.AlgaIncidentID, nil
	}
	return "", errors.New("incident_id is required")
}

// algaErr converts an SDK error into a descriptive Go error, surfacing auth
// failures distinctly from API errors. Used by legacy tool helpers; typed
// tools use the typed Err[O](err) helper instead.
func algaErr(err error) error {
	if err == nil {
		return nil
	}
	var authErr *alga.AlgaAuthError
	if errors.As(err, &authErr) {
		return fmt.Errorf("alga auth error (%d): %s", authErr.StatusCode, strings.TrimSpace(authErr.Message))
	}
	var apiErr *alga.AlgaAPIError
	if errors.As(err, &apiErr) {
		return fmt.Errorf("alga api error (%d): %s", apiErr.StatusCode, strings.TrimSpace(apiErr.Message))
	}
	return fmt.Errorf("alga connection error: %w", err)
}

// algaCategory is the system-prompt group heading for Alga tools.
const algaCategory = "Alga Platform"

// buildAlgaTools returns every Alga tool wired to client. Implemented as a
// single closure so all tools share the same client reference without a
// package-level var.
func buildAlgaTools(client AlgaClient) []Tool {
	return append(append(append(append(append(
		alertTools(client),
		investigationTools(client)...),
		incidentTools(client)...),
		knowledgeMemoryTools(client)...),
		coordinationTools(client)...),
		miscTools(client)...)
}
