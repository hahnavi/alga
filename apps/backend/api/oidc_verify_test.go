package api

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/config"
	"alga/store"
)

// This file holds the H2 regression tests (ASVS V2.10/V7.1). They prove the
// OIDC callback verifies the ID-token signature and rejects tampered/unsigned
// tokens and unverified emails, and that a valid verified token links the
// account and audits the event. The tests stand up a fake OIDC provider that
// serves discovery, JWKS, token, and userinfo endpoints with a real RSA key.

// ---------------------------------------------------------------------------
// Fake OIDC provider
// ---------------------------------------------------------------------------

type fakeOIDCProvider struct {
	t        *testing.T
	server   *httptest.Server
	key      *rsa.PrivateKey
	kid      string
	issuer   string
	clientID string
	// idTokenOverride lets tests inject a tampered/unsigned token.
	idTokenOverride string
	// expectedNonce is the nonce the fake IdP embeds in the ID token.
	expectedNonce string
	// userinfoEmailVerified overrides the userinfo email_verified (default true).
	userinfoEmailVerified *bool
}

func newFakeOIDCProvider(t *testing.T, clientID string) *fakeOIDCProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	p := &fakeOIDCProvider{
		t:        t,
		key:      key,
		kid:      "test-key-1",
		clientID: clientID,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", p.handleDiscovery)
	mux.HandleFunc("/jwks", p.handleJWKS)
	mux.HandleFunc("/token", p.handleToken)
	mux.HandleFunc("/userinfo", p.handleUserinfo)
	p.server = httptest.NewServer(mux)
	p.issuer = p.server.URL
	t.Cleanup(p.server.Close)
	return p
}

func (p *fakeOIDCProvider) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"issuer":                 p.issuer,
		"authorization_endpoint": p.issuer + "/authorize",
		"token_endpoint":         p.issuer + "/token",
		"userinfo_endpoint":      p.issuer + "/userinfo",
		"jwks_uri":               p.issuer + "/jwks",
	})
}

func (p *fakeOIDCProvider) handleJWKS(w http.ResponseWriter, r *http.Request) {
	n := base64.RawURLEncoding.EncodeToString(p.key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(p.key.E)).Bytes())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"keys": []map[string]string{
			{"kid": p.kid, "kty": "RSA", "use": "sig", "alg": "RS256", "n": n, "e": e},
		},
	})
}

func (p *fakeOIDCProvider) handleToken(w http.ResponseWriter, r *http.Request) {
	idToken := p.idTokenOverride
	if idToken == "" {
		idToken = p.signIDToken(p.expectedNonce)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": "fake-access-token",
		"id_token":     idToken,
		"token_type":   "Bearer",
		"expires_in":   3600,
	})
}

func (p *fakeOIDCProvider) handleUserinfo(w http.ResponseWriter, r *http.Request) {
	verified := true
	if p.userinfoEmailVerified != nil {
		verified = *p.userinfoEmailVerified
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"sub":            "fake-sub-123",
		"email":          "oidcuser@example.com",
		"email_verified": verified,
		"name":           "OIDC User",
	})
}

// signIDToken builds and RS256-signs a valid ID token for the given nonce.
func (p *fakeOIDCProvider) signIDToken(nonce string) string {
	header := map[string]string{"alg": "RS256", "kid": p.kid, "typ": "JWT"}
	now := time.Now().Unix()
	claims := map[string]any{
		"iss":            p.issuer,
		"sub":            "fake-sub-123",
		"aud":            p.clientID,
		"exp":            now + 3600,
		"iat":            now,
		"nonce":          nonce,
		"email":          "oidcuser@example.com",
		"email_verified": true,
		"name":           "OIDC User",
	}
	return p.signJWT(header, claims)
}

