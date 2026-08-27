package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"alga/ics"
	"alga/rbac"
	"alga/store"

	"github.com/google/uuid"
)

type icsIncidentStoreAdapter struct {
	incStore store.IncidentStore
}

func (a *icsIncidentStoreAdapter) GetIncident(ctx context.Context, incidentNumber int64) (*ics.IncidentRecord, error) {
	rec, err := a.incStore.GetIncident(ctx, incidentNumber)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}
	return &ics.IncidentRecord{
		ID:                     rec.ID,
		IncidentNumber:         rec.IncidentNumber,
		Title:                  rec.Title,
		Status:                 rec.Status,
		Severity:               rec.Severity,
		WarRoomChannelID:       rec.WarRoomChannelID,
		WarRoomChannelProvider: rec.WarRoomChannelProvider,
		GoogleMeetSpaceName:    rec.GoogleMeetSpaceName,
		TriageReport:           rec.TriageReport,
	}, nil
}

func (a *icsIncidentStoreAdapter) AddTimelineEntry(ctx context.Context, entry *ics.TimelineEntry) error {
	return a.incStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: entry.IncidentNumber,
		EventType:      entry.EventType,
		ActorID:        entry.ActorID,
		ActorType:      entry.ActorType,
		Message:        entry.Message,
		Metadata:       entry.Metadata,
	})
}

func (a *icsIncidentStoreAdapter) SetWarRoomMeet(ctx context.Context, incidentNumber int64, spaceName, conferenceURL string) error {
	return a.incStore.SetIncidentWarRoomMeet(ctx, incidentNumber, spaceName, conferenceURL)
}

type icsRoleStoreAdapter struct {
	inner store.ICSRoleStore
}

func (a *icsRoleStoreAdapter) AssignRole(ctx context.Context, incidentNumber int64, roleType ics.RoleType, userID uuid.UUID, parentAssignmentID *uuid.UUID, scope *string) (*ics.RoleRecord, error) {
	rec, err := a.inner.AssignRole(ctx, incidentNumber, roleType, userID, parentAssignmentID, scope)
	if err != nil {
		return nil, err
	}
	return storeRoleToICS(rec), nil
}

func (a *icsRoleStoreAdapter) EndRole(ctx context.Context, assignmentID uuid.UUID, reason ics.EndReason) error {
	return a.inner.EndRole(ctx, assignmentID, reason)
}

func (a *icsRoleStoreAdapter) GetActiveRoles(ctx context.Context, incidentNumber int64) ([]ics.RoleRecord, error) {
	records, err := a.inner.GetActiveRoles(ctx, incidentNumber)
	if err != nil {
		return nil, err
	}
	result := make([]ics.RoleRecord, len(records))
	for i, r := range records {
		result[i] = *storeRoleToICS(&r)
	}
	return result, nil
}

func (a *icsRoleStoreAdapter) GetActiveIC(ctx context.Context, incidentNumber int64) (*ics.RoleRecord, error) {
	rec, err := a.inner.GetActiveIC(ctx, incidentNumber)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}
	return storeRoleToICS(rec), nil
}

func (a *icsRoleStoreAdapter) EndAllRolesForIncident(ctx context.Context, incidentNumber int64, reason ics.EndReason) error {
	return a.inner.EndAllRolesForIncident(ctx, incidentNumber, reason)
}

func (a *icsRoleStoreAdapter) AssignAgentRole(ctx context.Context, incidentNumber int64, roleType ics.RoleType, agentTokenID uuid.UUID, parentAssignmentID *uuid.UUID, scope *string) (*ics.RoleRecord, error) {
	rec, err := a.inner.AssignAgentRole(ctx, incidentNumber, roleType, agentTokenID, parentAssignmentID, scope)
	if err != nil {
		return nil, err
	}
	return storeRoleToICS(rec), nil
}

