package api

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"alga/config"
	"alga/logger"
	"alga/store"
)

type googleOAuthHandler struct {
	cfg           *config.Config
	userStore     store.UserStore
	sessionStore  store.SessionStore
	auditStore    store.AuditStore
	stateStore    oauthStateStore
	sessionExpiry time.Duration
}

func newGoogleOAuthHandler(cfg *config.Config, userStore store.UserStore, sessionStore store.SessionStore, auditStore store.AuditStore, stateStore oauthStateStore, sessionExpiry time.Duration) *googleOAuthHandler {
	return &googleOAuthHandler{
		cfg:           cfg,
		userStore:     userStore,
		sessionStore:  sessionStore,
		auditStore:    auditStore,
		stateStore:    stateStore,
		sessionExpiry: sessionExpiry,
	}
}

func (h *googleOAuthHandler) enabled() bool {
	return GoogleOAuthEnabled(h.cfg)
}

// GoogleOAuthEnabled reports whether Google OAuth can be used with the given
// config. Both the feature flag and the client credentials must be present.
func GoogleOAuthEnabled(cfg *config.Config) bool {
	return cfg.GoogleOAuthEnabled && cfg.GoogleClientID != "" && cfg.GoogleClientSecret != ""
}

func (h *googleOAuthHandler) handleEnabled(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": h.enabled()})
}

func (h *googleOAuthHandler) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		writeError(w, ErrorCodeNotFound, "Google Sign-In is not configured")
		return
	}

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		writeInternalError(w, err, "failed to generate OAuth state")
		return
	}
	state := hex.EncodeToString(stateBytes)

	if err := h.stateStore.Set("google:" + state); err != nil {
		logger.ErrorCtx(r.Context(), "failed to store OAuth state", "component", "api", "error", err)
		writeInternalError(w, err, "failed to store OAuth state")
		return
	}

	redirectURI := h.getRedirectURI(r)
	authURL := fmt.Sprintf(
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s",
		url.QueryEscape(h.cfg.GoogleClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape("openid email profile"),
		url.QueryEscape(state),
	)

	http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *googleOAuthHandler) handleCallback(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		http.Redirect(w, r, "/login?error=google_not_configured", http.StatusFound)
		return
	}

	q := r.URL.Query()
	if errParam := q.Get("error"); errParam != "" {
		http.Redirect(w, r, "/login?error=google_auth_failed", http.StatusFound)
		return
	}

	code := q.Get("code")
	state := q.Get("state")
	if code == "" || state == "" {
		http.Redirect(w, r, "/login?error=google_auth_failed", http.StatusFound)
		return
	}

	valid, err := h.stateStore.Validate("google:" + state)
	if err != nil || !valid {
		logger.WarnCtx(r.Context(), "google oauth invalid state", "component", "api", "valid", valid, "error", err)
		http.Redirect(w, r, "/login?error=google_invalid_state", http.StatusFound)
		return
	}

	tokenResp, err := h.exchangeCode(code, h.getRedirectURI(r))
	if err != nil {
		http.Redirect(w, r, "/login?error=google_auth_failed", http.StatusFound)
		return
	}

	claims, err := parseIDTokenPayload(tokenResp.IDToken)
	if err != nil {
		http.Redirect(w, r, "/login?error=google_auth_failed", http.StatusFound)
		return
	}

	if subtle.ConstantTimeCompare([]byte(claims.Audience), []byte(h.cfg.GoogleClientID)) != 1 {
		http.Redirect(w, r, "/login?error=google_auth_failed", http.StatusFound)
		return
	}

	if !claims.EmailVerified {
		http.Redirect(w, r, "/login?error=google_email_not_verified", http.StatusFound)
		return
	}

	var user *store.UserRecord
	if claims.Sub != "" {
		user, _ = h.userStore.GetByGoogleID(claims.Sub)
	}
	if user == nil {
		user, _ = h.userStore.GetByEmail(claims.Email)
	}
	if user == nil {
		h.auditStore.Log(store.AuditGoogleLoginFailed, nil, claims.Email, r.RemoteAddr, r.UserAgent(), false, map[string]any{
			"reason": "no_matching_account",
		})
		http.Redirect(w, r, "/login?error=google_no_account", http.StatusFound)
		return
	}

	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		h.auditStore.Log(store.AuditGoogleLoginFailed, &user.ID, user.Email, r.RemoteAddr, r.UserAgent(), false, map[string]any{
			"reason": "account_locked",
		})
		http.Redirect(w, r, "/login?error=google_account_locked", http.StatusFound)
		return
	}

	if claims.Sub != "" && user.GoogleID != claims.Sub {
		_ = h.userStore.UpdateGoogleID(user.ID, claims.Sub)
	}

	_ = h.userStore.RecordSuccessfulLogin(user.ID, r.RemoteAddr)

	h.auditStore.Log(store.AuditGoogleLoginSuccess, &user.ID, user.Email, r.RemoteAddr, r.UserAgent(), true, map[string]any{
		"google_email": claims.Email,
		"method":       "google_oauth",
	})

	logger.InfoCtx(r.Context(), "google oauth login success", "component", "api", "user_id", user.ID.String(), "email", user.Email)

	session, err := h.sessionStore.CreateSession(user.ID, r.RemoteAddr, r.UserAgent())
	if err != nil {
		logger.ErrorCtx(r.Context(), "google oauth session creation failed", "component", "api", "user_id", user.ID.String(), "error", err)
		h.auditStore.Log(store.AuditGoogleLoginFailed, &user.ID, user.Email, r.RemoteAddr, r.UserAgent(), false, map[string]any{
			"reason": "session_creation_failed",
			"error":  err.Error(),
		})
		http.Redirect(w, r, "/login?error=google_auth_failed", http.StatusFound)
		return
	}

	csrfToken, err := generateCSRFToken()
	if err != nil {
		http.Redirect(w, r, "/login?error=google_auth_failed", http.StatusFound)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "alga_session",
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cfg.SecureCookies,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(h.sessionExpiry.Seconds()),
	})
	setRefreshTokenCookie(w, h.cfg.SecureCookies, h.sessionExpiry, session.RefreshToken)
	http.SetCookie(w, &http.Cookie{
		Name:     "alga_csrf",
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: false,
		Secure:   h.cfg.SecureCookies,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(h.sessionExpiry.Seconds()),
	})

	http.Redirect(w, r, "/login?google=success", http.StatusFound)
}

