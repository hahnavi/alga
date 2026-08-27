// Package secretprovider abstracts the backends that resolve a shared secret
// into its plaintext value. The built-in "internal" provider decrypts secrets
// stored in the Alga database; external providers (HashiCorp Vault, AWS Secrets
// Manager, GCP Secret Manager, Azure Key Vault) proxy reads to a remote store.
//
// Each provider type is registered as a Factory. A Factory turns a provider's
// decrypted connection config into a Provider instance, so a provider can hold
// long-lived clients or validate its config at construction time. External
// providers are currently registered as stubs that surface a clear
// "not implemented" error; they exist so the UI and persistence model support
// multiple provider types and so a real implementation can drop in later by
// replacing the stub factory.
package secretprovider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	algacrypto "alga/crypto"
)

// Provider type identifiers. These mirror the values selectable in the admin UI
// and stored on credential_providers.type.
const (
	TypeInternal         = "internal"
	TypeHashiCorpVault   = "hashicorp_vault"
	TypeAWSSecretsMgr    = "aws_secrets_manager"
	TypeGCPSecretManager = "gcp_secret_manager"
	TypeAzureKeyVault    = "azure_key_vault"
)

// AllTypes is the full set of selectable provider types, in UI display order.
var AllTypes = []string{
	TypeInternal,
	TypeHashiCorpVault,
	TypeAWSSecretsMgr,
	TypeGCPSecretManager,
	TypeAzureKeyVault,
}

// IsValidType reports whether t is a known provider type.
func IsValidType(t string) bool {
	return slices.Contains(AllTypes, t)
}

// SecretRef carries the per-secret data a provider needs to resolve a value:
// the encrypted value for internal secrets, and the backend path/key for
// external secrets.
type SecretRef struct {
	ValueEncrypted string
	RemoteRef      string
}

// Provider resolves a single secret reference into its plaintext value using
// the provider instance's connection config.
type Provider interface {
	// Type returns the provider type identifier this implementation serves.
	Type() string
	// GetSecret returns the plaintext value for ref. Implementations MUST treat
	// the returned value as a secret and avoid logging it.
	GetSecret(ctx context.Context, ref SecretRef) (string, error)
}

// Factory builds a Provider from a provider's decrypted connection config.
// Returning an error here surfaces misconfiguration (e.g. a missing vault
// address) before any secret is requested.
type Factory func(config map[string]string) (Provider, error)

// Errors surfaced by providers and the registry.
var (
	ErrUnsupportedProvider = errors.New("secretprovider: unsupported provider type")
	ErrNotImplemented      = errors.New("secretprovider: provider type is registered but external fetch is not implemented")
	ErrMissingValue        = errors.New("secretprovider: secret has no stored value")
)

// Registry maps provider types to factories. The zero value is not usable; use
// NewRegistry (which also registers the built-in providers).
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry returns a registry preloaded with the internal provider and the
// external provider stubs. The external stubs are the credential-provider
// integration seam: a real vault implementation plugs in by replacing its
// stub factory via Register.
func NewRegistry() *Registry {
	r := &Registry{factories: map[string]Factory{}}
	r.Register(TypeInternal, newInternal)
	r.Register(TypeHashiCorpVault, newExternalStub(TypeHashiCorpVault))
	r.Register(TypeAWSSecretsMgr, newExternalStub(TypeAWSSecretsMgr))
	r.Register(TypeGCPSecretManager, newExternalStub(TypeGCPSecretManager))
	r.Register(TypeAzureKeyVault, newExternalStub(TypeAzureKeyVault))
	return r
}

// Register adds or replaces a factory for a provider type. It is safe to call
// concurrently with Resolve.
func (r *Registry) Register(pType string, f Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[pType] = f
}

// Resolve builds a Provider for pType from the given connection config. Returns
// ErrUnsupportedProvider when the type is unknown.
func (r *Registry) Resolve(pType string, config map[string]string) (Provider, error) {
	r.mu.RLock()
	f, ok := r.factories[pType]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedProvider, pType)
	}
	return f(config)
}

// internalProvider decrypts secrets stored in the Alga database.
type internalProvider struct{}

func newInternal(_ map[string]string) (Provider, error) { return internalProvider{}, nil }
func (internalProvider) Type() string                   { return TypeInternal }
func (internalProvider) GetSecret(_ context.Context, ref SecretRef) (string, error) {
	if ref.ValueEncrypted == "" {
		return "", ErrMissingValue
	}
	pt, err := algacrypto.Default().DecryptString(ref.ValueEncrypted)
	if err != nil {
		return "", fmt.Errorf("secretprovider: decrypt internal value: %w", err)
	}
	return pt, nil
}

// externalStub is a placeholder for a not-yet-wired external provider. It lets
// the persistence and UI model support multiple provider types today; replacing
// the factory with a real implementation later requires no schema change.
type externalStub struct{ pType string }

// newExternalStub returns the placeholder factory for an external vault type.
// This is a designed extension point, not forgotten work: provider rows of
// these types stay selectable so schema/UI need no change when real
// implementations land, while GetSecret fails loudly with ErrNotImplemented,
// which the API edge maps to 501 instead of leaking a raw error. An
// implementation replaces its stub via Register and must keep that sentinel
// → mapped-error contract until every stub type is replaced.
func newExternalStub(pType string) Factory {
	return func(_ map[string]string) (Provider, error) { return externalStub{pType: pType}, nil }
}
func (s externalStub) Type() string { return s.pType }
func (externalStub) GetSecret(_ context.Context, _ SecretRef) (string, error) {
	return "", ErrNotImplemented
}