func (a *icsRoleStoreAdapter) GetActiveRolesForAgent(ctx context.Context, agentTokenID uuid.UUID) ([]ics.RoleRecord, error) {
	records, err := a.inner.GetActiveRolesForAgent(ctx, agentTokenID)
	if err != nil {
		return nil, err
	}
	result := make([]ics.RoleRecord, len(records))
	for i, r := range records {
		result[i] = *storeRoleToICS(&r)
	}
	return result, nil
}

func (a *icsRoleStoreAdapter) EndRolesForAgent(ctx context.Context, agentTokenID uuid.UUID, reason ics.EndReason) error {
	return a.inner.EndRolesForAgent(ctx, agentTokenID, reason)
}

func storeRoleToICS(r *store.ICSRoleRecord) *ics.RoleRecord {
	return &ics.RoleRecord{
		ID:                 r.ID,
		IncidentNumber:     r.IncidentNumber,
		RoleType:           r.RoleType,
		AssigneeType:       r.AssigneeType,
		UserID:             r.UserID,
		UserName:           r.UserName,
		UserEmail:          r.UserEmail,
		AgentTokenID:       r.AgentTokenID,
		AgentName:          r.AgentName,
		AgentType:          r.AgentType,
		ParentAssignmentID: r.ParentAssignmentID,
		ScopeDescription:   r.ScopeDescription,
		Status:             r.Status,
		EndedReason:        r.EndedReason,
		StartedAt:          r.StartedAt,
		EndedAt:            r.EndedAt,
	}
}

type icsDocumentStoreAdapter struct {
	inner store.IncidentDocumentStore
}

func (a *icsDocumentStoreAdapter) GetAllSections(ctx context.Context, incidentNumber int64) ([]ics.DocumentRecord, error) {
	records, err := a.inner.GetAllSections(ctx, incidentNumber)
	if err != nil {
		return nil, err
	}
	result := make([]ics.DocumentRecord, len(records))
	for i, r := range records {
		result[i] = ics.DocumentRecord{
			ID:             r.ID,
			IncidentNumber: r.IncidentNumber,
			Section:        r.Section,
			Content:        r.Content,
			Version:        r.Version,
			UpdatedBy:      r.UpdatedBy,
			UpdatedAt:      r.UpdatedAt,
		}
	}
	return result, nil
}

func (a *icsDocumentStoreAdapter) UpsertSection(ctx context.Context, incidentNumber int64, section ics.DocumentSection, content string, version int, userID uuid.UUID) (*ics.DocumentRecord, error) {
	rec, err := a.inner.UpsertSection(ctx, incidentNumber, section, content, version, userID)
	if err != nil {
		return nil, err
	}
	return &ics.DocumentRecord{
		ID:             rec.ID,
		IncidentNumber: rec.IncidentNumber,
		Section:        rec.Section,
		Content:        rec.Content,
		Version:        rec.Version,
		UpdatedBy:      rec.UpdatedBy,
		UpdatedAt:      rec.UpdatedAt,
	}, nil
}

func (a *icsDocumentStoreAdapter) InitializeDocument(ctx context.Context, incidentNumber int64, sections map[ics.DocumentSection]string) error {
	return a.inner.InitializeDocument(ctx, incidentNumber, sections)
}

func NewICSWarRoomProvisioner(roleStore store.ICSRoleStore, docStore store.IncidentDocumentStore, incStore store.IncidentStore, meetClient ics.MeetSpaceCreator) *ics.WarRoomProvisioner {
	roleAdapter := &icsRoleStoreAdapter{inner: roleStore}
	docAdapter := &icsDocumentStoreAdapter{inner: docStore}
	docManager := ics.NewDocumentManager(docAdapter)
	incAdapter := &icsIncidentStoreAdapter{incStore: incStore}
	return ics.NewWarRoomProvisioner(incAdapter, roleAdapter, docManager, meetClient)
}

func (s *Server) requireICSRoleStore(w http.ResponseWriter) bool {
	if s.icsRoleStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "ICS role store not configured")
		return false
	}
	return true
}

