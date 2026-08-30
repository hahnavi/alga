package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/api/platform"
	"alga/config"
	"alga/store"
)

// stubPMIncidentStore satisfies the incident reads and timeline writes the
// post-mortem handlers perform; everything else panics via the nil embedded
// interface.
type stubPMIncidentStore struct {
	store.IncidentStore
	inc      *store.IncidentRecord
	timeline []string
}

func (s *stubPMIncidentStore) GetIncident(_ context.Context, number int64) (*store.IncidentRecord, error) {
	if s.inc != nil && s.inc.IncidentNumber == number {
		return s.inc, nil
	}
	return nil, nil
}

func (s *stubPMIncidentStore) GetIncidentByID(_ context.Context, id uuid.UUID) (*store.IncidentRecord, error) {
	if s.inc != nil && s.inc.ID == id {
		return s.inc, nil
	}
	return nil, nil
}

func (s *stubPMIncidentStore) AddTimelineEntry(_ context.Context, rec *store.IncidentTimelineEntryRecord) error {
	s.timeline = append(s.timeline, rec.EventType)
	return nil
}

// stubPMStore backs the post-mortem handlers; it returns a fixed record for
// GetByIncidentID and records Update/Delete calls.
type stubPMStore struct {
	store.PostMortemStore
	pm       *store.PostMortemRecord
	updated  *store.PostMortemRecord
	deleted  bool
	getCalls int
}

func (s *stubPMStore) GetByIncidentID(_ context.Context, _ uuid.UUID) (*store.PostMortemRecord, error) {
	s.getCalls++
	return s.pm, nil
}

func (s *stubPMStore) Create(_ context.Context, record *store.PostMortemRecord) (*store.PostMortemRecord, error) {
	record.ID = uuid.New()
	s.pm = record
	return record, nil
}

func (s *stubPMStore) ExistsByIncidentID(_ context.Context, _ uuid.UUID) (bool, error) {
	return s.pm != nil, nil
}

func (s *stubPMStore) Update(_ context.Context, _ uuid.UUID, record *store.PostMortemRecord) (*store.PostMortemRecord, error) {
	s.updated = record
	return record, nil
}

func (s *stubPMStore) UpdateStatus(_ context.Context, _ uuid.UUID, status string, _ *uuid.UUID) (*store.PostMortemRecord, error) {
	updated := *s.pm
	updated.Status = status
	return &updated, nil
}

func (s *stubPMStore) Delete(_ context.Context, _ uuid.UUID) error {
	s.deleted = true
	return nil
}

// stubPMActionItemStore captures action item mutations.
type stubPMActionItemStore struct {
	store.ActionItemStore
	created *store.ActionItemRecord
	updated *store.ActionItemRecord
	deleted bool
}

func (s *stubPMActionItemStore) Create(_ context.Context, record *store.ActionItemRecord) (*store.ActionItemRecord, error) {
	s.created = record
	record.ID = uuid.New()
	return record, nil
}

func (s *stubPMActionItemStore) GetByID(_ context.Context, _ uuid.UUID) (*store.ActionItemRecord, error) {
	return nil, nil
}

func (s *stubPMActionItemStore) ListByPostMortem(context.Context, uuid.UUID) ([]store.ActionItemRecord, error) {
	return nil, nil
}

func (s *stubPMActionItemStore) ListByPostMortemIDs(context.Context, []uuid.UUID) (map[uuid.UUID][]store.ActionItemRecord, error) {
	return nil, nil
}

func (s *stubPMActionItemStore) ListOpen(context.Context) ([]store.ActionItemRecord, error) {
	return nil, nil
}

func (s *stubPMActionItemStore) ListOverdue(context.Context) ([]store.ActionItemRecord, error) {
	return nil, nil
}

func (s *stubPMActionItemStore) Update(_ context.Context, _ uuid.UUID, record *store.ActionItemRecord) (*store.ActionItemRecord, error) {
	s.updated = record
	return record, nil
}

func (s *stubPMActionItemStore) Delete(_ context.Context, _ uuid.UUID) error {
	s.deleted = true
	return nil
}

func (s *stubPMActionItemStore) DeleteByPostMortemID(context.Context, uuid.UUID) error {
	return nil
}

// stubPMUserStore validates assignee ids: any id equal to validAssignee passes,
// anything else is "not found".
type stubPMUserStore struct {
	store.UserStore
	validAssignee uuid.UUID
}

