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

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListServices(w, r)
	case http.MethodPost:
		s.handleCreateService(w, r)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.ServicesRead) {
		return
	}
	if !s.requireServiceStore(w) {
		return
	}

	limit, skip := parseLimitSkip(r, 100)
	filter := store.ListServicesFilter{
		Status: r.URL.Query().Get("status"),
		Query:  r.URL.Query().Get("q"),
		Limit:  int(limit),
		Skip:   int(skip),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cacheKey := fmt.Sprintf("%s:%s:%s:%d:%d", valkey.PrefixServicesList, filter.Status, filter.Query, filter.Limit, filter.Skip)
	servicesJSON, err := s.cache.GetOrSet(ctx, cacheKey, valkey.TTLServicesList, func(ctx context.Context) ([]byte, error) {
		records, total, err := s.serviceStore.ListServices(ctx, filter)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{
			"items": ensureSlice(records),
			"total": int64(total),
		})
	})
	if err != nil {
		writeInternalError(w, err, "failed to list services")
		return
	}

	writeRawJSON(w, http.StatusOK, servicesJSON)
}

func (s *Server) handleCreateService(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.ServicesWrite) {
		return
	}
	if !s.requireServiceStore(w) {
		return
	}

	var req struct {
		Name               string           `json:"name"`
		DisplayName        string           `json:"display_name,omitempty"`
		Description        string           `json:"description,omitempty"`
		OwnerTeamID        *string          `json:"owner_team_id,omitempty"`
		EscalationPolicyID *string          `json:"escalation_policy_id,omitempty"`
		LabelMatchers      []map[string]any `json:"label_matchers,omitempty"`
		SLAResponseMinutes int              `json:"sla_response_minutes,omitempty"`
		SLAResolveMinutes  int              `json:"sla_resolve_minutes,omitempty"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "name is required")
		return
	}

	record := &store.ServiceRecord{
		Name:               strings.TrimSpace(req.Name),
		DisplayName:        req.DisplayName,
		Description:        req.Description,
		LabelMatchers:      req.LabelMatchers,
		SLAResponseMinutes: req.SLAResponseMinutes,
		SLAResolveMinutes:  req.SLAResolveMinutes,
	}
	if req.OwnerTeamID != nil {
		if uid, err := uuid.Parse(*req.OwnerTeamID); err == nil {
			record.OwnerTeamID = &uid
		}
	}
	if req.EscalationPolicyID != nil {
		if uid, err := uuid.Parse(*req.EscalationPolicyID); err == nil {
			record.EscalationPolicyID = &uid
		}
	}

	created, err := s.serviceStore.CreateService(r.Context(), record)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to create service", "component", "api", "error", err)
		writeInternalError(w, err, "failed to create service")
		return
	}

	logger.InfoCtx(r.Context(), "service created", "component", "api", "service_id", created.ID.String(), "name", req.Name)
	s.audit(r, store.AuditServiceCreated, map[string]any{
		"service_id": created.ID.String(),
		"name":       req.Name,
	})
	s.invalidateServicesCache(r)
	writeData(w, http.StatusCreated, created)
}

func (s *Server) handleServiceRoutes(w http.ResponseWriter, r *http.Request) {
	suffix := pathID(r, "/api/v1/services/")
	if suffix == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing service id")
		return
	}

	if idx := strings.Index(suffix, "/dependencies/"); idx != -1 {
		if r.Method != http.MethodDelete {
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
			return
		}
		serviceID := suffix[:idx]
		targetID := suffix[idx+len("/dependencies/"):]
		s.handleRemoveServiceDependency(w, r, serviceID, targetID)
		return
	}

	if suffix == "dependents" || strings.HasSuffix(suffix, "/dependents") {
		serviceID := strings.TrimSuffix(suffix, "/dependents")
		if serviceID == "dependents" {
			serviceID = ""
		}
		if serviceID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing service id")
			return
		}
		s.handleListServiceDependents(w, r, serviceID)
		return
	}

	if suffix == "dependencies" || strings.HasSuffix(suffix, "/dependencies") {
		serviceID := strings.TrimSuffix(suffix, "/dependencies")
		if serviceID == "dependencies" {
			serviceID = ""
		}
		if serviceID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing service id")
			return
		}
		switch r.Method {
		case http.MethodGet:
			s.handleListServiceDependencies(w, r, serviceID)
		case http.MethodPost:
			s.handleAddServiceDependency(w, r, serviceID)
		default:
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		}
		return
	}

	if suffix == "incidents" || strings.HasSuffix(suffix, "/incidents") {
		serviceID := strings.TrimSuffix(suffix, "/incidents")
		if serviceID == "incidents" {
			serviceID = ""
		}
		if serviceID == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing service id")
			return
		}
		s.handleListServiceIncidents(w, r, serviceID)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetService(w, r, suffix)
	case http.MethodPatch:
		s.handlePatchService(w, r, suffix)
	case http.MethodDelete:
		s.handleDeleteService(w, r, suffix)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) getServiceOrError(w http.ResponseWriter, r *http.Request, id string) (*store.ServiceRecord, bool) {
	record, err := s.serviceStore.GetService(r.Context(), id)
	if err != nil {
		writeInternalError(w, err, "failed to get service")
		return nil, false
	}
	if record == nil {
		writeError(w, ErrorCodeNotFound, "service not found")
		return nil, false
	}
	return record, true
}

func (s *Server) handleGetService(w http.ResponseWriter, r *http.Request, id string) {
	if !s.checkPermission(w, r, rbac.ServicesRead) {
		return
	}
	if !s.requireServiceStore(w) {
		return
	}
	record, ok := s.getServiceOrError(w, r, id)
	if !ok {
		return
	}

	uid, _ := uuid.Parse(id)
	if deps, err := s.serviceStore.GetDependencies(r.Context(), uid); err == nil {
		record.Dependencies = deps
	}
	if dependents, err := s.serviceStore.GetDependents(r.Context(), uid); err == nil {
		record.Dependents = dependents
	}
	if s.incidentStore != nil {
		if count, err := s.incidentStore.CountActiveByServiceID(r.Context(), id); err == nil {
			record.ActiveIncidentCount = count
		}
	}
	writeData(w, http.StatusOK, record)
}

func (s *Server) handlePatchService(w http.ResponseWriter, r *http.Request, id string) {
	if !s.checkPermission(w, r, rbac.ServicesWrite) {
		return
	}
	if !s.requireServiceStore(w) {
		return
	}

	var req struct {
		Name               *string          `json:"name"`
		DisplayName        *string          `json:"display_name"`
		Description        *string          `json:"description"`
		OwnerTeamID        *string          `json:"owner_team_id"`
		EscalationPolicyID *string          `json:"escalation_policy_id"`
		LabelMatchers      []map[string]any `json:"label_matchers"`
		SLAResponseMinutes *int             `json:"sla_response_minutes"`
		SLAResolveMinutes  *int             `json:"sla_resolve_minutes"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	current, ok := s.getServiceOrError(w, r, id)
	if !ok {
		return
	}

	if req.Name != nil {
		current.Name = *req.Name
	}
	if req.DisplayName != nil {
		current.DisplayName = *req.DisplayName
	}
	if req.Description != nil {
		current.Description = *req.Description
	}
	if req.OwnerTeamID != nil {
		if uid, err := uuid.Parse(*req.OwnerTeamID); err == nil {
			current.OwnerTeamID = &uid
		} else {
			current.OwnerTeamID = nil
		}
	}
	if req.EscalationPolicyID != nil {
		if uid, err := uuid.Parse(*req.EscalationPolicyID); err == nil {
			current.EscalationPolicyID = &uid
		} else {
			current.EscalationPolicyID = nil
		}
	}
	if req.LabelMatchers != nil {
		current.LabelMatchers = req.LabelMatchers
	}
	if req.SLAResponseMinutes != nil {
		current.SLAResponseMinutes = *req.SLAResponseMinutes
	}
	if req.SLAResolveMinutes != nil {
		current.SLAResolveMinutes = *req.SLAResolveMinutes
	}

	updated, err := s.serviceStore.UpdateService(r.Context(), id, current)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to update service", "component", "api", "service_id", id, "error", err)
		writeInternalError(w, err, "failed to update service")
		return
	}

	logger.InfoCtx(r.Context(), "service updated", "component", "api", "service_id", id)
	s.audit(r, store.AuditServiceUpdated, map[string]any{
		"service_id": id,
	})
	if s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{Type: "service_updated", Data: updated})
	}
	s.invalidateServicesCache(r)
	writeData(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteService(w http.ResponseWriter, r *http.Request, id string) {
	if !s.checkPermission(w, r, rbac.ServicesWrite) {
		return
	}
	if !s.requireServiceStore(w) {
		return
	}

	if err := s.serviceStore.DeleteService(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, ErrorCodeNotFound, "service not found")
			return
		}
		writeInternalError(w, err, "failed to delete service")
		return
	}

	logger.InfoCtx(r.Context(), "service deleted", "component", "api", "service_id", id)
	s.audit(r, store.AuditServiceDeleted, map[string]any{
		"service_id": id,
	})
	if s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{Type: "service_deleted", Data: map[string]string{"service_id": id}})
	}
	s.invalidateServicesCache(r)
	writeStatus(w, "deleted")
}

