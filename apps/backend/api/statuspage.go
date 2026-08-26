package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/logger"
	"alga/rbac"
	"alga/store"
)

var (
	validStatusPageSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}[a-z0-9]$`)
	validComponentStat  = map[string]bool{
		store.StatusComponentOperational:   true,
		store.StatusComponentDegraded:      true,
		store.StatusComponentPartialOutage: true,
		store.StatusComponentMajorOutage:   true,
		store.StatusComponentMaintenance:   true,
	}
	validPageVisibilities = map[string]bool{
		store.StatusPageVisibilityInternal: true,
		store.StatusPageVisibilityPublic:   true,
	}
)

// componentStatusRank orders component statuses from least to most severe so the
// page-level "overall status" can be derived as the worst across components.
var componentStatusRank = map[string]int{
	store.StatusComponentOperational:   0,
	store.StatusComponentMaintenance:   1,
	store.StatusComponentDegraded:      2,
	store.StatusComponentPartialOutage: 3,
	store.StatusComponentMajorOutage:   4,
}

func overallStatus(components []store.StatusPageComponentRecord) string {
	worst := store.StatusComponentOperational
	worstRank := -1
	for _, c := range components {
		r, ok := componentStatusRank[c.Status]
		if !ok {
			r = 0
		}
		if r > worstRank {
			worstRank = r
			worst = c.Status
		}
	}
	if worstRank < 0 {
		return store.StatusComponentOperational
	}
	return worst
}

type statusPageRequest struct {
	Name        string `json:"name,omitempty"`
	Slug        string `json:"slug,omitempty"`
	Description string `json:"description,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
	OwnerTeamID string `json:"owner_team_id,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

type statusPageComponentRequest struct {
	Name         string `json:"name,omitempty"`
	Description  string `json:"description,omitempty"`
	ServiceID    string `json:"service_id,omitempty"`
	DisplayOrder *int   `json:"display_order,omitempty"`
	Status       string `json:"status,omitempty"`
}

func (s *Server) handleListStatusPages(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.statusPageStore, "status page store") {
		return
	}
	q := store.StatusPageQuery{}
	if v := r.URL.Query().Get("enabled"); v != "" {
		enabled := strings.EqualFold(v, "true") || v == "1"
		q.Enabled = &enabled
	}
	if v := r.URL.Query().Get("search"); v != "" {
		q.Search = v
	}
	limit, skip := parseLimitSkip(r, 50)
	q.Limit = int(limit)
	q.Skip = int(skip)
	items, total, err := s.statusPageStore.ListPages(r.Context(), q)
	if err != nil {
		writeInternalError(w, err, "failed to list status pages")
		return
	}
	writePaginatedJSON(w, ensureSlice(items), total)
}

func (s *Server) handleCreateStatusPage(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.statusPageStore, "status page store") {
		return
	}
	var req statusPageRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	slug := strings.ToLower(strings.TrimSpace(req.Slug))
	if name == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "name is required")
		return
	}
	if !validStatusPageSlug.MatchString(slug) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "slug must be 2-64 chars: lowercase letters, digits, and hyphens (no leading/trailing hyphen)")
		return
	}
	visibility := strings.TrimSpace(req.Visibility)
	if visibility == "" {
		visibility = store.StatusPageVisibilityInternal
	}
	if !validPageVisibilities[visibility] {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid visibility (expected internal or public)")
		return
	}

	record := &store.StatusPageRecord{
		Name:        name,
		Slug:        slug,
		Description: strings.TrimSpace(req.Description),
		Visibility:  visibility,
		Enabled:     true,
	}
	if req.Enabled != nil {
		record.Enabled = *req.Enabled
	}
	if req.OwnerTeamID != "" {
		uid, err := uuid.Parse(req.OwnerTeamID)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid owner_team_id")
			return
		}
		record.OwnerTeamID = &uid
	}

	out, err := s.statusPageStore.CreatePage(r.Context(), record)
	if err != nil {
		if store.IsDuplicateKey(err) {
			writeError(w, ErrorCodeConflict, "a status page with that slug already exists")
			return
		}
		writeInternalError(w, err, "failed to create status page")
		return
	}
	logger.InfoCtx(r.Context(), "status page created", "component", "api", "status_page_id", out.ID.String(), "slug", out.Slug)
	s.audit(r, store.AuditStatusPageCreated, map[string]any{
		"status_page_id": out.ID.String(),
		"slug":           out.Slug,
	})
	writeData(w, http.StatusCreated, out)
}

// View-models for the slug view (WP-B1). Field allow-list is exactly what a
// public renderer could show, so an unauthenticated route can later become a
// thin wrapper over this serializer without re-litigating the payload.

type statusPageViewPage struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
}

type statusPageViewComponent struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Status       string `json:"status"`
	DisplayOrder int    `json:"display_order"`
}

type statusPageViewIncident struct {
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	Severity  string     `json:"severity"`
	StartedAt *time.Time `json:"started_at,omitempty"`
}

type statusPageViewResponse struct {
	Page          statusPageViewPage        `json:"page"`
	OverallStatus string                    `json:"overall_status"`
	Components    []statusPageViewComponent `json:"components"`
	Incidents     []statusPageViewIncident  `json:"incidents"`
}

// handleStatusPageViewBySlug returns the page payload: the page name/slug/
// description, its components (ordered), the derived overall status, and
// active incidents scoped to the services the page's components map to.
//
// Hardening per WP-B1: disabled pages 404 uniformly with missing slugs; the
// payload is an allow-listed view model — no internal ids, owner team,
// Slack/war-room linkage, SLA or responder fields. The route stays behind
// authMiddleware(rbac.StatusPagesRead); this shape doubles as the contract
// for any future public renderer (spec S3).
func (s *Server) handleStatusPageViewBySlug(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.statusPageStore, "status page store") {
		return
	}
	slug := strings.ToLower(strings.TrimSpace(r.PathValue("slug")))
	if slug == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing slug")
		return
	}
	page, err := s.statusPageStore.GetPageBySlug(r.Context(), slug)
	if err != nil {
		writeInternalError(w, err, "failed to get status page")
		return
	}
	if page == nil || !page.Enabled {
		// Uniform 404: never reveal that a disabled page exists. Management
		// continues via /status-pages/{id} CRUD.
		writeError(w, ErrorCodeNotFound, "status page not found")
		return
	}
	components, err := s.statusPageStore.ListComponents(r.Context(), page.ID)
	if err != nil {
		writeInternalError(w, err, "failed to list status page components")
		return
	}
	if components == nil {
		components = []store.StatusPageComponentRecord{}
	}

	viewComponents := make([]statusPageViewComponent, 0, len(components))
	serviceIDs := make([]uuid.UUID, 0, len(components))
	for _, c := range components {
		viewComponents = append(viewComponents, statusPageViewComponent{
			Name:         c.Name,
			Description:  c.Description,
			Status:       c.Status,
			DisplayOrder: c.DisplayOrder,
		})
		if c.ServiceID != nil {
			serviceIDs = append(serviceIDs, *c.ServiceID)
		}
	}

	incidents := []statusPageViewIncident{}
	if s.incidentStore != nil && len(serviceIDs) > 0 {
		active, err := s.incidentStore.ListActiveIncidentsForServices(r.Context(), serviceIDs)
		if err != nil {
			logger.Warn("status page view: failed to list active incidents", "component", "api", "error", err)
		} else {
			incidents = make([]statusPageViewIncident, 0, len(active))
			for i := range active {
				inc := &active[i]
				incidents = append(incidents, statusPageViewIncident{
					Title:     inc.Title,
					Status:    inc.Status,
					Severity:  inc.Severity,
					StartedAt: inc.StartedAt,
				})
			}
		}
	}
	writeData(w, http.StatusOK, statusPageViewResponse{
		Page: statusPageViewPage{
			Name:        page.Name,
			Slug:        page.Slug,
			Description: page.Description,
		},
		OverallStatus: overallStatus(components),
		Components:    viewComponents,
		Incidents:     incidents,
	})
}

// handleStatusPageRoutes dispatches /api/v1/status-pages/{id},
// /api/v1/status-pages/{id}/components, and
// /api/v1/status-pages/{id}/components/{component_id}.
func (s *Server) handleStatusPageRoutes(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.statusPageStore, "status page store") {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/status-pages/")
	rest = strings.TrimSuffix(rest, "/")
	if rest == "" {
		writeError(w, ErrorCodeNotFound, "not found")
		return
	}
	parts := strings.SplitN(rest, "/", 3)
	pageID, err := uuid.Parse(parts[0])
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid status page id")
		return
	}

	if len(parts) >= 2 && parts[1] == "components" {
		if len(parts) == 2 {
			s.handleStatusPageComponentsCollection(w, r, pageID)
			return
		}
		componentID, err := uuid.Parse(parts[2])
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid component id")
			return
		}
		s.handleStatusPageComponentItem(w, r, pageID, componentID)
		return
	}
	if len(parts) >= 2 {
		writeError(w, ErrorCodeNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !s.checkPermission(w, r, rbac.StatusPagesRead) {
			return
		}
		rec, err := s.statusPageStore.GetPage(r.Context(), pageID)
		if err != nil {
			writeInternalError(w, err, "failed to get status page")
			return
		}
		if rec == nil {
			writeError(w, ErrorCodeNotFound, "status page not found")
			return
		}
		writeData(w, http.StatusOK, rec)
	case http.MethodPut:
		if !s.checkPermission(w, r, rbac.StatusPagesWrite) {
			return
		}
		s.updateStatusPage(w, r, pageID)
	case http.MethodDelete:
		if !s.checkPermission(w, r, rbac.StatusPagesDelete) {
			return
		}
		if err := s.statusPageStore.DeletePage(r.Context(), pageID); err != nil {
			if errors.Is(err, store.ErrStatusPageNotFound) || errors.Is(err, store.ErrStatusPageComponentNotFound) {
				writeError(w, ErrorCodeNotFound, "status page not found")
				return
			}
			writeInternalError(w, err, "failed to delete status page")
			return
		}
		logger.InfoCtx(r.Context(), "status page deleted", "component", "api", "status_page_id", pageID.String())
		s.audit(r, store.AuditStatusPageDeleted, map[string]any{"status_page_id": pageID.String()})
		writeStatus(w, "deleted")
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) updateStatusPage(w http.ResponseWriter, r *http.Request, pageID uuid.UUID) {
	var req statusPageRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	visibility := strings.TrimSpace(req.Visibility)
	if visibility != "" && !validPageVisibilities[visibility] {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid visibility (expected internal or public)")
		return
	}
	slug := strings.ToLower(strings.TrimSpace(req.Slug))
	if slug != "" && !validStatusPageSlug.MatchString(slug) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid slug format")
		return
	}

	patch := &store.StatusPageRecord{
		Name:        strings.TrimSpace(req.Name),
		Slug:        slug,
		Description: strings.TrimSpace(req.Description),
		Visibility:  visibility,
	}
	if req.OwnerTeamID != "" {
		uid, err := uuid.Parse(req.OwnerTeamID)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid owner_team_id")
			return
		}
		patch.OwnerTeamID = &uid
	}
	if req.Enabled != nil {
		patch.Enabled = *req.Enabled
		patch.EnabledSet = true
	}

	out, err := s.statusPageStore.UpdatePage(r.Context(), pageID, patch)
	if err != nil {
		if errors.Is(err, store.ErrStatusPageNotFound) || errors.Is(err, store.ErrStatusPageComponentNotFound) {
			writeError(w, ErrorCodeNotFound, "status page not found")
			return
		}
		if store.IsDuplicateKey(err) {
			writeError(w, ErrorCodeConflict, "a status page with that slug already exists")
			return
		}
		writeInternalError(w, err, "failed to update status page")
		return
	}
	s.audit(r, store.AuditStatusPageUpdated, map[string]any{
		"status_page_id": pageID.String(),
		"enabled":        out.Enabled,
	})
	writeData(w, http.StatusOK, out)
}

func (s *Server) handleStatusPageComponentsCollection(w http.ResponseWriter, r *http.Request, pageID uuid.UUID) {
	switch r.Method {
	case http.MethodGet:
		if !s.checkPermission(w, r, rbac.StatusPagesRead) {
			return
		}
		items, err := s.statusPageStore.ListComponents(r.Context(), pageID)
		if err != nil {
			writeInternalError(w, err, "failed to list status page components")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": ensureSlice(items)})
	case http.MethodPost:
		if !s.checkPermission(w, r, rbac.StatusPagesWrite) {
			return
		}
		s.createStatusPageComponent(w, r, pageID)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) createStatusPageComponent(w http.ResponseWriter, r *http.Request, pageID uuid.UUID) {
	var req statusPageComponentRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "name is required")
		return
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = store.StatusComponentOperational
	}
	if !validComponentStat[status] {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid status")
		return
	}

	record := &store.StatusPageComponentRecord{
		StatusPageID: pageID,
		Name:         name,
		Description:  strings.TrimSpace(req.Description),
		Status:       status,
	}
	if req.DisplayOrder != nil {
		record.DisplayOrder = *req.DisplayOrder
	}
	if req.ServiceID != "" {
		uid, err := uuid.Parse(req.ServiceID)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid service_id")
			return
		}
		record.ServiceID = &uid
	}

	out, err := s.statusPageStore.CreateComponent(r.Context(), record)
	if err != nil {
		writeInternalError(w, err, "failed to create status page component")
		return
	}
	s.audit(r, store.AuditStatusPageComponentCreated, map[string]any{
		"status_page_id": pageID.String(),
		"component_id":   out.ID.String(),
	})
	writeData(w, http.StatusCreated, out)
}

func (s *Server) handleStatusPageComponentItem(w http.ResponseWriter, r *http.Request, pageID, componentID uuid.UUID) {
	switch r.Method {
	case http.MethodGet:
		if !s.checkPermission(w, r, rbac.StatusPagesRead) {
			return
		}
		rec, err := s.statusPageStore.GetComponent(r.Context(), componentID)
		if err != nil {
			writeInternalError(w, err, "failed to get component")
			return
		}
		if rec == nil {
			writeError(w, ErrorCodeNotFound, "component not found")
			return
		}
		writeData(w, http.StatusOK, rec)
	case http.MethodPut:
		if !s.checkPermission(w, r, rbac.StatusPagesWrite) {
			return
		}
		s.updateStatusPageComponent(w, r, pageID, componentID)
	case http.MethodDelete:
		if !s.checkPermission(w, r, rbac.StatusPagesDelete) {
			return
		}
		if err := s.statusPageStore.DeleteComponent(r.Context(), componentID); err != nil {
			if errors.Is(err, store.ErrStatusPageNotFound) || errors.Is(err, store.ErrStatusPageComponentNotFound) {
				writeError(w, ErrorCodeNotFound, "component not found")
				return
			}
			writeInternalError(w, err, "failed to delete status page component")
			return
		}
		s.audit(r, store.AuditStatusPageComponentDeleted, map[string]any{
			"status_page_id": pageID.String(),
			"component_id":   componentID.String(),
		})
		writeStatus(w, "deleted")
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) updateStatusPageComponent(w http.ResponseWriter, r *http.Request, pageID, componentID uuid.UUID) {
	var req statusPageComponentRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	status := strings.TrimSpace(req.Status)
	if status != "" && !validComponentStat[status] {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid status")
		return
	}

	patch := &store.StatusPageComponentRecord{
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		Status:      status,
	}
	if req.DisplayOrder != nil {
		patch.DisplayOrder = *req.DisplayOrder
		patch.DisplayOrderSet = true
	}
	if req.ServiceID != "" {
		uid, err := uuid.Parse(req.ServiceID)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid service_id")
			return
		}
		patch.ServiceID = &uid
	}

	out, err := s.statusPageStore.UpdateComponent(r.Context(), componentID, patch)
	if err != nil {
		if errors.Is(err, store.ErrStatusPageNotFound) || errors.Is(err, store.ErrStatusPageComponentNotFound) {
			writeError(w, ErrorCodeNotFound, "component not found")
			return
		}
		writeInternalError(w, err, "failed to update status page component")
		return
	}
	s.audit(r, store.AuditStatusPageComponentUpdated, map[string]any{
		"status_page_id": pageID.String(),
		"component_id":   componentID.String(),
		"status":         out.Status,
	})
	writeData(w, http.StatusOK, out)
}
