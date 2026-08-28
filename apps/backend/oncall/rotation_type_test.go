package oncall

import "testing"

// TestValidRotationType pins the rotation-type vocabulary shared by the
// resolver, the schedule_layers CHECK constraint (migration 00016), and the
// PATCH /on-call/schedules layer validation. The legacy `custom` value was
// folded to weekly by that migration and must stay rejected.
func TestValidRotationType(t *testing.T) {
	t.Parallel()
	valid := []string{"hourly", "daily", "weekly", "monthly"}
	for _, rt := range valid {
		if !ValidRotationType(rt) {
			t.Errorf("ValidRotationType(%q) = false, want true", rt)
		}
	}

	invalid := []string{"", "custom", "yearly", "WEEKLY", "biweekly"}
	for _, rt := range invalid {
		if ValidRotationType(rt) {
			t.Errorf("ValidRotationType(%q) = true, want false", rt)
		}
	}
}
