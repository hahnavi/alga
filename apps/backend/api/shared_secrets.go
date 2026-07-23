package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/logger"
	"alga/rbac"
	"alga/secretprovider"
	"alga/store"
)

// --- Credential providers (admin) ---

type credentialProviderRequest struct {
	Name    *string            `json:"name,omitempty"`
	Type    *string            `json:"type,omitempty"`
	Enabled *bool              `json:"enabled,omitempty"`
	Config  *map[string]string `json:"config,omitempty"`
}

// handleCredentialProviders handles /api/v1/credential-providers (list + create).
func (s *Server) handleCredentialProviders(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.credentialProviderStore, "credential provider store") {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !s.checkPermission(w, r, rbac.CredentialSecretsRead) {
			return
		}
		s.listCredentialProviders(w, r)
	case http.MethodPost:
		if !s.checkPermission(w, r, rbac.CredentialSecretsManage) {
			return
		}
		s.createCredentialProvider(w, r)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

// handleCredentialProviderByID handles /api/v1/credential-providers/{id}.
func (s *Server) handleCredentialProviderByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.credentialProviderStore, "credential provider store") {
		return
	}
	id, ok := parseUUIDPath(w, r, "/api/v1/credential-providers/")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !s.checkPermission(w, r, rbac.CredentialSecretsRead) {
			return
		}
		rec, err := s.credentialProviderStore.GetProvider(r.Context(), id)
		if err != nil {
			writeInternalError(w, err, "failed to get credential provider")
			return
		}
		writeData(w, http.StatusOK, rec)
	case http.MethodPatch:
		if !s.checkPermission(w, r, rbac.CredentialSecretsManage) {
			return
		}
		s.updateCredentialProvider(w, r, id)
	case http.MethodDelete:
		if !s.checkPermission(w, r, rbac.CredentialSecretsManage) {
			return
		}
		if err := s.credentialProviderStore.DeleteProvider(r.Context(), id); err != nil {
			switch {
			case errors.Is(err, store.ErrSystemCredentialProvider):
				writeError(w, ErrorCodeConflict, "this provider is a system default and cannot be removed")
				return
			case errors.Is(err, store.ErrCredentialProviderNotFound):
				writeError(w, ErrorCodeNotFound, "credential provider not found")
				return
			}
			writeInternalError(w, err, "failed to delete credential provider")
			return
		}
		s.audit(r, store.AuditCredentialProviderDeleted, map[string]any{"provider_id": id.String()})
		writeStatus(w, "deleted")
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) listCredentialProviders(w http.ResponseWriter, r *http.Request) {
	q := store.CredentialProviderQuery{
		Type:   r.URL.Query().Get("type"),
		Search: r.URL.Query().Get("q"),
	}
	if raw := r.URL.Query().Get("enabled"); raw != "" {
		enabled := raw == "true" || raw == "1"
		q.Enabled = &enabled
	}
	limit, skip := parseLimitSkip(r, 50)
	q.Limit = int(limit)
	q.Skip = int(skip)
	items, total, err := s.credentialProviderStore.ListProviders(r.Context(), q)
	if err != nil {
		writeInternalError(w, err, "failed to list credential providers")
		return
	}
	writePaginatedJSON(w, ensureSlice(items), total)
}

func (s *Server) createCredentialProvider(w http.ResponseWriter, r *http.Request) {
	var req credentialProviderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(derefString(req.Name))
	if name == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "name is required")
		return
	}
	pt := store.CredentialProviderTypeInternal
	if req.Type != nil {
		pt = strings.TrimSpace(*req.Type)
	}
	if !store.IsValidCredentialProviderType(pt) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid provider type")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	config := map[string]string{}
	if req.Config != nil {
		config = *req.Config
	}

	rec, err := s.credentialProviderStore.CreateProvider(r.Context(), &store.CredentialProviderRecord{
		Name:    name,
		Type:    pt,
		Enabled: enabled,
	}, config)
	if err != nil {
		writeInternalError(w, err, "failed to create credential provider")
		return
	}
	s.audit(r, store.AuditCredentialProviderCreated, map[string]any{
		"provider_id": rec.ID.String(),
		"name":        rec.Name,
		"type":        rec.Type,
		"has_config":  rec.ConfigConfigured,
	})
	writeData(w, http.StatusCreated, rec)
}

