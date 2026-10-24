package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/rbac"
	"alga/store"
)

type samlProviderResponse struct {
	ID                 uuid.UUID `json:"id"`
	Name               string    `json:"name"`
	IDPEntityID        string    `json:"idp_entity_id"`
	EmailAttribute     string    `json:"email_attribute"`
	Enabled            bool      `json:"enabled"`
	MetadataURL        string    `json:"metadata_url"`
	ACSURL             string    `json:"acs_url"`
	AuthorizeURL       string    `json:"authorize_url"`
	EntityID           string    `json:"entity_id"`
	MetadataConfigured bool      `json:"metadata_configured"`
	ConfigurationValid bool      `json:"configuration_valid"`
	CreatedAt          string    `json:"created_at"`
	UpdatedAt          string    `json:"updated_at"`
}

type samlProviderInput struct {
	Name           *string `json:"name"`
	MetadataXML    *string `json:"metadata_xml"`
	EmailAttribute *string `json:"email_attribute"`
	Enabled        *bool   `json:"enabled"`
}

func (h *samlHandler) listProviders(w http.ResponseWriter, r *http.Request) {
	limit, skip := parseLimitSkip(r, 100)
	query := store.SAMLProviderQuery{Search: strings.TrimSpace(r.URL.Query().Get("search")), Limit: int(limit), Skip: int(skip)}
	switch r.URL.Query().Get("enabled") {
	case "true":
		enabled := true
		query.Enabled = &enabled
	case "false":
		enabled := false
		query.Enabled = &enabled
	}
	providers, total, err := h.providerStore.ListProviders(r.Context(), query)
	if err != nil {
		writeInternalError(w, err, "failed to list saml providers")
		return
	}
	items := make([]samlProviderResponse, 0, len(providers))
	for i := range providers {
		items = append(items, h.providerResponse(&providers[i]))
	}
	writePaginatedJSON(w, items, total)
}

func (h *samlHandler) createProvider(w http.ResponseWriter, r *http.Request) {
	var input samlProviderInput
	if !decodeJSON(w, r, &input) {
		return
	}
	name := ""
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
	}
	if name == "" || len(name) > 128 {
		writeError(w, ErrorCodeValidationFailed, "name is required and must be at most 128 characters")
		return
	}
	metadataXML := ""
	idpEntityID := ""
	if input.MetadataXML != nil && strings.TrimSpace(*input.MetadataXML) != "" {
		metadataXML = strings.TrimSpace(*input.MetadataXML)
		metadata, err := parseSAMLMetadata(metadataXML)
		if err != nil {
			writeError(w, ErrorCodeValidationFailed, "identity provider metadata must include a valid issuer, HTTPS sign-in endpoint, and signing certificate")
			return
		}
		idpEntityID = metadata.EntityID
	}
	emailAttribute := "email"
	if input.EmailAttribute != nil {
		emailAttribute = strings.TrimSpace(*input.EmailAttribute)
	}
	if !validSAMLAttributeName(emailAttribute) {
		writeError(w, ErrorCodeValidationFailed, "email attribute name is required and must be at most 256 characters")
		return
	}
	enabled := false
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	if metadataXML == "" {
		if input.Enabled != nil && enabled {
			writeError(w, ErrorCodeValidationFailed, "add identity provider metadata before enabling this provider")
			return
		}
		enabled = false
	}
	created, err := h.providerStore.CreateProvider(r.Context(), &store.SAMLProviderRecord{
		Name:           name,
		IDPEntityID:    idpEntityID,
		MetadataXML:    metadataXML,
		EmailAttribute: emailAttribute,
		Enabled:        enabled,
	})
	if err != nil {
		if store.IsDuplicateKey(err) {
			writeError(w, ErrorCodeConflict, "a provider with this name or issuer is already configured")
			return
		}
		writeInternalError(w, err, "failed to create saml provider")
		return
	}
	h.auditProvider(r, store.AuditSAMLProviderCreated, created)
	writeJSON(w, http.StatusCreated, h.providerResponse(created))
}

