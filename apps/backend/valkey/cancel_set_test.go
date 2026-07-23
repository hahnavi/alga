package valkey

import "testing"

func TestCancelKeys(t *testing.T) {
	if got := CancelKeyAlert("fp1"); got != "alga:cancel:alert:fp1" {
		t.Fatalf("alert key = %q", got)
	}
	if got := CancelKeyAlertNum(42); got != "alga:cancel:alert_num:42" {
		t.Fatalf("alert_num key = %q", got)
	}
	if got := CancelKeyIncident(7); got != "alga:cancel:incident:7" {
		t.Fatalf("incident key = %q", got)
	}
	if got := CancelKeyInvestigation("INV-1"); got != "alga:cancel:investigation:INV-1" {
		t.Fatalf("investigation key = %q", got)
	}
}

func TestCancelSetNilSafe(t *testing.T) {
	var cs *CancelSet
	if cs.Available() {
		t.Fatal("nil CancelSet must not be Available")
	}
	if cs.Contains(t.Context(), "alga:cancel:alert:fp1") {
		t.Fatal("nil CancelSet Contains must return false")
	}
	if err := cs.Add(t.Context(), "alga:cancel:alert:fp1"); err != nil {
		t.Fatalf("nil CancelSet Add must be a no-op, got %v", err)
	}
}
