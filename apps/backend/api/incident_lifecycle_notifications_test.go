package api

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"alga/store"
)

// fakeICSRoleStore resolves only GetActiveRoles; everything else panics via
// the nil embedded interface, which is the desired signal if a test reaches it.
type fakeICSRoleStore struct {
	store.ICSRoleStore
	roles []store.ICSRoleRecord
	err   error
}

func (f *fakeICSRoleStore) GetActiveRoles(_ context.Context, _ int64) ([]store.ICSRoleRecord, error) {
	return f.roles, f.err
}

// TestIncidentNotificationRecipients pins the WP-C4 recipient rule: every
// active human ICS role holder plus commander/on-call-responder fallbacks,
// deduplicated, with agents (nil UserID) excluded.
func TestIncidentNotificationRecipients(t *testing.T) {
	t.Parallel()

	commander := uuid.New()
	responder := uuid.New()
	fallbackResponder := uuid.New()
	incident := &store.IncidentRecord{IncidentNumber: 42, CommanderID: &commander, OnCallResponderID: &fallbackResponder}

	t.Run("roles deduped with record fallbacks, agent roles dropped", func(t *testing.T) {
		t.Parallel()
		s := &Server{icsRoleStore: &fakeICSRoleStore{roles: []store.ICSRoleRecord{
			{RoleType: "incident_commander", UserID: &commander},
			{RoleType: "responder", UserID: &responder},
			{RoleType: "responder", UserID: &commander}, // duplicate holder
			{RoleType: "communications_lead"},           // agent assignment: no user id
			{RoleType: "responder", UserID: &fallbackResponder},
		}}}

		got := s.incidentNotificationRecipients(context.Background(), incident)
		want := []uuid.UUID{commander, responder, fallbackResponder}
		if len(got) != len(want) {
			t.Fatalf("recipients = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("recipients = %v, want %v", got, want)
			}
		}
	})

	t.Run("record fields used when no roles are assigned", func(t *testing.T) {
		t.Parallel()
		s := &Server{icsRoleStore: &fakeICSRoleStore{}}

		got := s.incidentNotificationRecipients(context.Background(), incident)
		if len(got) != 2 || got[0] != commander || got[1] != fallbackResponder {
			t.Fatalf("recipients = %v, want [%v %v]", got, commander, fallbackResponder)
		}
	})

	t.Run("role store failure still yields record fallbacks", func(t *testing.T) {
		t.Parallel()
		s := &Server{icsRoleStore: &fakeICSRoleStore{err: context.DeadlineExceeded}}

		got := s.incidentNotificationRecipients(context.Background(), incident)
		if len(got) != 2 {
			t.Fatalf("recipients = %v, want the 2 record fallbacks", got)
		}
	})

	t.Run("no publisher-independent crash on nil incident", func(t *testing.T) {
		t.Parallel()
		s := &Server{}
		if got := s.incidentNotificationRecipients(context.Background(), nil); got != nil {
			t.Fatalf("recipients = %v, want nil", got)
		}
	})
}

// TestPublishIncidentLifecycleNotificationsNoPublisher verifies a Server built
// without RabbitMQ wiring no-ops instead of panicking.
func TestPublishIncidentLifecycleNotificationsNoPublisher(t *testing.T) {
	t.Parallel()

	s := &Server{}
	cmd := uuid.New()
	s.publishIncidentLifecycleNotifications(
		context.Background(),
		&store.IncidentRecord{IncidentNumber: 7, CommanderID: &cmd},
		"incident_resolved",
		"resolved",
	)
}