// signJWT signs an arbitrary header+claims pair with the provider key.
func (p *fakeOIDCProvider) signJWT(header, claims any) string {
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(cb)
	hashed := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, hashed[:])
	if err != nil {
		p.t.Fatalf("sign jwt: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// ---------------------------------------------------------------------------
// Test handler wiring
// ---------------------------------------------------------------------------

// newOIDCCallbackTestHandler builds an oidcHandler wired against the fake
// provider, with controllable state/login stores and a pre-seeded user.
// Returns the handler and the created provider id.
func newOIDCCallbackTestHandler(t *testing.T, p *fakeOIDCProvider, opts ...cbOpt) (*oidcHandler, *capturingAuditStore) {
	t.Helper()
	o := &cbOptions{
		email:    "oidcuser@example.com",
		verified: true,
		state:    "test-state",
		nonce:    "test-nonce",
	}
	for _, fn := range opts {
		fn(o)
	}

	ps := newMockOIDCProviderStore()
	created, err := ps.CreateProvider(context.Background(), &store.OIDCProviderRecord{
		Name: "Fake", Issuer: p.issuer, ClientID: p.clientID, Enabled: true,
	}, "secret")
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	is := newMockOIDCIdentityStore()
	us := &mockUserStore{users: []store.UserRecord{
		{ID: uuid.New(), Email: o.email, Role: "admin", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}}
	ss := &mockSessionStore{sessions: map[string]*store.SessionRecord{}}
	as := &capturingAuditStore{}

	// The handler validates the state with an "oidc:" prefix.
	stateStore := &inMemOAuthStateStore{states: map[string]bool{"oidc:" + o.state: true}}
	h := &oidcHandler{
		cfg:            &config.Config{},
		providerStore:  ps,
		identityStore:  is,
		userStore:      us,
		sessionStore:   ss,
		auditStore:     as,
		stateStore:     stateStore,
		loginStore:     &inMemoryOIDCLoginStore2{items: map[string]oidcLoginPayload{}},
		discoveryCache: newOIDCDiscoveryCache(),
		jwksCache:      newOIDCJWKSCache(),
		sessionExpiry:  time.Hour,
	}

	// Seed the login payload so the callback can consume the state/nonce.
	loginStore := h.loginStore.(*inMemoryOIDCLoginStore2)
	loginStore.items[o.state] = oidcLoginPayload{ProviderID: created.ID.String(), Verifier: "verifier", Nonce: o.nonce}
	// Tell the fake IdP which nonce to embed in the ID token (it would
	// normally be echoed from the authorize request).
	p.expectedNonce = o.nonce
	h.testProviderID = created.ID
	return h, as
}

type cbOptions struct {
	state    string
	nonce    string
	email    string
	verified bool
}

type cbOpt func(*cbOptions)

func withStateNonce(state, nonce string) cbOpt {
	return func(o *cbOptions) { o.state = state; o.nonce = nonce }
}

// inMemOAuthStateStore is a minimal in-memory oauthStateStore for callback tests.
type inMemOAuthStateStore struct {
	mu     sync.Mutex
	states map[string]bool
}

func (s *inMemOAuthStateStore) Set(state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state] = true
	return nil
}

func (s *inMemOAuthStateStore) Validate(state string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// OAuth state is stored with a "oidc:" prefix by the handler.
	return s.states[state] || s.states[strings.TrimPrefix(state, "oidc:")], nil
}

// inMemoryOIDCLoginStore2 is a minimal in-memory oidcLoginStore for callback tests.
type inMemoryOIDCLoginStore2 struct {
	mu    sync.Mutex
	items map[string]oidcLoginPayload
}

func (s *inMemoryOIDCLoginStore2) Set(state string, payload oidcLoginPayload) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[state] = payload
	return nil
}

func (s *inMemoryOIDCLoginStore2) Consume(state string) (oidcLoginPayload, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[state]
	if !ok {
		return oidcLoginPayload{}, false, nil
	}
	delete(s.items, state)
	return item, true, nil
}

// ---------------------------------------------------------------------------
// Unit tests for verifyOIDCIDToken (pure function)
// ---------------------------------------------------------------------------

func TestVerifyOIDCIDToken_RejectsTamperedSignature(t *testing.T) {
	p := newFakeOIDCProvider(t, "client-123")
	jwks := newOIDCJWKSCache()
	tok := p.signIDToken("nonce-abc")

	// Flip a byte in the signature to tamper.
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatal("expected 3 jwt parts")
	}
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	if len(sigBytes) == 0 {
		t.Fatal("empty signature")
	}
	sigBytes[0] ^= 0xFF
	parts[2] = base64.RawURLEncoding.EncodeToString(sigBytes)
	tampered := strings.Join(parts, ".")

	_, err = verifyOIDCIDToken(context.Background(), jwks, p.issuer+"/jwks", tampered, p.issuer, "client-123", "nonce-abc")
	if err == nil {
		t.Fatal("expected tampered ID token to be rejected")
	}
	if !strings.Contains(err.Error(), "signature") {
		t.Fatalf("expected signature error, got: %v", err)
	}
}

