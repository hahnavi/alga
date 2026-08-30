package worker

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/sse"
	"alga/store"
)

type stubActionItemStore struct {
	overdue []store.ActionItemRecord
}

func (s *stubActionItemStore) Create(context.Context, *store.ActionItemRecord) (*store.ActionItemRecord, error) {
	return nil, nil
}
func (s *stubActionItemStore) GetByID(context.Context, uuid.UUID) (*store.ActionItemRecord, error) {
	return nil, nil
}
func (s *stubActionItemStore) ListByPostMortem(context.Context, uuid.UUID) ([]store.ActionItemRecord, error) {
	return nil, nil
}
func (s *stubActionItemStore) ListByPostMortemIDs(context.Context, []uuid.UUID) (map[uuid.UUID][]store.ActionItemRecord, error) {
	return nil, nil
}
func (s *stubActionItemStore) ListOpen(context.Context) ([]store.ActionItemRecord, error) {
	return nil, nil
}
func (s *stubActionItemStore) ListOverdue(context.Context) ([]store.ActionItemRecord, error) {
	return s.overdue, nil
}
func (s *stubActionItemStore) Update(context.Context, uuid.UUID, *store.ActionItemRecord) (*store.ActionItemRecord, error) {
	return nil, nil
}
func (s *stubActionItemStore) Delete(context.Context, uuid.UUID) error { return nil }
func (s *stubActionItemStore) DeleteByPostMortemID(context.Context, uuid.UUID) error {
	return nil
}

type stubNotificationStore struct {
	created []store.NotificationRecord
}

func (s *stubNotificationStore) Create(_ context.Context, n *store.NotificationRecord) (*store.NotificationRecord, error) {
	n.ID = uuid.New()
	s.created = append(s.created, *n)
	return n, nil
}
func (s *stubNotificationStore) ListByUser(context.Context, string, int64, int64) ([]store.NotificationRecord, error) {
	return nil, nil
}
func (s *stubNotificationStore) GetUnreadCount(context.Context, string) (int64, error) {
	return 0, nil
}
func (s *stubNotificationStore) MarkRead(context.Context, string, string) error { return nil }
func (s *stubNotificationStore) MarkAllRead(context.Context, string) error      { return nil }

func TestActionItemSweepSignals(t *testing.T) {
	t.Parallel()

	assignee := uuid.New()
	item := store.ActionItemRecord{
		ID:           uuid.New(),
		PostMortemID: uuid.New(),
		Description:  "rotate leaked credential",
		Status:       "open",
		AssigneeID:   &assignee,
		AssigneeName: "alice",
		DueDate:      ptrTime(time.Now().Add(-time.Hour)),
	}

	notifications := &stubNotificationStore{}
	broker := sse.NewBroker()
	userCh := make(chan sse.Event, 8)
	broker.SubscribeUser(assignee.String(), "test-client", userCh)
	globalCh := broker.Subscribe("test-global")

	w := NewActionItemSweepWorker(&stubActionItemStore{overdue: []store.ActionItemRecord{item}}, nil)
	w.SetSignals(notifications, &sse.DualPublisher{Broker: broker}, nil)

	w.tick(context.Background())

	// SSE: global broadcast + per-user event.
	var sawGlobal, sawUser bool
	for {
		select {
		case ev := <-globalCh:
			if ev.Type == "action_item_overdue" {
				sawGlobal = true
			}
			if len(globalCh) == 0 && !sawUser {
				continue
			}
		case ev := <-userCh:
			if ev.Type == "action_item_overdue" {
				sawUser = true
			}
		default:
			goto done
		}
	}
done:
	if !sawGlobal || !sawUser {
		t.Fatalf("SSE signals missing: global=%v user=%v", sawGlobal, sawUser)
	}

	// In-app notification for the assignee.
	if len(notifications.created) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notifications.created))
	}
	n := notifications.created[0]
	if n.UserID != assignee.String() || n.Type != "info" || n.ResourceType != "post_mortem" {
		t.Fatalf("unexpected notification: %+v", n)
	}
	if !strings.Contains(n.Message, "rotate leaked credential") {
		t.Fatalf("message %q does not mention the item", n.Message)
	}
}

func TestActionItemSweepNoAssigneeSkipsNotification(t *testing.T) {
	t.Parallel()

	item := store.ActionItemRecord{
		ID:           uuid.New(),
		PostMortemID: uuid.New(),
		Description:  "unassigned follow-up",
		DueDate:      ptrTime(time.Now().Add(-2 * time.Hour)),
	}

	notifications := &stubNotificationStore{}
	w := NewActionItemSweepWorker(&stubActionItemStore{overdue: []store.ActionItemRecord{item}}, nil)
	w.SetSignals(notifications, &sse.DualPublisher{Broker: sse.NewBroker()}, nil)

	w.tick(context.Background())

	if len(notifications.created) != 0 {
		t.Fatalf("notifications = %d, want 0 for unassigned item", len(notifications.created))
	}
}

func TestActionItemSweepLogOnlyWhenNoSignalsWired(t *testing.T) {
	t.Parallel()

	item := store.ActionItemRecord{ID: uuid.New(), PostMortemID: uuid.New(), DueDate: ptrTime(time.Now())}
	w := NewActionItemSweepWorker(&stubActionItemStore{overdue: []store.ActionItemRecord{item}}, nil)

	w.tick(context.Background()) // must not panic with all signal deps nil
}

