package store

import (
	"testing"

	"github.com/google/uuid"

	"alga/ent"
)

func TestAlgaAgentDMChatID(t *testing.T) {
	if got, want := AlgaAgentDMChatID(), AlgaAgentDMChatIDLiteral; got != want {
		t.Fatalf("AlgaAgentDMChatID: got %q want %q", got, want)
	}
}

func TestIsAlgaAgentDMChatID(t *testing.T) {
	for _, ok := range []string{"alga_dm", "ALGA_DM", " alga_dm "} {
		if !IsAlgaAgentDMChatID(ok) {
			t.Fatalf("expected true for %q", ok)
		}
	}
	for _, bad := range []string{"", "alga", "alga_dm_", "alga_dm_507f1f77bcf86cd799439011", "investigation_507f1f77bcf86cd799439011"} {
		if IsAlgaAgentDMChatID(bad) {
			t.Fatalf("expected false for %q", bad)
		}
	}
}

func TestAgentDMStoreMutationsAreScopedToAgentToken(t *testing.T) {
	client, cleanup := newAgentDMEntTestClient(t)
	defer cleanup()
	st := newPGAgentDMStore(client)

	agentID := uuid.New()
	otherAgentID := uuid.New()
	if _, err := client.AgentToken.Create().
		SetID(agentID).
		SetName("agent-one").
		SetAgentType("hermes").
		SetTokenHash("hash-one").
		SetLookupPrefix("lookup-one").
		Save(t.Context()); err != nil {
		t.Fatalf("create agent token: %v", err)
	}
	if _, err := client.AgentToken.Create().
		SetID(otherAgentID).
		SetName("agent-two").
		SetAgentType("hermes").
		SetTokenHash("hash-two").
		SetLookupPrefix("lookup-two").
		Save(t.Context()); err != nil {
		t.Fatalf("create other agent token: %v", err)
	}

	otherMsg, err := st.AddMessage(otherAgentID.String(), AgentDMRoleUser, "Other body", nil, nil)
	if err != nil {
		t.Fatalf("AddMessage other: %v", err)
	}

	if err := st.UpdateMessageBody(agentID.String(), otherMsg.ID.String(), "tampered", false); err == nil {
		t.Fatal("expected cross-agent update to fail")
	}
	if err := st.DeleteMessage(agentID.String(), otherMsg.ID.String()); err == nil {
		t.Fatal("expected cross-agent delete to fail")
	}

	msgs, _, err := st.ListMessages(otherAgentID.String(), nil, 10)
	if err != nil {
		t.Fatalf("ListMessages other: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected one other-agent message, got %d", len(msgs))
	}
	if msgs[0].ID != otherMsg.ID || msgs[0].Body != "Other body" {
		t.Fatalf("message after cross-agent mutation = %s/%q, want %s/Other body", msgs[0].ID, msgs[0].Body, otherMsg.ID)
	}
}

func newAgentDMEntTestClient(t *testing.T) (*ent.Client, func()) {
	t.Helper()
	return newTestEntClient(t), func() {}
}