func TestVerifyOIDCIDToken_RejectsUnsignedToken(t *testing.T) {
	p := newFakeOIDCProvider(t, "client-123")
	jwks := newOIDCJWKSCache()

	// Build a token whose signature is random garbage (not signed by the key).
	header := map[string]string{"alg": "RS256", "kid": p.kid, "typ": "JWT"}
	claims := map[string]any{
		"iss": p.issuer, "sub": "x", "aud": "client-123",
		"exp": time.Now().Unix() + 3600, "email_verified": true,
	}
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(cb)
	junk := make([]byte, 256)
	_, _ = rand.Read(junk)
	unsigned := signingInput + "." + base64.RawURLEncoding.EncodeToString(junk)

	_, err := verifyOIDCIDToken(context.Background(), jwks, p.issuer+"/jwks", unsigned, p.issuer, "client-123", "")
	if err == nil {
		t.Fatal("expected unsigned ID token to be rejected")
	}
}

func TestVerifyOIDCIDToken_RejectsWrongIssuer(t *testing.T) {
	p := newFakeOIDCProvider(t, "client-123")
	jwks := newOIDCJWKSCache()
	tok := p.signIDToken("")

	_, err := verifyOIDCIDToken(context.Background(), jwks, p.issuer+"/jwks", tok, "https://wrong-issuer.example", "client-123", "")
	if err == nil || !strings.Contains(err.Error(), "issuer") {
		t.Fatalf("expected issuer error, got: %v", err)
	}
}

func TestVerifyOIDCIDToken_RejectsWrongAudience(t *testing.T) {
	p := newFakeOIDCProvider(t, "client-123")
	jwks := newOIDCJWKSCache()
	tok := p.signIDToken("")

	_, err := verifyOIDCIDToken(context.Background(), jwks, p.issuer+"/jwks", tok, p.issuer, "wrong-client", "")
	if err == nil || !strings.Contains(err.Error(), "audience") {
		t.Fatalf("expected audience error, got: %v", err)
	}
}

func TestVerifyOIDCIDToken_RejectsNonceMismatch(t *testing.T) {
	p := newFakeOIDCProvider(t, "client-123")
	jwks := newOIDCJWKSCache()
	tok := p.signIDToken("correct-nonce")

	_, err := verifyOIDCIDToken(context.Background(), jwks, p.issuer+"/jwks", tok, p.issuer, "client-123", "wrong-nonce")
	if err == nil || !strings.Contains(err.Error(), "nonce") {
		t.Fatalf("expected nonce error, got: %v", err)
	}
}

func TestVerifyOIDCIDToken_AcceptsValidToken(t *testing.T) {
	p := newFakeOIDCProvider(t, "client-123")
	jwks := newOIDCJWKSCache()
	tok := p.signIDToken("nonce-xyz")

	claims, err := verifyOIDCIDToken(context.Background(), jwks, p.issuer+"/jwks", tok, p.issuer, "client-123", "nonce-xyz")
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if claims.Email != "oidcuser@example.com" || !claims.EmailVerified {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if claims.Subject != "fake-sub-123" {
		t.Fatalf("unexpected sub: %s", claims.Subject)
	}
}

// ---------------------------------------------------------------------------
// End-to-end callback tests (H2 SPEC §8)
// ---------------------------------------------------------------------------

func TestOIDCCallback_RejectsTamperedIDToken(t *testing.T) {
	p := newFakeOIDCProvider(t, "client-123")
	// Override the token endpoint to return a token signed with a DIFFERENT key.
	attackerKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate attacker key: %v", err)
	}
	header := map[string]string{"alg": "RS256", "kid": p.kid, "typ": "JWT"}
	now := time.Now().Unix()
	claims := map[string]any{
		"iss": p.issuer, "sub": "fake-sub-123", "aud": p.clientID,
		"exp": now + 3600, "iat": now, "email": "oidcuser@example.com",
		"email_verified": true,
	}
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(cb)
	hashed := sha256.Sum256([]byte(signingInput))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, attackerKey, crypto.SHA256, hashed[:])
	p.idTokenOverride = signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)

	h, _ := newOIDCCallbackTestHandler(t, p, withStateNonce("state-1", "nonce-1"))
	rec := callOIDCCallback(t, h, "state-1")

	if !strings.Contains(rec.Body.String(), "oidc_auth_failed") && !strings.Contains(rec.Body.String(), "oidc_email_not_verified") {
		// Tampered token must not log the user in.
		if hasSessionCookie(rec) {
			t.Fatalf("tampered ID token should not create a session; got Set-Cookie session")
		}
	}
	if hasSessionCookie(rec) {
		t.Fatal("tampered ID token must not establish a session")
	}
}