// stubPostMortemStore answers GetByID for the deep-link resolution; every
// other method panics via the nil embedded interface.
type stubPostMortemStore struct {
	store.PostMortemStore
	pm *store.PostMortemRecord
}

func (s *stubPostMortemStore) GetByID(context.Context, uuid.UUID) (*store.PostMortemRecord, error) {
	if s.pm == nil {
		return nil, nil
	}
	return s.pm, nil
}

func (s *stubPostMortemStore) ExistsByIncidentID(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}

// stubIncidentStore resolves GetIncidentByID for the deep-link resolution.
type stubIncidentStore struct {
	store.IncidentStore
	number int64
}

func (s *stubIncidentStore) GetIncidentByID(context.Context, uuid.UUID) (*store.IncidentRecord, error) {
	return &store.IncidentRecord{ID: uuid.New(), IncidentNumber: s.number}, nil
}

// The overdue notification deep-links to the incident that owns the
// post-mortem (resource_type=incident, resource_id=incident number) so the
// frontend notification click can route somewhere useful.
func TestActionItemSweepNotificationDeepLinksToIncident(t *testing.T) {
	t.Parallel()

	assignee := uuid.New()
	incidentID := uuid.New()
	item := store.ActionItemRecord{
		ID:           uuid.New(),
		PostMortemID: uuid.New(),
		Description:  "patch the leak",
		Status:       "open",
		AssigneeID:   &assignee,
		DueDate:      ptrTime(time.Now().Add(-time.Hour)),
	}

	notifications := &stubNotificationStore{}
	w := NewActionItemSweepWorker(
		&stubActionItemStore{overdue: []store.ActionItemRecord{item}},
		&stubIncidentStore{number: 42},
	)
	w.SetPostMortemStore(&stubPostMortemStore{pm: &store.PostMortemRecord{ID: item.PostMortemID, IncidentID: incidentID}})
	w.SetSignals(notifications, nil, nil)

	w.tick(context.Background())

	if len(notifications.created) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notifications.created))
	}
	n := notifications.created[0]
	if n.ResourceType != "incident" || n.ResourceID != "42" {
		t.Fatalf("deep-link = %s/%s, want incident/42", n.ResourceType, n.ResourceID)
	}
}

// Without a post-mortem store wired the notification keeps the legacy
// post_mortem resource targeting rather than failing the sweep.
func TestActionItemSweepNotificationFallsBackWithoutPMStore(t *testing.T) {
	t.Parallel()

	assignee := uuid.New()
	item := store.ActionItemRecord{
		ID:           uuid.New(),
		PostMortemID: uuid.New(),
		Description:  "patch the leak",
		Status:       "open",
		AssigneeID:   &assignee,
		DueDate:      ptrTime(time.Now().Add(-time.Hour)),
	}

	notifications := &stubNotificationStore{}
	w := NewActionItemSweepWorker(&stubActionItemStore{overdue: []store.ActionItemRecord{item}}, nil)
	w.SetSignals(notifications, nil, nil)

	w.tick(context.Background())

	if len(notifications.created) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notifications.created))
	}
	if n := notifications.created[0]; n.ResourceType != "post_mortem" {
		t.Fatalf("resource_type = %s, want post_mortem fallback", n.ResourceType)
	}
}

// Both SSE and the in-app notification share one per-item 24h dedup window:
// a second tick within the window delivers nothing.
func TestActionItemSweepSignalsOncePerDedupWindow(t *testing.T) {
	t.Parallel()

	assignee := uuid.New()
	item := store.ActionItemRecord{
		ID:           uuid.New(),
		PostMortemID: uuid.New(),
		Description:  "rotate leaked credential",
		Status:       "open",
		AssigneeID:   &assignee,
		DueDate:      ptrTime(time.Now().Add(-time.Hour)),
	}

	notifications := &stubNotificationStore{}
	broker := sse.NewBroker()
	userCh := make(chan sse.Event, 8)
	broker.SubscribeUser(assignee.String(), "test-client", userCh)

	w := NewActionItemSweepWorker(&stubActionItemStore{overdue: []store.ActionItemRecord{item}}, nil)
	signaled := map[string]bool{}
	w.overdueDedup = func(_ context.Context, itemID string) bool {
		if signaled[itemID] {
			return false
		}
		signaled[itemID] = true
		return true
	}
	w.SetSignals(notifications, &sse.DualPublisher{Broker: broker}, nil)

	w.tick(context.Background())
	if len(notifications.created) != 1 {
		t.Fatalf("first tick: notifications = %d, want 1", len(notifications.created))
	}

	w.tick(context.Background())
	if len(notifications.created) != 1 {
		t.Fatalf("second tick: notifications = %d, want still 1 (dedup)", len(notifications.created))
	}
	if len(userCh) == 0 {
		t.Fatal("first tick: expected a user SSE event")
	}
	// Drain SSE events from the first tick.
	for len(userCh) > 0 {
		<-userCh
	}
	w.tick(context.Background())
	if len(userCh) != 0 {
		t.Fatal("third tick: SSE must be deduplicated within the 24h window")
	}
}
