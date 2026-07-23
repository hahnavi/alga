package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
)

func newTestKeyring(t *testing.T, kids ...int) *Keyring {
	t.Helper()
	k := &Keyring{keys: map[int][]byte{}}
	if len(kids) == 0 {
		kids = []int{1}
	}
	highest := 0
	for _, kid := range kids {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			t.Fatalf("rand: %v", err)
		}
		k.keys[kid] = key
		highest = max(highest, kid)
	}
	k.activeKID = highest
	pep := make([]byte, 32)
	if _, err := rand.Read(pep); err != nil {
		t.Fatalf("rand: %v", err)
	}
	k.pepper = pep
	return k
}

func TestKeyringRoundTrip(t *testing.T) {
	k := newTestKeyring(t)
	plain := []byte("hello world")

	ct, err := k.Encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(ct, "enc:v1:") {
		t.Fatalf("missing kid prefix: %q", ct)
	}

	pt, err := k.Decrypt(ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(pt) != string(plain) {
		t.Fatalf("got %q want %q", pt, plain)
	}
}

func TestKeyringMultiKID(t *testing.T) {
	k := newTestKeyring(t, 1, 2, 5)
	if active, _ := k.Active(); active != 5 {
		t.Fatalf("expected highest kid as active, got %d", active)
	}

	ct, err := k.Encrypt([]byte("payload"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(ct, "enc:v5:") {
		t.Fatalf("active kid not used in envelope: %q", ct)
	}
}

func TestKeyringDecryptUnknownKID(t *testing.T) {
	k := newTestKeyring(t, 1)
	_, err := k.Decrypt("enc:v9:" + base64.StdEncoding.EncodeToString([]byte("garbage")))
	if !errors.Is(err, ErrUnknownKID) {
		t.Fatalf("expected ErrUnknownKID, got %v", err)
	}
}

func TestKeyringDecryptRejectsLegacyFormat(t *testing.T) {
	k := newTestKeyring(t, 1)
	if _, err := k.Decrypt("enc:abc=="); !errors.Is(err, ErrInvalidCiphertext) {
		t.Fatalf("expected ErrInvalidCiphertext for legacy format, got %v", err)
	}
}

func TestKeyringEmptyInputPassThrough(t *testing.T) {
	k := newTestKeyring(t)
	if got, err := k.Encrypt(nil); err != nil || got != "" {
		t.Fatalf("encrypt empty: got=%q err=%v", got, err)
	}
	if got, err := k.Decrypt(""); err != nil || got != nil {
		t.Fatalf("decrypt empty: got=%v err=%v", got, err)
	}
}

func TestKeyringDisabled(t *testing.T) {
	k := &Keyring{keys: map[int][]byte{}}
	if _, err := k.Encrypt([]byte("x")); !errors.Is(err, ErrEncryptionDisabled) {
		t.Fatalf("expected ErrEncryptionDisabled, got %v", err)
	}
	if _, err := k.Decrypt("enc:v1:abc"); !errors.Is(err, ErrEncryptionDisabled) {
		t.Fatalf("expected ErrEncryptionDisabled on decrypt, got %v", err)
	}
}

func TestHMACDeterministic(t *testing.T) {
	k := newTestKeyring(t)
	a := k.HMACString("session-token-xyz")
	b := k.HMACString("session-token-xyz")
	if a != b {
		t.Fatalf("HMAC not deterministic: %q vs %q", a, b)
	}
	if len(a) != 64 {
		t.Fatalf("expected 64-char hex, got %d", len(a))
	}
}

func TestHMACDistinct(t *testing.T) {
	k := newTestKeyring(t)
	a := k.HMACString("alpha")
	b := k.HMACString("beta")
	if a == b {
		t.Fatalf("HMAC collision on distinct inputs")
	}
}

func TestHMACWithoutPepperPanics(t *testing.T) {
	k := &Keyring{keys: map[int][]byte{}}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when calling HMAC without pepper")
		}
	}()
	_ = k.HMAC([]byte("x"))
}

func TestConstantTimeEqual(t *testing.T) {
	if !ConstantTimeEqual([]byte("abc"), []byte("abc")) {
		t.Fatal("equal bytes returned false")
	}
	if ConstantTimeEqual([]byte("abc"), []byte("abd")) {
		t.Fatal("unequal bytes returned true")
	}
	if ConstantTimeEqual([]byte("abc"), []byte("abcd")) {
		t.Fatal("length mismatch returned true")
	}
}

func TestPasswordHashAndVerify(t *testing.T) {
	SetDefault(newTestKeyring(t))
	t.Cleanup(resetDefault)

	hash, err := HashPassword("CorrectHorseBatteryStaple1!")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("expected argon2id PHC, got %q", hash)
	}

	ok, needsRehash, err := VerifyPassword(hash, "CorrectHorseBatteryStaple1!")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatal("verify returned ok=false for the correct password")
	}
	if needsRehash {
		t.Fatal("freshly-hashed password should not require rehash")
	}

	ok, _, err = VerifyPassword(hash, "wrong-password")
	if err != nil {
		t.Fatalf("verify wrong: %v", err)
	}
	if ok {
		t.Fatal("verify returned ok=true for the wrong password")
	}
}

