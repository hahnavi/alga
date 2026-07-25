package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestOIDCProviderCreateAndGet(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	s := newPGOIDCProviderStore(client)

	out, err := s.CreateProvider(context.Background(), &OIDCProviderRecord{
		Name:     "Google",
		Issuer:   "https://accounts.google.com",
		ClientID: "test-client-id",
		Scopes:   []string{"openid", "email"},
		Enabled:  true,
	}, "super-secret")
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if out.ID == uuid.Nil {
		t.Fatal("expected non-nil ID")
	}
	if out.ClientSecretConfigured != true {
		t.Fatal("expected client_secret_configured=true")
	}

	got, err := s.GetProvider(context.Background(), out.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil provider")
	}
	if got.Name != "Google" {
		t.Fatalf("name = %q, want Google", got.Name)
	}
	if got.Issuer != "https://accounts.google.com" {
		t.Fatalf("issuer = %q", got.Issuer)
	}
}

func TestOIDCProviderGetWithSecretDecrypts(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	s := newPGOIDCProviderStore(client)

	created, err := s.CreateProvider(context.Background(), &OIDCProviderRecord{
		Name:     "Keycloak",
		Issuer:   "https://kc.example.com",
		ClientID: "kc-client",
		Enabled:  true,
	}, "my-client-secret")
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}

	got, err := s.GetProviderWithSecret(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetProviderWithSecret: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.ClientSecret() != "my-client-secret" {
		t.Fatalf("ClientSecret = %q, want my-client-secret", got.ClientSecret())
	}
}

func TestOIDCProviderUpdateChangesFields(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	s := newPGOIDCProviderStore(client)

	created, _ := s.CreateProvider(context.Background(), &OIDCProviderRecord{
		Name:     "Old",
		Issuer:   "https://old.example.com",
		ClientID: "old-id",
		Enabled:  true,
	}, "old-secret")

	updated, err := s.UpdateProvider(context.Background(), created.ID, &OIDCProviderRecord{
		Name:       "New",
		Enabled:    false,
		EnabledSet: true,
	}, nil)
	if err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	if updated.Name != "New" {
		t.Fatalf("name = %q, want New", updated.Name)
	}
	if updated.Enabled != false {
		t.Fatal("expected enabled=false")
	}
}

func TestOIDCProviderUpdateChangesSecret(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	s := newPGOIDCProviderStore(client)

	created, _ := s.CreateProvider(context.Background(), &OIDCProviderRecord{
		Name:     "P",
		Issuer:   "https://p.example.com",
		ClientID: "c",
		Enabled:  true,
	}, "old-secret")

	newSecret := "new-secret"
	_, err := s.UpdateProvider(context.Background(), created.ID, &OIDCProviderRecord{
		Name: "P",
	}, &newSecret)
	if err != nil {
		t.Fatalf("UpdateProvider with secret: %v", err)
	}

	got, _ := s.GetProviderWithSecret(context.Background(), created.ID)
	if got.ClientSecret() != "new-secret" {
		t.Fatalf("secret = %q, want new-secret", got.ClientSecret())
	}
}

func TestOIDCProviderDeleteRemovesIdentities(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	ps := newPGOIDCProviderStore(client)
	is := newPGOIDCIdentityStore(client)

	userID := mustCreateUser(t, client)
	created, _ := ps.CreateProvider(context.Background(), &OIDCProviderRecord{
		Name: "P", Issuer: "https://p.example.com", ClientID: "c", Enabled: true,
	}, "s")

	_, err := is.CreateLink(context.Background(), &OIDCIdentityRecord{
		UserID:     userID,
		ProviderID: created.ID,
		Subject:    "sub-123",
		Email:      "user@example.com",
		Issuer:     "https://p.example.com",
	})
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	if err := ps.DeleteProvider(context.Background(), created.ID); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}

	existing, _ := is.GetByProviderSubject(context.Background(), created.ID, "sub-123")
	if existing != nil {
		t.Fatal("expected identity to be cascade-deleted")
	}
}

func TestOIDCIdentityGetByProviderSubject(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	ps := newPGOIDCProviderStore(client)
	is := newPGOIDCIdentityStore(client)

	userID := mustCreateUser(t, client)
	created, _ := ps.CreateProvider(context.Background(), &OIDCProviderRecord{
		Name: "P", Issuer: "https://p.example.com", ClientID: "c", Enabled: true,
	}, "s")

	link, err := is.CreateLink(context.Background(), &OIDCIdentityRecord{
		UserID:     userID,
		ProviderID: created.ID,
		Subject:    "unique-subject",
		Email:      "user@test.com",
		Issuer:     "https://p.example.com",
	})
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	got, err := is.GetByProviderSubject(context.Background(), created.ID, "unique-subject")
	if err != nil {
		t.Fatalf("GetByProviderSubject: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.UserID != userID {
		t.Fatalf("user_id = %v, want %v", got.UserID, userID)
	}
	if got.ID != link.ID {
		t.Fatalf("id mismatch")
	}
}