func (s *Server) updateCredentialProvider(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var req credentialProviderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	patch := &store.CredentialProviderRecord{}
	if req.Name != nil {
		patch.Name = strings.TrimSpace(*req.Name)
	}
	if req.Type != nil {
		pt := strings.TrimSpace(*req.Type)
		if pt != "" && !store.IsValidCredentialProviderType(pt) {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid provider type")
			return
		}
		patch.Type = pt
	}
	if req.Enabled != nil {
		patch.Enabled = *req.Enabled
		patch.EnabledSet = true
	}

	var configPtr *map[string]string
	if req.Config != nil {
		c := *req.Config
		configPtr = &c
	}

	rec, err := s.credentialProviderStore.UpdateProvider(r.Context(), id, patch, configPtr)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrSystemCredentialProvider):
			writeError(w, ErrorCodeConflict, "this provider is a system default and cannot be reconfigured")
			return
		case errors.Is(err, store.ErrCredentialProviderNotFound):
			writeError(w, ErrorCodeNotFound, "credential provider not found")
			return
		}
		writeInternalError(w, err, "failed to update credential provider")
		return
	}
	s.audit(r, store.AuditCredentialProviderUpdated, map[string]any{
		"provider_id": rec.ID.String(),
		"name":        rec.Name,
		"type":        rec.Type,
	})
	writeData(w, http.StatusOK, rec)
}

// --- Shared secrets (admin) ---

type sharedSecretRequest struct {
	ProviderID      *string   `json:"provider_id,omitempty"`
	Name            *string   `json:"name,omitempty"`
	Description     *string   `json:"description,omitempty"`
	RemoteRef       *string   `json:"remote_ref,omitempty"`
	Value           *string   `json:"value,omitempty"`
	AllowedAgentIDs *[]string `json:"allowed_agent_ids,omitempty"`
}

// handleSharedSecrets handles /api/v1/shared-secrets (list + create).
func (s *Server) handleSharedSecrets(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.sharedSecretStore, "shared secret store") {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !s.checkPermission(w, r, rbac.CredentialSecretsRead) {
			return
		}
		s.listSharedSecrets(w, r)
	case http.MethodPost:
		if !s.checkPermission(w, r, rbac.CredentialSecretsManage) {
			return
		}
		s.createSharedSecret(w, r)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

// handleSharedSecretByID handles /api/v1/shared-secrets/{id}.
func (s *Server) handleSharedSecretByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.sharedSecretStore, "shared secret store") {
		return
	}
	id, ok := parseUUIDPath(w, r, "/api/v1/shared-secrets/")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !s.checkPermission(w, r, rbac.CredentialSecretsRead) {
			return
		}
		rec, err := s.sharedSecretStore.GetSecretByID(r.Context(), id)
		if err != nil {
			writeInternalError(w, err, "failed to get shared secret")
			return
		}
		writeData(w, http.StatusOK, rec)
	case http.MethodPatch:
		if !s.checkPermission(w, r, rbac.CredentialSecretsManage) {
			return
		}
		s.updateSharedSecret(w, r, id)
	case http.MethodDelete:
		if !s.checkPermission(w, r, rbac.CredentialSecretsManage) {
			return
		}
		if err := s.sharedSecretStore.DeleteSecret(r.Context(), id); err != nil {
			if errors.Is(err, store.ErrSharedSecretNotFound) {
				writeError(w, ErrorCodeNotFound, "shared secret not found")
				return
			}
			writeInternalError(w, err, "failed to delete shared secret")
			return
		}
		s.audit(r, store.AuditSharedSecretDeleted, map[string]any{"secret_id_ref": id.String()})
		writeStatus(w, "deleted")
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) listSharedSecrets(w http.ResponseWriter, r *http.Request) {
	q := store.SharedSecretQuery{Search: r.URL.Query().Get("q")}
	if raw := r.URL.Query().Get("provider_id"); raw != "" {
		if pid, err := uuid.Parse(raw); err == nil {
			q.ProviderID = &pid
		}
	}
	limit, skip := parseLimitSkip(r, 50)
	q.Limit = int(limit)
	q.Skip = int(skip)
	items, total, err := s.sharedSecretStore.ListSecrets(r.Context(), q)
	if err != nil {
		writeInternalError(w, err, "failed to list shared secrets")
		return
	}

	// Attach provider summaries so the list view can render the provider type
	// without N+1 round-trips per item: a single batched lookup by provider id.
	if s.credentialProviderStore != nil && len(items) > 0 {
		seen := map[uuid.UUID]*store.CredentialProviderRecord{}
		for i := range items {
			pid := items[i].ProviderID
			if _, ok := seen[pid]; ok {
				items[i].Provider = seen[pid]
				continue
			}
			p, err := s.credentialProviderStore.GetProvider(r.Context(), pid)
			if err == nil && p != nil {
				seen[pid] = p
				items[i].Provider = p
			}
		}
	}
	writePaginatedJSON(w, ensureSlice(items), total)
}

