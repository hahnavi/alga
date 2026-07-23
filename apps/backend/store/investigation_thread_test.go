package store

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"alga/ent"
)

func TestInvestigationThreadStoreCreatesAlertThreadMessage(t *testing.T) {
	client, cleanup := setupTestEntClient(t)
	defer cleanup()

	st := newPGInvestigationThreadStore(client)
	ctx := context.Background()

	thread, err := st.EnsureThread(ctx, ThreadOwnerAlert, "42")
	if err != nil {
		t.Fatalf("ensure thread: %v", err)
	}

	msg, err := st.AddMessage(ctx, thread.ThreadID, InvestigationThreadMessage{
		Type:     "comment",
		Source:   "user",
		Message:  "checked the pod restarts",
		UserID:   uuid.Nil.String(),
		Username: "operator",
	})
	if err != nil {
		t.Fatalf("add message: %v", err)
	}

	got, _, err := st.GetThreadByOwner(ctx, ThreadOwnerAlert, "42", 50, 0)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if got.OwnerType != ThreadOwnerAlert || got.OwnerID != "42" {
		t.Fatalf("owner = %s/%s, want alert/42", got.OwnerType, got.OwnerID)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(got.Messages))
	}
	if got.Messages[0].ID != msg.ID || got.Messages[0].Message != "checked the pod restarts" {
		t.Fatalf("message = %#v, want saved message", got.Messages[0])
	}
}

func TestInvestigationThreadStorePaginatesThreadMessages(t *testing.T) {
	client, cleanup := setupTestEntClient(t)
	defer cleanup()

	st := newPGInvestigationThreadStore(client)
	ctx := context.Background()

	thread, err := st.EnsureThread(ctx, ThreadOwnerAlert, "42")
	if err != nil {
		t.Fatalf("ensure thread: %v", err)
	}
	for _, text := range []string{"first", "second", "third"} {
		if _, err := st.AddMessage(ctx, thread.ThreadID, InvestigationThreadMessage{Message: text}); err != nil {
			t.Fatalf("add message %q: %v", text, err)
		}
	}

	got, total, err := st.GetThreadByOwner(ctx, ThreadOwnerAlert, "42", 2, 1)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(got.Messages))
	}
	if got.Messages[0].Message != "second" || got.Messages[1].Message != "third" {
		t.Fatalf("messages = %#v, want second/third", got.Messages)
	}
}

func setupTestEntClient(t *testing.T) (*ent.Client, func()) {
	t.Helper()
	return newTestEntClient(t), func() {}
}