func (s *Server) requireIncidentDocumentStore(w http.ResponseWriter) bool {
	if s.incidentDocumentStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "incident document store not configured")
		return false
	}
	return true
}

func (s *Server) icsRoleAdapter() *icsRoleStoreAdapter {
	return &icsRoleStoreAdapter{inner: s.icsRoleStore}
}

func (s *Server) icsDocAdapter() *icsDocumentStoreAdapter {
	return &icsDocumentStoreAdapter{inner: s.incidentDocumentStore}
}

func (s *Server) handleICSRoles(w http.ResponseWriter, r *http.Request) {
	incidentID := strings.TrimSuffix(pathID(r, "/api/v1/incidents/"), "/ics/roles")
	if incidentID == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing incident id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListICSRoles(w, r, incidentID)
	case http.MethodPost:
		s.handleAssignICSRole(w, r, incidentID)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleICSRoleRoutes(w http.ResponseWriter, r *http.Request) {
	suffix := pathID(r, "/api/v1/incidents/")
	parts := strings.SplitN(suffix, "/ics/roles/", 2)
	if len(parts) != 2 || parts[0] == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid URL")
		return
	}
	roleID, err := uuid.Parse(parts[1])
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid role assignment id")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		s.handleUpdateICSRole(w, r, parts[0], roleID)
	case http.MethodDelete:
		s.handleEndICSRole(w, r, parts[0], roleID)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleListICSRoles(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsRead) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}
	if !s.requireICSRoleStore(w) {
		return
	}
	roles, err := s.icsRoleStore.GetAllRoles(r.Context(), mustParseIncidentNumber(incidentID))
	if err != nil {
		writeInternalError(w, err, "failed to list ICS roles")
		return
	}
	writeData(w, http.StatusOK, ensureSlice(roles))
}

func (s *Server) handleAssignICSRole(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}
	if !s.requireICSRoleStore(w) {
		return
	}
	var req struct {
		RoleType         string  `json:"role_type"`
		UserID           string  `json:"user_id,omitempty"`
		AgentTokenID     string  `json:"agent_token_id,omitempty"`
		ScopeDescription *string `json:"scope_description,omitempty"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.RoleType == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "role_type is required")
		return
	}
	if req.UserID == "" && req.AgentTokenID == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "user_id or agent_token_id is required")
		return
	}
	if req.UserID != "" && req.AgentTokenID != "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "provide either user_id or agent_token_id, not both")
		return
	}
	if !ics.ValidRoleType(ics.RoleType(req.RoleType)) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid role_type")
		return
	}

	rm := ics.NewRoleManager(s.icsRoleAdapter())
	incidentNumber := mustParseIncidentNumber(incidentID)

	if req.AgentTokenID != "" {
		atid, err := uuid.Parse(req.AgentTokenID)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid agent_token_id")
			return
		}
		record, err := rm.AssignAgentRole(r.Context(), incidentNumber, ics.RoleType(req.RoleType), atid, nil, req.ScopeDescription)
		if err != nil {
			if errors.Is(err, ics.ErrRoleNotAssignable) {
				writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
				return
			}
			if errors.Is(err, store.ErrAgentNotFoundInactive) {
				writeError(w, ErrorCodeNotFound, "agent not found or inactive")
				return
			}
			if errors.Is(err, store.ErrAgentCapabilityMismatch) {
				writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
				return
			}
			writeInternalError(w, err, "failed to assign agent ICS role")
			return
		}
		s.audit(r, store.AuditAgentRoleAssigned, map[string]any{
			"incident_number": incidentID,
			"role_type":       req.RoleType,
			"agent_token_id":  atid.String(),
		})
		s.addIncidentTimeline(r, incidentID, "ics_role_assigned", "Agent ICS role assigned: "+req.RoleType)
		s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": incidentID})
		writeData(w, http.StatusCreated, record)
		return
	}

	uid, err := uuid.Parse(req.UserID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid user_id")
		return
	}
	record, err := rm.AssignRole(r.Context(), incidentNumber, ics.RoleType(req.RoleType), uid, nil, req.ScopeDescription)
	if err != nil {
		if errors.Is(err, ics.ErrRoleNotAssignable) {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
			return
		}
		writeInternalError(w, err, "failed to assign ICS role")
		return
	}
	s.audit(r, store.AuditIncidentRoleAssigned, map[string]any{
		"incident_number": incidentID,
		"role_type":       req.RoleType,
		"user_id":         uid.String(),
	})
	s.addIncidentTimeline(r, incidentID, "ics_role_assigned", "ICS role assigned: "+req.RoleType)
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": incidentID})
	writeData(w, http.StatusCreated, record)
}

func (s *Server) handleUpdateICSRole(w http.ResponseWriter, r *http.Request, incidentID string, roleID uuid.UUID) {
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}
	if !s.requireICSRoleStore(w) {
		return
	}
	var req struct {
		ScopeDescription string `json:"scope_description"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	roles, err := s.icsRoleStore.GetAllRoles(r.Context(), mustParseIncidentNumber(incidentID))
	if err != nil {
		writeInternalError(w, err, "failed to list ICS roles")
		return
	}
	var found *store.ICSRoleRecord
	for i := range roles {
		if roles[i].ID == roleID {
			found = &roles[i]
			break
		}
	}
	if found == nil {
		writeError(w, ErrorCodeNotFound, "role assignment not found")
		return
	}
	scope := req.ScopeDescription
	if found.UserID == nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "cannot update agent-assigned role via this endpoint")
		return
	}
	_, err = s.icsRoleStore.AssignRole(r.Context(), mustParseIncidentNumber(incidentID), ics.RoleType(found.RoleType), *found.UserID, found.ParentAssignmentID, &scope)
	if err != nil {
		writeInternalError(w, err, "failed to update ICS role scope")
		return
	}
	s.addIncidentTimeline(r, incidentID, "ics_role_updated", "ICS role scope updated")
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": incidentID})
	writeStatus(w, "updated")
}

