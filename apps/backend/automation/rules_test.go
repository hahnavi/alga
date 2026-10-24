package automation

import (
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
)

func TestValidateRuleAndMatch(t *testing.T) {
	policy := HostPolicy{Allowed: []string{"hooks.example.com"}}
	definition := Definition{
		Name: "Critical database alert", TriggerType: "alert.created",
		Conditions: []Condition{{Field: "severity", Operator: "equals", Value: "critical"}, {Field: "label.alertname", Operator: "equals", Value: "DBDown"}},
		Actions:    []Action{{Type: "webhook", URL: "https://hooks.example.com/alga"}},
	}
	if err := Validate(definition, policy); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}
	matching, err := json.Marshal(Event{EventType: "alert.created", Severity: "critical", Labels: map[string]string{"alertname": "DBDown"}})
	if err != nil {
		t.Fatal(err)
	}
	if !Match(matching, definition.Conditions) {
		t.Fatal("expected matching event")
	}
	unmatched, err := json.Marshal(Event{Severity: "warning", Labels: map[string]string{"alertname": "DBDown"}})
	if err != nil {
		t.Fatal(err)
	}
	if Match(unmatched, definition.Conditions) {
		t.Fatal("wrong severity matched")
	}
	silenced, err := json.Marshal(Event{EntityType: "alert", Severity: "critical", Silenced: true, Labels: map[string]string{"alertname": "DBDown"}})
	if err != nil {
		t.Fatal(err)
	}
	if Match(silenced, definition.Conditions) {
		t.Fatal("silenced alert matched")
	}
	definition.Actions[0].URL = "http://hooks.example.com/alga"
	if err := Validate(definition, policy); err == nil {
		t.Fatal("HTTP destination accepted")
	}
	definition.Actions[0].URL = "https://unapproved.example.com/alga"
	if err := Validate(definition, policy); err == nil {
		t.Fatal("unapproved destination accepted")
	}
	definition.Actions[0].URL = "https://hooks.example.com/" + strings.Repeat("a", 2048)
	if err := Validate(definition, policy); err == nil {
		t.Fatal("oversized destination accepted")
	}
}

func TestHostPolicyBlocksUnsafeAddresses(t *testing.T) {
	policy := HostPolicy{Allowed: []string{"hooks.example.com"}, PrivateHosts: []string{"hooks.example.com"}}
	for _, raw := range []string{"127.0.0.1", "169.254.169.254", "100.64.0.1", "198.51.100.2", "::1", "::ffff:127.0.0.1"} {
		if policy.PermitIP("hooks.example.com", netip.MustParseAddr(raw)) {
			t.Errorf("unsafe address allowed: %s", raw)
		}
	}
	if !policy.PermitIP("hooks.example.com", netip.MustParseAddr("10.0.0.4")) {
		t.Fatal("opted-in private host rejected")
	}
	if policy.PermitIP("other.example.com", netip.MustParseAddr("10.0.0.4")) {
		t.Fatal("unapproved private host allowed")
	}
	if !policy.PermitIP("hooks.example.com", netip.MustParseAddr("1.1.1.1")) {
		t.Fatal("public address rejected")
	}
}

func TestIncidentNoteRequiresIncidentTrigger(t *testing.T) {
	d := Definition{Name: "Note", TriggerType: "alert.created", Actions: []Action{{Type: "incident_note", Message: "Review"}}}
	if err := Validate(d, HostPolicy{}); err == nil {
		t.Fatal("incident note accepted for alert trigger")
	}
	d.TriggerType = "incident.created"
	if err := Validate(d, HostPolicy{}); err != nil {
		t.Fatalf("incident note rejected: %v", err)
	}
}
