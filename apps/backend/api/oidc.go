package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"alga/config"
	"alga/logger"
	"alga/store"
	"alga/valkey"
)

// ---------------------------------------------------------------------------
// OIDC login state store (carries PKCE verifier + provider ID, single-use)
// ---------------------------------------------------------------------------

type oidcLoginPayload struct {
	ProviderID string `json:"provider_id"`
	Verifier   string `json:"verifier"`
	Nonce      string `json:"nonce"`
	NextPath   string `json:"next_path"`
}

type oidcLoginStore interface {
	Set(state string, payload oidcLoginPayload) error
	Consume(state string) (oidcLoginPayload, bool, error)
}

type valkeyOIDCLoginStore struct {
	client *valkey.Client
}

func (s *valkeyOIDCLoginStore) Set(state string, payload oidcLoginPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal oidc login payload: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = s.client.SetNX(ctx, "alga:oidc-login:"+state, string(data), 10*time.Minute)
	if err != nil {
		return fmt.Errorf("oidc login store set: %w", err)
	}
	return nil
}

func (s *valkeyOIDCLoginStore) Consume(state string) (oidcLoginPayload, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	key := "alga:oidc-login:" + state
	cmd := s.client.Builder().Get().Key(key).Build()
	result := s.client.Do(ctx, cmd)
	raw, err := result.ToString()
	if err != nil {
		return oidcLoginPayload{}, false, nil
	}
	_ = s.client.Del(ctx, key)
	var payload oidcLoginPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return oidcLoginPayload{}, false, fmt.Errorf("unmarshal oidc login payload: %w", err)
	}
	return payload, true, nil
}

type memoryOIDCLoginStore struct {
	mu     sync.Mutex
	items  map[string]oidcLoginItem
	stopCh chan struct{}
}

type oidcLoginItem struct {
	payload oidcLoginPayload
	expires time.Time
}

func newMemoryOIDCLoginStore() *memoryOIDCLoginStore {
	s := &memoryOIDCLoginStore{
		items:  make(map[string]oidcLoginItem),
		stopCh: make(chan struct{}),
	}
	go s.cleanup()
	return s
}

func (s *memoryOIDCLoginStore) cleanup() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("oidc login store cleanup panicked", "component", "api", "panic", r, "stack", string(debug.Stack()))
		}
	}()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for k, v := range s.items {
				if now.After(v.expires) {
					delete(s.items, k)
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *memoryOIDCLoginStore) Stop() { close(s.stopCh) }

func (s *memoryOIDCLoginStore) Set(state string, payload oidcLoginPayload) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[state] = oidcLoginItem{payload: payload, expires: time.Now().Add(10 * time.Minute)}
	return nil
}

func (s *memoryOIDCLoginStore) Consume(state string) (oidcLoginPayload, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[state]
	if !ok {
		return oidcLoginPayload{}, false, nil
	}
	delete(s.items, state)
	if time.Now().After(item.expires) {
		return oidcLoginPayload{}, false, nil
	}
	return item.payload, true, nil
}

// ---------------------------------------------------------------------------
// OIDC discovery document cache
// ---------------------------------------------------------------------------

type oidcDiscoveryDoc struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JwksURI               string `json:"jwks_uri"`
}

type oidcDiscoveryCache struct {
	mu      sync.RWMutex
	docs    map[string]oidcDiscoveryDoc
	expires map[string]time.Time
}

func newOIDCDiscoveryCache() *oidcDiscoveryCache {
	return &oidcDiscoveryCache{
		docs:    make(map[string]oidcDiscoveryDoc),
		expires: make(map[string]time.Time),
	}
}

