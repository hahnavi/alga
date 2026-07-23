package oncall

import (
	"context"
	"time"

	"github.com/google/uuid"

	"alga/logger"
	"alga/store"
)

// ResolveOnCallUserForIncident resolves the user currently on-call for an
// incident's owning service escalation policy. Policy levels are walked in
// ascending order and the first user resolved from a "team" target is
// returned (a team target resolves through the team's auto-provisioned
// on-call schedule). It returns (nil, nil) when the incident has no service,
// the service has no escalation policy, the policy has no team target, or no
// on-call user can be resolved.
func ResolveOnCallUserForIncident(
	ctx context.Context,
	incident *store.IncidentRecord,
	serviceStore store.ServiceStore,
	escalationStore store.EscalationStore,
	onCallStore store.OnCallStore,
	resolver *Resolver,
) (*uuid.UUID, error) {
	if incident == nil || incident.ServiceID == nil {
		return nil, nil
	}
	if serviceStore == nil || escalationStore == nil || onCallStore == nil || resolver == nil {
		return nil, nil
	}

	svc, err := serviceStore.GetService(ctx, incident.ServiceID.String())
	if err != nil {
		return nil, err
	}
	if svc == nil || svc.EscalationPolicyID == nil {
		return nil, nil
	}

	policy, err := escalationStore.GetPolicy(ctx, *svc.EscalationPolicyID)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		return nil, nil
	}

	now := time.Now()
	for _, level := range policy.Levels {
		for _, target := range level.Targets {
			if target.TargetType != "team" || target.TargetTeamID == nil {
				continue
			}
			sched, schedErr := onCallStore.GetScheduleByTeam(ctx, *target.TargetTeamID)
			if schedErr != nil {
				logger.WarnCtx(ctx, "failed to load schedule for team", "component", "oncall", "team_id", *target.TargetTeamID, "error", schedErr)
				continue
			}
			uid, resolveErr := resolver.ResolveWhoIsOnCall(ctx, sched.ID, now)
			if resolveErr != nil {
				logger.WarnCtx(ctx, "failed to resolve on-call schedule", "component", "oncall", "schedule_id", sched.ID, "error", resolveErr)
				continue
			}
			if uid != nil {
				return uid, nil
			}
		}
	}
	return nil, nil
}