func (s *Server) handleListServiceDependencies(w http.ResponseWriter, r *http.Request, serviceID string) {
	if !s.checkPermission(w, r, rbac.ServicesRead) {
		return
	}
	if !s.requireServiceStore(w) {
		return
	}

	uid, err := uuid.Parse(serviceID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid service id")
		return
	}

	deps, err := s.serviceStore.GetDependencies(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to list dependencies")
		return
	}
	writeData(w, http.StatusOK, ensureSlice(deps))
}

func (s *Server) handleAddServiceDependency(w http.ResponseWriter, r *http.Request, serviceID string) {
	if !s.checkPermission(w, r, rbac.ServicesWrite) {
		return
	}
	if !s.requireServiceStore(w) {
		return
	}

	var req struct {
		DependentOnID string `json:"dependent_on_service_id"`
		DepType       string `json:"dependency_type"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.DependentOnID == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "dependent_on_service_id is required")
		return
	}

	svcUID, err := uuid.Parse(serviceID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid service id")
		return
	}
	depUID, err := uuid.Parse(req.DependentOnID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid dependent_on_service_id")
		return
	}

	depType := req.DepType
	if depType == "" {
		depType = "depends_on"
	}

	if svcUID == depUID {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "cannot add self-dependency")
		return
	}

	hasCycle, err := s.serviceStore.HasCircularDependency(r.Context(), svcUID, depUID)
	if err != nil {
		writeInternalError(w, err, "failed to check circular dependency")
		return
	}
	if hasCycle {
		writeError(w, ErrorCodeConflict, "circular dependency detected")
		return
	}

	if err := s.serviceStore.AddDependency(r.Context(), svcUID, depUID, depType); err != nil {
		writeInternalError(w, err, "failed to add dependency")
		return
	}

	s.audit(r, store.AuditServiceUpdated, map[string]any{
		"service_id":              serviceID,
		"dependent_on_service_id": req.DependentOnID,
		"action":                  "add_dependency",
	})
	s.invalidateServicesCache(r)
	writeStatus(w, "added")
}

func (s *Server) handleRemoveServiceDependency(w http.ResponseWriter, r *http.Request, serviceID, targetID string) {
	if !s.checkPermission(w, r, rbac.ServicesWrite) {
		return
	}
	if !s.requireServiceStore(w) {
		return
	}

	svcUID, err := uuid.Parse(serviceID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid service id")
		return
	}
	targetUID, err := uuid.Parse(targetID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid target service id")
		return
	}

	if err := s.serviceStore.RemoveDependency(r.Context(), svcUID, targetUID); err != nil {
		writeInternalError(w, err, "failed to remove dependency")
		return
	}

	s.audit(r, store.AuditServiceUpdated, map[string]any{
		"service_id":              serviceID,
		"dependent_on_service_id": targetID,
		"action":                  "remove_dependency",
	})
	s.invalidateServicesCache(r)
	writeStatus(w, "removed")
}

func (s *Server) handleListServiceIncidents(w http.ResponseWriter, r *http.Request, serviceID string) {
	if !s.checkPermission(w, r, rbac.IncidentsRead) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}

	uid, err := uuid.Parse(serviceID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid service id")
		return
	}

	limit, skip := parseLimitSkip(r, 20)
	filter := store.IncidentListFilter{
		ServiceID: uid.String(),
		Limit:     int(limit),
		Skip:      int(skip),
	}

	records, _, err := s.incidentStore.ListIncidents(r.Context(), filter)
	if err != nil {
		writeInternalError(w, err, "failed to list service incidents")
		return
	}
	writeData(w, http.StatusOK, ensureSlice(records))
}

func (s *Server) handleListServiceDependents(w http.ResponseWriter, r *http.Request, serviceID string) {
	if !s.checkPermission(w, r, rbac.ServicesRead) {
		return
	}
	if !s.requireServiceStore(w) {
		return
	}

	uid, err := uuid.Parse(serviceID)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid service id")
		return
	}

	dependents, err := s.serviceStore.GetDependents(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to list dependents")
		return
	}
	writeData(w, http.StatusOK, ensureSlice(dependents))
}

func (s *Server) invalidateServicesCache(r *http.Request) {
	if s.cache != nil {
		_ = s.cache.InvalidatePrefix(r.Context(), valkey.PrefixServicesList)
		_ = s.cache.Invalidate(r.Context(), valkey.PrefixDashboardStats)
	}
}
