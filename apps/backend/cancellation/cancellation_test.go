package cancellation

import (
	"context"
	"errors"
	"testing"
	"time"

	"alga/store"
)

type fakeAlertLookup struct {
	byNumber   map[int64]*store.AlertRecord
	byOpenFP   map[string]*store.AlertRecord
	numErr     error
	fpErr      error
	numCalls   int
	fpCalls    int
	lastNumber int64
	lastFP     string
}

func (f *fakeAlertLookup) GetByAlertNumber(n int64) (*store.AlertRecord, error) {
	f.numCalls++
	f.lastNumber = n
	if f.numErr != nil {
		return nil, f.numErr
	}
	if rec, ok := f.byNumber[n]; ok {
		return rec, nil
	}
	return nil, store.ErrNotFound
}

func (f *fakeAlertLookup) GetOpenByFingerprint(fp string) (*store.AlertRecord, error) {
	f.fpCalls++
	f.lastFP = fp
	if f.fpErr != nil {
		return nil, f.fpErr
	}
	if rec, ok := f.byOpenFP[fp]; ok {
		return rec, nil
	}
	return nil, store.ErrNotFound
}

func deletedAt(t time.Time) *time.Time { return &t }

// Regression: alert #33 was dropped as "primary alert deleted" because the
// cancel-set fingerprint key from a previously-resolved alert with the same
// fingerprint (alert #31) was still alive. The fix routes the canonical check
// through alert_number so a reused fingerprint cannot suppress the new alert.
func TestAlertCancelled_ReusedFingerprintWithCanonicalAlertNumber(t *testing.T) {
	t.Parallel()

	deleted := deletedAt(time.Date(2026, 6, 21, 2, 59, 35, 0, time.UTC))
	lookup := &fakeAlertLookup{
		byNumber: map[int64]*store.AlertRecord{
			31: {AlertNumber: 31, Fingerprint: "c266bcfcc08677bb", Status: "resolved", DeletedAt: deleted},
			33: {AlertNumber: 33, Fingerprint: "c266bcfcc08677bb", Status: "firing"},
		},
	}

	if AlertCancelled(context.Background(), nil, lookup, "c266bcfcc08677bb", 33) {
		t.Fatal("alert #33 (live, fingerprint reused by deleted #31) must not be reported as cancelled")
	}
	if lookup.numCalls != 1 || lookup.lastNumber != 33 {
		t.Fatalf("expected canonical GetByAlertNumber(33), got numCalls=%d lastNumber=%d", lookup.numCalls, lookup.lastNumber)
	}
	if lookup.fpCalls != 0 {
		t.Fatalf("fingerprint lookup must not be consulted when alert_number is provided (calls=%d)", lookup.fpCalls)
	}
}

func TestAlertCancelled_SoftDeletedByNumber(t *testing.T) {
	t.Parallel()

	deleted := deletedAt(time.Now().Add(-time.Minute))
	lookup := &fakeAlertLookup{
		byNumber: map[int64]*store.AlertRecord{
			42: {AlertNumber: 42, Fingerprint: "fp-42", Status: "resolved", DeletedAt: deleted},
		},
	}

	if !AlertCancelled(context.Background(), nil, lookup, "fp-42", 42) {
		t.Fatal("soft-deleted alert #42 must be reported as cancelled via canonical alert_number")
	}
}

func TestAlertCancelled_LiveAlertByNumber(t *testing.T) {
	t.Parallel()

	lookup := &fakeAlertLookup{
		byNumber: map[int64]*store.AlertRecord{
			42: {AlertNumber: 42, Fingerprint: "fp-42", Status: "firing"},
		},
	}

	if AlertCancelled(context.Background(), nil, lookup, "fp-42", 42) {
		t.Fatal("live alert #42 must not be reported as cancelled")
	}
}

func TestAlertCancelled_MissingAlertByNumber(t *testing.T) {
	t.Parallel()

	lookup := &fakeAlertLookup{byNumber: map[int64]*store.AlertRecord{}}

	if AlertCancelled(context.Background(), nil, lookup, "fp-missing", 99) {
		t.Fatal("missing alert #99 must not be reported as cancelled via the canonical path")
	}
}

func TestAlertCancelled_NumberLookupErrorDoesNotCancel(t *testing.T) {
	t.Parallel()

	lookup := &fakeAlertLookup{numErr: errors.New("pg unavailable")}

	if AlertCancelled(context.Background(), nil, lookup, "fp-x", 7) {
		t.Fatal("lookup errors must not silently cancel an alert; the canonical path should defer to durable work")
	}
}

func TestAlertCancelled_FingerprintFallbackOpenMissing(t *testing.T) {
	t.Parallel()

	lookup := &fakeAlertLookup{byOpenFP: map[string]*store.AlertRecord{}}

	if !AlertCancelled(context.Background(), nil, lookup, "fp-none", 0) {
		t.Fatal("fingerprint fallback must report cancelled when no open alert exists for the fingerprint")
	}
}

func TestAlertCancelled_FingerprintFallbackOpenPresent(t *testing.T) {
	t.Parallel()

	lookup := &fakeAlertLookup{
		byOpenFP: map[string]*store.AlertRecord{
			"fp-live": {AlertNumber: 5, Fingerprint: "fp-live", Status: "firing"},
		},
	}

	if AlertCancelled(context.Background(), nil, lookup, "fp-live", 0) {
		t.Fatal("fingerprint fallback must not cancel an alert that still has an open row")
	}
}

func TestAlertCancelled_NilLookupSafe(t *testing.T) {
	t.Parallel()

	// With no Valkey and no lookup, the function cannot confirm cancellation
	// and must defer to the durable PG guard rather than dropping in-flight work.
	if AlertCancelled(context.Background(), nil, nil, "fp-x", 0) {
		t.Fatal("nil lookup + nil Valkey must not report the alert as cancelled")
	}
	if AlertCancelled(context.Background(), nil, nil, "", 0) {
		t.Fatal("empty fingerprint + no alert_number + nil lookup must not be cancelled")
	}
}
