package capability

import (
	"fmt"
	"slices"
)

const (
	Investigate = "investigate"
	Communicate = "communicate"
	Command     = "command"
	// Secrets gates GET /api/v1/agent/secrets/{id}: fetching shared
	// credentials requires an explicit grant on the token. Existing tokens
	// were grandfathered in via migration 00015; new tokens must opt in.
	Secrets = "secrets"
)

var All = []string{Investigate, Communicate, Command, Secrets}

var validMap = func() map[string]struct{} {
	m := make(map[string]struct{}, len(All))
	for _, c := range All {
		m[c] = struct{}{}
	}
	return m
}()

func IsValid(c string) bool {
	_, ok := validMap[c]
	return ok
}

func Validate(caps []string) error {
	for _, c := range caps {
		if !IsValid(c) {
			return fmt.Errorf("invalid capability: %q", c)
		}
	}
	return nil
}

func Normalize(caps []string) []string {
	if len(caps) == 0 {
		return []string{Investigate}
	}
	seen := make(map[string]struct{}, len(caps))
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		if !IsValid(c) {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	if len(out) == 0 {
		return []string{Investigate}
	}
	slices.Sort(out)
	return out
}

func Has(caps []string, target string) bool {
	return slices.Contains(caps, target)
}

// HasAny reports whether caps contains at least one of the target capabilities.
func HasAny(caps []string, targets ...string) bool {
	for _, c := range caps {
		if slices.Contains(targets, c) {
			return true
		}
	}
	return false
}
