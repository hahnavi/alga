package incmetrics

import (
	"math"
	"testing"
	"time"
)

func TestComputeMetrics_EmptyInput(t *testing.T) {
	t.Parallel()
	m := ComputeMetrics(nil, 0)
	if m == nil {
		t.Fatal("ComputeMetrics(nil, 0) returned nil")
	}
	if m.MTTA != 0 {
		t.Fatalf("MTTA = %f, want 0", m.MTTA)
	}
	if m.MTTR != 0 {
		t.Fatalf("MTTR = %f, want 0", m.MTTR)
	}
	if m.MTTM != 0 {
		t.Fatalf("MTTM = %f, want 0", m.MTTM)
	}
	if m.TotalCreated != 0 {
		t.Fatalf("TotalCreated = %d, want 0", m.TotalCreated)
	}
	if m.TotalResolved != 0 {
		t.Fatalf("TotalResolved = %d, want 0", m.TotalResolved)
	}
	if len(m.BySeverity) != 0 {
		t.Fatalf("BySeverity = %d entries, want 0", len(m.BySeverity))
	}
	if len(m.ByService) != 0 {
		t.Fatalf("ByService = %d entries, want 0", len(m.ByService))
	}
	if len(m.Trend) != 0 {
		t.Fatalf("Trend = %d entries, want 0", len(m.Trend))
	}

	m = ComputeMetrics([]IncidentData{}, 0)
	if m == nil {
		t.Fatal("ComputeMetrics([], 0) returned nil")
	}
	if m.TotalCreated != 0 {
		t.Fatalf("TotalCreated = %d, want 0", m.TotalCreated)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestComputeMetrics_SingleIncidentFullSLA(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	ack := created.Add(5 * time.Minute)
	mit := created.Add(30 * time.Minute)
	resolved := created.Add(120 * time.Minute)

	incidents := []IncidentData{
		{
			CreatedAt:         created,
			AcknowledgedAt:    ptrTime(ack),
			MitigatedAt:       ptrTime(mit),
			ResolvedAt:        ptrTime(resolved),
			Severity:          "critical",
			ServiceID:         "svc-1",
			SLATargetRespond:  ptrTime(created.Add(15 * time.Minute)),
			SLAAcknowledgedAt: ptrTime(ack),
			SLATargetResolve:  ptrTime(created.Add(180 * time.Minute)),
			SLAResolvedAt:     ptrTime(resolved),
		},
	}

	m := ComputeMetrics(incidents, 0)

	if m.TotalCreated != 1 {
		t.Fatalf("TotalCreated = %d, want 1", m.TotalCreated)
	}
	if m.TotalResolved != 1 {
		t.Fatalf("TotalResolved = %d, want 1", m.TotalResolved)
	}
	if math.Abs(m.MTTA-5.0) > 0.01 {
		t.Fatalf("MTTA = %f, want ~5.0", m.MTTA)
	}
	if math.Abs(m.MTTR-120.0) > 0.01 {
		t.Fatalf("MTTR = %f, want ~120.0", m.MTTR)
	}
	if math.Abs(m.MTTM-30.0) > 0.01 {
		t.Fatalf("MTTM = %f, want ~30.0", m.MTTM)
	}

	sev, ok := m.BySeverity["critical"]
	if !ok {
		t.Fatal("BySeverity missing 'critical'")
	}
	if sev.Count != 1 {
		t.Fatalf("critical Count = %d, want 1", sev.Count)
	}
	if math.Abs(sev.MTTA-5.0) > 0.01 {
		t.Fatalf("critical MTTA = %f, want ~5.0", sev.MTTA)
	}
	if math.Abs(sev.MTTR-120.0) > 0.01 {
		t.Fatalf("critical MTTR = %f, want ~120.0", sev.MTTR)
	}

	svc, ok := m.ByService["svc-1"]
	if !ok {
		t.Fatal("ByService missing 'svc-1'")
	}
	if svc.Count != 1 {
		t.Fatalf("svc-1 Count = %d, want 1", svc.Count)
	}
}

func TestComputeMetrics_SLABreachDetection(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	ack := created.Add(30 * time.Minute)
	resolved := created.Add(200 * time.Minute)

	incidents := []IncidentData{
		{
			CreatedAt:         created,
			AcknowledgedAt:    ptrTime(ack),
			ResolvedAt:        ptrTime(resolved),
			Severity:          "warning",
			SLATargetRespond:  ptrTime(created.Add(15 * time.Minute)),
			SLAAcknowledgedAt: ptrTime(ack),
			SLATargetResolve:  ptrTime(created.Add(180 * time.Minute)),
			SLAResolvedAt:     ptrTime(resolved),
		},
	}

	m := ComputeMetrics(incidents, 0)

	if m.SLACompliance.TotalWithSLA != 1 {
		t.Fatalf("TotalWithSLA = %d, want 1", m.SLACompliance.TotalWithSLA)
	}
	if m.SLACompliance.ResponseBreaches != 1 {
		t.Fatalf("ResponseBreaches = %d, want 1", m.SLACompliance.ResponseBreaches)
	}
	if m.SLACompliance.ResolveBreaches != 1 {
		t.Fatalf("ResolveBreaches = %d, want 1", m.SLACompliance.ResolveBreaches)
	}
	if math.Abs(m.SLACompliance.ResponseSLACompliance-0.0) > 0.01 {
		t.Fatalf("ResponseSLACompliance = %f, want ~0.0", m.SLACompliance.ResponseSLACompliance)
	}
	if math.Abs(m.SLACompliance.ResolveSLACompliance-0.0) > 0.01 {
		t.Fatalf("ResolveSLACompliance = %f, want ~0.0", m.SLACompliance.ResolveSLACompliance)
	}
}

func TestComputeMetrics_BySeverityBreakdown(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	incidents := []IncidentData{
		{CreatedAt: base, Severity: "critical", SLAAcknowledgedAt: ptrTime(base.Add(10 * time.Minute)), ResolvedAt: ptrTime(base.Add(60 * time.Minute))},
		{CreatedAt: base, Severity: "critical", SLAAcknowledgedAt: ptrTime(base.Add(20 * time.Minute)), ResolvedAt: ptrTime(base.Add(100 * time.Minute))},
		{CreatedAt: base, Severity: "warning", SLAAcknowledgedAt: ptrTime(base.Add(2 * time.Minute)), ResolvedAt: ptrTime(base.Add(30 * time.Minute))},
		{CreatedAt: base, Severity: "info"},
	}

	m := ComputeMetrics(incidents, 0)

	if len(m.BySeverity) != 3 {
		t.Fatalf("BySeverity has %d entries, want 3", len(m.BySeverity))
	}

	crit := m.BySeverity["critical"]
	if crit.Count != 2 {
		t.Fatalf("critical Count = %d, want 2", crit.Count)
	}
	if math.Abs(crit.MTTA-15.0) > 0.01 {
		t.Fatalf("critical MTTA = %f, want ~15.0", crit.MTTA)
	}
	if math.Abs(crit.MTTR-80.0) > 0.01 {
		t.Fatalf("critical MTTR = %f, want ~80.0", crit.MTTR)
	}

	warn := m.BySeverity["warning"]
	if warn.Count != 1 {
		t.Fatalf("warning Count = %d, want 1", warn.Count)
	}

	info := m.BySeverity["info"]
	if info.Count != 1 {
		t.Fatalf("info Count = %d, want 1", info.Count)
	}
	if info.MTTA != 0 {
		t.Fatalf("info MTTA = %f, want 0 (no ack)", info.MTTA)
	}
	if info.MTTR != 0 {
		t.Fatalf("info MTTR = %f, want 0 (no resolve)", info.MTTR)
	}
}

func TestComputeMetrics_NilSLAFields(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	incidents := []IncidentData{
		{
			CreatedAt:         base,
			Severity:          "info",
			SLATargetRespond:  nil,
			SLAAcknowledgedAt: nil,
			SLATargetResolve:  nil,
			SLAResolvedAt:     nil,
		},
	}

	m := ComputeMetrics(incidents, 0)

	if m.MTTA != 0 {
		t.Fatalf("MTTA = %f, want 0", m.MTTA)
	}
	if m.MTTR != 0 {
		t.Fatalf("MTTR = %f, want 0", m.MTTR)
	}
	if m.SLACompliance.TotalWithSLA != 0 {
		t.Fatalf("TotalWithSLA = %d, want 0", m.SLACompliance.TotalWithSLA)
	}
}

func TestComputeMetrics_TrendComputation(t *testing.T) {
	t.Parallel()
	day1 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 1, 2, 14, 0, 0, 0, time.UTC)

	incidents := []IncidentData{
		{
			CreatedAt:         day1,
			ResolvedAt:        ptrTime(day1.Add(60 * time.Minute)),
			SLAAcknowledgedAt: ptrTime(day1.Add(5 * time.Minute)),
			Severity:          "critical",
		},
		{
			CreatedAt:         day2,
			ResolvedAt:        ptrTime(day2.Add(120 * time.Minute)),
			SLAAcknowledgedAt: ptrTime(day2.Add(10 * time.Minute)),
			Severity:          "warning",
		},
	}

	m := ComputeMetrics(incidents, 7)

	if len(m.Trend) != 2 {
		t.Fatalf("Trend has %d entries, want 2", len(m.Trend))
	}

	if m.Trend[0].Date != "2026-01-01" {
		t.Fatalf("Trend[0].Date = %q, want %q", m.Trend[0].Date, "2026-01-01")
	}
	if m.Trend[0].Created != 1 {
		t.Fatalf("Trend[0].Created = %d, want 1", m.Trend[0].Created)
	}
	if m.Trend[0].Resolved != 1 {
		t.Fatalf("Trend[0].Resolved = %d, want 1", m.Trend[0].Resolved)
	}
	if math.Abs(m.Trend[0].MTTAMin-5.0) > 0.01 {
		t.Fatalf("Trend[0].MTTAMin = %f, want ~5.0", m.Trend[0].MTTAMin)
	}
	if math.Abs(m.Trend[0].MTTRMin-60.0) > 0.01 {
		t.Fatalf("Trend[0].MTTRMin = %f, want ~60.0", m.Trend[0].MTTRMin)
	}

	if m.Trend[1].Date != "2026-01-02" {
		t.Fatalf("Trend[1].Date = %q, want %q", m.Trend[1].Date, "2026-01-02")
	}
	if math.Abs(m.Trend[1].MTTAMin-10.0) > 0.01 {
		t.Fatalf("Trend[1].MTTAMin = %f, want ~10.0", m.Trend[1].MTTAMin)
	}
	if math.Abs(m.Trend[1].MTTRMin-120.0) > 0.01 {
		t.Fatalf("Trend[1].MTTRMin = %f, want ~120.0", m.Trend[1].MTTRMin)
	}
}

func TestComputeMetrics_TrendZeroDays(t *testing.T) {
	t.Parallel()
	incidents := []IncidentData{
		{CreatedAt: time.Now(), Severity: "info"},
	}
	m := ComputeMetrics(incidents, 0)
	if len(m.Trend) != 0 {
		t.Fatalf("Trend has %d entries with trendDays=0, want 0", len(m.Trend))
	}
}

func TestComputeMetrics_SLAAckNil_CountsAsBreach(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	incidents := []IncidentData{
		{
			CreatedAt:         created,
			Severity:          "critical",
			SLATargetRespond:  ptrTime(created.Add(15 * time.Minute)),
			SLAAcknowledgedAt: nil,
		},
	}

	m := ComputeMetrics(incidents, 0)
	if m.SLACompliance.ResponseBreaches != 1 {
		t.Fatalf("ResponseBreaches = %d, want 1 (nil ack is a breach)", m.SLACompliance.ResponseBreaches)
	}
	if m.SLACompliance.TotalWithSLA != 1 {
		t.Fatalf("TotalWithSLA = %d, want 1", m.SLACompliance.TotalWithSLA)
	}
}

func TestPct(t *testing.T) {
	t.Parallel()
	cases := []struct {
		num, total int64
		want       float64
	}{
		{num: 0, total: 0, want: 100.0},
		{num: 8, total: 10, want: 80.0},
		{num: 10, total: 10, want: 100.0},
		{num: 0, total: 10, want: 0.0},
	}
	for _, tc := range cases {
		got := pct(tc.num, tc.total)
		if math.Abs(got-tc.want) > 0.001 {
			t.Errorf("pct(%d, %d) = %f, want %f", tc.num, tc.total, got, tc.want)
		}
	}
}
