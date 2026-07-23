package slack

import "testing"

func TestMrkdwn_Bold(t *testing.T) {
	got := Mrkdwn("this is **bold** text")
	want := "this is *bold* text"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestMrkdwn_Italic(t *testing.T) {
	got := Mrkdwn("this is __italic__ text")
	want := "this is _italic_ text"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestMrkdwn_Headings(t *testing.T) {
	input := "## Summary\n\n### Details\nsome text"
	got := Mrkdwn(input)
	if strings := []string{"*Summary*", "*Details*"}; !containsAll(got, strings) {
		t.Fatalf("got %q, expected headings converted to bold", got)
	}
}

func TestMrkdwn_BulletList(t *testing.T) {
	input := "- item one\n- item two\n  - nested"
	got := Mrkdwn(input)
	if !containsAll(got, []string{"- item one", "- item two", "- nested"}) {
		t.Fatalf("got %q, expected bullet markers preserved", got)
	}
}

func TestMrkdwn_NumberedList(t *testing.T) {
	input := "1. first\n2. second"
	got := Mrkdwn(input)
	if !containsAll(got, []string{"1. first", "2. second"}) {
		t.Fatalf("got %q, expected numbered markers preserved", got)
	}
}

func TestMrkdwn_CodeUnchanged(t *testing.T) {
	input := "use `code` here"
	got := Mrkdwn(input)
	want := "use `code` here"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestMrkdwn_MixedAgentMessage(t *testing.T) {
	input := "## Summary\n\nI've investigated **INV-20**.\n\n### Results\n\n- No failure found\n- System is __normal__\n\n```\nsome code\n```\n\n1. Step one\n2. Step two"
	got := Mrkdwn(input)
	for _, s := range []string{"*Summary*", "*INV-20*", "*Results*", "- No failure found", "_normal_", "1. Step one"} {
		if !contains(got, s) {
			t.Fatalf("got %q, missing expected %q", got, s)
		}
	}
}

func TestMrkdwn_AlreadySlackBold(t *testing.T) {
	input := "this is *already bold*"
	got := Mrkdwn(input)
	want := "this is *already bold*"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func containsAll(s string, subs []string) bool {
	for _, sub := range subs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
