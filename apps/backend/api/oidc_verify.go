package api

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// This file implements OIDC ID-token verification (ASVS V2.10/V7.1, SPEC gap
// H2). Previously the OIDC callback trusted userinfo claims without verifying
// the ID-token signature, the issuer, the audience, expiry, or email_verified.
// These helpers mirror the existing verified Google flow (parseIDTokenPayload)
// but are generalized to any OIDC issuer and its discovered jwks_uri.

// ---------------------------------------------------------------------------
// OIDC JWKS cache (per-issuer, with refresh-on-unknown-kid)
// ---------------------------------------------------------------------------

// oidcJWKSCache caches the public signing keys for one or more OIDC issuers.
// Keys are public, so no encryption is needed. A key set is refreshed when its
// TTL expires or when a token references a kid that is not present (key
// rotation in flight).
type oidcJWKSCache struct {
	mu   sync.Mutex
	sets map[string]*oidcKeySet
}

type oidcKeySet struct {
	keys    map[string]crypto.PublicKey // keyed by kid
	expires time.Time
}

func newOIDCJWKSCache() *oidcJWKSCache {
	return &oidcJWKSCache{sets: make(map[string]*oidcKeySet)}
}

// getKey returns the public key for kid from the cached set for jwksURI,
// fetching (once, under the lock) when the set is missing/expired or the kid
// is unknown.
func (c *oidcJWKSCache) getKey(ctx context.Context, jwksURI, kid string) (crypto.PublicKey, error) {
	refresh := false
	c.mu.Lock()
	set, ok := c.sets[jwksURI]
	if ok && time.Now().Before(set.expires) {
		if key := set.keys[kid]; key != nil {
			c.mu.Unlock()
			return key, nil
		}
		// Set is fresh but the kid is unknown: a key rotation is in flight.
		refresh = true
	}
	c.mu.Unlock()

	if !ok || refresh {
		if err := c.fetch(ctx, jwksURI); err != nil {
			return nil, err
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	set = c.sets[jwksURI]
	if set == nil {
		return nil, errors.New("no OIDC keys available after fetch")
	}
	key, found := set.keys[kid]
	if !found {
		return nil, fmt.Errorf("OIDC key ID %q not found in JWKS at %s", kid, jwksURI)
	}
	return key, nil
}

func (c *oidcJWKSCache) fetch(ctx context.Context, jwksURI string) error {
	c.mu.Lock()
	// Double-checked: another caller may have refreshed while we waited.
	if set, ok := c.sets[jwksURI]; ok && time.Now().Before(set.expires) {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURI, nil)
	if err != nil {
		return fmt.Errorf("create JWKS request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch OIDC JWKS: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OIDC JWKS fetch returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read OIDC JWKS response: %w", err)
	}

	var jwks oidcJWKSResponse
	if err := json.Unmarshal(body, &jwks); err != nil {
		return fmt.Errorf("parse OIDC JWKS response: %w", err)
	}

	keys := make(map[string]crypto.PublicKey, len(jwks.Keys))
	for _, jwk := range jwks.Keys {
		if jwk.Use != "" && jwk.Use != "sig" {
			continue
		}
		pub, err := parseJWK(&jwk)
		if err != nil {
			// Skip keys we cannot use rather than failing the whole set.
			continue
		}
		if jwk.Kid != "" {
			keys[jwk.Kid] = pub
		}
	}
	if len(keys) == 0 {
		return errors.New("OIDC JWKS contained no usable signing keys")
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
	c.sets[jwksURI] = &oidcKeySet{keys: keys, expires: expiry}
	c.mu.Unlock()
	return nil
}

type oidcJWKSResponse struct {
	Keys []oidcJWK `json:"keys"`
}

type oidcJWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	// RSA
	N string `json:"n"`
	E string `json:"e"`
	// EC
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// parseJWK converts a JWK into a crypto.PublicKey. Supports RSA and EC
// (P-256/P-384/P-521) keys — the key types permitted for OIDC ID tokens.
func parseJWK(jwk *oidcJWK) (crypto.PublicKey, error) {
	switch jwk.Kty {
	case "RSA":
		nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
		if err != nil {
			return nil, fmt.Errorf("decode RSA modulus: %w", err)
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
		if err != nil {
			return nil, fmt.Errorf("decode RSA exponent: %w", err)
		}
		n := new(big.Int).SetBytes(nBytes)
		e := new(big.Int).SetBytes(eBytes)
		if !e.IsUint64() || e.Uint64() > 1<<31-1 {
			return nil, errors.New("RSA exponent too large")
		}
		return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
	case "EC":
		crv := ecCurve(jwk.Crv)
		if crv == nil {
			return nil, fmt.Errorf("unsupported EC curve: %s", jwk.Crv)
		}
		xBytes, err := base64.RawURLEncoding.DecodeString(jwk.X)
		if err != nil {
			return nil, fmt.Errorf("decode EC x: %w", err)
		}
		yBytes, err := base64.RawURLEncoding.DecodeString(jwk.Y)
		if err != nil {
			return nil, fmt.Errorf("decode EC y: %w", err)
		}
		return &ecdsa.PublicKey{
			Curve: crv,
			X:     new(big.Int).SetBytes(xBytes),
			Y:     new(big.Int).SetBytes(yBytes),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported JWK key type: %s", jwk.Kty)
	}
}

func ecCurve(crv string) elliptic.Curve {
	switch crv {
	case "P-256":
		return elliptic.P256()
	case "P-384":
		return elliptic.P384()
	case "P-521":
		return elliptic.P521()
	default:
		return nil
	}
}

// ---------------------------------------------------------------------------
// ID-token verification
// ---------------------------------------------------------------------------

type oidcJWTHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

// oidcIDTokenClaims is the verified claim set extracted from an OIDC ID token.
type oidcIDTokenClaims struct {
	Issuer        string `json:"iss"`
	Subject       string `json:"sub"`
	Audience      any    `json:"aud"` // string or []string
	ExpiresAt     int64  `json:"exp"`
	IssuedAt      int64  `json:"iat"`
	Nonce         string `json:"nonce"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

// verifyOIDCIDToken verifies an OIDC ID token against the provider JWKS and the
// expected issuer/client ID. It checks the RS256/ES256 signature, iss
// (constant-time), aud (constant-time), and exp. If expectedNonce is non-empty
// it is also matched (constant-time) against the token nonce.
func verifyOIDCIDToken(ctx context.Context, jwks *oidcJWKSCache, jwksURI, idToken, expectedIssuer, expectedClientID, expectedNonce string) (*oidcIDTokenClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid ID token format: expected 3 parts")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode ID token header: %w", err)
	}
	var header oidcJWTHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("parse ID token header: %w", err)
	}

	signingInput := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode ID token signature: %w", err)
	}

	pub, err := jwks.getKey(ctx, jwksURI, header.Kid)
	if err != nil {
		return nil, fmt.Errorf("lookup OIDC signing key: %w", err)
	}

	if err := verifyJWTSignature(header.Alg, pub, []byte(signingInput), signature); err != nil {
		return nil, fmt.Errorf("ID token signature verification failed: %w", err)
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode ID token payload: %w", err)
	}
	var claims oidcIDTokenClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("parse ID token claims: %w", err)
	}

	// iss must match the configured issuer (constant-time).
	if subtle.ConstantTimeCompare([]byte(claims.Issuer), []byte(expectedIssuer)) != 1 {
		// Some providers issue with or without a trailing slash; accept an
		// exact match or a slash-normalized match.
		if !strings.EqualFold(strings.TrimRight(claims.Issuer, "/"), strings.TrimRight(expectedIssuer, "/")) {
			return nil, fmt.Errorf("invalid ID token issuer: %q", claims.Issuer)
		}
	}

	// aud must contain the configured client ID (constant-time per element).
	if !audienceContains(claims.Audience, expectedClientID) {
		return nil, errors.New("ID token audience does not include client_id")
	}

	// exp must be in the future.
	if claims.ExpiresAt > 0 && time.Now().Unix() > claims.ExpiresAt {
		return nil, errors.New("ID token expired")
	}

	// nonce, if used, must match the value stashed at authorize time.
	if expectedNonce != "" {
		if subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(expectedNonce)) != 1 {
			return nil, errors.New("ID token nonce mismatch")
		}
	}

	return &claims, nil
}

// audienceContains reports whether aud (a string or []string per RFC 7519)
// contains expect in constant time.
func audienceContains(aud any, expect string) bool {
	switch v := aud.(type) {
	case string:
		return subtle.ConstantTimeCompare([]byte(v), []byte(expect)) == 1
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok && subtle.ConstantTimeCompare([]byte(s), []byte(expect)) == 1 {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if subtle.ConstantTimeCompare([]byte(s), []byte(expect)) == 1 {
				return true
			}
		}
	}
	return false
}

func verifyJWTSignature(alg string, pub crypto.PublicKey, signingInput, signature []byte) error {
	switch alg {
	case "RS256":
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			return errors.New("RS256 token signed with non-RSA key")
		}
		hashed := sha256.Sum256(signingInput)
		return rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, hashed[:], signature)
	case "ES256", "ES384", "ES512":
		ecPub, ok := pub.(*ecdsa.PublicKey)
		if !ok {
			return errors.New("ES* token signed with non-EC key")
		}
		wantBits, hash, err := esAlgParams(alg)
		if err != nil {
			return err
		}
		if ecPub.Curve == nil || ecPub.Curve.Params().BitSize != wantBits {
			return fmt.Errorf("%s token signed with mismatched EC curve (%d bits)", alg, curveBits(ecPub))
		}
		// JWS ECDSA signatures are raw R||S, each field half the key size.
		fieldLen := wantBits / 8
		if len(signature) != 2*fieldLen {
			return fmt.Errorf("%s signature has wrong length: got %d want %d", alg, len(signature), 2*fieldLen)
		}
		r := new(big.Int).SetBytes(signature[:fieldLen])
		s := new(big.Int).SetBytes(signature[fieldLen:])
		var digest []byte
		switch hash {
		case crypto.SHA256:
			h := sha256.Sum256(signingInput)
			digest = h[:]
		case crypto.SHA384:
			h := sha512.Sum384(signingInput)
			digest = h[:]
		case crypto.SHA512:
			h := sha512.Sum512(signingInput)
			digest = h[:]
		}
		if !ecdsa.Verify(ecPub, digest, r, s) {
			return errors.New("ECDSA signature verification failed")
		}
		return nil
	default:
		return fmt.Errorf("unsupported ID token algorithm: %s (only RS256/ES256/ES384/ES512)", alg)
	}
}

func esAlgParams(alg string) (int, crypto.Hash, error) {
	switch alg {
	case "ES256":
		return 256, crypto.SHA256, nil
	case "ES384":
		return 384, crypto.SHA384, nil
	case "ES512":
		return 521, crypto.SHA512, nil // P-521 is 521 bits
	default:
		return 0, 0, fmt.Errorf("unknown ES algorithm: %s", alg)
	}
}

func curveBits(pub *ecdsa.PublicKey) int {
	if pub == nil || pub.Curve == nil || pub.Params() == nil {
		return 0
	}
	return pub.Params().BitSize
}