func TestOIDCCallback_RejectsUnverifiedEmail(t *testing.T) {
	p := newFakeOIDCProvider(t, "client-123")
	h, _ := newOIDCCallbackTestHandler(t, p, withStateNonce("state-2", "nonce-2"))
	// Now that we know the nonce the handler expects, build a properly signed
	// token that carries email_verified=false (but valid iss/aud/exp/nonce so
	// signature verification passes and we reach the email_verified gate).
	header := map[string]string{"alg": "RS256", "kid": p.kid, "typ": "JWT"}
	now := time.Now().Unix()
	claims := map[string]any{
		"iss": p.issuer, "sub": "fake-sub-123", "aud": p.clientID,
		"exp": now + 3600, "iat": now, "nonce": "nonce-2",
		"email": "oidcuser@example.com", "email_verified": false,
	}
	p.idTokenOverride = p.signJWT(header, claims)

	rec := callOIDCCallback(t, h, "state-2")

	if !strings.Contains(rec.Body.String(), "oidc_email_not_verified") {
		t.Fatalf("expected redirect with oidc_email_not_verified, got: %s", rec.Body.String())
	}
	if hasSessionCookie(rec) {
		t.Fatal("unverified email must not establish a session")
	}
}

func TestOIDCCallback_AcceptsValidVerifiedTokenAndLinks(t *testing.T) {
	p := newFakeOIDCProvider(t, "client-123")
	h, as := newOIDCCallbackTestHandler(t, p, withStateNonce("state-3", "nonce-3"))
	linksBefore := len(as.events)

	rec := callOIDCCallback(t, h, "state-3")

	if !hasSessionCookie(rec) {
		t.Fatalf("valid verified token should establish a session; body: %s", rec.Body.String())
	}
	// The user had no prior OIDC identity link, so a verified-email auto-link
	// must have been created and audited.
	linked := false
	for _, e := range as.events[linksBefore:] {
		if e.Event == store.AuditOIDCIdentityLinked {
			linked = true
		}
	}
	if !linked {
		t.Fatalf("expected an %s audit event for verified-email auto-link", store.AuditOIDCIdentityLinked)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func callOIDCCallback(t *testing.T, h *oidcHandler, state string) *httptest.ResponseRecorder {
	t.Helper()
	providerID := h.testProviderID
	url := fmt.Sprintf("/api/v1/auth/oidc/%s/callback?code=fake-code&state=%s", providerID, state)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	h.callback(rec, req, providerID)
	return rec
}

func hasSessionCookie(rec *httptest.ResponseRecorder) bool {
	for _, c := range rec.Result().Cookies() {
		if c.Name == "alga_session" && c.Value != "" {
			return true
		}
	}
	return false
}

// capturingAuditStore records audit events so tests can assert they fired.
type capturingAuditStore struct {
	mu     sync.Mutex
	events []store.AuditRecord
}

func (c *capturingAuditStore) Log(event store.AuditEvent, userID *uuid.UUID, username, ip, userAgent string, success bool, details map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, store.AuditRecord{
		Event: event, UserID: userID, Username: username, Success: success,
	})
}

func (c *capturingAuditStore) Query(filter map[string]any) ([]store.AuditRecord, error) {
	return nil, nil
}
func (c *capturingAuditStore) GetRecentEvents(limit int) ([]store.AuditRecord, error) {
	return nil, nil
}
