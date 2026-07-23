package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. Defaults follow OWASP 2026 baseline (m=64 MiB, t=3,
// p=2, 16-byte salt, 32-byte tag) and are configurable via env so production
// can tune for ~250–500 ms / hash on actual hardware without recompiling.
//
// Memory is the dominant cost: each in-flight verify holds m KiB of working
// memory for the duration of the call. Combined with hashSemaphore (below)
// this bounds the API process at roughly Memory * MaxConcurrent KiB.
type passwordParams struct {
	Memory      uint32 // KiB
	Time        uint32 // iterations
	Parallelism uint8
	SaltLen     uint32
	KeyLen      uint32
}

func defaultParams() passwordParams {
	return passwordParams{
		Memory:      64 * 1024,
		Time:        3,
		Parallelism: 2,
		SaltLen:     16,
		KeyLen:      32,
	}
}

var (
	pwParamsOnce sync.Once
	pwParams     passwordParams
	pwSemaOnce   sync.Once
	pwSema       chan struct{}
)

// loadPasswordParams resolves runtime parameters from env. Lazy and idempotent;
// callers can read pwParams directly after the first call.
func loadPasswordParams() passwordParams {
	pwParamsOnce.Do(func() {
		pwParams = defaultParams()
		if v := strings.TrimSpace(os.Getenv("ARGON2_MEMORY_KIB")); v != "" {
			if n, err := strconv.ParseUint(v, 10, 32); err == nil && n >= 8*1024 {
				pwParams.Memory = uint32(n)
			}
		}
		if v := strings.TrimSpace(os.Getenv("ARGON2_TIME")); v != "" {
			if n, err := strconv.ParseUint(v, 10, 32); err == nil && n >= 1 {
				pwParams.Time = uint32(n)
			}
		}
		if v := strings.TrimSpace(os.Getenv("ARGON2_PARALLELISM")); v != "" {
			if n, err := strconv.ParseUint(v, 10, 8); err == nil && n >= 1 {
				pwParams.Parallelism = uint8(n)
			}
		}
		if v := strings.TrimSpace(os.Getenv("ARGON2_SALT_LEN")); v != "" {
			if n, err := strconv.ParseUint(v, 10, 32); err == nil && n >= 8 {
				pwParams.SaltLen = uint32(n)
			}
		}
		if v := strings.TrimSpace(os.Getenv("ARGON2_KEY_LEN")); v != "" {
			if n, err := strconv.ParseUint(v, 10, 32); err == nil && n >= 16 {
				pwParams.KeyLen = uint32(n)
			}
		}
	})
	return pwParams
}

// passwordSemaphore caps concurrent argon2id hash/verify operations across
// the process. A sustained login burst would otherwise multiply Memory KiB
// across goroutines and push the API container into OOM. Default is
// runtime.NumCPU * 2; override via MAX_CONCURRENT_PASSWORD_HASHES.
func passwordSemaphore() chan struct{} {
	pwSemaOnce.Do(func() {
		size := runtime.NumCPU() * 2
		if v := strings.TrimSpace(os.Getenv("MAX_CONCURRENT_PASSWORD_HASHES")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				size = n
			}
		}
		pwSema = make(chan struct{}, size)
	})
	return pwSema
}

// HashPassword pre-hashes the plaintext with the server pepper (HMAC-SHA-256)
// and then runs argon2id over the result, returning a self-describing PHC
// string:
//
//	$argon2id$v=19$m=<memory>,t=<time>,p=<parallelism>$<salt_b64>$<hash_b64>
//
// The PHC representation lets VerifyPassword detect parameter drift and
// trigger a transparent rehash on next login when the configured params have
// strengthened.
//
// HashPassword requires the keyring's pepper. It returns an error rather
// than panicking so callers (HTTP handlers) can surface a 500 instead of
// crashing the process.
func HashPassword(plaintext string) (string, error) {
	k := Default()
	if !k.PepperEnabled() {
		return "", errors.New("crypto: HashPassword requires SECRET_PEPPER to be configured")
	}
	if plaintext == "" {
		return "", errors.New("crypto: refusing to hash empty password")
	}

	params := loadPasswordParams()
	sema := passwordSemaphore()
	sema <- struct{}{}
	defer func() { <-sema }()

	salt := make([]byte, params.SaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("crypto: read salt: %w", err)
	}

	prehash := k.HMAC([]byte(plaintext))
	hash := argon2.IDKey(prehash, salt, params.Time, params.Memory, params.Parallelism, params.KeyLen)

	return encodePHC(params, salt, hash), nil
}

