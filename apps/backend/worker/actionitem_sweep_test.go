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