func (s *Server) handleEndICSRole(w http.ResponseWriter, r *http.Request, incidentID string, roleID uuid.UUID) {
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}
	if !s.requireICSRoleStore(w) {
		return
	}
	var req struct {
		EndedReason string `json:"ended_reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.EndedReason == "" {
		req.EndedReason = string(ics.EndReasonReplaced)
	}
	if !ics.ValidEndReason(ics.EndReason(req.EndedReason)) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "unknown ended_reason")
		return
	}
	if err := s.icsRoleStore.EndRole(r.Context(), roleID, ics.EndReason(req.EndedReason)); err != nil {
		if errors.Is(err, store.ErrICSRoleNotFound) || errors.Is(err, store.ErrIncidentNotFound) {
			writeError(w, ErrorCodeNotFound, "role assignment not found")
			return
		}
		writeInternalError(w, err, "failed to end ICS role")
		return
	}
	s.addIncidentTimeline(r, incidentID, "ics_role_ended", "ICS role ended: "+req.EndedReason)
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": incidentID})
	writeStatus(w, "ended")
}

func (s *Server) handleICSDocument(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	if !s.checkPermission(w, r, rbac.IncidentsRead) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}
	if !s.requireIncidentDocumentStore(w) {
		return
	}
	suffix := pathID(r, "/api/v1/incidents/")
	incidentID := strings.TrimSuffix(strings.TrimSuffix(suffix, "/ics/document"), "/ics/document/")
	if incidentID == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing incident id")
		return
	}
	sections, err := s.incidentDocumentStore.GetAllSections(r.Context(), mustParseIncidentNumber(incidentID))
	if err != nil {
		writeInternalError(w, err, "failed to get incident document")
		return
	}
	writeData(w, http.StatusOK, ensureSlice(sections))
}

func (s *Server) handleICSDocumentRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	if !s.checkPermission(w, r, rbac.IncidentsWrite) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}
	if !s.requireIncidentDocumentStore(w) {
		return
	}
	suffix := pathID(r, "/api/v1/incidents/")
	parts := strings.SplitN(suffix, "/ics/document/", 2)
	if len(parts) != 2 || parts[0] == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid URL")
		return
	}
	section := ics.DocumentSection(parts[1])
	if !ics.ValidDocumentSection(section) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid document section")
		return
	}
	var req struct {
		Content string `json:"content"`
		Version int    `json:"version"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "unauthorized")
		return
	}

	dm := ics.NewDocumentManager(s.icsDocAdapter())
	updated, err := dm.UpdateSection(r.Context(), mustParseIncidentNumber(parts[0]), section, req.Content, req.Version, user.ID)
	if err != nil {
		if errors.Is(err, store.ErrDocumentVersionConflict) {
			writeError(w, ErrorCodeConflict, "document version conflict — fetch latest and retry")
			return
		}
		writeInternalError(w, err, "failed to update document section")
		return
	}
	s.addIncidentTimeline(r, parts[0], string(ics.ICSEventDocumentUpdated), "Document section updated: "+string(section))
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": parts[0]})
	writeData(w, http.StatusOK, updated)
}

