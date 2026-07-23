package cancellation

import (
	"context"
	"errors"

	"alga/store"
	"alga/valkey"
)

// AlertLookup is the subset of store.Store that AlertCancelled needs. It is
// defined locally so the cancel package can be tested with lightweight fakes
// instead of a full Postgres-backed store. The canonical identity is
// alert_number; GetOpenByFingerprint is only consulted as a fallback when no
// alert_number is supplied.
type AlertLookup interface {
	GetByAlertNumber(alertNumber int64) (*store.AlertRecord, error)
	GetOpenByFingerprint(fingerprint string) (*store.AlertRecord, error)
}

// AlertCancelled reports whether the alert identified by alertNumber (canonical)
// or fingerprint (fallback) has been cancelled. alertNumber is the unique alert
// identifier per the domain invariants and is checked first; fingerprint is only
// consulted when no alertNumber is supplied.
//
// Fingerprints are deduplication keys, not unique identifiers: the same
// fingerprint is reused when a new alert fires after a previous one is
// resolved. Treating a fingerprint as a stable identity (e.g. via a long-TTL
// cancel-set key) suppresses the next alert that reuses the fingerprint —
// that regression previously caused alert #33 to be dropped as "primary alert
// deleted" because alert #31 (same fingerprint, resolved) had poisoned the
// cancel set. alert_number is therefore the authoritative identity.
//
// The Valkey cancel set is the fast path; the Postgres deleted_at check is the
// durable backstop that keeps the system correct when Valkey is absent or
// evicted.
func AlertCancelled(ctx context.Context, cs *valkey.CancelSet, as AlertLookup, fingerprint string, alertNumber int64) bool {
	if alertNumber > 0 {
		if cs != nil && cs.Contains(ctx, valkey.CancelKeyAlertNum(alertNumber)) {
			return true
		}
		if as != nil {
			rec, err := as.GetByAlertNumber(alertNumber)
			if err == nil && rec != nil && rec.DeletedAt != nil {
				return true
			}
		}
		return false
	}
	if cs != nil && cs.Contains(ctx, valkey.CancelKeyAlert(fingerprint)) {
		return true
	}
	if as != nil && fingerprint != "" {
		rec, err := as.GetOpenByFingerprint(fingerprint)
		if errors.Is(err, store.ErrNotFound) || (err == nil && rec == nil) {
			return true
		}
	}
	return false
}

func IncidentCancelled(ctx context.Context, cs *valkey.CancelSet, is store.IncidentStore, incidentNumber int64) bool {
	if cs != nil && cs.Contains(ctx, valkey.CancelKeyIncident(incidentNumber)) {
		return true
	}
	if is != nil && incidentNumber > 0 {
		if inc, err := is.GetIncident(ctx, incidentNumber); err == nil && inc != nil && inc.DeletedAt != nil {
			return true
		}
	}
	return false
}

func InvestigationCancelled(ctx context.Context, cs *valkey.CancelSet, investigationID string) bool {
	if cs != nil && investigationID != "" {
		return cs.Contains(ctx, valkey.CancelKeyInvestigation(investigationID))
	}
	return false
}
