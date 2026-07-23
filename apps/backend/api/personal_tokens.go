package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"alga/rbac"
	"alga/store"
)

type createPATRequest struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
	ExpiresAt   string   `json:"expires_at"`
}

func (s *Server) handleUserTokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listUserPATs(w, r)
	case http.MethodPost:
		s.createUserPAT(w, r)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleUserTokenByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	s.revokeUserPAT(w, r)
}

func (s *Server) listUserPATs(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "unauthorized")
		return
	}
	tokens, err := s.personalAccessTokenStore.ListByUser(user.ID)
	if err != nil {
		writeInternalError(w, err, "failed to list personal access tokens")
		return
	}
	out := make([]map[string]any, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, serializePAT(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (s *Server) createUserPAT(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "unauthorized")
		return
	}

	var req createPATRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	if !validateTokenName(w, req.Name) {
		return
	}

	if len(req.Permissions) == 0 {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "at least one permission is required")
		return
	}

	validPerms := rbac.AllPermissions(user.Role)
	validSet := make(map[string]bool, len(validPerms))
	for _, p := range validPerms {
		validSet[string(p)] = true
	}
	for _, p := range req.Permissions {
		if !validSet[p] {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "permission not available for your role: "+p)
			return
		}
	}

	var expPtr *time.Time
	if req.ExpiresAt != "" {
		var ok bool
		expPtr, ok = parseAndValidateExpiry(w, req.ExpiresAt)
		if !ok {
			return
		}
	}

	record, err := s.personalAccessTokenStore.CreateToken(user.ID, req.Name, req.Permissions, expPtr)
	if err != nil {
		writeInternalError(w, err, "failed to create personal access token")
		return
	}

	s.audit(r, store.AuditPATCreated, map[string]any{
		"token_id":    record.ID.String(),
		"token_name":  record.Name,
		"permissions": record.Permissions,
	})
	writeData(w, http.StatusCreated, serializePATWithToken(*record))
}

func (s *Server) revokeUserPAT(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "unauthorized")
		return
	}
	idHex := pathID(r, "/api/v1/user/tokens/")
	id, err := uuid.Parse(idHex)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid token id")
		return
	}
	if err := s.personalAccessTokenStore.RevokeToken(id, user.ID); err != nil {
		if errors.Is(err, store.ErrTokenNotFound) {
			writeError(w, ErrorCodeNotFound, "token not found")
			return
		}
		writeInternalError(w, err, "failed to revoke personal access token")
		return
	}
	s.audit(r, store.AuditPATRevoked, map[string]any{
		"token_id": idHex,
	})
	writeStatus(w, "revoked")
}

func (s *Server) handleAdminTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	tokens, err := s.personalAccessTokenStore.ListAll()
	if err != nil {
		writeInternalError(w, err, "failed to list personal access tokens")
		return
	}
	out := make([]map[string]any, 0, len(tokens))
	for _, t := range tokens {
		entry := serializePAT(t)
		entry["user_id"] = t.UserID.String()
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (s *Server) handleAdminTokenByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	s.revokeAdminPAT(w, r)
}

func (s *Server) revokeAdminPAT(w http.ResponseWriter, r *http.Request) {
	idHex := pathID(r, "/api/v1/admin/tokens/")
	id, err := uuid.Parse(idHex)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid token id")
		return
	}
	if err := s.personalAccessTokenStore.RevokeTokenAdmin(id); err != nil {
		if errors.Is(err, store.ErrTokenNotFound) {
			writeError(w, ErrorCodeNotFound, "token not found")
			return
		}
		writeInternalError(w, err, "failed to revoke personal access token")
		return
	}
	s.audit(r, store.AuditPATRevoked, map[string]any{
		"token_id": idHex,
	})
	writeStatus(w, "revoked")
}

func serializePAT(t store.PATRecord) map[string]any {
	m := map[string]any{
		"id":          t.ID.String(),
		"name":        t.Name,
		"permissions": t.Permissions,
		"created_at":  t.CreatedAt.Format(time.RFC3339),
		"revoked":     t.Revoked,
	}
	if t.ExpiresAt != nil {
		m["expires_at"] = t.ExpiresAt.Format(time.RFC3339)
	}
	if t.LastUsedAt != nil {
		m["last_used_at"] = t.LastUsedAt.Format(time.RFC3339)
	}
	return m
}

func serializePATWithToken(t store.PATRecord) map[string]any {
	m := serializePAT(t)
	if t.Token != "" {
		m["token"] = t.Token
	}
	return m
}