func (s *Server) createSharedSecret(w http.ResponseWriter, r *http.Request) {
	var req sharedSecretRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ProviderID == nil || strings.TrimSpace(*req.ProviderID) == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "provider_id is required")
		return
	}
	providerID, err := uuid.Parse(strings.TrimSpace(*req.ProviderID))
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid provider_id")
		return
	}
	if s.credentialProviderStore != nil {
		prov, err := s.credentialProviderStore.GetProvider(r.Context(), providerID)
		if err != nil || prov == nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "provider_id does not reference an existing provider")
			return
		}
		if !prov.Enabled {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "provider is disabled")
			return
		}
	}
	name := strings.TrimSpace(derefString(req.Name))
	if name == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "name is required")
		return
	}
	// secret_id is always server-generated so agents fetch by an unpredictable
	// identifier; any client-supplied value is ignored.
	secretID := uuid.NewString()
	allowed, ok := parseAgentIDList(w, req.AllowedAgentIDs)
	if !ok {
		return
	}

	value := derefString(req.Value)
	remoteRef := derefString(req.RemoteRef)

	rec, err := s.sharedSecretStore.CreateSecret(r.Context(), &store.SharedSecretRecord{
		ProviderID:      providerID,
		Name:            name,
		SecretID:        secretID,
		Description:     derefString(req.Description),
		RemoteRef:       remoteRef,
		AllowedAgentIDs: allowed,
	}, value)
	if err != nil {
		writeInternalError(w, err, "failed to create shared secret")
		return
	}
	s.audit(r, store.AuditSharedSecretCreated, map[string]any{
		"secret_id_internal":  rec.ID.String(),
		"secret_id":           rec.SecretID,
		"name":                rec.Name,
		"provider_id":         rec.ProviderID.String(),
		"has_value":           rec.ValueConfigured,
		"allowed_agent_count": len(rec.AllowedAgentIDs),
	})
	writeData(w, http.StatusCreated, rec)
}

func (s *Server) updateSharedSecret(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var req sharedSecretRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	patch := &store.SharedSecretUpdate{}
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		patch.Name = &n
	}
	if req.Description != nil {
		patch.Description = req.Description
	}
	if req.RemoteRef != nil {
		patch.RemoteRef = req.RemoteRef
	}
	if req.AllowedAgentIDs != nil {
		allowed, ok := parseAgentIDList(w, req.AllowedAgentIDs)
		if !ok {
			return
		}
		patch.AllowedAgentIDs = &allowed
	}

	var valuePtr *string
	if req.Value != nil {
		v := *req.Value
		valuePtr = &v
	}

	rec, err := s.sharedSecretStore.UpdateSecret(r.Context(), id, patch, valuePtr)
	if err != nil {
		if errors.Is(err, store.ErrSharedSecretNotFound) {
			writeError(w, ErrorCodeNotFound, "shared secret not found")
			return
		}
		writeInternalError(w, err, "failed to update shared secret")
		return
	}
	s.audit(r, store.AuditSharedSecretUpdated, map[string]any{
		"secret_id_internal": rec.ID.String(),
		"secret_id":          rec.SecretID,
		"name":               rec.Name,
		"value_rotated":      valuePtr != nil,
	})
	writeData(w, http.StatusOK, rec)
}