func (c *oidcDiscoveryCache) get(ctx context.Context, issuer string) (oidcDiscoveryDoc, error) {
	c.mu.RLock()
	if doc, ok := c.docs[issuer]; ok && time.Now().Before(c.expires[issuer]) {
		c.mu.RUnlock()
		return doc, nil
	}
	c.mu.RUnlock()

	doc, err := c.fetch(ctx, issuer)
	if err != nil {
		return oidcDiscoveryDoc{}, err
	}

	c.mu.Lock()
	c.docs[issuer] = doc
	c.expires[issuer] = time.Now().Add(1 * time.Hour)
	c.mu.Unlock()
	return doc, nil
}

func (c *oidcDiscoveryCache) fetch(ctx context.Context, issuer string) (oidcDiscoveryDoc, error) {
	wellKnown := strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		return oidcDiscoveryDoc{}, fmt.Errorf("create discovery request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return oidcDiscoveryDoc{}, fmt.Errorf("fetch discovery document: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return oidcDiscoveryDoc{}, fmt.Errorf("discovery endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return oidcDiscoveryDoc{}, fmt.Errorf("read discovery response: %w", err)
	}

	var doc oidcDiscoveryDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return oidcDiscoveryDoc{}, fmt.Errorf("parse discovery document: %w", err)
	}

	if doc.AuthorizationEndpoint == "" || doc.TokenEndpoint == "" {
		return oidcDiscoveryDoc{}, errors.New("discovery document missing required endpoints")
	}
	return doc, nil
}

// ---------------------------------------------------------------------------
// PKCE helpers
// ---------------------------------------------------------------------------

