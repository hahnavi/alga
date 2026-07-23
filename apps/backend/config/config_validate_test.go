package config

import (
	"testing"

	algacrypto "alga/crypto"
)

// TestValidateFailsClosedWithoutCryptoAnyEnv verifies M4: startup fails when
// crypto config (ENCRYPTION_KEYS / SECRET_PEPPER) is absent, regardless of the
// ENVIRONMENT value — including when ENVIRONMENT is unset (ASVS V2.1/V6.2,
// SPEC gap M4). Previously the crypto gate was prod-only, so a missing/typo'd
// ENVIRONMENT silently disabled enforcement.
func TestValidateFailsClosedWithoutCryptoAnyEnv(t *testing.T) {
	cases := []struct {
		name        string
		environment string
	}{
		{"unset_env", ""},
		{"dev_env", "development"},
		{"prod_env", "production"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ENVIRONMENT", tc.environment)
			t.Setenv("ENCRYPTION_KEYS", "")
			t.Setenv("SECRET_PEPPER", "")
			// Install a keyring that reflects the absent config (disabled,
			// no pepper) so Validate's keyring.Validate() fails regardless of
			// any prior test's SetDefault.
			k, err := algacrypto.LoadFromEnv()
			if err != nil {
				t.Fatalf("LoadFromEnv with empty env: %v", err)
			}
			algacrypto.SetDefault(k)

			cfg := Defaults()
			cfg.Environment = tc.environment

			err = cfg.Validate()
			if err == nil {
				t.Fatalf("Validate must fail when crypto config is absent (env=%q)", tc.environment)
			}
		})
	}
}

// TestValidatePassesWithCryptoConfig confirms a valid crypto config allows
// startup in a non-prod environment.
func TestValidatePassesWithCryptoConfig(t *testing.T) {
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("ENCRYPTION_KEYS", "1:MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=")
	t.Setenv("SECRET_PEPPER", "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=")
	// The keyring is a process-global singleton initialized once; reset it with
	// the test env before Validate reads it.
	k, err := algacrypto.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	algacrypto.SetDefault(k)

	cfg := Defaults()
	cfg.Environment = "development"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate should pass with crypto config present: %v", err)
	}
}
