package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"alga/logger"
	"alga/rbac"
	"alga/sse"
	"alga/store"
	"alga/valkey"

	"github.com/google/uuid"
)

var allowedEscalationTargetTypes = map[string]struct{}{
	"user": {},
	"team": {},
}

type escalationLevelRequest struct {
	LevelNumber    int      `json:"level_number"`
	DelayMinutes   int      `json:"delay_minutes"`
	NotifyChannels []string `json:"notify_channels,omitempty"`
	Targets        []struct {
		TargetType   string `json:"target_type"`
		TargetUserID string `json:"target_user_id,omitempty"`
		TargetTeamID string `json:"target_team_id,omitempty"`
	} `json:"targets,omitempty"`
}

func parseEscalationLevels(levels []escalationLevelRequest) ([]store.EscalationLevelRecord, error) {
	out := make([]store.EscalationLevelRecord, 0, len(levels))
	seenLevels := make(map[int]struct{}, len(levels))
	for i, lv := range levels {
		if lv.LevelNumber <= 0 {
			return nil, fmt.Errorf("levels[%d].level_number must be positive", i)
		}
		if _, dup := seenLevels[lv.LevelNumber]; dup {
			return nil, fmt.Errorf("levels[%d].level_number %d is duplicated", i, lv.LevelNumber)
		}
		seenLevels[lv.LevelNumber] = struct{}{}
		if lv.DelayMinutes < 0 {
			return nil, fmt.Errorf("levels[%d].delay_minutes must be non-negative", i)
		}
		if len(lv.Targets) == 0 {
			return nil, fmt.Errorf("levels[%d] must have at least one target", i)
		}
		level := store.EscalationLevelRecord{
			LevelNumber:    lv.LevelNumber,
			DelayMinutes:   lv.DelayMinutes,
			NotifyChannels: lv.NotifyChannels,
		}
		for j, t := range lv.Targets {
			if _, ok := allowedEscalationTargetTypes[t.TargetType]; !ok {
				return nil, fmt.Errorf("levels[%d].targets[%d].target_type %q is not allowed", i, j, t.TargetType)
			}
			target := store.EscalationTargetRecord{
				TargetType: t.TargetType,
			}
			if t.TargetUserID != "" {
				if uid, err := uuid.Parse(t.TargetUserID); err == nil {
					target.TargetUserID = &uid
				} else {
					return nil, fmt.Errorf("levels[%d].targets[%d].target_user_id is not a valid UUID", i, j)
				}
			}
			if t.TargetTeamID != "" {
				if uid, err := uuid.Parse(t.TargetTeamID); err == nil {
					target.TargetTeamID = &uid
				} else {
					return nil, fmt.Errorf("levels[%d].targets[%d].target_team_id is not a valid UUID", i, j)
				}
			}
			if target.TargetUserID == nil && target.TargetTeamID == nil {
				return nil, fmt.Errorf("levels[%d].targets[%d] must include target_user_id or target_team_id", i, j)
			}
			level.Targets = append(level.Targets, target)
		}
		out = append(out, level)
	}
	return out, nil
}

func (s *Server) handleEscalationPolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListEscalationPolicies(w, r)
	case http.MethodPost:
		s.handleCreateEscalationPolicy(w, r)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleListEscalationPolicies(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.EscalationRead) {
		return
	}
	if !s.requireEscalationStore(w) {
		return
	}

	limit, skip := parseLimitSkip(r, 50)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cacheKey := fmt.Sprintf("%s:%d:%d", valkey.PrefixEscalationPolicies, limit, skip)
	policiesJSON, err := s.cache.GetOrSet(ctx, cacheKey, valkey.TTLEscalationPolicies, func(ctx context.Context) ([]byte, error) {
		records, total, err := s.escalationStore.ListPolicies(ctx, int(limit), int(skip))
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{
			"items": ensureSlice(records),
			"total": int64(total),
		})
	})
	if err != nil {
		writeInternalError(w, err, "failed to list escalation policies")
		return
	}

	writeRawJSON(w, http.StatusOK, policiesJSON)
}

func (s *Server) handleCreateEscalationPolicy(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.EscalationWrite) {
		return
	}
	if !s.requireEscalationStore(w) {
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		RepeatCount int    `json:"repeat_count,omitempty"`
		Levels      []escalationLevelRequest
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "name is required")
		return
	}
	if req.RepeatCount < 0 {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "repeat_count must be non-negative")
		return
	}

	levels, err := parseEscalationLevels(req.Levels)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
		return
	}
	if len(levels) == 0 {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "at least one level is required")
		return
	}
	if err := s.validateEscalationTargetsExist(r.Context(), levels); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
		return
	}

	record := &store.EscalationPolicyRecord{
		Name:        strings.TrimSpace(req.Name),
		Description: req.Description,
		RepeatCount: req.RepeatCount,
		Levels:      levels,
	}

	created, err := s.escalationStore.CreatePolicy(r.Context(), record)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to create escalation policy", "component", "api", "error", err)
		writeInternalError(w, err, "failed to create escalation policy")
		return
	}

	logger.InfoCtx(r.Context(), "escalation policy created", "component", "api", "policy_id", created.ID.String(), "name", req.Name)
	s.audit(r, store.AuditEscalationPolicyCreated, map[string]any{
		"policy_id": created.ID.String(),
		"name":      req.Name,
	})
	if s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{Type: "escalation_policy_created", Data: created})
	}
	s.invalidateEscalationCache(r)
	writeData(w, http.StatusCreated, created)
}

