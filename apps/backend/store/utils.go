package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// normalizeLower returns the trimmed, lowercased form of s. A small shared
// helper for case-insensitive enum/identifier normalization across stores.
func normalizeLower(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// generateSecureToken generates a cryptographically secure random hex token
func generateSecureToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
