package api

import (
	"errors"
	"net/http"
	"strings"

	"alga/logger"
	"alga/rbac"
	"alga/sse"
	"alga/store"

	"github.com/google/uuid"
)

func (s *Server) handleTeams(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListTeams(w, r)
	case http.MethodPost:
		s.handleCreateTeam(w, r)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleListTeams(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.OnCallRead) {
		return
	}
	if !s.requireTeamStore(w) {
		return
	}

	limit, skip := parseLimitSkip(r, 50)
	records, total, err := s.teamStore.ListTeams(r.Context(), int(limit), int(skip))
	if err != nil {
		writeInternalError(w, err, "failed to list teams")
		return
	}
	writePaginatedJSON(w, ensureSlice(records), int64(total))
}

func (s *Server) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.OnCallWrite) {
		return
	}
	if !s.requireTeamStore(w) {
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "name is required")
		return
	}

	record := &store.TeamRecord{
		Name:        strings.TrimSpace(req.Name),
		Description: req.Description,
	}

	created, err := s.teamStore.CreateTeam(r.Context(), record)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to create team", "component", "api", "error", err)
		writeInternalError(w, err, "failed to create team")
		return
	}

	// Auto-provision an empty schedule for the new team (one schedule per
	// team, Opsgenie-style). The schedule carries no name or timezone of its
	// own: its display name is derived dynamically from the team, and timezone
	// lives per-rotation (layer). If the schedule cannot be created, roll back
	// the team so we never leave a team without its schedule.
	if s.onCallStore != nil {
		sched := &store.OnCallScheduleRecord{
			TeamID: &created.ID,
		}
		if _, sErr := s.onCallStore.CreateSchedule(r.Context(), sched); sErr != nil {
			logger.ErrorCtx(r.Context(), "failed to auto-create schedule for team; compensating", "component", "api", "team_id", created.ID.String(), "error", sErr)
			if dErr := s.teamStore.DeleteTeam(r.Context(), created.ID); dErr != nil {
				logger.ErrorCtx(r.Context(), "failed to compensate-delete team after schedule creation failure", "component", "api", "team_id", created.ID.String(), "error", dErr)
			}
			writeInternalError(w, sErr, "failed to create schedule for team")
			return
		}
		logger.InfoCtx(r.Context(), "auto-created schedule for team", "component", "api", "team_id", created.ID.String())
	}

	logger.InfoCtx(r.Context(), "team created", "component", "api", "team_id", created.ID.String(), "name", req.Name)
	s.audit(r, store.AuditTeamCreated, map[string]any{
		"team_id": created.ID.String(),
		"name":    req.Name,
	})
	if s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{Type: "team_created", Data: created})
	}
	writeData(w, http.StatusCreated, created)
}

func (s *Server) handleTeamRoutes(w http.ResponseWriter, r *http.Request) {
	suffix := pathID(r, "/api/v1/teams/")
	if suffix == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing team id")
		return
	}

	if idx := strings.Index(suffix, "/members/"); idx != -1 {
		teamID := suffix[:idx]
		userID := suffix[idx+len("/members/"):]
		switch r.Method {
		case http.MethodPatch:
			s.handleUpdateTeamMemberRole(w, r, teamID, userID)
		case http.MethodDelete:
			s.handleRemoveTeamMember(w, r, teamID, userID)
		default:
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		}
		return
	}

	if suffix == "members" || strings.HasSuffix(suffix, "/members") {
		teamID := strings.TrimSuffix(suffix, "/members")
		if teamID == "members" {
			teamID = ""
		}
		if teamID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing team id")
			return
		}
		switch r.Method {
		case http.MethodGet:
			s.handleListTeamMembers(w, r, teamID)
		case http.MethodPost:
			s.handleAddTeamMember(w, r, teamID)
		default:
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetTeam(w, r, suffix)
	case http.MethodPatch:
		s.handlePatchTeam(w, r, suffix)
	case http.MethodDelete:
		s.handleDeleteTeam(w, r, suffix)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) getTeamOrError(w http.ResponseWriter, r *http.Request, id string) (*store.TeamRecord, bool) {
	uid, err := uuid.Parse(id)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid team id")
		return nil, false
	}
	record, err := s.teamStore.GetTeam(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to get team")
		return nil, false
	}
	if record == nil {
		writeError(w, ErrorCodeNotFound, "team not found")
		return nil, false
	}
	return record, true
}

func (s *Server) handleGetTeam(w http.ResponseWriter, r *http.Request, id string) {
	if !s.checkPermission(w, r, rbac.OnCallRead) {
		return
	}
	if !s.requireTeamStore(w) {
		return
	}
	record, ok := s.getTeamOrError(w, r, id)
	if !ok {
		return
	}
	writeData(w, http.StatusOK, record)
}