// VerifyPassword parses an argon2id PHC string and returns:
//
//   - ok:           the supplied plaintext matched the stored hash
//   - needsRehash:  the stored hash uses weaker params than the current
//     targets (or a different alg entirely), so callers SHOULD
//     re-hash and persist on success
//   - err:          parse/format errors only; a wrong password is (false, _, nil)
//
// VerifyPassword performs constant-time comparison of the decoded hash bytes
// to defeat timing leaks across attempts.
func VerifyPassword(encoded, plaintext string) (ok bool, needsRehash bool, err error) {
	k := Default()
	if !k.PepperEnabled() {
		return false, false, errors.New("crypto: VerifyPassword requires SECRET_PEPPER to be configured")
	}
	if encoded == "" || plaintext == "" {
		return false, false, nil
	}

	storedParams, salt, want, err := decodePHC(encoded)
	if err != nil {
		return false, false, err
	}

	sema := passwordSemaphore()
	sema <- struct{}{}
	defer func() { <-sema }()

	prehash := k.HMAC([]byte(plaintext))
	got := argon2.IDKey(prehash, salt, storedParams.Time, storedParams.Memory, storedParams.Parallelism, uint32(len(want))) //#nosec G115 -- []byte length cannot overflow uint32

	if !ConstantTimeEqual(got, want) {
		return false, false, nil
	}

	target := loadPasswordParams()
	needsRehash = storedParams.Memory < target.Memory ||
		storedParams.Time < target.Time ||
		storedParams.Parallelism < target.Parallelism ||
		uint32(len(want)) < target.KeyLen || //#nosec G115 -- []byte length cannot overflow uint32
		uint32(len(salt)) < target.SaltLen //#nosec G115 -- []byte length cannot overflow uint32
	return true, needsRehash, nil
}

func encodePHC(p passwordParams, salt, hash []byte) string {
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		p.Memory, p.Time, p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

// decodePHC accepts the standard argon2id PHC string. It rejects
// non-argon2id variants (argon2i, argon2d) and anything that isn't the
// expected version so legacy bcrypt or hand-rolled formats fail loudly
// rather than silently accepting weak hashes.
func decodePHC(encoded string) (passwordParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" {
		return passwordParams{}, nil, nil, errors.New("crypto: invalid PHC string (segment count)")
	}
	if parts[1] != "argon2id" {
		return passwordParams{}, nil, nil, fmt.Errorf("crypto: unsupported password algorithm %q (expected argon2id)", parts[1])
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return passwordParams{}, nil, nil, fmt.Errorf("crypto: invalid PHC version field: %w", err)
	}
	if version != argon2.Version {
		return passwordParams{}, nil, nil, fmt.Errorf("crypto: unsupported argon2 version %d", version)
	}

	var p passwordParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Parallelism); err != nil {
		return passwordParams{}, nil, nil, fmt.Errorf("crypto: invalid PHC params: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return passwordParams{}, nil, nil, fmt.Errorf("crypto: invalid PHC salt: %w", err)
	}
	p.SaltLen = uint32(len(salt)) //#nosec G115 -- []byte length cannot overflow uint32
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return passwordParams{}, nil, nil, fmt.Errorf("crypto: invalid PHC hash: %w", err)
	}
	p.KeyLen = uint32(len(hash)) //#nosec G115 -- []byte length cannot overflow uint32
	return p, salt, hash, nil
}
