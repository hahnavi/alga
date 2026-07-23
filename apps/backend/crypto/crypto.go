// Package crypto centralizes all secret-handling primitives for Alga: an AEAD
// keyring with per-message key versioning, a server-wide HMAC pepper, and
// constant-time helpers. All other packages MUST go through this package
// rather than re-implementing AES, HMAC, or comparison logic.
//
// All keys are loaded once from environment variables at process start (see
// LoadFromEnv). The crypto package never reads from disk or makes network
// calls; it has no external dependencies beyond the Go standard library and
// golang.org/x/crypto.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Ciphertext format:
//
//	enc:v<kid>:<base64(nonce|ct)>
//
// kid is a positive integer, allowing key rotation without rewriting history.
// nonce length is gcm.NonceSize() (12 bytes); ciphertext is appended in-place
// by gcm.Seal so reads slice it back out by nonce length.
const (
	cipherPrefix = "enc:v"
	cipherSep    = ":"
)

// ErrEncryptionDisabled is returned when an Encrypt/Decrypt call is made on a
// keyring that was never initialized with key material.
var ErrEncryptionDisabled = errors.New("crypto: encryption disabled (no keys loaded)")

// ErrUnknownKID is returned when a ciphertext references a key id that is not
// loaded. Operationally this means the active keyring is missing a historical
// key required to decrypt old data; restore the missing key and retry.
var ErrUnknownKID = errors.New("crypto: unknown key id")

// ErrInvalidCiphertext means the input is missing the enc:v<kid>: header,
// has malformed base64, or the AEAD tag failed verification.
var ErrInvalidCiphertext = errors.New("crypto: invalid ciphertext")

// Keyring holds the active and historical AES-256 keys plus the HMAC pepper.
//
// A Keyring is safe for concurrent use after construction. Construction is not
// atomic across goroutines; the package-level Default() helper ensures init
// runs exactly once via sync.Once.
type Keyring struct {
	keys      map[int][]byte
	activeKID int
	pepper    []byte
}

// Active reports the active key id and its raw bytes. The returned slice MUST
// NOT be mutated; callers needing a copy should copy explicitly.
func (k *Keyring) Active() (int, []byte) {
	return k.activeKID, k.keys[k.activeKID]
}

// Enabled reports whether the keyring has any key material loaded. A keyring
// without keys still works for HMAC if a pepper was loaded — that's checked
// separately via PepperEnabled.
func (k *Keyring) Enabled() bool {
	return k != nil && len(k.keys) > 0 && k.activeKID > 0
}

// PepperEnabled reports whether HMAC pepper material is available. HMAC
// without a pepper is a programming error in this codebase: every call site
// expects the server-side pepper to be present.
func (k *Keyring) PepperEnabled() bool {
	return k != nil && len(k.pepper) > 0
}

// Encrypt seals plaintext with the active key and returns the
// "enc:v<kid>:<b64(nonce|ct)>" envelope. Empty plaintext is returned as the
// empty string unchanged so callers can store optional fields without writing
// a wrapper.
func (k *Keyring) Encrypt(plaintext []byte) (string, error) {
	if len(plaintext) == 0 {
		return "", nil
	}
	if !k.Enabled() {
		return "", ErrEncryptionDisabled
	}

	key := k.keys[k.activeKID]
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: new gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: read nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)

	return cipherPrefix + strconv.Itoa(k.activeKID) + cipherSep + base64.StdEncoding.EncodeToString(sealed), nil
}

// EncryptString is a convenience over Encrypt for the common case of a UTF-8
// secret stored as a Go string.
func (k *Keyring) EncryptString(plaintext string) (string, error) {
	return k.Encrypt([]byte(plaintext))
}

