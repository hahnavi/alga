package api

import (
	"context"
	"errors"

	"alga/logger"
	"alga/sse"
	"alga/store"
)

type AlertInvestigationLifecycleService struct {
	alertStore         store.Store
	investigationStore store.AlertInvestigationStore
	auditStore         store.AuditStore
	ssePublisher       *sse.DualPublisher
	pendingNotifier    pendingNotifier
}

func NewAlertInvestigationLifecycleService(alertStore store.Store, invStore store.AlertInvestigationStore, auditStore store.AuditStore, ssePublisher *sse.DualPublisher, pendingNotifier pendingNotifier) *AlertInvestigationLifecycleService {
	return &AlertInvestigationLifecycleService{alertStore: alertStore, investigationStore: invStore, auditStore: auditStore, ssePublisher: ssePublisher, pendingNotifier: pendingNotifier}
}

func (s *AlertInvestigationLifecycleService) RequireAlertActionAllowed(ctx context.Context, alertNumber int64, agent *store.AgentTokenRecord) error {
	if s == nil || s.investigationStore == nil || agent == nil {
		return nil
	}
	inv, err := s.investigationStore.GetCurrentAlertInvestigationByAlertNumber(ctx, alertNumber)
	if err != nil {
		if errors.Is(err, store.ErrInvestigationNotFound) {
			return nil
		}
		return err
	}
	if inv == nil {
		return nil
	}
	if inv.PromotedIncidentID != nil {
		return nil
	}
	if inv.AgentID == "" {
		return nil
	}
	if inv.AgentID != agent.ID.String() {
		return errors.New("not assigned to this investigation")
	}
	return nil
}

func (s *AlertInvestigationLifecycleService) CompleteIfAllAlertsResolved(ctx context.Context, req store.AlertInvestigationLifecycleCompletionRequest) error {
	if s == nil || s.alertStore == nil || s.investigationStore == nil || req.AlertNumber <= 0 {
		return nil
	}
	inv, err := s.investigationStore.GetCurrentAlertInvestigationByAlertNumber(ctx, req.AlertNumber)
	if err != nil {
		if errors.Is(err, store.ErrInvestigationNotFound) {
			return nil
		}
		return err
	}
	if inv == nil || store.IsTerminalInvestigationStatus(inv.Status) || inv.Status == store.AlertInvestigationStatusComplete {
		return nil
	}
	if !store.AllAlertInvestigationAlertsResolved(s.alertStore, inv) {
		return nil
	}
	if req.Reason == "" {
		req.Reason = store.AlertInvestigationCompletedReasonAlertsResolved
	}
	if req.ActorType == "" {
		req.ActorType = store.InvestigationActorSystem
	}
	if err := s.investigationStore.CompleteAlertInvestigation(ctx, inv.ID.String(), store.AlertInvestigationCompletion{
		Reason:      req.Reason,
		ActorType:   req.ActorType,
		ActorID:     req.ActorID,
		ActorName:   req.ActorName,
		EventReason: "all linked alerts resolved",
	}); err != nil {
		return err
	}
	if s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{Type: "investigation_status_changed", Data: map[string]any{"alert_investigation_id": inv.AlertInvestigationID, "status": store.AlertInvestigationStatusComplete}})
	}
	logger.InfoCtx(ctx, "completed alert investigation after linked alerts resolved", "alert_investigation_id", inv.AlertInvestigationID, "alert_number", req.AlertNumber)
	return nil
}
