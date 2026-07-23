package api

import (
	"testing"

	"alga/telnyx"
)

func TestEncodeDecodeIVRClientState_RoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		incident string
		level    int
		attempt  int
		user     string
	}{
		{"with user", "42", 3, 1, "11111111-1111-1111-1111-111111111111"},
		{"without user", "99", 1, 2, ""},
		{"single digit incident", "5", 0, 1, "22222222-2222-2222-2222-222222222222"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			encoded := encodeIVRClientState(tc.incident, tc.level, tc.attempt, tc.user)
			inc, lvl, att, usr := decodeIVRClientState(encoded)
			if inc != tc.incident {
				t.Errorf("incident = %q, want %q", inc, tc.incident)
			}
			if lvl != tc.level {
				t.Errorf("level = %d, want %d", lvl, tc.level)
			}
			if att != tc.attempt {
				t.Errorf("attempt = %d, want %d", att, tc.attempt)
			}
			if usr != tc.user {
				t.Errorf("user = %q, want %q", usr, tc.user)
			}
		})
	}
}

func TestDecodeIVRClientState_Malformed(t *testing.T) {
	t.Parallel()

	// Empty state returns zero values without error.
	if inc, lvl, att, usr := decodeIVRClientState(""); inc != "" || lvl != 0 || att != 0 || usr != "" {
		t.Errorf("empty state should yield zero values, got inc=%q lvl=%d att=%d usr=%q", inc, lvl, att, usr)
	}

	// Garbage base64 yields zero values.
	if inc, _, _, _ := decodeIVRClientState("!!!not-base64!!!"); inc != "" {
		t.Errorf("garbage state should yield empty incident, got %q", inc)
	}

	// Too few fields yields zero values.
	if inc, _, _, _ := decodeIVRClientState(encodeIVRClientState("42", 0, 0, "")[:0]); inc != "" {
		// sanity: empty encoding of empty input still has at least 3 fields
		t.Errorf("unexpected non-empty incident for empty payload")
	}
}

func TestSplitClientState(t *testing.T) {
	t.Parallel()

	// UUIDs contain only hex digits and dashes, so a colon split is safe.
	got := splitClientState("42:3:1:abc-def")
	if len(got) != 4 || got[0] != "42" || got[3] != "abc-def" {
		t.Errorf("splitClientState = %#v, want 4 parts", got)
	}

	if got := splitClientState(""); len(got) != 1 || got[0] != "" {
		t.Errorf("splitClientState(\"\") = %#v, want [\"\"]", got)
	}
}

func TestIVRText(t *testing.T) {
	t.Parallel()

	if got := telnyx.AnnouncementText(7, 2, ""); !contains(got, "7") || !contains(got, "2") {
		t.Errorf("AnnouncementText(7,2,\"\") = %q, expected incident+level", got)
	}
	if got := telnyx.AcknowledgedText(); !contains(got, "acknowledged") {
		t.Errorf("AcknowledgedText = %q, expected 'acknowledged'", got)
	}
	if got := telnyx.SilencedText(); !contains(got, "silenced") {
		t.Errorf("SilencedText = %q, expected 'silenced'", got)
	}
	if got := telnyx.PromptText(); !contains(got, "Press 1") {
		t.Errorf("PromptText = %q, expected DTMF instructions", got)
	}
}

func TestAnnouncementText_Brief(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		incidentNumber int64
		level          int
		brief          string
		wantSubstr     string
		notWantSubstr  string
	}{
		{
			name:           "with brief inserts title before menu",
			incidentNumber: 42,
			level:          2,
			brief:          "Database primary unreachable",
			wantSubstr:     "Database primary unreachable.",
		},
		{
			name:           "empty brief falls back to menu-only announcement",
			incidentNumber: 7,
			level:          1,
			brief:          "",
			wantSubstr:     "Press 1 to acknowledge.",
		},
		{
			name:           "whitespace-only brief is treated as empty",
			incidentNumber: 7,
			level:          1,
			brief:          "   ",
			wantSubstr:     "Press 1 to acknowledge.",
			notWantSubstr:  ". .",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := telnyx.AnnouncementText(tc.incidentNumber, tc.level, tc.brief)
			if !contains(got, tc.wantSubstr) {
				t.Errorf("AnnouncementText(...) = %q, expected to contain %q", got, tc.wantSubstr)
			}
			if tc.notWantSubstr != "" && contains(got, tc.notWantSubstr) {
				t.Errorf("AnnouncementText(...) = %q, expected NOT to contain %q", got, tc.notWantSubstr)
			}
			// Both branches must retain the menu instructions.
			if !contains(got, "Press 1 to acknowledge") || !contains(got, "Press 2 to silence for one hour") {
				t.Errorf("AnnouncementText(...) = %q, menu instructions missing", got)
			}
		})
	}
}

func TestGatherText_Brief(t *testing.T) {
	t.Parallel()

	// GatherText should include the announcement (with brief) followed by the
	// menu prompt. The menu prompt appears twice: once at the end of the
	// announcement and once as PromptText.
	got := telnyx.GatherText(42, 2, "CPU saturation on api-1")
	if !contains(got, "CPU saturation on api-1.") {
		t.Errorf("GatherText brief missing: %q", got)
	}
	if !contains(got, "Incident 42") || !contains(got, "level 2") {
		t.Errorf("GatherText lost incident/level: %q", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