// Decrypt reverses Encrypt. Empty input is returned as nil unchanged.
func (k *Keyring) Decrypt(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	if !k.Enabled() {
		return nil, ErrEncryptionDisabled
	}

	if !strings.HasPrefix(s, cipherPrefix) {
		return nil, ErrInvalidCiphertext
	}
	rest := s[len(cipherPrefix):]
	idx := strings.IndexByte(rest, ':')
	if idx <= 0 {
		return nil, ErrInvalidCiphertext
	}
	kid, err := strconv.Atoi(rest[:idx])
	if err != nil || kid <= 0 {
		return nil, ErrInvalidCiphertext
	}
	key, ok := k.keys[kid]
	if !ok {
		return nil, fmt.Errorf("%w: %d", ErrUnknownKID, kid)
	}

	data, err := base64.StdEncoding.DecodeString(rest[idx+1:])
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCiphertext, err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return nil, ErrInvalidCiphertext
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCiphertext, err)
	}
	return pt, nil
}

// DecryptString is the string-typed companion to Decrypt.
func (k *Keyring) DecryptString(s string) (string, error) {
	pt, err := k.Decrypt(s)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// HMAC returns HMAC-SHA-256(pepper, data). Caller MUST treat the result as
// opaque bytes; the digest length is 32. Used to derive lookup-safe hashes
// for long-lived bearer tokens, session ids, and refresh tokens.
//
// HMAC panics if no pepper is loaded. This is intentional: every caller
// expects pepper-protected hashing, and silently degrading to a bare SHA-256
// would be a security regression. Surface the misconfiguration loudly at
// startup, not as a quiet boot success followed by weak hashes.
func (k *Keyring) HMAC(data []byte) []byte {
	if !k.PepperEnabled() {
		panic("crypto: HMAC called without pepper loaded")
	}
	mac := hmac.New(sha256.New, k.pepper)
	mac.Write(data)
	return mac.Sum(nil)
}

// HMACString hashes the UTF-8 bytes of s and returns the digest as
// lowercase hex (compact, BSON/JSON-safe, equality-comparable in queries).
func (k *Keyring) HMACString(s string) string {
	return hexEncode(k.HMAC([]byte(s)))
}

// PlainSHA256Hex returns the lowercase hex SHA-256 of s without the pepper.
//
// Use only for non-secret derivations such as the lookup_prefix on token
// records: the prefix is short enough that brute force from the prefix is
// trivial, so we do not gain anything from using HMAC, and using plain SHA
// keeps the value computable from the plaintext alone (e.g. by rotation
// tooling that doesn't have the runtime pepper).
func PlainSHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hexEncode(sum[:])
}

