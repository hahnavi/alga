package matching

import (
	"regexp"
	"strings"
	"sync"
)

// regexCacheEntry is the cached outcome of compiling a pattern. A nil Regexp
// with Err != nil poisons the pattern so subsequent calls fail fast.
type regexCacheEntry struct {
	Regexp *regexp.Regexp
	Err    error
}

var (
	regexCacheMu sync.RWMutex
	regexCache   = make(map[string]regexCacheEntry, maxRegexCacheSize)
)

const maxRegexCacheSize = 256

// GetCompiledRegex returns a cached compiled regexp for pattern. Subsequent
// calls with the same pattern return the cached value; failed compilations are
// poisoned (cached as a non-nil Err) so a malformed pattern fails fast. Cache
// hits take only a read lock so concurrent condition evaluation stays parallel;
// the cache is bounded to maxRegexCacheSize entries.
func GetCompiledRegex(pattern string) (*regexp.Regexp, error) {
	regexCacheMu.RLock()
	entry, ok := regexCache[pattern]
	regexCacheMu.RUnlock()
	if ok {
		return entry.Regexp, entry.Err
	}

	regexCacheMu.Lock()
	defer regexCacheMu.Unlock()

	// Re-check: another goroutine may have compiled it between the RUnlock and Lock.
	if entry, ok := regexCache[pattern]; ok {
		return entry.Regexp, entry.Err
	}

	// Bound the cache: when full, drop roughly half (the oldest insertions,
	// since Go map iteration is insertion-ordered for the eviction pass).
	if len(regexCache) >= maxRegexCacheSize {
		drop := maxRegexCacheSize / 2
		for key := range regexCache {
			delete(regexCache, key)
			drop--
			if drop == 0 {
				break
			}
		}
	}

	re, err := regexp.Compile(pattern)
	regexCache[pattern] = regexCacheEntry{Regexp: re, Err: err}
	if err != nil {
		return nil, err
	}
	return re, nil
}

func MatchCondition(fieldValue, operator, pattern string) bool {
	switch operator {
	case "exact":
		return fieldValue == pattern
	case "contains":
		return strings.Contains(fieldValue, pattern)
	case "prefix":
		return strings.HasPrefix(fieldValue, pattern)
	case "suffix":
		return strings.HasSuffix(fieldValue, pattern)
	case "regex":
		if len(pattern) > 256 {
			return false
		}
		re, err := GetCompiledRegex(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(fieldValue)
	case "exists":
		return fieldValue != ""
	case "not_exists":
		return fieldValue == ""
	case "wildcard":
		return WildcardMatch(pattern, fieldValue)
	default:
		return fieldValue == pattern
	}
}

func WildcardMatch(pattern, s string) bool {
	if !strings.Contains(pattern, "*") {
		return s == pattern
	}
	parts := strings.Split(pattern, "*")

	if parts[0] != "" && !strings.HasPrefix(s, parts[0]) {
		return false
	}
	if parts[len(parts)-1] != "" && !strings.HasSuffix(s, parts[len(parts)-1]) {
		return false
	}

	pos := 0
	for _, p := range parts {
		if p == "" {
			continue
		}
		idx := strings.Index(s[pos:], p)
		if idx < 0 {
			return false
		}
		pos += idx + len(p)
	}
	return true
}