func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func codeChallengeS256(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// ---------------------------------------------------------------------------
// OIDC handler
// ---------------------------------------------------------------------------

type oidcHandler struct {
	cfg            *config.Config
	providerStore  store.OIDCProviderStore
	identityStore  store.OIDCIdentityStore
	userStore      store.UserStore
	sessionStore   store.SessionStore
	auditStore     store.AuditStore
	stateStore     oauthStateStore
	loginStore     oidcLoginStore
	discoveryCache *oidcDiscoveryCache
	jwksCache      *oidcJWKSCache
	sessionExpiry  time.Duration

	// testProviderID lets callback tests inject the provider id without going
	// through URL path parsing. Only set in tests.
	testProviderID uuid.UUID
}

func newOIDCHandler(
	cfg *config.Config,
	providerStore store.OIDCProviderStore,
	identityStore store.OIDCIdentityStore,
	userStore store.UserStore,
	sessionStore store.SessionStore,
	auditStore store.AuditStore,
	stateStore oauthStateStore,
	loginStore oidcLoginStore,
	sessionExpiry time.Duration,
) *oidcHandler {
	return &oidcHandler{
		cfg:            cfg,
		providerStore:  providerStore,
		identityStore:  identityStore,
		userStore:      userStore,
		sessionStore:   sessionStore,
		auditStore:     auditStore,
		stateStore:     stateStore,
		loginStore:     loginStore,
		discoveryCache: newOIDCDiscoveryCache(),
		jwksCache:      newOIDCJWKSCache(),
		sessionExpiry:  sessionExpiry,
	}
}

// ---------------------------------------------------------------------------
// Admin provider CRUD
// ---------------------------------------------------------------------------

func (h *oidcHandler) listProviders(w http.ResponseWriter, r *http.Request) {
	limit, skip := parseLimitSkip(r, 100)
	enabled := r.URL.Query().Get("enabled")
	q := store.OIDCProviderQuery{Search: r.URL.Query().Get("search"), Limit: int(limit), Skip: int(skip)}
	switch enabled {
	case "true":
		t := true
		q.Enabled = &t
	case "false":
		f := false
		q.Enabled = &f
	}
	items, total, err := h.providerStore.ListProviders(r.Context(), q)
	if err != nil {
		writeInternalError(w, err, "failed to list oidc providers")
		return
	}
	writePaginatedJSON(w, items, total)
}

type oidcProviderInput struct {
	Name         string   `json:"name"`
	Issuer       string   `json:"issuer"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	Scopes       []string `json:"scopes"`
	Enabled      *bool    `json:"enabled"`
}

func (h *oidcHandler) createProvider(w http.ResponseWriter, r *http.Request) {
	var input oidcProviderInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Name == "" || input.Issuer == "" || input.ClientID == "" || input.ClientSecret == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "name, issuer, client_id, and client_secret are required")
		return
	}

	record := &store.OIDCProviderRecord{
		Name:     input.Name,
		Issuer:   input.Issuer,
		ClientID: input.ClientID,
		Scopes:   input.Scopes,
		Enabled:  true,
	}
	if input.Enabled != nil {
		record.Enabled = *input.Enabled
	}
	record.EnabledSet = true

	created, err := h.providerStore.CreateProvider(r.Context(), record, input.ClientSecret)
	if err != nil {
		writeInternalError(w, err, "failed to create oidc provider")
		return
	}

	user := userFromContext(r.Context())
	var userID *uuid.UUID
	if user != nil {
		userID = &user.ID
	}
	h.auditStore.Log(store.AuditOIDCProviderCreated, userID, "", r.RemoteAddr, r.UserAgent(), true, map[string]any{
		"provider_id": created.ID,
		"name":        created.Name,
		"issuer":      created.Issuer,
	})
	writeData(w, http.StatusCreated, created)
}

func (h *oidcHandler) getProvider(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	rec, err := h.providerStore.GetProvider(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, ErrorCodeNotFound, "oidc provider not found")
			return
		}
		writeInternalError(w, err, "failed to fetch oidc provider")
		return
	}
	if rec == nil {
		writeError(w, ErrorCodeNotFound, "oidc provider not found")
		return
	}
	writeData(w, http.StatusOK, rec)
}

func (h *oidcHandler) updateProvider(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var input oidcProviderInput
	if !decodeJSON(w, r, &input) {
		return
	}

	patch := &store.OIDCProviderRecord{
		Name:     input.Name,
		Issuer:   input.Issuer,
		ClientID: input.ClientID,
		Scopes:   input.Scopes,
	}
	if input.Enabled != nil {
		patch.Enabled = *input.Enabled
		patch.EnabledSet = true
	}

	var secretPtr *string
	if input.ClientSecret != "" {
		s := input.ClientSecret
		secretPtr = &s
	}

	updated, err := h.providerStore.UpdateProvider(r.Context(), id, patch, secretPtr)
	if err != nil {
		writeInternalError(w, err, "failed to update oidc provider")
		return
	}

	user := userFromContext(r.Context())
	var userID *uuid.UUID
	if user != nil {
		userID = &user.ID
	}
	h.auditStore.Log(store.AuditOIDCProviderUpdated, userID, "", r.RemoteAddr, r.UserAgent(), true, map[string]any{
		"provider_id": id,
		"name":        updated.Name,
	})
	writeData(w, http.StatusOK, updated)
}

func (h *oidcHandler) deleteProvider(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	if err := h.providerStore.DeleteProvider(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, ErrorCodeNotFound, "oidc provider not found")
			return
		}
		writeInternalError(w, err, "failed to delete oidc provider")
		return
	}

	user := userFromContext(r.Context())
	var userID *uuid.UUID
	if user != nil {
		userID = &user.ID
	}
	h.auditStore.Log(store.AuditOIDCProviderDeleted, userID, "", r.RemoteAddr, r.UserAgent(), true, map[string]any{
		"provider_id": id,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// Public endpoints: list enabled providers, authorize, callback
// ---------------------------------------------------------------------------

type oidcProviderPublic struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

func (h *oidcHandler) listPublicProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := h.providerStore.ListEnabledProviders(r.Context())
	if err != nil {
		writeInternalError(w, err, "failed to list oidc providers")
		return
	}
	out := make([]oidcProviderPublic, 0, len(providers))
	for _, p := range providers {
		out = append(out, oidcProviderPublic{ID: p.ID, Name: p.Name})
	}
	writeData(w, http.StatusOK, out)
}

func (h *oidcHandler) authorize(w http.ResponseWriter, r *http.Request, providerID uuid.UUID) {
	provider, err := h.providerStore.GetProviderWithSecret(r.Context(), providerID)
	if err != nil || provider == nil || !provider.Enabled {
		writeError(w, ErrorCodeNotFound, "oidc provider not found")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	doc, err := h.discoveryCache.get(ctx, provider.Issuer)
	if err != nil {
		writeInternalError(w, err, "failed to fetch oidc discovery document")
		return
	}

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		writeInternalError(w, err, "failed to generate state")
		return
	}
	state := hex.EncodeToString(stateBytes)

	verifier, err := generateCodeVerifier()
	if err != nil {
		writeInternalError(w, err, "failed to generate PKCE verifier")
		return
	}
	challenge := codeChallengeS256(verifier)

	nonceBytes := make([]byte, 32)
	if _, err := rand.Read(nonceBytes); err != nil {
		writeInternalError(w, err, "failed to generate nonce")
		return
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)

	if err := h.stateStore.Set("oidc:" + state); err != nil {
		writeInternalError(w, err, "failed to store state")
		return
	}

	nextPath := r.URL.Query().Get("next")
	if nextPath == "" {
		nextPath = "/"
	}

	if err := h.loginStore.Set(state, oidcLoginPayload{
		ProviderID: providerID.String(),
		Verifier:   verifier,
		Nonce:      nonce,
		NextPath:   nextPath,
	}); err != nil {
		writeInternalError(w, err, "failed to store login session")
		return
	}

	redirectURI := h.getRedirectURI(r, providerID)

	scopes := provider.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "email", "profile"}
	}

	authURL, err := url.Parse(doc.AuthorizationEndpoint)
	if err != nil {
		writeInternalError(w, err, "invalid authorization endpoint")
		return
	}
	q := authURL.Query()
	q.Set("client_id", provider.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(scopes, " "))
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("nonce", nonce)
	authURL.RawQuery = q.Encode()

	http.Redirect(w, r, authURL.String(), http.StatusFound)
}

func (h *oidcHandler) callback(w http.ResponseWriter, r *http.Request, providerID uuid.UUID) {
	q := r.URL.Query()
	if errParam := q.Get("error"); errParam != "" {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("oidc:"+errParam), http.StatusFound)
		return
	}

	code := q.Get("code")
	state := q.Get("state")
	if code == "" || state == "" {
		http.Redirect(w, r, "/login?error=oidc_auth_failed", http.StatusFound)
		return
	}

	valid, err := h.stateStore.Validate("oidc:" + state)
	if err != nil || !valid {
		logger.WarnCtx(r.Context(), "oidc callback: invalid state", "component", "api", "valid", valid, "error", err)
		http.Redirect(w, r, "/login?error=oidc_invalid_state", http.StatusFound)
		return
	}

	payload, ok, err := h.loginStore.Consume(state)
	if err != nil || !ok {
		logger.WarnCtx(r.Context(), "oidc callback: login payload not found", "component", "api", "ok", ok, "error", err)
		http.Redirect(w, r, "/login?error=oidc_invalid_state", http.StatusFound)
		return
	}

	provider, err := h.providerStore.GetProviderWithSecret(r.Context(), providerID)
	if err != nil || provider == nil || !provider.Enabled {
		http.Redirect(w, r, "/login?error=oidc_provider_not_found", http.StatusFound)
		return
	}

	redirectURI := h.getRedirectURI(r, providerID)

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	doc, err := h.discoveryCache.get(ctx, provider.Issuer)
	if err != nil {
		logger.ErrorCtx(r.Context(), "oidc discovery fetch failed", "component", "api", "error", err)
		http.Redirect(w, r, "/login?error=oidc_discovery_failed", http.StatusFound)
		return
	}

	tokenResp, err := h.exchangeCode(ctx, doc, code, redirectURI, provider, payload.Verifier)
	if err != nil {
		logger.ErrorCtx(r.Context(), "oidc token exchange failed", "component", "api", "error", err)
		http.Redirect(w, r, "/login?error=oidc_token_exchange_failed", http.StatusFound)
		return
	}

	// Verify the ID token (signature, iss, aud, exp, nonce). The ID token is
	// the authoritative identity assertion; userinfo is only supplementary.
	if tokenResp.IDToken == "" {
		logger.ErrorCtx(r.Context(), "oidc callback: missing id_token", "component", "api", "provider_id", providerID)
		h.auditStore.Log(store.AuditOIDCLoginFailed, nil, "", r.RemoteAddr, r.UserAgent(), false, map[string]any{
			"reason":      "missing_id_token",
			"provider_id": providerID,
		})
		http.Redirect(w, r, "/login?error=oidc_auth_failed", http.StatusFound)
		return
	}
	claims, err := verifyOIDCIDToken(ctx, h.jwksCache, doc.JwksURI, tokenResp.IDToken, provider.Issuer, provider.ClientID, payload.Nonce)
	if err != nil {
		logger.WarnCtx(r.Context(), "oidc id token verification failed", "component", "api", "provider_id", providerID, "error", err)
		h.auditStore.Log(store.AuditOIDCLoginFailed, nil, "", r.RemoteAddr, r.UserAgent(), false, map[string]any{
			"reason":      "id_token_verification_failed",
			"provider_id": providerID,
		})
		http.Redirect(w, r, "/login?error=oidc_auth_failed", http.StatusFound)
		return
	}

	// Reject unverified email before any account lookup or auto-linking.
	// This closes the OIDC email-takeover vector (ASVS V2.10/V7.1).
	if !claims.EmailVerified {
		h.auditStore.Log(store.AuditOIDCLoginFailed, nil, claims.Email, r.RemoteAddr, r.UserAgent(), false, map[string]any{
			"reason":      "email_not_verified",
			"provider_id": providerID,
		})
		http.Redirect(w, r, "/login?error=oidc_email_not_verified", http.StatusFound)
		return
	}

	if claims.Email == "" {
		h.auditStore.Log(store.AuditOIDCLoginFailed, nil, "", r.RemoteAddr, r.UserAgent(), false, map[string]any{
			"reason":      "no_email_claim",
			"provider_id": providerID,
		})
		http.Redirect(w, r, "/login?error=oidc_no_email", http.StatusFound)
		return
	}

	// Supplement with userinfo for claims not carried in the ID token (e.g.
	// name/picture). A userinfo failure is non-fatal: the verified ID token
	// already carries the authoritative identity (sub + verified email).
	if doc.UserinfoEndpoint != "" {
		if extra, err := h.fetchUserinfo(ctx, doc, tokenResp.AccessToken); err == nil {
			if claims.Name == "" {
				claims.Name = extra.Name
			}
		} else {
			logger.WarnCtx(r.Context(), "oidc userinfo fetch failed (non-fatal)", "component", "api", "error", err)
		}
	}

	var user *store.UserRecord
	if claims.Subject != "" {
		if existing, _ := h.identityStore.GetByProviderSubject(r.Context(), providerID, claims.Subject); existing != nil {
			user, _ = h.userStore.GetByID(existing.UserID)
		}
	}
	// Auto-linking by email is permitted only because email_verified == true
	// was enforced above. This is the trusted-provider linking policy.
	if user == nil && claims.Email != "" {
		user, _ = h.userStore.GetByEmail(claims.Email)
		if user != nil {
			_, linkErr := h.identityStore.CreateLink(r.Context(), &store.OIDCIdentityRecord{
				UserID:     user.ID,
				ProviderID: providerID,
				Subject:    claims.Subject,
				Issuer:     provider.Issuer,
				Email:      claims.Email,
			})
			if linkErr != nil {
				logger.WarnCtx(r.Context(), "oidc identity link creation failed", "component", "api", "user_id", user.ID, "error", linkErr)
			} else {
				uid := user.ID
				h.auditStore.Log(store.AuditOIDCIdentityLinked, &uid, user.Email, r.RemoteAddr, r.UserAgent(), true, map[string]any{
					"provider_id":    providerID,
					"provider":       provider.Name,
					"subject":        claims.Subject,
					"email_verified": true,
				})
			}
		}
	}
	if user == nil {
		h.auditStore.Log(store.AuditOIDCLoginFailed, nil, claims.Email, r.RemoteAddr, r.UserAgent(), false, map[string]any{
			"reason":      "no_matching_account",
			"provider_id": providerID,
			"email":       claims.Email,
		})
		http.Redirect(w, r, "/login?error=oidc_no_account", http.StatusFound)
		return
	}

	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		uid := user.ID
		h.auditStore.Log(store.AuditOIDCLoginFailed, &uid, user.Email, r.RemoteAddr, r.UserAgent(), false, map[string]any{
			"reason": "account_locked",
		})
		http.Redirect(w, r, "/login?error=oidc_account_locked", http.StatusFound)
		return
	}

	_ = h.userStore.RecordSuccessfulLogin(user.ID, r.RemoteAddr)

	uid := user.ID
	h.auditStore.Log(store.AuditOIDCLoginSuccess, &uid, user.Email, r.RemoteAddr, r.UserAgent(), true, map[string]any{
		"provider":    provider.Name,
		"provider_id": providerID,
		"method":      "oidc",
	})

	logger.InfoCtx(r.Context(), "oidc login success", "component", "api", "user_id", user.ID.String(), "email", user.Email, "provider", provider.Name)

	session, err := h.sessionStore.CreateSession(user.ID, r.RemoteAddr, r.UserAgent())
	if err != nil {
		logger.ErrorCtx(r.Context(), "oidc session creation failed", "component", "api", "user_id", user.ID.String(), "error", err)
		http.Redirect(w, r, "/login?error=oidc_auth_failed", http.StatusFound)
		return
	}

	csrfToken, err := generateCSRFToken()
	if err != nil {
		http.Redirect(w, r, "/login?error=oidc_auth_failed", http.StatusFound)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "alga_session",
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.sessionExpiry.Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "alga_csrf",
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: false,
		Secure:   h.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.sessionExpiry.Seconds()),
	})

	nextPath := payload.NextPath
	if nextPath == "" || !strings.HasPrefix(nextPath, "/") {
		nextPath = "/"
	}
	http.Redirect(w, r, nextPath, http.StatusFound)
}

func (h *oidcHandler) getRedirectURI(r *http.Request, providerID uuid.UUID) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/api/v1/auth/oidc/%s/callback", scheme, r.Host, providerID)
}

// ---------------------------------------------------------------------------
// Token exchange + userinfo
// ---------------------------------------------------------------------------

type oidcTokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func (h *oidcHandler) exchangeCode(ctx context.Context, doc oidcDiscoveryDoc, code, redirectURI string, provider *store.OIDCProviderRecord, codeVerifier string) (*oidcTokenResponse, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {provider.ClientID},
		"client_secret": {provider.ClientSecret()},
		"code_verifier": {codeVerifier},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, doc.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp oidcTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	return &tokenResp, nil
}

type oidcClaims struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

func (h *oidcHandler) fetchUserinfo(ctx context.Context, doc oidcDiscoveryDoc, accessToken string) (*oidcClaims, error) {
	if doc.UserinfoEndpoint == "" {
		return nil, errors.New("no userinfo endpoint in discovery document")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, doc.UserinfoEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read userinfo response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo returned status %d: %s", resp.StatusCode, string(body))
	}

	var claims oidcClaims
	if err := json.Unmarshal(body, &claims); err != nil {
		return nil, fmt.Errorf("parse userinfo claims: %w", err)
	}
	return &claims, nil
}