func (s *Server) handleEscalationPolicyRoutes(w http.ResponseWriter, r *http.Request) {
	suffix := pathID(r, "/api/v1/escalation-policies/")
	if suffix == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing policy id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetEscalationPolicy(w, r, suffix)
	case http.MethodPatch:
		s.handlePatchEscalationPolicy(w, r, suffix)
	case http.MethodDelete:
		s.handleDeleteEscalationPolicy(w, r, suffix)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) getPolicyOrError(w http.ResponseWriter, r *http.Request, id string) (*store.EscalationPolicyRecord, bool) {
	uid, err := uuid.Parse(id)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid policy id")
		return nil, false
	}
	record, err := s.escalationStore.GetPolicy(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to get escalation policy")
		return nil, false
	}
	if record == nil {
		writeError(w, ErrorCodeNotFound, "escalation policy not found")
		return nil, false
	}
	return record, true
}

func (s *Server) handleGetEscalationPolicy(w http.ResponseWriter, r *http.Request, id string) {
	if !s.checkPermission(w, r, rbac.EscalationRead) {
		return
	}
	if !s.requireEscalationStore(w) {
		return
	}
	record, ok := s.getPolicyOrError(w, r, id)
	if !ok {
		return
	}
	writeData(w, http.StatusOK, record)
}

func (s *Server) handlePatchEscalationPolicy(w http.ResponseWriter, r *http.Request, id string) {
	if !s.checkPermission(w, r, rbac.EscalationWrite) {
		return
	}
	if !s.requireEscalationStore(w) {
		return
	}

	uid, err := uuid.Parse(id)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid policy id")
		return
	}

	current, err := s.escalationStore.GetPolicy(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to get escalation policy")
		return
	}
	if current == nil {
		writeError(w, ErrorCodeNotFound, "escalation policy not found")
		return
	}

	var req struct {
		Name        *string                  `json:"name"`
		Description *string                  `json:"description"`
		RepeatCount *int                     `json:"repeat_count"`
		Levels      []escalationLevelRequest `json:"levels"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "name must not be empty")
			return
		}
		current.Name = *req.Name
	}
	if req.Description != nil {
		current.Description = *req.Description
	}
	if req.RepeatCount != nil {
		if *req.RepeatCount < 0 {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "repeat_count must be non-negative")
			return
		}
		current.RepeatCount = *req.RepeatCount
	}
	if req.Levels != nil {
		levels, perr := parseEscalationLevels(req.Levels)
		if perr != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, perr.Error())
			return
		}
		if len(levels) == 0 {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "at least one level is required")
			return
		}
		if terr := s.validateEscalationTargetsExist(r.Context(), levels); terr != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, terr.Error())
			return
		}
		current.Levels = levels
	}

	updated, err := s.escalationStore.UpdatePolicy(r.Context(), uid, current)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to update escalation policy", "component", "api", "policy_id", id, "error", err)
		writeInternalError(w, err, "failed to update escalation policy")
		return
	}

	logger.InfoCtx(r.Context(), "escalation policy updated", "component", "api", "policy_id", id)
	s.audit(r, store.AuditEscalationPolicyUpdated, map[string]any{
		"policy_id": id,
	})
	if s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{Type: "escalation_policy_updated", Data: updated})
	}
	s.invalidateEscalationCache(r)
	writeData(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteEscalationPolicy(w http.ResponseWriter, r *http.Request, id string) {
	if !s.checkPermission(w, r, rbac.EscalationWrite) {
		return
	}
	if !s.requireEscalationStore(w) {
		return
	}

	uid, err := uuid.Parse(id)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid policy id")
		return
	}

	if err := s.escalationStore.DeletePolicy(r.Context(), uid); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, ErrorCodeNotFound, "escalation policy not found")
			return
		}
		writeInternalError(w, err, "failed to delete escalation policy")
		return
	}

	logger.InfoCtx(r.Context(), "escalation policy deleted", "component", "api", "policy_id", id)
	s.audit(r, store.AuditEscalationPolicyDeleted, map[string]any{
		"policy_id": id,
	})
	if s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{Type: "escalation_policy_deleted", Data: map[string]string{"policy_id": id}})
	}
	s.invalidateEscalationCache(r)
	writeStatus(w, "deleted")
}

func (s *Server) invalidateEscalationCache(r *http.Request) {
	if s.cache != nil {
		_ = s.cache.InvalidatePrefix(r.Context(), valkey.PrefixEscalationPolicies)
	}
}

// validateEscalationTargetsExist walks the level targets and confirms that
// every referenced user / team / schedule row exists. A miss returns a 400
// so the operator fixes the form rather than silently saving a policy that
// will fail to dispatch at the next incident.
func (s *Server) validateEscalationTargetsExist(ctx context.Context, levels []store.EscalationLevelRecord) error {
	if len(levels) == 0 {
		return nil
	}
	for i, lvl := range levels {
		for j, tgt := range lvl.Targets {
			switch tgt.TargetType {
			case "user":
				if tgt.TargetUserID == nil {
					continue
				}
				if s.userStore == nil {
					continue
				}
				if _, err := s.userStore.GetByID(*tgt.TargetUserID); err != nil {
					return fmt.Errorf("levels[%d].targets[%d]: user %s not found", i, j, tgt.TargetUserID)
				}
			case "team":
				if tgt.TargetTeamID == nil {
					continue
				}
				if s.teamStore == nil {
					continue
				}
				if _, err := s.teamStore.GetTeam(ctx, *tgt.TargetTeamID); err != nil {
					return fmt.Errorf("levels[%d].targets[%d]: team %s not found", i, j, tgt.TargetTeamID)
				}
			}
		}
	}
	return nil
}
