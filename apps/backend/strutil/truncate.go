package strutil

import "strings"

func Truncate(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// Prefix returns at most the first n bytes of s. Unlike Truncate it never
// appends an ellipsis, which is what callers want when building short
// identifiers (e.g. an 8-byte alert-fingerprint display name). If n is
// non-positive or s is already shorter than n, s is returned unchanged.
func Prefix(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}

func TruncateOneLine(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.Join(strings.Fields(s), " ")
	if max > 0 && len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}