// --- Agent secret fetch ---

// handleAgentSecretByID handles GET /api/v1/agent/secrets/{secret_id}. An agent
// fetches a shared secret by its stable secret_id and receives the plaintext
// value (internal provider) or a proxied value (external provider). Access is
// restricted to the secret's allow-list: only agent token IDs in the list may
// fetch it, and an empty list denies every agent.
func (s *Server) handleAgentSecretByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.sharedSecretStore, "shared secret store") {
		return
	}
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	agent, ok := requireAgent(w, r)
	if !ok {
		return
	}
	secretID := strings.TrimSuffix(pathID(r, "/api/v1/agent/secrets/"), "/")
	if secretID == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing secret_id")
		return
	}

	rec, err := s.sharedSecretStore.GetSecretBySecretID(r.Context(), secretID)
	if err != nil {
		writeInternalError(w, err, "failed to load shared secret")
		return
	}
	if rec == nil {
		writeError(w, ErrorCodeNotFound, "secret not found")
		return
	}
	if !rec.AgentAllowed(agent.ID) {
		writeError(
			// Return a generic not-found to avoid leaking the existence of secrets
			// an agent is not authorized to read.
			w, ErrorCodeNotFound, "secret not found")
		return
	}

	provider, err := s.resolveSecretProvider(r.Context(), rec.ProviderID)
	if err != nil {
		writeInternalError(w, err, "failed to resolve credential provider")
		return
	}
	if provider == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "credential provider not configured")
		return
	}

	value, err := provider.GetSecret(r.Context(), secretprovider.SecretRef{
		ValueEncrypted: rec.ValueEncrypted,
		RemoteRef:      rec.RemoteRef,
	})
	if err != nil {
		switch {
		case errors.Is(err, secretprovider.ErrNotImplemented):
			writeErrorStatus(w, http.StatusNotImplemented, ErrorCodeInternal, "external provider fetch is not implemented")
		case errors.Is(err, secretprovider.ErrMissingValue):
			writeError(w, ErrorCodeConflict, "secret has no stored value")
		default:
			writeInternalError(w, err, "failed to resolve secret")
		}
		return
	}

	s.auditAgent(r, store.AuditSharedSecretAccessed, agent.Name, map[string]any{
		"secret_id":   rec.SecretID,
		"name":        rec.Name,
		"provider_id": rec.ProviderID.String(),
	})
	logger.InfoCtx(r.Context(), "agent fetched shared secret", "component", "api",
		"agent", agent.Name, "secret_id", rec.SecretID)
	writeJSON(w, http.StatusOK, map[string]any{
		"secret_id":  rec.SecretID,
		"name":       rec.Name,
		"value":      value,
		"fetched_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// resolveSecretProvider loads the provider record (with decrypted config) and
// builds a Provider instance from the registry. Returns (nil, nil) when the
// provider store is unavailable or the provider does not exist.
func (s *Server) resolveSecretProvider(ctx context.Context, providerID uuid.UUID) (secretprovider.Provider, error) {
	if s.credentialProviderStore == nil {
		return nil, nil
	}
	rec, err := s.credentialProviderStore.GetProviderWithConfig(ctx, providerID)
	if err != nil || rec == nil {
		return nil, nil
	}
	if !rec.Enabled {
		return nil, nil
	}
	reg := s.secretProviderRegistry
	if reg == nil {
		reg = secretprovider.NewRegistry()
	}
	return reg.Resolve(rec.Type, rec.Config)
}

// --- helpers ---

func parseUUIDPath(w http.ResponseWriter, r *http.Request, prefix string) (uuid.UUID, bool) {
	raw := strings.TrimSuffix(pathID(r, prefix), "/")
	id, err := uuid.Parse(raw)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid id")
		return uuid.Nil, false
	}
	return id, true
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func parseAgentIDList(w http.ResponseWriter, raw *[]string) ([]uuid.UUID, bool) {
	if raw == nil {
		return nil, true
	}
	out := make([]uuid.UUID, 0, len(*raw))
	for _, s := range *raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		id, err := uuid.Parse(s)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid allowed_agent_ids entry: "+s)
			return nil, false
		}
		out = append(out, id)
	}
	return out, true
}