func (s *stubPMUserStore) GetByID(id uuid.UUID) (*store.UserRecord, error) {
	if s.validAssignee != uuid.Nil && id == s.validAssignee {
		return &store.UserRecord{ID: id}, nil
	}
	return nil, nil
}

func newPMTestServer(inc *store.IncidentRecord, pm *store.PostMortemRecord) (*Server, *stubPMStore, *stubPMIncidentStore) {
	s := &Server{
		cfg:             &config.Config{},
		incidentStore:   &stubPMIncidentStore{inc: inc},
		postmortemStore: &stubPMStore{pm: pm},
		actionItemStore: &stubPMActionItemStore{},
		auditStore:      &recordingAuditStore{},
		ipExtractor:     newIPExtractor(&config.Config{}),
	}
	return s, s.postmortemStore.(*stubPMStore), s.incidentStore.(*stubPMIncidentStore)
}

func pmRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	return req.WithContext(platform.WithUser(req.Context(), &store.UserRecord{Role: "admin"}))
}

// A post-mortem whose author never confirmed the blameless gate cannot enter
// review, even though the status transition itself is legal.
func TestSubmitReviewRequiresBlamelessConfirmation(t *testing.T) {
	t.Parallel()

	inc := &store.IncidentRecord{ID: uuid.New(), IncidentNumber: 7}
	pm := &store.PostMortemRecord{ID: uuid.New(), IncidentID: inc.ID, Status: "draft", BlamelessConfirmed: false}
	s, st, _ := newPMTestServer(inc, pm)

	w := httptest.NewRecorder()
	s.handlePostMortemRoutes(w, pmRequest(http.MethodPost, "/api/v1/incidents/7/post-mortem/submit-review", `{}`), "7")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
	if st.updated != nil {
		t.Fatal("UpdateStatus must not run when the blameless gate fails")
	}

	// Confirming blameless unlocks the same transition.
	st.pm.BlamelessConfirmed = true
	w = httptest.NewRecorder()
	s.handlePostMortemRoutes(w, pmRequest(http.MethodPost, "/api/v1/incidents/7/post-mortem/submit-review", `{}`), "7")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 after confirmation (body=%s)", w.Code, w.Body.String())
	}
}

// Published post-mortems are immutable: PATCH returns 409 and never reaches
// the store.
func TestPublishedPostMortemPatchRejected(t *testing.T) {
	t.Parallel()

	inc := &store.IncidentRecord{ID: uuid.New(), IncidentNumber: 9}
	pm := &store.PostMortemRecord{ID: uuid.New(), IncidentID: inc.ID, Status: "published"}
	s, st, _ := newPMTestServer(inc, pm)

	w := httptest.NewRecorder()
	s.handlePostMortemRoutes(w, pmRequest(http.MethodPatch, "/api/v1/incidents/9/post-mortem", `{"title":"rewrite"}`), "9")

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body=%s)", w.Code, w.Body.String())
	}
	if st.updated != nil {
		t.Fatal("Update must not run for a published post-mortem")
	}
}

// UUID-addressed requests must produce the same side effects as number-
// addressed ones: the timeline entry is recorded (previously it was silently
// dropped because the UUID failed to parse as an incident number).
func TestUUIDAddressedCreateAddsTimeline(t *testing.T) {
	t.Parallel()

	inc := &store.IncidentRecord{ID: uuid.New(), IncidentNumber: 12}
	s, _, incSt := newPMTestServer(inc, nil)

	path := "/api/v1/incidents/" + inc.ID.String() + "/post-mortem"
	w := httptest.NewRecorder()
	s.handlePostMortemRoutes(w, pmRequest(http.MethodPost, path, `{}`), inc.ID.String())

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%s)", w.Code, w.Body.String())
	}
	if len(incSt.timeline) == 0 || incSt.timeline[0] != "postmortem_created" {
		t.Fatalf("timeline events = %v, want postmortem_created", incSt.timeline)
	}
}