func (h *samlHandler) updateProvider(w http.ResponseWriter, r *http.Request, providerID uuid.UUID) {
	var input samlProviderInput
	if !decodeJSON(w, r, &input) {
		return
	}
	patch := &store.SAMLProviderRecord{}
	if input.Name != nil {
		patch.Name = strings.TrimSpace(*input.Name)
		if patch.Name == "" || len(patch.Name) > 128 {
			writeError(w, ErrorCodeValidationFailed, "name must be between 1 and 128 characters")
			return
		}
	}
	if input.MetadataXML != nil && strings.TrimSpace(*input.MetadataXML) != "" {
		patch.MetadataXML = strings.TrimSpace(*input.MetadataXML)
		metadata, err := parseSAMLMetadata(patch.MetadataXML)
		if err != nil {
			writeError(w, ErrorCodeValidationFailed, "identity provider metadata must include a valid issuer, HTTPS sign-in endpoint, and signing certificate")
			return
		}
		patch.MetadataXMLSet = true
		patch.IDPEntityID = metadata.EntityID
	}
	if input.EmailAttribute != nil {
		patch.EmailAttribute = strings.TrimSpace(*input.EmailAttribute)
		if !validSAMLAttributeName(patch.EmailAttribute) {
			writeError(w, ErrorCodeValidationFailed, "email attribute name is required and must be at most 256 characters")
			return
		}
	}
	if input.Enabled != nil {
		patch.Enabled = *input.Enabled
		patch.EnabledSet = true
	}
	if patch.Name == "" && !patch.MetadataXMLSet && patch.EmailAttribute == "" && !patch.EnabledSet {
		writeError(w, ErrorCodeValidationFailed, "at least one provider field must be updated")
		return
	}
	if patch.MetadataXMLSet || patch.EnabledSet && patch.Enabled {
		current, err := h.providerStore.GetProvider(r.Context(), providerID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, ErrorCodeNotFound, "saml provider not found")
				return
			}
			writeInternalError(w, err, "failed to load saml provider")
			return
		}
		if patch.MetadataXMLSet && current.IDPEntityID != "" && current.IDPEntityID != patch.IDPEntityID {
			writeError(w, ErrorCodeValidationFailed, "identity provider issuer cannot be changed; create a new provider to change identity sources")
			return
		}
		if patch.EnabledSet && patch.Enabled {
			metadataXML := current.MetadataXML
			entityID := current.IDPEntityID
			if patch.MetadataXMLSet {
				metadataXML = patch.MetadataXML
				entityID = patch.IDPEntityID
			}
			metadata, err := parseSAMLMetadata(metadataXML)
			if err != nil || metadata.EntityID != entityID {
				writeError(w, ErrorCodeValidationFailed, "add valid identity provider metadata before enabling this provider")
				return
			}
		}
	}
	updated, err := h.providerStore.UpdateProvider(r.Context(), providerID, patch)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, ErrorCodeNotFound, "saml provider not found")
			return
		}
		if store.IsDuplicateKey(err) {
			writeError(w, ErrorCodeConflict, "a provider with this name or issuer is already configured")
			return
		}
		writeInternalError(w, err, "failed to update saml provider")
		return
	}
	h.auditProvider(r, store.AuditSAMLProviderUpdated, updated)
	writeJSON(w, http.StatusOK, h.providerResponse(updated))
}

func (h *samlHandler) deleteProvider(w http.ResponseWriter, r *http.Request, providerID uuid.UUID) {
	if err := h.providerStore.DeleteProvider(r.Context(), providerID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, ErrorCodeNotFound, "saml provider not found")
			return
		}
		writeInternalError(w, err, "failed to delete saml provider")
		return
	}
	user := userFromContext(r.Context())
	var userID *uuid.UUID
	if user != nil {
		userID = &user.ID
	}
	h.server.auditStore.Log(store.AuditSAMLProviderDeleted, userID, "", r.RemoteAddr, r.UserAgent(), true, map[string]any{
		"provider_id": providerID,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *samlHandler) providerResponse(provider *store.SAMLProviderRecord) samlProviderResponse {
	response := samlProviderResponse{
		ID:                 provider.ID,
		Name:               provider.Name,
		IDPEntityID:        provider.IDPEntityID,
		EmailAttribute:     provider.EmailAttribute,
		Enabled:            provider.Enabled,
		MetadataConfigured: provider.MetadataXML != "",
		CreatedAt:          provider.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:          provider.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	sp, err := h.serviceProviderEndpoints(provider)
	if err != nil {
		return response
	}
	response.MetadataURL = sp.MetadataURL.String()
	response.ACSURL = sp.AcsURL.String()
	baseURL, err := h.publicBaseURL()
	if err != nil {
		return response
	}
	authorizeURL := samlEndpointURL(baseURL, provider.ID, "authorize")
	response.AuthorizeURL = authorizeURL.String()
	response.EntityID = sp.EntityID
	metadata, err := parseSAMLMetadata(provider.MetadataXML)
	if err == nil && metadata.EntityID == provider.IDPEntityID {
		response.ConfigurationValid = true
	}
	return response
}

func validSAMLAttributeName(name string) bool {
	return name != "" && len(name) <= 256 && !strings.ContainsAny(name, "\r\n\x00")
}

func (h *samlHandler) auditProvider(r *http.Request, event store.AuditEvent, provider *store.SAMLProviderRecord) {
	user := userFromContext(r.Context())
	var userID *uuid.UUID
	if user != nil {
		userID = &user.ID
	}
	h.server.auditStore.Log(event, userID, "", r.RemoteAddr, r.UserAgent(), true, map[string]any{
		"provider_id":   provider.ID,
		"name":          provider.Name,
		"idp_entity_id": provider.IDPEntityID,
	})
}

func (s *Server) handleSAMLListProviders(w http.ResponseWriter, r *http.Request) {
	if s.samlHandler == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "saml sign-in is not configured")
		return
	}
	s.samlHandler.listProviders(w, r)
}

func (s *Server) handleSAMLCreateProvider(w http.ResponseWriter, r *http.Request) {
	if s.samlHandler == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "saml sign-in is not configured")
		return
	}
	s.samlHandler.createProvider(w, r)
}

func (s *Server) handleSAMLProviderByID(w http.ResponseWriter, r *http.Request) {
	if s.samlHandler == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "saml sign-in is not configured")
		return
	}
	providerID, ok := samlPathID(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodPatch:
		if !s.checkPermission(w, r, rbac.OIDCManage) {
			return
		}
		s.samlHandler.updateProvider(w, r, providerID)
	case http.MethodDelete:
		if !s.checkPermission(w, r, rbac.OIDCManage) {
			return
		}
		s.samlHandler.deleteProvider(w, r, providerID)
	default:
		w.Header().Set("Allow", "PATCH, DELETE")
		writeError(w, ErrorCodeMethodNotAllowed, "method not allowed")
	}
}
