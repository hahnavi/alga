//go:build integration

package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestSAMLProviderAndIdentityLifecycle(t *testing.T) {
	stores := newTestStores(t)
	ctx := context.Background()
	user, err := stores.User.CreateUser("saml-integration@example.test", "hashed-password", "operator")
	if err != nil {
		t.Fatalf("create local user: %v", err)
	}
	provider, err := stores.SAMLProvider.CreateProvider(ctx, &SAMLProviderRecord{
		Name:           "AWS IAM Identity Center",
		EmailAttribute: "email",
	})
	if err != nil {
		t.Fatalf("create SAML provider draft: %v", err)
	}
	if provider.Enabled || provider.MetadataXML != "" || provider.ID == uuid.Nil {
		t.Fatalf("created provider draft = %+v", provider)
	}
	metadataXML := "<EntityDescriptor>integration metadata</EntityDescriptor>"
	provider, err = stores.SAMLProvider.UpdateProvider(ctx, provider.ID, &SAMLProviderRecord{
		IDPEntityID:    "https://identity.example.test/saml",
		MetadataXML:    metadataXML,
		MetadataXMLSet: true,
		Enabled:        true,
		EnabledSet:     true,
	})
	if err != nil {
		t.Fatalf("configure SAML provider: %v", err)
	}
	if !provider.Enabled || provider.IDPEntityID != "https://identity.example.test/saml" || provider.MetadataXML != metadataXML {
		t.Fatalf("configured provider = %+v", provider)
	}
	enabled, err := stores.SAMLProvider.ListEnabledProviders(ctx)
	if err != nil {
		t.Fatalf("list enabled SAML providers: %v", err)
	}
	if len(enabled) != 1 || enabled[0].ID != provider.ID {
		t.Fatalf("enabled providers = %+v, want provider %s", enabled, provider.ID)
	}
	identity, err := stores.SAMLIdentity.CreateLink(ctx, &SAMLIdentityRecord{
		UserID:     user.ID,
		ProviderID: provider.ID,
		Subject:    "aws-user-123",
		Email:      user.Email,
	})
	if err != nil {
		t.Fatalf("link SAML identity: %v", err)
	}
	gotIdentity, err := stores.SAMLIdentity.GetByProviderSubject(ctx, provider.ID, "aws-user-123")
	if err != nil {
		t.Fatalf("get SAML identity: %v", err)
	}
	if gotIdentity.ID != identity.ID || gotIdentity.UserID != user.ID {
		t.Fatalf("SAML identity = %+v, want identity %s for user %s", gotIdentity, identity.ID, user.ID)
	}
	if err := stores.SAMLProvider.DeleteProvider(ctx, provider.ID); err != nil {
		t.Fatalf("delete SAML provider: %v", err)
	}
	remaining, err := stores.SAMLIdentity.GetByProviderSubject(ctx, provider.ID, "aws-user-123")
	if err != nil || remaining != nil {
		t.Fatalf("identity after provider delete = (%+v, %v), want (nil, nil)", remaining, err)
	}
	if _, err := stores.User.GetByID(user.ID); err != nil {
		t.Fatalf("local Alga user should remain after provider delete: %v", err)
	}
}