// Date-only due dates (what the DatePicker sends) must be accepted, not
// silently dropped; invalid values must 400 rather than persist NULL.
func TestCreateActionItemDueDateFormats(t *testing.T) {
	t.Parallel()

	inc := &store.IncidentRecord{ID: uuid.New(), IncidentNumber: 3}
	s, _, _ := newPMTestServer(inc, &store.PostMortemRecord{ID: uuid.New(), IncidentID: inc.ID, Status: "draft"})
	aiSt := s.actionItemStore.(*stubPMActionItemStore)

	// Date-only accepted, normalized to end of day UTC.
	w := httptest.NewRecorder()
	s.handlePostMortemRoutes(w, pmRequest(http.MethodPost, "/api/v1/incidents/3/post-mortem/action-items", `{"description":"patch","due_date":"2026-09-05"}`), "3")
	if w.Code != http.StatusCreated {
		t.Fatalf("date-only: status = %d, want 201 (body=%s)", w.Code, w.Body.String())
	}
	if aiSt.created.DueDate == nil {
		t.Fatal("date-only due_date must not be dropped")
	}
	want := time.Date(2026, 9, 5, 23, 59, 59, 0, time.UTC)
	if !aiSt.created.DueDate.Equal(want) {
		t.Fatalf("date-only due_date = %v, want %v", aiSt.created.DueDate, want)
	}

	// Full RFC3339 accepted as-is.
	w = httptest.NewRecorder()
	s.handlePostMortemRoutes(w, pmRequest(http.MethodPost, "/api/v1/incidents/3/post-mortem/action-items", `{"description":"patch","due_date":"2026-09-05T12:00:00Z"}`), "3")
	if w.Code != http.StatusCreated {
		t.Fatalf("rfc3339: status = %d, want 201 (body=%s)", w.Code, w.Body.String())
	}
	if aiSt.created.DueDate == nil || aiSt.created.DueDate.Format(time.RFC3339) != "2026-09-05T12:00:00Z" {
		t.Fatalf("rfc3339 due_date = %v, want 12:00:00Z", aiSt.created.DueDate)
	}

	// Invalid format 400s instead of silently clearing the due date.
	w = httptest.NewRecorder()
	s.handlePostMortemRoutes(w, pmRequest(http.MethodPost, "/api/v1/incidents/3/post-mortem/action-items", `{"description":"patch","due_date":"next tuesday"}`), "3")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid: status = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
}

// An assignee id that does not reference a user must 400 up front; previously
// it sailed through and failed later at the notifications FK (500) or in the
// dead-letter queue.
func TestCreateActionItemRejectsUnknownAssignee(t *testing.T) {
	t.Parallel()

	inc := &store.IncidentRecord{ID: uuid.New(), IncidentNumber: 4}
	s, _, _ := newPMTestServer(inc, &store.PostMortemRecord{ID: uuid.New(), IncidentID: inc.ID, Status: "draft"})

	validAssignee := uuid.New()
	s.userStore = &stubPMUserStore{validAssignee: validAssignee}

	// Known user accepted.
	w := httptest.NewRecorder()
	s.handlePostMortemRoutes(w, pmRequest(http.MethodPost, "/api/v1/incidents/4/post-mortem/action-items", `{"description":"patch","assignee_id":"`+validAssignee.String()+`"}`), "4")
	if w.Code != http.StatusCreated {
		t.Fatalf("valid assignee: status = %d, want 201 (body=%s)", w.Code, w.Body.String())
	}

	// Unknown user rejected before the store write.
	freshStore := &stubPMActionItemStore{}
	s.actionItemStore = freshStore
	w = httptest.NewRecorder()
	s.handlePostMortemRoutes(w, pmRequest(http.MethodPost, "/api/v1/incidents/4/post-mortem/action-items", `{"description":"patch","assignee_id":"`+uuid.NewString()+`"}`), "4")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown assignee: status = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
	if freshStore.created != nil {
		t.Fatal("Create must not run for an unknown assignee")
	}
}

// Invalid action item enums must 400, not surface as CHECK-constraint 500s.
func TestUpdateActionItemValidatesEnums(t *testing.T) {
	t.Parallel()

	inc := &store.IncidentRecord{ID: uuid.New(), IncidentNumber: 5}
	pmID := uuid.New()
	s, _, _ := newPMTestServer(inc, &store.PostMortemRecord{ID: pmID, IncidentID: inc.ID, Status: "draft"})

	item := &store.ActionItemRecord{ID: uuid.New(), PostMortemID: pmID, Description: "x", Status: "open", Priority: "medium", Type: "investigate"}
	aiSt2 := &stubReassignActionItemStore{stubPMActionItemStore{}, item}
	s.actionItemStore = aiSt2

	for name, body := range map[string]string{
		"bad status":   `{"status":"detected"}`,
		"bad priority": `{"priority":"urgent"}`,
		"bad type":     `{"type":"destroy"}`,
	} {
		w := httptest.NewRecorder()
		s.handlePostMortemRoutes(w, pmRequest(http.MethodPatch, "/api/v1/incidents/5/post-mortem/action-items/"+item.ID.String(), body), "5")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400 (body=%s)", name, w.Code, w.Body.String())
		}
		if aiSt2.updated != nil {
			t.Fatalf("%s: Update must not run for an invalid enum", name)
		}
	}

	w := httptest.NewRecorder()
	s.handlePostMortemRoutes(w, pmRequest(http.MethodPatch, "/api/v1/incidents/5/post-mortem/action-items/"+item.ID.String(), `{"status":"in_progress"}`), "5")
	if w.Code != http.StatusOK {
		t.Fatalf("valid status: status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	if aiSt2.updated == nil || aiSt2.updated.Status != "in_progress" {
		t.Fatalf("valid status: update not applied: %+v", aiSt2.updated)
	}
}

