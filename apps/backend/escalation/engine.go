package escalation

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/logger"
	"alga/oncall"
	"alga/store"
)

const defaultOpsTeamName = "ops-team"

type PolicyEngine struct {
	escalationStore store.EscalationStore
	onCallResolver  *oncall.Resolver
	onCallStore     store.OnCallStore
	teamStore       store.TeamStore
	opsTeamName     string
}

// NewPolicyEngine constructs a PolicyEngine. onCallStore is required: every
// "team" target resolves through the team's auto-provisioned schedule, so the
// engine must be able to look up schedules by team.
func NewPolicyEngine(
	escalationStore store.EscalationStore,
	onCallStore store.OnCallStore,
	onCallResolver *oncall.Resolver,
	teamStore store.TeamStore,
) *PolicyEngine {
	return NewPolicyEngineWithOpsTeam(escalationStore, onCallStore, onCallResolver, teamStore, defaultOpsTeamName)
}

func NewPolicyEngineWithOpsTeam(
	escalationStore store.EscalationStore,
	onCallStore store.OnCallStore,
	onCallResolver *oncall.Resolver,
	teamStore store.TeamStore,
	opsTeamName string,
) *PolicyEngine {
	if opsTeamName == "" {
		opsTeamName = defaultOpsTeamName
	}
	return &PolicyEngine{
		escalationStore: escalationStore,
		onCallResolver:  onCallResolver,
		onCallStore:     onCallStore,
		teamStore:       teamStore,
		opsTeamName:     opsTeamName,
	}
}

func (e *PolicyEngine) EvaluatePolicy(ctx context.Context, policyID uuid.UUID, level int) ([]uuid.UUID, []string, bool, error) {
	policy, err := e.escalationStore.GetPolicy(ctx, policyID)
	if err != nil {
		return nil, nil, false, fmt.Errorf("failed to get escalation policy: %w", err)
	}
	if policy == nil {
		return nil, nil, false, fmt.Errorf("escalation policy %s not found", policyID)
	}

	var targetLevel *store.EscalationLevelRecord
	for i := range policy.Levels {
		if policy.Levels[i].LevelNumber == level {
			targetLevel = &policy.Levels[i]
			break
		}
	}
	if targetLevel == nil {
		return nil, nil, false, fmt.Errorf("level %d not found in policy %s", level, policyID)
	}

	opsTeamID, opsTeamKnown := e.lookupOpsTeamID(ctx)

	now := time.Now().UTC()
	var userIDs []uuid.UUID
	userSet := make(map[uuid.UUID]bool)
	notifyChannelSet := make(map[string]bool)
	var notifyChannels []string
	forcedChannels := false

	for _, tgt := range targetLevel.Targets {
		switch tgt.TargetType {
		case "user":
			if tgt.TargetUserID != nil && !userSet[*tgt.TargetUserID] {
				userSet[*tgt.TargetUserID] = true
				userIDs = append(userIDs, *tgt.TargetUserID)
			}
		case "team":
			// A "team" target resolves to whoever is currently on call for the
			// team's auto-provisioned schedule. Paging an entire team's
			// membership is intentionally not supported — the team's on-call
			// schedule is the canonical "who responds for this team right now".
			if tgt.TargetTeamID == nil || e.onCallResolver == nil || e.onCallStore == nil {
				continue
			}
			// The ops-team schedule forces voice at dispatch time. Compare the
			// target team directly to the configured ops-team ID, which avoids
			// an extra schedule lookup.
			if opsTeamKnown && *tgt.TargetTeamID == opsTeamID {
				forcedChannels = true
			}
			sched, err := e.onCallStore.GetScheduleByTeam(ctx, *tgt.TargetTeamID)
			if err != nil {
				logger.WarnCtx(ctx, "EvaluatePolicy: failed to load schedule for team", "component", "escalation", "team_id", *tgt.TargetTeamID, "error", err)
				continue
			}
			userID, err := e.onCallResolver.ResolveWhoIsOnCall(ctx, sched.ID, now)
			if err != nil {
				logger.WarnCtx(ctx, "EvaluatePolicy: failed to resolve on-call for team", "component", "escalation", "team_id", *tgt.TargetTeamID, "schedule_id", sched.ID, "error", err)
				continue
			}
			if userID != nil && !userSet[*userID] {
				userSet[*userID] = true
				userIDs = append(userIDs, *userID)
			}
		}
	}

	for _, ch := range targetLevel.NotifyChannels {
		if !notifyChannelSet[ch] {
			notifyChannelSet[ch] = true
			notifyChannels = append(notifyChannels, ch)
		}
	}

	return userIDs, notifyChannels, forcedChannels, nil
}

func (e *PolicyEngine) lookupOpsTeamID(ctx context.Context) (uuid.UUID, bool) {
	if e.teamStore == nil || e.opsTeamName == "" {
		return uuid.Nil, false
	}
	team, err := e.teamStore.GetTeamByName(ctx, e.opsTeamName)
	if err != nil || team == nil {
		return uuid.Nil, false
	}
	return team.ID, true
}
