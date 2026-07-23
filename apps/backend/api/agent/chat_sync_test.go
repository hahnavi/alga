package agent_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"alga/api/agent"
	"alga/store"
)

func TestUserSlackThreadMessageUsesLinkedSlackIdentity(t *testing.T) {
	user := &store.UserRecord{
		ID:               uuid.New(),
		Email:            "ada@example.com",
		FullName:         "Ada Lovelace",
		SlackUserID:      "U123456",
		SlackDisplayName: "Ada L.",
	}

	cs := &agent.ChatSyncService{}
	msg, customize := cs.UserSlackThreadMessage(user, "**checking** logs")
	if msg != "*checking* logs" {
		t.Fatalf("message = %q, want converted Slack mrkdwn without sender prefix", msg)
	}
	if customize == nil {
		t.Fatal("expected Slack post customization")
	}
	if customize.Username != "Ada L." {
		t.Fatalf("username = %q, want Slack display name", customize.Username)
	}
	if !strings.Contains(customize.IconURL, "U123456") {
		t.Fatalf("icon URL = %q, want Slack user ID seed", customize.IconURL)
	}
}

func TestUserSlackThreadMessageFallsBackWithoutLinkedSlackIdentity(t *testing.T) {
	user := &store.UserRecord{
		ID:       uuid.New(),
		Email:    "ada@example.com",
		FullName: "Ada Lovelace",
	}

	cs := &agent.ChatSyncService{}
	msg, customize := cs.UserSlackThreadMessage(user, "checking logs")
	if msg != "*Ada Lovelace*: checking logs" {
		t.Fatalf("message = %q, want prefixed fallback", msg)
	}
	if customize != nil {
		t.Fatal("expected no Slack post customization")
	}
}
