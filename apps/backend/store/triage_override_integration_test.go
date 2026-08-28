//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/db"
	"alga/db/models"
)

// TestTriageOverrideReasonRoundTrip pins the override contract: the
// patch sent by the override endpoint (outcome, decision, who, when, why)
// survives a store Update + Get cycle, including the override_reason column
// added by migration 00022.
func TestTriageOverrideReasonRoundTrip(t *testing.T) {
	bunDB := newTestDB(t)
	cli := &db.Client{DB: bunDB}
	stores, err := NewStores(cli, time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatalf("create stores: %v", err)
	}

	ctx := context.Background()
	// overridden_by carries an FK to users, so the fixture needs a real one.
	user, err := stores.User.CreateUser(
		"triage-override-"+uuid.NewString()[:8]+"@example.com",
		"correct horse battery staple",
		"admin",
	)
	if err != nil {
		t.Fatalf("create fixture user: %v", err)
	}
	created, err := stores.TriageResult.Create(ctx, &TriageResultRecord{
		CorrelationKey: "override-test-" + uuid.NewString(),
		Decision:       TriageDecisionInvestigate,
		Confidence:     0.42,
	})
	if err != nil {
		t.Fatalf("create triage result: %v", err)
	}
	t.Cleanup(func() {
		_, _ = bunDB.NewDelete().Model((*models.TriageResult)(nil)).
			Where("id = ?", created.ID).Exec(context.Background())
		_, _ = bunDB.NewDelete().Model((*models.User)(nil)).
			Where("id = ?", user.ID).Exec(context.Background())
	})

	overriddenAt := time.Now().UTC().Truncate(time.Second)
	overriddenBy := user.ID
	updated, err := stores.TriageResult.Update(ctx, created.ID.String(), &TriageResultRecord{
		Outcome:        TriageResultOutcomeOverridden,
		OverriddenTo:   TriageDecisionSuppress,
		OverriddenBy:   &overriddenBy,
		OverriddenAt:   &overriddenAt,
		OverrideReason: "operator knows this is a known-flaky probe",
	})
	if err != nil {
		t.Fatalf("update triage result: %v", err)
	}
	if updated.OverrideReason != "operator knows this is a known-flaky probe" {
		t.Errorf("updated.OverrideReason = %q, want the operator reason", updated.OverrideReason)
	}

	got, err := stores.TriageResult.Get(ctx, created.ID.String())
	if err != nil {
		t.Fatalf("get triage result: %v", err)
	}
	if got == nil {
		t.Fatal("triage result not found after override")
	}
	if got.Outcome != TriageResultOutcomeOverridden {
		t.Errorf("outcome = %q, want %q", got.Outcome, TriageResultOutcomeOverridden)
	}
	if got.OverriddenTo != TriageDecisionSuppress {
		t.Errorf("overridden_to = %q, want %q", got.OverriddenTo, TriageDecisionSuppress)
	}
	if got.OverrideReason != "operator knows this is a known-flaky probe" {
		t.Errorf("override_reason = %q, want the operator reason", got.OverrideReason)
	}
	if got.OverriddenBy == nil || *got.OverriddenBy != overriddenBy {
		t.Errorf("overridden_by = %v, want %s", got.OverriddenBy, overriddenBy)
	}
	if got.OverriddenAt == nil || !got.OverriddenAt.Equal(overriddenAt) {
		t.Errorf("overridden_at = %v, want %s", got.OverriddenAt, overriddenAt)
	}
}

// TestAutoPromoteCandidate pins the auto-promote lookup: the candidate
// decision is the latest confirmed decision for the key, and confirmations
// counts confirmed rows sharing that key+decision pair.
func TestAutoPromoteCandidate(t *testing.T) {
	bunDB := newTestDB(t)
	cli := &db.Client{DB: bunDB}
	stores, err := NewStores(cli, time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatalf("create stores: %v", err)
	}

	ctx := context.Background()
	key := "autopromote-test-" + uuid.NewString()
	base := time.Now().UTC().Add(-time.Hour)

	t.Cleanup(func() {
		_, _ = bunDB.NewDelete().Model((*models.TriageResult)(nil)).
			Where("correlation_key = ?", key).Exec(context.Background())
	})

	insert := func(t *testing.T, decision, outcome string, age time.Duration) {
		t.Helper()
		var triageNumber int64
		if err := bunDB.NewSelect().ColumnExpr("nextval('triage_number_seq')").Scan(ctx, &triageNumber); err != nil {
			t.Fatalf("allocate triage number: %v", err)
		}
		ts := base.Add(age)
		tr := &models.TriageResult{
			ID:             models.NewUUID(),
			TriageNumber:   triageNumber,
			CorrelationKey: key,
			Decision:       decision,
			Outcome:        outcome,
			CreatedAt:      ts,
			UpdatedAt:      ts,
		}
		if _, err := bunDB.NewInsert().Model(tr).Exec(ctx); err != nil {
			t.Fatalf("insert triage fixture: %v", err)
		}
	}

	// Oldest to newest: a confirmed suppress, then three confirmed
	// investigate, then a pending investigate (must not count).
	insert(t, TriageDecisionSuppress, TriageResultOutcomeConfirmed, 0)
	for i := 1; i <= 3; i++ {
		insert(t, TriageDecisionInvestigate, TriageResultOutcomeConfirmed, time.Duration(i)*time.Minute)
	}
	insert(t, TriageDecisionInvestigate, TriageResultOutcomePending, 4*time.Minute)

	decision, confirmations, err := stores.TriageResult.AutoPromoteCandidate(ctx, key)
	if err != nil {
		t.Fatalf("AutoPromoteCandidate: %v", err)
	}
	if decision != TriageDecisionInvestigate {
		t.Errorf("decision = %q, want %q (latest confirmed)", decision, TriageDecisionInvestigate)
	}
	if confirmations != 3 {
		t.Errorf("confirmations = %d, want 3 (pending row excluded, suppress not double-counted)", confirmations)
	}

	decision, confirmations, err = stores.TriageResult.AutoPromoteCandidate(ctx, "autopromote-test-nothing-"+uuid.NewString())
	if err != nil {
		t.Fatalf("AutoPromoteCandidate unknown key: %v", err)
	}
	if decision != "" || confirmations != 0 {
		t.Errorf("unknown key = (%q, %d), want empty", decision, confirmations)
	}
}
