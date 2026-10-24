//go:build integration

package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"alga/automation"
	"alga/db/models"
)

func TestAutomationCapturesAndDispatchesAlertEvents(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	st := newPGAutomationStore(db, automation.HostPolicy{Allowed: []string{"hooks.example.com"}})
	d := automation.Definition{Name: "Critical alerts", TriggerType: "alert.created", Enabled: true,
		Conditions: []automation.Condition{{Field: "severity", Operator: "equals", Value: "critical"}},
		Actions:    []automation.Action{{Type: "webhook", URL: "https://hooks.example.com/alga", BearerToken: "a-secret-token"}}}
	rule, err := st.CreateRule(ctx, d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rule.Enabled {
		t.Fatal("new rule should start disabled")
	}
	if !rule.Actions[0].HasBearerToken || rule.Actions[0].BearerToken != "" {
		t.Fatal("credential was not redacted")
	}
	d.Actions[0].BearerToken = ""
	rule, err = st.UpdateRule(ctx, rule.ID, rule.Version, d)
	if err != nil {
		t.Fatal(err)
	}
	if !rule.Actions[0].HasBearerToken {
		t.Fatal("credential was not preserved on update")
	}
	if _, err := st.UpdateRule(ctx, rule.ID, 1, d); !errors.Is(err, ErrAutomationVersionConflict) {
		t.Fatalf("stale rule update = %v", err)
	}

	alertID := models.NewUUID()
	if _, err := db.NewRaw(`INSERT INTO alerts (id, fingerprint, alert_number, status, labels) VALUES (?, ?, ?, 'firing', ?::jsonb)`,
		alertID, "automation-test-fingerprint", 900001, `{"severity":"critical","alertname":"DBDown"}`).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	processed, err := st.DispatchOne(ctx)
	if err != nil || !processed {
		t.Fatalf("dispatch: processed=%v err=%v", processed, err)
	}
	runs, total, err := st.ListRuns(ctx, &rule.ID, 20, 0)
	if err != nil || total != 1 {
		t.Fatalf("runs: total=%d err=%v", total, err)
	}
	if runs[0].Actions[0].BearerToken != "" || !runs[0].Actions[0].HasBearerToken {
		t.Fatal("run exposed or lost credential")
	}
	encoded, err := json.Marshal(runs[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "a-secret-token") || strings.Contains(string(encoded), "bearer_token_enc") {
		t.Fatal("secret leaked in run JSON")
	}
	if processed, err := st.DispatchOne(ctx); err != nil || processed {
		t.Fatalf("duplicate event dispatch: processed=%v err=%v", processed, err)
	}

	if _, err := db.NewRaw("UPDATE alerts SET status = 'resolved' WHERE id = ?", alertID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.NewRaw(`INSERT INTO alerts (id, fingerprint, alert_number, status, labels) VALUES (?, ?, ?, 'firing', ?::jsonb)`,
		models.NewUUID(), "automation-test-fingerprint", 900002, `{"severity":"critical"}`).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := st.DispatchOne(ctx); err != nil {
			t.Fatal(err)
		}
	}
	_, total, err = st.ListRuns(ctx, &rule.ID, 20, 0)
	if err != nil || total != 2 {
		t.Fatalf("new alert identity was lost: total=%d err=%v", total, err)
	}
	if _, err := db.NewRaw(`INSERT INTO alerts (id, fingerprint, alert_number, status, labels, silenced) VALUES (?, ?, ?, 'firing', ?::jsonb, true)`,
		models.NewUUID(), "automation-silenced-fingerprint", 900003, `{"severity":"critical"}`).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DispatchOne(ctx); err != nil {
		t.Fatal(err)
	}
	_, total, err = st.ListRuns(ctx, &rule.ID, 20, 0)
	if err != nil || total != 2 {
		t.Fatalf("silenced alert ran automation: total=%d err=%v", total, err)
	}
	d.Actions[0].ClearBearerToken = true
	updated, err := st.UpdateRule(ctx, rule.ID, rule.Version, d)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Actions[0].HasBearerToken {
		t.Fatal("stored credential was not removed")
	}
	olderRun, err := st.GetRun(ctx, runs[0].ID)
	if err != nil || !olderRun.Actions[0].HasBearerToken {
		t.Fatalf("existing run snapshot changed: run=%+v err=%v", olderRun, err)
	}
	if _, err := db.NewRaw("UPDATE automation_runs SET status = 'running', attempt = 5, lease_until = ?, next_attempt_at = ? WHERE id = ?",
		time.Now().Add(-time.Minute), time.Now().Add(-time.Minute), runs[0].ID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	claim, err := st.ClaimRun(ctx)
	if err != nil || claim != nil {
		t.Fatalf("expired fifth attempt was claimed: claim=%+v err=%v", claim, err)
	}
	failed, err := st.GetRun(ctx, runs[0].ID)
	if err != nil || failed.Status != "failed" || failed.LastError != "lease_expired" || len(failed.Steps) != 1 {
		t.Fatalf("expired run=%+v err=%v", failed, err)
	}
	if err := st.RetryRun(ctx, failed.ID); err != nil {
		t.Fatal(err)
	}
	retried, err := st.GetRun(ctx, failed.ID)
	if err != nil || retried.Status != "pending" || retried.CompletedAt != nil || retried.Attempt != 0 {
		t.Fatalf("retried run=%+v err=%v", retried, err)
	}
}

func TestAutomationIncidentNoteIsAtomicAndResumeSafe(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	st := newPGAutomationStore(db, automation.HostPolicy{})
	d := automation.Definition{Name: "Incident note", TriggerType: "incident.created", Enabled: true,
		Actions: []automation.Action{{Type: "incident_note", Message: "Start the response checklist"}}}
	rule, err := st.CreateRule(ctx, d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateRule(ctx, rule.ID, rule.Version, d); err != nil {
		t.Fatal(err)
	}
	incidentID := models.NewUUID()
	if _, err := db.NewRaw("INSERT INTO incidents (id, incident_number, title) VALUES (?, ?, ?)", incidentID, 900001, "Automation incident").Exec(ctx); err != nil {
		t.Fatal(err)
	}
	processed, err := st.DispatchOne(ctx)
	if err != nil || !processed {
		t.Fatalf("dispatch: %v %v", processed, err)
	}
	claim, err := st.ClaimRun(ctx)
	if err != nil || claim == nil {
		t.Fatalf("claim: %v %v", claim, err)
	}
	if err := st.CompleteAction(ctx, *claim, nil, true); err != nil {
		t.Fatal(err)
	}
	if err := st.CompleteAction(ctx, *claim, nil, true); !errors.Is(err, ErrAutomationClaimLost) {
		t.Fatalf("duplicate completion = %v", err)
	}
	var count int
	if err := db.NewRaw("SELECT count(*) FROM incident_timeline_entries WHERE incident_id = ? AND metadata->>'automation_run_id' = ?", incidentID, claim.ID.String()).Scan(ctx, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("timeline notes=%d, want 1", count)
	}
	run, err := st.GetRun(ctx, claim.ID)
	if err != nil || run.Status != "succeeded" || len(run.Steps) != 1 {
		t.Fatalf("run=%+v err=%v", run, err)
	}
}