func (s *Server) handlePatchTeam(w http.ResponseWriter, r *http.Request, id string) {
	if !s.checkPermission(w, r, rbac.OnCallWrite) {
		return
	}
	if !s.requireTeamStore(w) {
		return
	}

	uid, err := uuid.Parse(id)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid team id")
		return
	}

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	current, err := s.teamStore.GetTeam(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to get team")
		return
	}
	if current == nil {
		writeError(w, ErrorCodeNotFound, "team not found")
		return
	}

	if req.Name != nil {
		current.Name = *req.Name
	}
	if req.Description != nil {
		current.Description = *req.Description
	}

	updated, err := s.teamStore.UpdateTeam(r.Context(), uid, current)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to update team", "component", "api", "team_id", id, "error", err)
		writeInternalError(w, err, "failed to update team")
		return
	}

	logger.InfoCtx(r.Context(), "team updated", "component", "api", "team_id", id)
	s.audit(r, store.AuditTeamUpdated, map[string]any{
		"team_id": id,
	})
	if s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{Type: "team_updated", Data: updated})
	}
	writeData(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteTeam(w http.ResponseWriter, r *http.Request, id string) {
	if !s.checkPermission(w, r, rbac.OnCallWrite) {
		return
	}
	if !s.requireTeamStore(w) {
		return
	}

	uid, err := uuid.Parse(id)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid team id")
		return
	}

	if err := s.teamStore.DeleteTeam(r.Context(), uid); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, ErrorCodeNotFound, "team not found")
			return
		}
		writeInternalError(w, err, "failed to delete team")
		return
	}

	logger.InfoCtx(r.Context(), "team deleted", "component", "api", "team_id", id)
	s.audit(r, store.AuditTeamDeleted, map[string]any{
		"team_id": id,
	})
	if s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{Type: "team_deleted", Data: map[string]string{"team_id": id}})
	}
	writeStatus(w, "deleted")
}

func (s *Server) handleListTeamMembers(w http.ResponseWriter, r *http.Request, teamID string) {
	if !s.checkPermission(w, r, rbac.OnCallRead) {
		return
	}
	if !s.requireTeamStore(w) {
		return
	}

	uid, err := uuid.Parse(teamID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid team id")
		return
	}

	members, err := s.teamStore.GetMembers(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to list team members")
		return
	}
	writeData(w, http.StatusOK, ensureSlice(members))
}

func (s *Server) handleAddTeamMember(w http.ResponseWriter, r *http.Request, teamID string) {
	if !s.checkPermission(w, r, rbac.OnCallWrite) {
		return
	}
	if !s.requireTeamStore(w) {
		return
	}

	teamUID, err := uuid.Parse(teamID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid team id")
		return
	}

	var req struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.UserID == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "user_id is required")
		return
	}

	userUID, err := uuid.Parse(req.UserID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid user_id")
		return
	}

	role := req.Role
	if role == "" {
		role = "member"
	}

	member, err := s.teamStore.AddMember(r.Context(), teamUID, userUID, role)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to add team member", "component", "api", "team_id", teamID, "user_id", req.UserID, "error", err)
		writeInternalError(w, err, "failed to add team member")
		return
	}

	logger.InfoCtx(r.Context(), "team member added", "component", "api", "team_id", teamID, "user_id", req.UserID)
	s.audit(r, store.AuditTeamUpdated, map[string]any{
		"team_id": teamID,
		"user_id": req.UserID,
		"action":  "add_member",
	})
	writeData(w, http.StatusCreated, member)
}

func (s *Server) handleUpdateTeamMemberRole(w http.ResponseWriter, r *http.Request, teamID, userID string) {
	if !s.checkPermission(w, r, rbac.OnCallWrite) {
		return
	}
	if !s.requireTeamStore(w) {
		return
	}

	teamUID, err := uuid.Parse(teamID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid team id")
		return
	}
	userUID, err := uuid.Parse(userID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid user id")
		return
	}

	var req struct {
		Role string `json:"role"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Role == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "role is required")
		return
	}

	if err := s.teamStore.UpdateMemberRole(r.Context(), teamUID, userUID, req.Role); err != nil {
		logger.ErrorCtx(r.Context(), "failed to update team member role", "component", "api", "team_id", teamID, "user_id", userID, "error", err)
		writeInternalError(w, err, "failed to update team member role")
		return
	}

	logger.InfoCtx(r.Context(), "team member role updated", "component", "api", "team_id", teamID, "user_id", userID, "role", req.Role)
	s.audit(r, store.AuditTeamUpdated, map[string]any{
		"team_id": teamID,
		"user_id": userID,
		"action":  "update_role",
		"role":    req.Role,
	})
	writeStatus(w, "updated")
}

func (s *Server) handleRemoveTeamMember(w http.ResponseWriter, r *http.Request, teamID, userID string) {
	if !s.checkPermission(w, r, rbac.OnCallWrite) {
		return
	}
	if !s.requireTeamStore(w) {
		return
	}

	teamUID, err := uuid.Parse(teamID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid team id")
		return
	}
	userUID, err := uuid.Parse(userID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid user id")
		return
	}

	if err := s.teamStore.RemoveMember(r.Context(), teamUID, userUID); err != nil {
		logger.ErrorCtx(r.Context(), "failed to remove team member", "component", "api", "team_id", teamID, "user_id", userID, "error", err)
		writeInternalError(w, err, "failed to remove team member")
		return
	}

	logger.InfoCtx(r.Context(), "team member removed", "component", "api", "team_id", teamID, "user_id", userID)
	s.audit(r, store.AuditTeamUpdated, map[string]any{
		"team_id": teamID,
		"user_id": userID,
		"action":  "remove_member",
	})
	writeStatus(w, "removed")
}
