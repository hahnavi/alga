package correlator

import (
	"reflect"
	"strings"
	"testing"
)

func TestCorrelationKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		labels  map[string]string
		wantKey string
		wantDis map[string]string
	}{
		{
			name:    "deployment_wins",
			labels:  map[string]string{"namespace": "prod", "deployment": "api", "alertname": "HighCPU"},
			wantKey: "prod:api:HighCPU",
			wantDis: map[string]string{"namespace": "prod", "alertname": "HighCPU", "deployment": "api"},
		},
		{
			name:    "statefulset_when_no_deployment",
			labels:  map[string]string{"namespace": "prod", "statefulset": "db", "alertname": "HighDisk"},
			wantKey: "prod:db:HighDisk",
			wantDis: map[string]string{"namespace": "prod", "alertname": "HighDisk", "statefulset": "db"},
		},
		{
			name:    "fallback_namespace_alertname",
			labels:  map[string]string{"namespace": "prod", "alertname": "Pending"},
			wantKey: "prod:Pending",
			wantDis: map[string]string{"namespace": "prod", "alertname": "Pending"},
		},
		{
			name:    "empty_labels_produces_unique_unkeyed",
			labels:  nil,
			wantKey: "unkeyed:e3b0c44298fc1c14",
			wantDis: map[string]string{},
		},
		{
			name:    "trims_whitespace",
			labels:  map[string]string{"namespace": " prod ", "deployment": " api ", "alertname": " HighCPU "},
			wantKey: "prod:api:HighCPU",
			wantDis: map[string]string{"namespace": "prod", "alertname": "HighCPU", "deployment": "api"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotKey, gotDis := CorrelationKey(tc.labels)
			if gotKey != tc.wantKey {
				t.Errorf("key = %q want %q", gotKey, tc.wantKey)
			}
			if !reflect.DeepEqual(gotDis, tc.wantDis) {
				t.Errorf("discriminators = %+v want %+v", gotDis, tc.wantDis)
			}
		})
	}
}

func TestCorrelationKeyDeploymentBeatsStatefulset(t *testing.T) {
	t.Parallel()
	labels := map[string]string{
		"namespace":   "prod",
		"deployment":  "api",
		"statefulset": "db",
		"alertname":   "HighCPU",
	}
	key, _ := CorrelationKey(labels)
	if key != "prod:api:HighCPU" {
		t.Fatalf("deployment must take precedence, got %q", key)
	}
}

func TestCorrelationKeyWithSoloStrategy(t *testing.T) {
	t.Parallel()
	labels := map[string]string{
		"namespace": "prod",
		"alertname": "Watchdog",
	}
	rule := CorrelationRule{Strategy: "solo"}
	key, _ := CorrelationKeyWithRules(labels, &rule)
	if key == "" {
		t.Fatal("expected non-empty key")
	}
	if !strings.HasPrefix(key, "solo:") {
		t.Fatalf("expected solo: prefix, got %q", key)
	}
}

func TestCorrelationKeyWithNamespaceStrategy(t *testing.T) {
	t.Parallel()
	labels := map[string]string{
		"namespace":  "prod",
		"deployment": "api",
		"alertname":  "HighCPU",
	}
	rule := CorrelationRule{Strategy: "namespace"}
	key, _ := CorrelationKeyWithRules(labels, &rule)
	if key != "ns:prod" {
		t.Fatalf("expected ns:prod, got %q", key)
	}
}

func TestCorrelationKeyWithLabelMatchStrategy(t *testing.T) {
	t.Parallel()
	labels := map[string]string{
		"namespace": "prod",
		"alertname": "HighCPU",
		"cluster":   "us-east-1",
	}
	rule := CorrelationRule{
		Strategy:      "label_match",
		GroupByLabels: []string{"namespace", "cluster"},
	}
	key, _ := CorrelationKeyWithRules(labels, &rule)
	if key != "lbl:prod:us-east-1" {
		t.Fatalf("expected lbl:prod:us-east-1, got %q", key)
	}
}

func TestCorrelationKeyWithRulesNilFallsBack(t *testing.T) {
	t.Parallel()
	labels := map[string]string{"namespace": "prod", "deployment": "api", "alertname": "HighCPU"}
	key, disc := CorrelationKeyWithRules(labels, nil)
	fallbackKey, fallbackDisc := CorrelationKey(labels)
	if key != fallbackKey {
		t.Fatalf("nil rule should fall back: got %q, want %q", key, fallbackKey)
	}
	if len(disc) != len(fallbackDisc) {
		t.Fatalf("discriminators mismatch")
	}
}

func TestIsSuppressed(t *testing.T) {
	t.Parallel()
	rules := []SuppressionRule{
		{AlertName: "Watchdog"},
		{AlertName: "InfoInhibitor"},
	}
	if !IsSuppressed(map[string]string{"alertname": "Watchdog"}, rules) {
		t.Fatal("Watchdog should be suppressed")
	}
	if !IsSuppressed(map[string]string{"alertname": "InfoInhibitor"}, rules) {
		t.Fatal("InfoInhibitor should be suppressed")
	}
	if IsSuppressed(map[string]string{"alertname": "HighCPU"}, rules) {
		t.Fatal("HighCPU should not be suppressed")
	}
}

func TestIsSuppressedWithNamespace(t *testing.T) {
	t.Parallel()
	rules := []SuppressionRule{
		{AlertName: "Watchdog", Namespace: "staging"},
	}
	if !IsSuppressed(map[string]string{"alertname": "Watchdog", "namespace": "staging"}, rules) {
		t.Fatal("should suppress")
	}
	if IsSuppressed(map[string]string{"alertname": "Watchdog", "namespace": "prod"}, rules) {
		t.Fatal("should not suppress different namespace")
	}
}
