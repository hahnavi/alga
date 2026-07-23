package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"alga/logger"
	"alga/store"
	"alga/types"
	"alga/webhook"
)

const heartbeatSweepTick = 30 * time.Second

type HeartbeatSweepWorker struct {
	heartbeatStore store.HeartbeatStore
	alertStore     store.Store
	auditStore     store.AuditStore
	receiver       *webhook.Receiver
}

func NewHeartbeatSweepWorker(
	heartbeatStore store.HeartbeatStore,
	alertStore store.Store,
	auditStore store.AuditStore,
	receiver *webhook.Receiver,
) *HeartbeatSweepWorker {
	return &HeartbeatSweepWorker{
		heartbeatStore: heartbeatStore,
		alertStore:     alertStore,
		auditStore:     auditStore,
		receiver:       receiver,
	}
}

func (w *HeartbeatSweepWorker) Run(ctx context.Context) {
	runTickerLoop(ctx, heartbeatSweepTick, "heartbeat-sweep", w.tick)
}

func (w *HeartbeatSweepWorker) tick(ctx context.Context) {
	if w.heartbeatStore == nil {
		return
	}
	now := time.Now().UTC()
	expired, err := w.heartbeatStore.ListExpired(ctx, now)
	if err != nil {
		logger.Error("Heartbeat sweep: failed to list expired heartbeats", "component", "heartbeat-sweep", "error", err)
		return
	}
	for _, hb := range expired {
		w.processExpired(ctx, hb, now)
	}
}

func (w *HeartbeatSweepWorker) processExpired(ctx context.Context, hb store.HeartbeatRecord, now time.Time) {
	marked, err := w.heartbeatStore.MarkExpired(ctx, hb.ID, now)
	if err != nil {
		logger.Error("Heartbeat sweep: failed to mark expired", "component", "heartbeat-sweep", "heartbeat_id", hb.ID, "error", err)
		return
	}

	if w.receiver == nil {
		logger.Warn("Heartbeat sweep: alert receiver not configured; skipping alert", "component", "heartbeat-sweep", "heartbeat_id", hb.ID)
		return
	}

	fp := "heartbeat:" + hb.ID.String()
	alert := buildHeartbeatAlert(hb, fp, now)
	if _, err := w.receiver.IngestManualAlert(ctx, alert, heartbeatSystemActor()); err != nil {
		// An open alert already exists for this fingerprint (still firing): that
		// is the expected state if a prior sweep already fired. Not an error.
		if !errors.Is(err, store.ErrOpenAlertExists) {
			logger.Error("Heartbeat sweep: failed to ingest alert", "component", "heartbeat-sweep", "heartbeat_id", hb.ID, "error", err)
		}
		return
	}

	if w.auditStore != nil {
		w.auditStore.Log(store.AuditHeartbeatAlert, nil, "heartbeat", "", "", true, map[string]any{
			"heartbeat_id": hb.ID.String(),
			"name":         hb.Name,
			"fingerprint":  fp,
			"severity":     marked.Severity,
		})
	}
	logger.Info("Heartbeat sweep: fired expired-heartbeat alert", "component", "heartbeat-sweep", "heartbeat_id", hb.ID, "name", hb.Name, "fingerprint", fp)
}

func buildHeartbeatAlert(hb store.HeartbeatRecord, fingerprint string, now time.Time) types.Alert {
	labels := map[string]string{
		"alertname":    "HeartbeatExpired",
		"severity":     hb.Severity,
		"heartbeat":    hb.Name,
		"heartbeat_id": hb.ID.String(),
	}
	for k, v := range hb.Labels {
		if _, exists := labels[k]; !exists {
			labels[k] = v
		}
	}

	lastPing := "never"
	if hb.LastPingAt != nil {
		lastPing = hb.LastPingAt.UTC().Format(time.RFC3339)
	}
	description := fmt.Sprintf(
		"Heartbeat %q has not checked in within its interval (%ds plus %ds grace). Last ping: %s.",
		hb.Name, hb.IntervalSeconds, hb.GraceSeconds, lastPing,
	)

	return types.Alert{
		Status:       "firing",
		Labels:       labels,
		Annotations:  map[string]string{"summary": "Heartbeat expired: " + hb.Name, "description": description},
		StartsAt:     now.Format(time.RFC3339),
		Fingerprint:  fingerprint,
		GeneratorURL: "",
	}
}

func heartbeatSystemActor() *store.EventActor {
	return &store.EventActor{
		UserID:      "heartbeat",
		Username:    "heartbeat",
		DisplayName: "Heartbeat Monitor",
		Source:      "heartbeat",
	}
}
