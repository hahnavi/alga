package worker

import (
	"context"
	"strconv"
	"time"
)

// escalationKVStore is the minimal Valkey surface the guard helpers need.
// *valkey.Client satisfies it, as do narrower test fakes.
type escalationKVStore interface {
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	HGet(ctx context.Context, key, field string) (string, error)
	HSet(ctx context.Context, key, field, value string) error
}

// escalationGuards centralizes the pre-publish checks every escalation
// publisher (autoAssignIC, manual /escalate, SLA breach, stuck investigation)
// must run before enqueueing an EscalationMessage. The guards prevent:
//
//  1. A user who has acknowledged the incident from being re-paged.
//  2. A user who has silenced the incident for an hour from being re-paged
//     by anything other than the sweep (which already honors the flag).
//  3. Two publishers from racing the same incident: the first to claim
//     `current_level` via HSETNX wins; the rest are short-circuited.
//
// All checks are best-effort. If Valkey is unavailable, they fail open:
// we'd rather place an extra call than drop an escalation. The cost
// control intent is to stop the thundering-herd case, not to be a strict
// idempotency boundary (the per-(incident,user,level) call dedup in
// the dispatcher is that boundary).

type escalationGuardResult int

const (
	guardAllow escalationGuardResult = iota
	guardSkipAcknowledged
	guardSkipSilenced
	guardSkipClaimLost
)

func (r escalationGuardResult) String() string {
	switch r {
	case guardAllow:
		return "allow"
	case guardSkipAcknowledged:
		return "skipped_acknowledged"
	case guardSkipSilenced:
		return "skipped_silenced"
	case guardSkipClaimLost:
		return "skipped_claim_lost"
	default:
		return "unknown"
	}
}

// claimEscalationFirstPublish returns allow when this caller is the first
// publisher for the incident. Subsequent publishers (e.g. SLA breach fires
// while autoAssignIC's message is still in flight) read the existing
// current_level and skip.
//
// levelHint is the level the publisher intends to fire at; it is stored in
// the hash so the sweep can pick up from there on the next tick.
func claimEscalationFirstPublish(ctx context.Context, vk escalationKVStore, incidentNumber int64, policyID string, levelHint int) escalationGuardResult {
	if vk == nil {
		return guardAllow
	}
	hashKey := escHashPrefix + strconv.FormatInt(incidentNumber, 10)

	if verdict := preflightEscalationState(ctx, vk, hashKey); verdict != guardAllow {
		return verdict
	}

	ok, err := vk.SetNX(ctx, hashKey+":claim", "1", 60*time.Second)
	if err != nil || !ok {
		return guardSkipClaimLost
	}
	_ = vk.HSet(ctx, hashKey, "policy_id", policyID)
	_ = vk.HSet(ctx, hashKey, "current_level", strconv.Itoa(levelHint))
	return guardAllow
}

// preflightEscalationState returns the current guard verdict for the
// incident without claiming anything. Use this when the caller doesn't
// intend to publish (e.g. stuck-investigation checking before doing
// expensive work).
func preflightEscalationState(ctx context.Context, vk escalationKVStore, hashKey string) escalationGuardResult {
	if vk == nil {
		return guardAllow
	}
	ack, _ := vk.HGet(ctx, hashKey, "acknowledged")
	if ack == "1" {
		return guardSkipAcknowledged
	}
	silencedStr, _ := vk.HGet(ctx, hashKey, "silenced_until")
	if silencedStr != "" {
		if silenced, perr := strconv.ParseInt(silencedStr, 10, 64); perr == nil && silenced > time.Now().Unix() {
			return guardSkipSilenced
		}
	}
	return guardAllow
}