// Reassigning an action item must update the assignee (this pins the
// regression where the "old" assignee was captured after in-place mutation,
// which also broke the new-assignee notification condition downstream).
func TestUpdateActionItemReassignAppliesNewAssignee(t *testing.T) {
	t.Parallel()

	inc := &store.IncidentRecord{ID: uuid.New(), IncidentNumber: 6}
	pmID := uuid.New()
	s, _, _ := newPMTestServer(inc, &store.PostMortemRecord{ID: pmID, IncidentID: inc.ID, Status: "draft"})

	oldAssignee := uuid.New()
	newAssignee := uuid.New()
	s.userStore = &stubPMUserStore{validAssignee: newAssignee}

	item := &store.ActionItemRecord{
		ID:           uuid.New(),
		PostMortemID: pmID,
		Description:  "x",
		Status:       "open",
		Priority:     "medium",
		Type:         "investigate",
		AssigneeID:   &oldAssignee,
	}
	aiSt := &stubReassignActionItemStore{stubPMActionItemStore{}, item}
	s.actionItemStore = aiSt

	w := httptest.NewRecorder()
	s.handlePostMortemRoutes(w, pmRequest(http.MethodPatch, "/api/v1/incidents/6/post-mortem/action-items/"+item.ID.String(), `{"assignee_id":"`+newAssignee.String()+`"}`), "6")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	if aiSt.updated == nil || aiSt.updated.AssigneeID == nil || *aiSt.updated.AssigneeID != newAssignee {
		t.Fatalf("updated assignee = %v, want %s", aiSt.updated.AssigneeID, newAssignee)
	}
}

// An action item UUID belonging to a different incident's post-mortem must
// 404 rather than mutate (and audit) under the path incident's number.
func TestUpdateActionItemScopedToPathIncident(t *testing.T) {
	t.Parallel()

	inc := &store.IncidentRecord{ID: uuid.New(), IncidentNumber: 8}
	s, _, _ := newPMTestServer(inc, &store.PostMortemRecord{ID: uuid.New(), IncidentID: inc.ID, Status: "draft"})

	foreignPM := uuid.New()
	foreignItem := &store.ActionItemRecord{
		ID:           uuid.New(),
		PostMortemID: foreignPM,
		Description:  "x",
		Status:       "open",
		Priority:     "medium",
		Type:         "investigate",
	}
	aiSt := &stubReassignActionItemStore{stubPMActionItemStore{}, foreignItem}
	s.actionItemStore = aiSt

	w := httptest.NewRecorder()
	s.handlePostMortemRoutes(w, pmRequest(http.MethodPatch, "/api/v1/incidents/8/post-mortem/action-items/"+foreignItem.ID.String(), `{"status":"completed"}`), "8")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body=%s)", w.Code, w.Body.String())
	}
	if aiSt.updated != nil {
		t.Fatal("Update must not run for a cross-incident action item")
	}
}

// stubReassignActionItemStore returns a fixed item for GetByID.
type stubReassignActionItemStore struct {
	stubPMActionItemStore
	item *store.ActionItemRecord
}

func (s *stubReassignActionItemStore) GetByID(_ context.Context, _ uuid.UUID) (*store.ActionItemRecord, error) {
	return s.item, nil
}

// compile-time guards that the stubs satisfy the interfaces
var (
	_ store.IncidentStore   = (*stubPMIncidentStore)(nil)
	_ store.PostMortemStore = (*stubPMStore)(nil)
	_ store.ActionItemStore = (*stubPMActionItemStore)(nil)
	_ store.UserStore       = (*stubPMUserStore)(nil)
)
