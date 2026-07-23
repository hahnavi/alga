package store

import (
	"errors"
	"testing"

	"alga/crypto"
	"alga/ent"
)

// newAgentTokenEntTestClient wraps newTestEntClient so the agent-token tests
// share the same isolated PostgreSQL schema as the rest of the store tests.
// The schema includes the real FK from agent_dm_messages.agent_token_dm_messages
// to agent_tokens.id, which is what makes the soft-delete test meaningful.
func newAgentTokenEntTestClient(t *testing.T) (*ent.Client, func()) {
	t.Helper()
	return newTestEntClient(t), func() {}
}

// installTestKeyring ensures the global default keyring has a pepper so
// agent token hashing works inside these unit tests.
func installTestKeyring(t *testing.T) {
	t.Helper()
	const pepper = "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI="
	const keys = "1:MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI="
	t.Setenv("SECRET_PEPPER", pepper)
	t.Setenv("ENCRYPTION_KEYS", keys)
	k, err := crypto.LoadFromEnv()
	if err != nil {
		t.Fatalf("crypto.LoadFromEnv: %v", err)
	}
	crypto.SetDefault(k)
}

// TestRevokeTokenPreservesFKProtectedRows proves that revoking an agent with
// chat history and ICS role references succeeds by soft-deleting the row
// instead of hard-deleting (which would violate the agent_dm_messages FK).
func TestRevokeTokenPreservesFKProtectedRows(t *testing.T) {
	installTestKeyring(t)
	client, cleanup := newAgentTokenEntTestClient(t)
	defer cleanup()
	st := newPGAgentTokenStore(client)

	rec, err := st.CreateToken("hermes-fk", nil, "hermes", []string{"investigate"})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	id := rec.ID

	// Seed an agent_dm_messages row that references the token. This is what
	// the real PostgreSQL FK is protecting.
	if _, err := client.AgentDMMessage.Create().
		SetAgentTokenID(id).
		SetRole(string(AgentDMRoleUser)).
		SetBody("hello").
		Save(t.Context()); err != nil {
		t.Fatalf("create agent_dm_message: %v", err)
	}

	// Soft-delete must succeed even with the FK-protected child row present.
	if err := st.RevokeToken(id); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	// The token must no longer appear in the admin list (ListTokens filters
	// revoked=false), and the secret must no longer validate.
	tokens, err := st.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	for _, tk := range tokens {
		if tk.ID == id {
			t.Errorf("expected revoked token to be excluded from ListTokens, got %+v", tk)
		}
	}
	if vrec, err := st.ValidateToken(rec.Token); err != nil {
		t.Fatalf("ValidateToken: %v", err)
	} else if vrec != nil {
		t.Errorf("expected ValidateToken to return nil for revoked token, got %+v", vrec)
	}

	// The row must still exist (soft delete) so the FK from agent_dm_messages
	// remains valid for forensic/audit value.
	if _, derr := client.AgentToken.Get(t.Context(), id); derr != nil {
		t.Fatalf("revoked token row should still exist for FK integrity, got: %v", derr)
	}
}

// TestMutatorsRejectRevokedAgentToken proves that UpdateAgentConfig and
// SetAgentEnabled refuse to mutate a soft-deleted (revoked) agent token.
func TestMutatorsRejectRevokedAgentToken(t *testing.T) {
	installTestKeyring(t)
	client, cleanup := newAgentTokenEntTestClient(t)
	defer cleanup()
	st := newPGAgentTokenStore(client)

	rec, err := st.CreateToken("hermes-mutators", nil, "hermes", []string{"investigate"})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	if err := st.RevokeToken(rec.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	if err := st.UpdateAgentConfig(rec.ID, "all", nil, []string{"investigate"}); err == nil {
		t.Error("expected UpdateAgentConfig to fail on revoked token, got nil")
	} else if !isAgentNotFoundInactive(err) {
		t.Errorf("UpdateAgentConfig: want ErrAgentNotFoundInactive, got %v", err)
	}

	if err := st.SetAgentEnabled(rec.ID, true); err == nil {
		t.Error("expected SetAgentEnabled to fail on revoked token, got nil")
	} else if !isAgentNotFoundInactive(err) {
		t.Errorf("SetAgentEnabled: want ErrAgentNotFoundInactive, got %v", err)
	}
}

func isAgentNotFoundInactive(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrAgentNotFoundInactive)
}
