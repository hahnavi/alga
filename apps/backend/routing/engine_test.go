package routing

import (
	"testing"

	"alga/config"
	"alga/types"
)

func TestEngineRoute_MultiTarget(t *testing.T) {
	e := NewEngine([]config.RouteConfig{
		{
			MatchMode: "all",
			Conditions: []config.RouteCondition{
				{Source: "labels", Field: "team", Operator: "exact", Value: "a"},
			},
			Targets: []config.RouteTarget{
				{Provider: "mattermost", Channel: "ch1"},
				{Provider: "slack", Channel: "C99"},
			},
		},
	})
	alert := types.Alert{
		Labels: map[string]string{"team": "a", "alertname": "x"},
	}
	res := e.Route(alert)
	if res.Silenced {
		t.Fatal("expected not silenced")
	}
	if len(res.Destinations) != 2 {
		t.Fatalf("got %d destinations", len(res.Destinations))
	}
	if res.Destinations[0].Channel != "ch1" || res.Destinations[0].Provider != "mattermost" {
		t.Fatalf("dest0: %+v", res.Destinations[0])
	}
	if res.Destinations[1].Channel != "C99" || res.Destinations[1].Provider != "slack" {
		t.Fatalf("dest1: %+v", res.Destinations[1])
	}
}

func TestEngineRoute_SilencedRule(t *testing.T) {
	e := NewEngine([]config.RouteConfig{
		{
			Conditions: []config.RouteCondition{
				{Source: "labels", Field: "x", Operator: "exact", Value: "y"},
			},
			Silenced:  true,
			MatchMode: "all",
		},
	})
	alert := types.Alert{Labels: map[string]string{"x": "y"}}
	res := e.Route(alert)
	if !res.Silenced || len(res.Destinations) != 0 {
		t.Fatalf("got %+v", res)
	}
}

func TestEngineRoute_NoRulesUnsilenced(t *testing.T) {
	e := NewEngine(nil)
	res := e.Route(types.Alert{Labels: map[string]string{"a": "b"}})
	// "Nothing configured" stores the alert unsilenced without delivery; it
	// must not be conflated with an explicit silenced rule (silence also
	// suppresses investigations).
	if res.Silenced {
		t.Fatal("expected unsilenced with no rules and no defaults")
	}
	if len(res.Destinations) != 0 {
		t.Fatalf("expected no destinations, got %d", len(res.Destinations))
	}
}

func TestEngineRoute_DefaultsUsedWhenNoMatch(t *testing.T) {
	e := NewEngine(nil)
	e.SetDefaults([]Destination{
		{Provider: "mattermost", Channel: "alerts"},
		{Provider: "slack", Channel: "C111"},
	})
	res := e.Route(types.Alert{Labels: map[string]string{"a": "b"}})
	if res.Silenced {
		t.Fatal("expected not silenced when defaults are set")
	}
	if len(res.Destinations) != 2 {
		t.Fatalf("expected 2 destinations, got %d", len(res.Destinations))
	}
	if res.Destinations[0].Channel != "alerts" {
		t.Fatalf("expected alerts channel, got %s", res.Destinations[0].Channel)
	}
	if res.Destinations[1].Channel != "C111" {
		t.Fatalf("expected C111 channel, got %s", res.Destinations[1].Channel)
	}
}

func TestEngineRoute_DefaultsMergedWithRuleMatch(t *testing.T) {
	e := NewEngine([]config.RouteConfig{
		{
			MatchMode: "all",
			Conditions: []config.RouteCondition{
				{Source: "labels", Field: "team", Operator: "exact", Value: "backend"},
			},
			Targets: []config.RouteTarget{
				{Provider: "mattermost", Channel: "backend-alerts"},
			},
		},
	})
	e.SetDefaults([]Destination{
		{Provider: "mattermost", Channel: "alerts"},
		{Provider: "slack", Channel: "C111"},
	})
	alert := types.Alert{Labels: map[string]string{"team": "backend"}}
	res := e.Route(alert)
	if res.Silenced {
		t.Fatal("expected not silenced")
	}
	if len(res.Destinations) != 3 {
		t.Fatalf("expected 3 destinations (2 default + 1 rule), got %d", len(res.Destinations))
	}
	if res.Destinations[0].Channel != "alerts" {
		t.Fatalf("expected first dest to be default 'alerts', got %s", res.Destinations[0].Channel)
	}
	if res.Destinations[1].Channel != "C111" {
		t.Fatalf("expected second dest to be default 'C111', got %s", res.Destinations[1].Channel)
	}
	if res.Destinations[2].Channel != "backend-alerts" {
		t.Fatalf("expected third dest to be rule 'backend-alerts', got %s", res.Destinations[2].Channel)
	}
}

func TestEngineRoute_DefaultsDeduplicated(t *testing.T) {
	e := NewEngine([]config.RouteConfig{
		{
			MatchMode: "all",
			Conditions: []config.RouteCondition{
				{Source: "labels", Field: "team", Operator: "exact", Value: "backend"},
			},
			Targets: []config.RouteTarget{
				{Provider: "mattermost", Channel: "alerts"},
			},
		},
	})
	e.SetDefaults([]Destination{
		{Provider: "mattermost", Channel: "alerts"},
	})
	alert := types.Alert{Labels: map[string]string{"team": "backend"}}
	res := e.Route(alert)
	if res.Silenced {
		t.Fatal("expected not silenced")
	}
	if len(res.Destinations) != 1 {
		t.Fatalf("expected 1 destination (deduplicated), got %d", len(res.Destinations))
	}
}

func TestEngineRoute_SilencedOverridesDefaults(t *testing.T) {
	e := NewEngine([]config.RouteConfig{
		{
			Conditions: []config.RouteCondition{
				{Source: "labels", Field: "severity", Operator: "exact", Value: "low"},
			},
			Silenced:  true,
			MatchMode: "all",
		},
	})
	e.SetDefaults([]Destination{
		{Provider: "mattermost", Channel: "alerts"},
	})
	alert := types.Alert{Labels: map[string]string{"severity": "low"}}
	res := e.Route(alert)
	if !res.Silenced {
		t.Fatal("silenced rule should override defaults")
	}
}