func (h *googleOAuthHandler) getRedirectURI(r *http.Request) string {
	if h.cfg.GoogleOAuthRedirectURL != "" {
		return h.cfg.GoogleOAuthRedirectURL
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/api/v1/auth/google/callback", scheme, r.Host)
}

type googleTokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func (h *googleOAuthHandler) exchangeCode(code, redirectURI string) (*googleTokenResponse, error) {
	return ExchangeGoogleCode(code, h.cfg.GoogleClientID, h.cfg.GoogleClientSecret, redirectURI)
}

// ExchangeGoogleCode exchanges an OAuth authorization code for Google tokens.
// It is a free function so sign-in and per-user binding flows can share one
// implementation without duplicating the OAuth dance.
func ExchangeGoogleCode(code, clientID, clientSecret, redirectURI string) (*googleTokenResponse, error) {
	data := url.Values{
		"code":          {code},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(data.Encode()))
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

	var tokenResp googleTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}

	return &tokenResp, nil
}

type googleIDTokenClaims struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	Audience      string `json:"aud"`
	Issuer        string `json:"iss"`
	ExpiresAt     int64  `json:"exp"`
}

var googleJWKS = &googleKeyCache{}

type googleKeyCache struct {
	mu      sync.RWMutex
	keys    map[string]*rsa.PublicKey
	expires time.Time
}

type jwksResponse struct {
	Keys []struct {
		Kid string `json:"kid"`
		Kty string `json:"kty"`
		Use string `json:"use"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

func (c *googleKeyCache) getKey(kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	if c.keys != nil && time.Now().Before(c.expires) {
		key := c.keys[kid]
		c.mu.RUnlock()
		if key != nil {
			return key, nil
		}
	} else {
		c.mu.RUnlock()
	}

	if err := c.fetch(); err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.keys == nil {
		return nil, errors.New("no keys available after fetch")
	}
	key, ok := c.keys[kid]
	if !ok {
		return nil, fmt.Errorf("key ID %q not found in Google JWKS", kid)
	}
	return key, nil
}

func (c *googleKeyCache) fetch() error {
	c.mu.Lock()
	if c.keys != nil && time.Now().Before(c.expires) {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v3/certs", nil)
	if err != nil {
		return fmt.Errorf("create JWKS request: %w", err)
	}

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS fetch returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read JWKS response: %w", err)
	}

	var jwks jwksResponse
	if err := json.Unmarshal(body, &jwks); err != nil {
		return fmt.Errorf("parse JWKS response: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, jwk := range jwks.Keys {
		if jwk.Kty != "RSA" || jwk.Use != "sig" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
		if err != nil {
			continue
		}
		n := new(big.Int).SetBytes(nBytes)
		e := new(big.Int).SetBytes(eBytes)
		keys[jwk.Kid] = &rsa.PublicKey{N: n, E: int(e.Int64())}
	}

	expiry := time.Now().Add(1 * time.Hour)
	if cc := resp.Header.Get("Cache-Control"); cc != "" {
		for _, part := range strings.Split(cc, ",") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "max-age=") {
				if secs, err := strconv.Atoi(strings.TrimPrefix(part, "max-age=")); err == nil && secs > 0 {
					expiry = time.Now().Add(time.Duration(secs) * time.Second)
				}
			}
		}
	}

	c.mu.Lock()
	c.keys = keys
	c.expires = expiry
	c.mu.Unlock()
	return nil
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

func parseIDTokenPayload(idToken string) (*googleIDTokenClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid ID token format: expected 3 parts, got %d", len(parts))
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode ID token header: %w", err)
	}

	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("parse ID token header: %w", err)
	}

	if header.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported ID token algorithm: %s", header.Alg)
	}

	signingInput := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode ID token signature: %w", err)
	}

	pubKey, err := googleJWKS.getKey(header.Kid)
	if err != nil {
		return nil, fmt.Errorf("lookup signing key: %w", err)
	}

	hashed := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hashed[:], signature); err != nil {
		return nil, fmt.Errorf("ID token signature verification failed: %w", err)
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode ID token payload: %w", err)
	}

	var claims googleIDTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("parse ID token claims: %w", err)
	}

	if claims.Issuer != "accounts.google.com" && claims.Issuer != "https://accounts.google.com" {
		return nil, fmt.Errorf("invalid ID token issuer: %s", claims.Issuer)
	}

	if claims.ExpiresAt > 0 && time.Now().Unix() > claims.ExpiresAt {
		return nil, errors.New("ID token expired")
	}

	return &claims, nil
}
