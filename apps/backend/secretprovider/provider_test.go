package secretprovider

import (
	"context"
	"errors"
	"testing"

	algacrypto "alga/crypto"
)

func installTestCrypto(t *testing.T) {
	t.Helper()
	t.Setenv("SECRET_PEPPER", "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=")
	t.Setenv("ENCRYPTION_KEYS", "1:MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=")
	k, err := algacrypto.LoadFromEnv()
	if err != nil {
		t.Fatalf("crypto.LoadFromEnv: %v", err)
	}
	algacrypto.SetDefault(k)
}

func TestIsValidType(t *testing.T) {
	for _, tp := range AllTypes {
		if !IsValidType(tp) {
			t.Errorf("expected %q to be valid", tp)
		}
	}
	if IsValidType("not-a-real-provider") {
		t.Fatal("expected unknown type to be invalid")
	}
}

func TestRegistryResolveUnsupported(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Resolve("nope", nil); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("expected ErrUnsupportedProvider, got %v", err)
	}
}

func TestInternalProviderDecryptsValue(t *testing.T) {
	installTestCrypto(t)
	r := NewRegistry()

	enc, err := algacrypto.Default().EncryptString("super-secret-value")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	p, err := r.Resolve(TypeInternal, nil)
	if err != nil {
		t.Fatalf("resolve internal: %v", err)
	}
	got, err := p.GetSecret(context.Background(), SecretRef{ValueEncrypted: enc})
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got != "super-secret-value" {
		t.Fatalf("expected decrypted value, got %q", got)
	}
	if p.Type() != TypeInternal {
		t.Fatalf("expected type internal, got %q", p.Type())
	}
}

func TestInternalProviderMissingValue(t *testing.T) {
	installTestCrypto(t)
	r := NewRegistry()
	p, err := r.Resolve(TypeInternal, nil)
	if err != nil {
		t.Fatalf("resolve internal: %v", err)
	}
	if _, err := p.GetSecret(context.Background(), SecretRef{}); !errors.Is(err, ErrMissingValue) {
		t.Fatalf("expected ErrMissingValue, got %v", err)
	}
}

func TestExternalStubsReturnNotImplemented(t *testing.T) {
	r := NewRegistry()
	for _, tp := range []string{TypeHashiCorpVault, TypeAWSSecretsMgr, TypeGCPSecretManager, TypeAzureKeyVault} {
		p, err := r.Resolve(tp, map[string]string{"address": "https://example"})
		if err != nil {
			t.Fatalf("resolve %s: %v", tp, err)
		}
		_, err = p.GetSecret(context.Background(), SecretRef{RemoteRef: "secret/data/x"})
		if !errors.Is(err, ErrNotImplemented) {
			t.Fatalf("expected ErrNotImplemented for %s, got %v", tp, err)
		}
		if p.Type() != tp {
			t.Fatalf("expected type %s, got %q", tp, p.Type())
		}
	}
}