// ConstantTimeEqual compares two byte slices in constant time. Returns false
// for length mismatches without leaking which side was longer.
func ConstantTimeEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// ConstantTimeEqualString is the string-typed form of ConstantTimeEqual.
// Strings of different lengths return false; same-length strings are compared
// byte-by-byte without an early exit.
func ConstantTimeEqualString(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Default returns the lazily-initialized process-wide keyring. The first
// caller triggers LoadFromEnv; subsequent callers read the cached result.
//
// The init error is captured so callers (typically the cmd/main bootstrap
// path) can surface it via Validate().
var (
	defaultKeyring *Keyring
	defaultErr     error
	defaultOnce    sync.Once
)

// Default returns the process-wide keyring, initializing it on first call.
//
// In tests that need to override the env-derived keyring, call SetDefault
// explicitly before any code path that calls Default().
func Default() *Keyring {
	defaultOnce.Do(func() {
		defaultKeyring, defaultErr = LoadFromEnv()
	})
	return defaultKeyring
}

// DefaultErr returns any error captured by the lazy init in Default().
// Bootstrap code should call Default() and then DefaultErr() to surface
// configuration problems with a useful message.
func DefaultErr() error {
	_ = Default()
	return defaultErr
}

// SetDefault overrides the process-wide keyring. Tests use this to install a
// deterministic keyring; production code never should.
func SetDefault(k *Keyring) {
	defaultOnce.Do(func() {}) // mark sync.Once as fired
	defaultKeyring = k
	defaultErr = nil
}

// LoadFromEnv builds a Keyring from environment variables.
//
// Recognized variables (highest precedence first):
//
//	ENCRYPTION_KEYS  Comma-separated list of "kid:base64(32B)" pairs. The
//	                 highest kid is the active key. All listed kids are
//	                 retained so historical ciphertexts decrypt.
//	SECRET_PEPPER    base64(>=32B) HMAC pepper. Required for any HMAC use
//	                 (sessions, bearer tokens, password pre-hash).
//
// LoadFromEnv returns a usable Keyring even when no env is set so non-prod
// developer setups boot without keys; callers in production paths MUST
// invoke Validate (or the higher-level config.Validate) to refuse such a
// configuration.
func LoadFromEnv() (*Keyring, error) {
	k := &Keyring{keys: map[int][]byte{}}

	rawKeys := strings.TrimSpace(os.Getenv("ENCRYPTION_KEYS"))
	if rawKeys != "" {
		for _, raw := range splitAndTrim(rawKeys, ",") {
			kid, key, err := parseKIDPair(raw)
			if err != nil {
				return nil, err
			}
			if _, dup := k.keys[kid]; dup {
				return nil, fmt.Errorf("crypto: duplicate kid %d in ENCRYPTION_KEYS", kid)
			}
			k.keys[kid] = key
		}
	}

	if len(k.keys) > 0 {
		kids := make([]int, 0, len(k.keys))
		for kid := range k.keys {
			kids = append(kids, kid)
		}
		slices.Sort(kids)
		k.activeKID = kids[len(kids)-1]
	}

	if pep := strings.TrimSpace(os.Getenv("SECRET_PEPPER")); pep != "" {
		decoded, err := base64.StdEncoding.DecodeString(pep)
		if err != nil {
			return nil, fmt.Errorf("crypto: SECRET_PEPPER: %w", err)
		}
		if len(decoded) < 32 {
			return nil, fmt.Errorf("crypto: SECRET_PEPPER must decode to >= 32 bytes (got %d)", len(decoded))
		}
		k.pepper = decoded
	}

	return k, nil
}

// Validate enforces production invariants: keys present, pepper present,
// active key matches an existing entry. Bootstrap code calls this when
// running in ENVIRONMENT=production to fail closed.
func (k *Keyring) Validate() error {
	if !k.Enabled() {
		return errors.New("crypto: ENCRYPTION_KEYS is required; generate with `openssl rand -base64 32`")
	}
	if !k.PepperEnabled() {
		return errors.New("crypto: SECRET_PEPPER is required; generate with `openssl rand -base64 32`")
	}
	if _, ok := k.keys[k.activeKID]; !ok {
		return fmt.Errorf("crypto: active kid %d is not present in keyring", k.activeKID)
	}
	return nil
}

func parseKIDPair(raw string) (int, []byte, error) {
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 {
		return 0, nil, fmt.Errorf("crypto: ENCRYPTION_KEYS entry %q must be kid:base64key", raw)
	}
	kid, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || kid <= 0 {
		return 0, nil, fmt.Errorf("crypto: ENCRYPTION_KEYS entry %q has invalid kid", raw)
	}
	key, err := decodeKey(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, nil, fmt.Errorf("crypto: ENCRYPTION_KEYS kid %d: %w", kid, err)
	}
	return kid, key, nil
}

func decodeKey(b64 string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64: %w", err)
	}
	if len(decoded) != 32 {
		return nil, fmt.Errorf("key must decode to exactly 32 bytes (got %d)", len(decoded))
	}
	return decoded, nil
}

func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

const hexAlphabet = "0123456789abcdef"

// hexEncode is a small, allocation-free hex encoder used for HMAC outputs.
// We avoid encoding/hex to keep this package's dep graph minimal.
func hexEncode(b []byte) string {
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexAlphabet[v>>4]
		out[i*2+1] = hexAlphabet[v&0x0f]
	}
	return string(out)
}
