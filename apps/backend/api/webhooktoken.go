// Code moved from http.go; see git history.

package api

import (
	"errors"
	"net/http"

	"alga/sse"
	"alga/store"

	"github.com/google/uuid"
)

func (s *Server) handleWebhookTokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tokens, err := s.webhookTokenStore.ListTokens()
		if err != nil {
			writeInternalError(w, err, "failed to list tokens")
			return
		}
		out := make([]map[string]any, 0, len(tokens))
		for _, t := range tokens {
			out = append(out, serializeWebhookTokenSummary(t, true))
		}
		writeData(w, http.StatusOK, out)
	case http.MethodPost:
		var req struct {
			Name      string `json:"name"`
			ExpiresAt string `json:"expires_at"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		if !validateTokenName(w, req.Name) {
			return
		}
		expPtr, ok := parseAndValidateExpiry(w, req.ExpiresAt)
		if !ok {
			return
		}
		record, err := s.webhookTokenStore.CreateToken(req.Name, expPtr)
		if err != nil {
			writeInternalError(w, err, "failed to create token")
			return
		}

		s.audit(r, store.AuditTokenCreated, map[string]any{
			"token_id":   record.ID.String(),
			"token_name": record.Name,
		})
		writeData(w, http.StatusCreated, serializeWebhookToken(*record, false))
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleWebhookTokenByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	idHex := pathID(r, "/api/v1/webhook-tokens/")
	s.revokeTokenByID(w, r, idHex, s.webhookTokenStore.RevokeToken, "webhook")
}

func (s *Server) revokeTokenByID(w http.ResponseWriter, r *http.Request, idHex string, revokeFn func(uuid.UUID) error, kind string) {
	id, err := uuid.Parse(idHex)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid token id")
		return
	}

	if err := revokeFn(id); err != nil {
		if errors.Is(err, store.ErrTokenNotFound) {
			writeError(w, ErrorCodeNotFound, "token not found")
			return
		}
		writeInternalError(w, err, "failed to revoke "+kind+" token")
		return
	}
	s.audit(r, store.AuditTokenRevoked, map[string]any{
		"token_id":   idHex,
		"token_kind": kind,
	})
	if kind == "agent" && s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{
			Type: "agent_revoked",
			Data: map[string]any{
				"agent_token_id": idHex,
				"token_kind":     kind,
			},
		})
	}
	writeStatus(w, "revoked")
}
