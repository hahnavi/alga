package matching

import (
	"testing"
)

func TestGetCompiledRegexCachesSuccessfulCompile(t *testing.T) {
	t.Parallel()
	pattern := "^cache-test-1[0-9a-f]+$"
	re1, err := GetCompiledRegex(pattern)
	if err != nil {
		t.Fatalf("GetCompiledRegex: %v", err)
	}
	if re1 == nil {
		t.Fatalf("expected non-nil compiled regex")
	}
	re2, err := GetCompiledRegex(pattern)
	if err != nil {
		t.Fatalf("GetCompiledRegex (cached): %v", err)
	}
	if re1 != re2 {
		t.Errorf("expected cached regex; got fresh instance")
	}
	if !re2.MatchString("cache-test-1abcdef") {
		t.Errorf("cached regex should match valid input")
	}
}

func TestGetCompiledRegexPoisonCachesFailedCompile(t *testing.T) {
	t.Parallel()
	pattern := "[invalid-poison-test-pattern"
	_, err := GetCompiledRegex(pattern)
	if err == nil {
		t.Fatalf("expected error for invalid pattern")
	}
	_, err2 := GetCompiledRegex(pattern)
	if err2 == nil {
		t.Errorf("expected cached error for invalid pattern")
	}
}