func (s *Server) handleBeginTriage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}
	suffix := pathID(r, "/api/v1/incidents/")
	incidentID := strings.TrimSuffix(suffix, "/begin-triage")
	if incidentID == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing incident id")
		return
	}
	record, ok := s.getIncidentOrError(w, r, incidentID)
	if !ok {
		return
	}
	if record.Status != "detected" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "incident must be in 'detected' status to begin triage")
		return
	}
	if err := s.incidentStore.TransitionIncidentStatus(r.Context(), mustParseIncidentNumber(incidentID), []string{"detected"}, "triaging"); err != nil {
		if errors.Is(err, store.ErrIncidentStatusConflict) {
			writeConflict(w, "incident status changed concurrently")
			return
		}
		writeInternalError(w, err, "failed to transition incident to triaging")
		return
	}
	s.addIncidentTimeline(r, incidentID, string(ics.ICSEventTriageStarted), "Triage started")
	if s.incidentDocumentStore != nil {
		dm := ics.NewDocumentManager(s.icsDocAdapter())
		_ = dm.InitializeForIncident(r.Context(), mustParseIncidentNumber(incidentID), nil)
	}
	updated, _ := s.incidentStore.GetIncident(r.Context(), mustParseIncidentNumber(incidentID))
	s.publishIncidentEvent("incident_updated", updated)
	s.audit(r, store.AuditEvent("incident_begin_triage"), map[string]any{
		"incident_number": incidentID,
	})
	writeData(w, http.StatusOK, updated)
}

func (s *Server) handlePromote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}
	suffix := pathID(r, "/api/v1/incidents/")
	incidentID := strings.TrimSuffix(suffix, "/promote")
	if incidentID == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing incident id")
		return
	}
	record, ok := s.getIncidentOrError(w, r, incidentID)
	if !ok {
		return
	}
	if record.Status != "triaging" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "incident must be in 'triaging' status to promote")
		return
	}
	if err := s.incidentStore.TransitionIncidentStatus(r.Context(), mustParseIncidentNumber(incidentID), []string{"triaging"}, "active"); err != nil {
		if errors.Is(err, store.ErrIncidentStatusConflict) {
			writeConflict(w, "incident status changed concurrently")
			return
		}
		writeInternalError(w, err, "failed to promote incident")
		return
	}
	s.addIncidentTimeline(r, incidentID, string(ics.ICSEventIncidentPromoted), "Incident promoted to active")
	updated, _ := s.incidentStore.GetIncident(r.Context(), mustParseIncidentNumber(incidentID))
	s.publishIncidentEvent("incident_updated", updated)
	s.audit(r, store.AuditEvent("incident_promote"), map[string]any{
		"incident_number": incidentID,
	})
	s.propagateServiceStatus(updated)
	writeData(w, http.StatusOK, updated)
}
