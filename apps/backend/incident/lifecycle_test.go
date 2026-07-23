package incident

import "testing"

func TestIsValidTransition_ValidTransitions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		from, to string
	}{
		{"detected", "triaging"},
		{"detected", "active"},
		{"detected", "cancelled"},
		{"active", "mitigated"},
		{"active", "resolved"},
		{"active", "cancelled"},
		{"mitigated", "resolved"},
		{"mitigated", "active"},
		{"resolved", "closed"},
		{"resolved", "active"},
		{"closed", "active"},
	}
	for _, tc := range cases {
		if !IsValidTransition(tc.from, tc.to) {
			t.Errorf("IsValidTransition(%q, %q) = false, want true", tc.from, tc.to)
		}
	}
}

func TestIsValidTransition_InvalidTransitions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		from, to string
	}{
		{"cancelled", "detected"},
		{"detected", "closed"},
		{"active", "detected"},
		{"mitigated", "detected"},
		{"resolved", "mitigated"},
		{"mitigated", "cancelled"},
		{"resolved", "cancelled"},
		{"closed", "resolved"},
		{"closed", "cancelled"},
	}
	for _, tc := range cases {
		if IsValidTransition(tc.from, tc.to) {
			t.Errorf("IsValidTransition(%q, %q) = true, want false", tc.from, tc.to)
		}
	}
}

func TestIsValidTransition_UnknownStatus(t *testing.T) {
	t.Parallel()
	if IsValidTransition("nonexistent", "active") {
		t.Error("IsValidTransition with unknown 'from' should return false")
	}
	if IsValidTransition("active", "nonexistent") {
		t.Error("IsValidTransition with unknown 'to' should return false")
	}
}

func TestIsValidTransition_SameStatus(t *testing.T) {
	t.Parallel()
	states := []string{"detected", "active", "mitigated", "resolved", "closed", "cancelled"}
	for _, s := range states {
		if IsValidTransition(s, s) {
			t.Errorf("IsValidTransition(%q, %q) = true, want false (no self-transition)", s, s)
		}
	}
}

func TestAvailableTransitions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status string
		want   []string
	}{
		{"detected", []string{"triaging", "active", "cancelled"}},
		{"active", []string{"mitigated", "resolved", "cancelled"}},
		{"mitigated", []string{"resolved", "active"}},
		{"resolved", []string{"closed", "active"}},
		{"closed", []string{"active"}},
		{"cancelled", []string{}},
		{"unknown", nil},
	}
	for _, tc := range cases {
		got := AvailableTransitions(tc.status)
		if tc.want == nil {
			if got != nil {
				t.Errorf("AvailableTransitions(%q) = %v, want nil", tc.status, got)
			}
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("AvailableTransitions(%q) = %v, want %v", tc.status, got, tc.want)
			continue
		}
		for i, v := range got {
			if v != tc.want[i] {
				t.Errorf("AvailableTransitions(%q)[%d] = %q, want %q", tc.status, i, v, tc.want[i])
			}
		}
	}
}

func TestAvailableTransitions_ReturnsCopy(t *testing.T) {
	t.Parallel()
	orig := AvailableTransitions("detected")
	orig[0] = "modified"
	fresh := AvailableTransitions("detected")
	if fresh[0] == "modified" {
		t.Error("AvailableTransitions should return a copy, not the underlying slice")
	}
}

func TestDetectedTransitions(t *testing.T) {
	if !IsValidTransition("detected", "triaging") {
		t.Error("detected should transition to triaging")
	}
	if !IsValidTransition("detected", "cancelled") {
		t.Error("detected should transition to cancelled")
	}
	if !IsValidTransition("detected", "active") {
		t.Error("detected should transition directly to active (auto-confirm)")
	}
}

func TestTriagingTransitions(t *testing.T) {
	if !IsValidTransition("triaging", "active") {
		t.Error("triaging should transition to active")
	}
	if !IsValidTransition("triaging", "cancelled") {
		t.Error("triaging should transition to cancelled")
	}
	if IsValidTransition("triaging", "mitigated") {
		t.Error("triaging should NOT transition to mitigated")
	}
}

func TestAvailableTransitionsDetected(t *testing.T) {
	transitions := AvailableTransitions("detected")
	expected := []string{"triaging", "active", "cancelled"}
	if len(transitions) != len(expected) {
		t.Fatalf("expected %d transitions for detected, got %d", len(expected), len(transitions))
	}
	for _, e := range expected {
		found := false
		for _, tr := range transitions {
			if tr == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected transition %q not found for detected", e)
		}
	}
}

func TestCancelledIsTerminal(t *testing.T) {
	t.Parallel()
	transitions := AvailableTransitions("cancelled")
	if len(transitions) != 0 {
		t.Errorf("'cancelled' should be terminal but has transitions: %v", transitions)
	}
}

func TestDetected_InvalidTransitions(t *testing.T) {
	t.Parallel()
	invalid := []struct{ from, to string }{
		{"detected", "detected"},
		{"detected", "mitigated"},
		{"detected", "resolved"},
		{"detected", "closed"},
	}
	for _, tc := range invalid {
		if IsValidTransition(tc.from, tc.to) {
			t.Errorf("IsValidTransition(%q, %q) = true, want false", tc.from, tc.to)
		}
	}
}

func TestTriaging_InvalidTransitions(t *testing.T) {
	t.Parallel()
	invalid := []struct{ from, to string }{
		{"triaging", "triaging"},
		{"triaging", "mitigated"},
		{"triaging", "resolved"},
		{"triaging", "closed"},
		{"triaging", "detected"},
	}
	for _, tc := range invalid {
		if IsValidTransition(tc.from, tc.to) {
			t.Errorf("IsValidTransition(%q, %q) = true, want false", tc.from, tc.to)
		}
	}
}

func TestAvailableTransitions_Triaging(t *testing.T) {
	transitions := AvailableTransitions("triaging")
	expected := []string{"active", "cancelled"}
	if len(transitions) != len(expected) {
		t.Fatalf("triaging: expected %d transitions, got %d", len(expected), len(transitions))
	}
	for i, v := range expected {
		if transitions[i] != v {
			t.Errorf("AvailableTransitions(triaging)[%d] = %q, want %q", i, transitions[i], v)
		}
	}
}

func TestDetectedAndTriaging_SelfTransitionBlocked(t *testing.T) {
	t.Parallel()
	if IsValidTransition("detected", "detected") {
		t.Error("detected should not self-transition")
	}
	if IsValidTransition("triaging", "triaging") {
		t.Error("triaging should not self-transition")
	}
}

func TestFullLifecycle_DetectedToClosed(t *testing.T) {
	t.Parallel()
	steps := []struct{ from, to string }{
		{"detected", "triaging"},
		{"triaging", "active"},
		{"active", "mitigated"},
		{"mitigated", "resolved"},
		{"resolved", "closed"},
	}
	for _, step := range steps {
		if !IsValidTransition(step.from, step.to) {
			t.Errorf("IsValidTransition(%q, %q) = false, want true", step.from, step.to)
		}
	}
}