func TestPasswordRejectsNonArgon2id(t *testing.T) {
	SetDefault(newTestKeyring(t))
	t.Cleanup(resetDefault)

	if _, _, err := VerifyPassword("$2a$10$abcdefghijklmnopqrstuvwxyz", "anything"); err == nil {
		t.Fatal("expected error rejecting bcrypt hash")
	}
}

func TestPasswordNeedsRehashOnWeakerParams(t *testing.T) {
	SetDefault(newTestKeyring(t))
	t.Cleanup(resetDefault)

	weak := passwordParams{Memory: 16 * 1024, Time: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}
	salt := make([]byte, weak.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	prehash := Default().HMAC([]byte("hunter2"))
	hash := encodePHC(weak, salt, runArgonForTest(prehash, salt, weak))

	ok, needsRehash, err := VerifyPassword(hash, "hunter2")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatal("verify failed for weak-params password")
	}
	if !needsRehash {
		t.Fatal("expected needsRehash=true for under-strength params")
	}
}

func TestLoadFromEnv(t *testing.T) {
	key1 := base64.StdEncoding.EncodeToString(make([]byte, 32))
	key2 := base64.StdEncoding.EncodeToString(append(make([]byte, 31), 1))
	t.Setenv("ENCRYPTION_KEYS", "1:"+key1+",2:"+key2)
	t.Setenv("SECRET_PEPPER", base64.StdEncoding.EncodeToString(make([]byte, 32)))

	k, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if active, _ := k.Active(); active != 2 {
		t.Fatalf("expected active=2, got %d", active)
	}
	if !k.PepperEnabled() {
		t.Fatal("pepper not loaded")
	}
}

func TestLoadFromEnvMissing(t *testing.T) {
	t.Setenv("ENCRYPTION_KEYS", "")
	t.Setenv("SECRET_PEPPER", "")
	k, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if k.Enabled() || k.PepperEnabled() {
		t.Fatal("expected disabled keyring when env unset")
	}
	if err := k.Validate(); err == nil {
		t.Fatal("Validate must fail when nothing is loaded")
	}
}

func TestLoadFromEnvRejectsShortKey(t *testing.T) {
	t.Setenv("ENCRYPTION_KEYS", "1:"+base64.StdEncoding.EncodeToString(make([]byte, 16)))
	t.Setenv("SECRET_PEPPER", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("expected error for 16-byte key")
	}
}

func TestLoadFromEnvRejectsShortPepper(t *testing.T) {
	t.Setenv("ENCRYPTION_KEYS", "1:"+base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("SECRET_PEPPER", base64.StdEncoding.EncodeToString(make([]byte, 16)))
	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("expected error for 16-byte pepper")
	}
}

// runArgonForTest mirrors what HashPassword does for a custom params struct so
// the rehash-detection test can fabricate a "weakly" hashed password.
func runArgonForTest(prehash, salt []byte, p passwordParams) []byte {
	return argon2.IDKey(prehash, salt, p.Time, p.Memory, p.Parallelism, p.KeyLen)
}

// resetDefault clears the package-level keyring so subsequent tests get a
// fresh init when they call SetDefault. We use a manual reset because
// sync.Once cannot be re-fired.
func resetDefault() {
	defaultKeyring = nil
	defaultErr = nil
}
