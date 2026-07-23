package incident

import "testing"

func TestComputePriority(t *testing.T) {
	tests := []struct {
		severity string
		impact   string
		want     string
	}{
		{"critical", "high", "P1"},
		{"critical", "medium", "P2"},
		{"critical", "low", "P3"},
		{"high", "high", "P2"},
		{"high", "medium", "P3"},
		{"high", "low", "P4"},
		{"warning", "high", "P3"},
		{"warning", "medium", "P4"},
		{"warning", "low", "P5"},
		{"info", "high", "P4"},
		{"info", "medium", "P5"},
		{"info", "low", "P5"},
	}
	for _, tt := range tests {
		got := ComputePriority(tt.severity, tt.impact)
		if got != tt.want {
			t.Errorf("ComputePriority(%q, %q) = %q, want %q", tt.severity, tt.impact, got, tt.want)
		}
	}
}

func TestComputePriorityInvalidInputs(t *testing.T) {
	tests := []struct {
		severity string
		impact   string
	}{
		{"unknown", "high"},
		{"critical", "unknown"},
		{"", "medium"},
		{"critical", ""},
	}
	for _, tt := range tests {
		got := ComputePriority(tt.severity, tt.impact)
		if got != "P5" {
			t.Errorf("ComputePriority(%q, %q) = %q, want P5 (fallback)", tt.severity, tt.impact, got)
		}
	}
}

func TestValidSeverity(t *testing.T) {
	for _, s := range []string{"critical", "high", "warning", "info"} {
		if !ValidSeverity(s) {
			t.Errorf("ValidSeverity(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"P1", "unknown", "", "page"} {
		if ValidSeverity(s) {
			t.Errorf("ValidSeverity(%q) = true, want false", s)
		}
	}
}

func TestValidImpact(t *testing.T) {
	for _, s := range []string{"high", "medium", "low"} {
		if !ValidImpact(s) {
			t.Errorf("ValidImpact(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"none", "unknown", "", "P1"} {
		if ValidImpact(s) {
			t.Errorf("ValidImpact(%q) = true, want false", s)
		}
	}
}

func TestValidPriority(t *testing.T) {
	for _, p := range []string{"P1", "P2", "P3", "P4", "P5"} {
		if !ValidPriority(p) {
			t.Errorf("ValidPriority(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"P0", "P6", "", "critical"} {
		if ValidPriority(p) {
			t.Errorf("ValidPriority(%q) = true, want false", p)
		}
	}
}

func TestSeverityRank(t *testing.T) {
	tests := []struct {
		severity string
		want     int
	}{
		{"critical", 0},
		{"high", 1},
		{"warning", 2},
		{"info", 3},
	}
	for _, tt := range tests {
		got := SeverityRank(tt.severity)
		if got != tt.want {
			t.Errorf("SeverityRank(%q) = %d, want %d", tt.severity, got, tt.want)
		}
	}
}

func TestPriorityRank(t *testing.T) {
	tests := []struct {
		priority string
		want     int
	}{
		{"P1", 0},
		{"P2", 1},
		{"P3", 2},
		{"P4", 3},
		{"P5", 4},
	}
	for _, tt := range tests {
		got := PriorityRank(tt.priority)
		if got != tt.want {
			t.Errorf("PriorityRank(%q) = %d, want %d", tt.priority, got, tt.want)
		}
	}
}
