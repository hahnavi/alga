package servicetracker

import (
	"testing"
)

func TestSeverityWeight(t *testing.T) {
	tests := []struct {
		severity string
		want     float64
	}{
		{"P1", 5},
		{"P2", 4},
		{"P3", 3},
		{"P4", 2},
		{"P5", 1},
		{"unknown", 1},
	}
	for _, tt := range tests {
		if got := priorityWeight(tt.severity); got != tt.want {
			t.Errorf("priorityWeight(%q) = %v, want %v", tt.severity, got, tt.want)
		}
	}
}

func TestScoreToStatus(t *testing.T) {
	tests := []struct {
		score float64
		want  string
	}{
		{0, "operational"},
		{1, "degraded"},
		{3, "degraded"},
		{4, "degraded"},
		{4.1, "partial_outage"},
		{9, "partial_outage"},
		{9.1, "major_outage"},
		{15, "major_outage"},
	}
	for _, tt := range tests {
		if got := scoreToStatus(tt.score); got != tt.want {
			t.Errorf("scoreToStatus(%v) = %q, want %q", tt.score, got, tt.want)
		}
	}
}
