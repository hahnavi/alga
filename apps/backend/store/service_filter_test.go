package store

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestListServicesFilter_DefaultZeroValues(t *testing.T) {
	t.Parallel()
	var f ListServicesFilter
	if f.Status != "" {
		t.Errorf("default Status = %q, want empty", f.Status)
	}
	if f.Query != "" {
		t.Errorf("default Query = %q, want empty", f.Query)
	}
	if f.Limit != 0 {
		t.Errorf("default Limit = %d, want 0", f.Limit)
	}
	if f.Skip != 0 {
		t.Errorf("default Skip = %d, want 0", f.Skip)
	}
}

func TestListServicesFilter_ConstructedWithValues(t *testing.T) {
	t.Parallel()
	f := ListServicesFilter{
		Status: "operational",
		Query:  "api-gateway",
		Limit:  10,
		Skip:   5,
	}
	if f.Status != "operational" {
		t.Errorf("Status = %q, want operational", f.Status)
	}
	if f.Query != "api-gateway" {
		t.Errorf("Query = %q, want api-gateway", f.Query)
	}
	if f.Limit != 10 {
		t.Errorf("Limit = %d, want 10", f.Limit)
	}
	if f.Skip != 5 {
		t.Errorf("Skip = %d, want 5", f.Skip)
	}
}

func TestServiceRecord_DefaultStatusIsEmpty(t *testing.T) {
	t.Parallel()
	var r ServiceRecord
	if r.Status != "" {
		t.Errorf("default Status = %q, want empty (store sets operational)", r.Status)
	}
	if r.SLAResponseMinutes != 0 {
		t.Errorf("default SLAResponseMinutes = %d, want 0", r.SLAResponseMinutes)
	}
	if r.SLAResolveMinutes != 0 {
		t.Errorf("default SLAResolveMinutes = %d, want 0", r.SLAResolveMinutes)
	}
	if r.Name != "" {
		t.Errorf("default Name = %q, want empty", r.Name)
	}
	if r.ActiveIncidentCount != 0 {
		t.Errorf("default ActiveIncidentCount = %d, want 0", r.ActiveIncidentCount)
	}
}

func TestServiceRecord_WithAllFields(t *testing.T) {
	t.Parallel()
	teamID := uuid.New()
	polID := uuid.New()
	r := ServiceRecord{
		ID:                 uuid.New(),
		Name:               "payment-service",
		DisplayName:        "Payment Service",
		Description:        "Handles payments",
		OwnerTeamID:        &teamID,
		EscalationPolicyID: &polID,
		SLAResponseMinutes: 15,
		SLAResolveMinutes:  60,
		Status:             "degraded",
	}
	if r.Name != "payment-service" {
		t.Errorf("Name = %q", r.Name)
	}
	if *r.OwnerTeamID != teamID {
		t.Error("OwnerTeamID mismatch")
	}
	if *r.EscalationPolicyID != polID {
		t.Error("EscalationPolicyID mismatch")
	}
	if r.SLAResponseMinutes != 15 {
		t.Errorf("SLAResponseMinutes = %d", r.SLAResponseMinutes)
	}
	if r.SLAResolveMinutes != 60 {
		t.Errorf("SLAResolveMinutes = %d", r.SLAResolveMinutes)
	}
	if r.Status != "degraded" {
		t.Errorf("Status = %q", r.Status)
	}
}

func TestServiceDependencyRecord_Fields(t *testing.T) {
	t.Parallel()
	svcID := uuid.New()
	depID := uuid.New()
	r := ServiceDependencyRecord{
		ID:                     uuid.New(),
		ServiceID:              svcID,
		DependentOnServiceID:   depID,
		DependencyType:         "hard",
		CreatedAt:              time.Now(),
		DependentOnServiceName: "auth-service",
	}
	if r.ServiceID != svcID {
		t.Error("ServiceID mismatch")
	}
	if r.DependentOnServiceID != depID {
		t.Error("DependentOnServiceID mismatch")
	}
	if r.DependencyType != "hard" {
		t.Errorf("DependencyType = %q", r.DependencyType)
	}
	if r.DependentOnServiceName != "auth-service" {
		t.Errorf("DependentOnServiceName = %q", r.DependentOnServiceName)
	}
}
