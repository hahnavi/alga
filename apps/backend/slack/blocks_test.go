package slack

import (
	"encoding/json"
	"strings"
	"testing"

	"alga/types"
)

func TestBuildAlertBlocks_OmitsStyleForNeutralAcknowledgeButton(t *testing.T) {
	alert := types.Alert{
		Status: "firing",
		Labels: map[string]string{
			"alertname": "TestAlert",
		},
		Annotations: map[string]string{
			"summary": "Test summary",
		},
		Fingerprint: "fp-123",
	}

	blocks, _ := BuildAlertBlocks(alert)
	elements := firstActionsElements(t, blocks)
	if got, want := len(elements), 2; got != want {
		t.Fatalf("unexpected action count: got %d want %d", got, want)
	}

	ack := elements[0]
	if ack.ActionID != "acknowledge" {
		t.Fatalf("unexpected first action id: got %q", ack.ActionID)
	}
	if ack.Style != "" {
		t.Fatalf("expected acknowledge style to be omitted, got %q", ack.Style)
	}

	resolve := elements[1]
	if resolve.ActionID != "resolve" {
		t.Fatalf("unexpected second action id: got %q", resolve.ActionID)
	}
	if resolve.Style != "primary" {
		t.Fatalf("expected resolve style to remain primary, got %q", resolve.Style)
	}
}

func TestBuildAlertBlocks_ContextElementsAreTextObjects(t *testing.T) {
	alert := types.Alert{
		Status:       "firing",
		Labels:       map[string]string{"alertname": "TestAlert"},
		Annotations:  map[string]string{"summary": "Test summary"},
		Fingerprint:  "fp-ctx",
		GeneratorURL: "https://grafana.example/alert",
	}

	blocks, _ := BuildAlertBlocks(alert)
	encoded, err := json.Marshal(blocks)
	if err != nil {
		t.Fatalf("failed to marshal blocks: %v", err)
	}

	var decoded []map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("failed to unmarshal blocks: %v", err)
	}

	for _, block := range decoded {
		if blockType, _ := block["type"].(string); blockType == "context" {
			rawElements, ok := block["elements"].([]any)
			if !ok {
				t.Fatalf("context elements missing or invalid type")
			}
			for _, raw := range rawElements {
				elem, ok := raw.(map[string]any)
				if !ok {
					t.Fatalf("context element has invalid type")
				}
				if _, isString := elem["text"].(string); !isString {
					t.Fatalf("context element 'text' should be a string, got %T: %v", elem["text"], elem["text"])
				}
			}
			return
		}
	}
	t.Fatalf("context block not found")
}

func TestBuildAlertBlocks_UsesOpsgenieStyleWithoutDotOrFingerprint(t *testing.T) {
	alert := types.Alert{
		Status: "firing",
		Labels: map[string]string{
			"alertname": "HighCPU",
			"severity":  "critical",
			"service":   "api",
		},
		Annotations: map[string]string{
			"description": "CPU usage is above threshold",
		},
		Fingerprint: "fp-hidden",
	}

	blocks, fallback := BuildAlertBlocks(alert)
	if fallback != "[Open] HighCPU" {
		t.Fatalf("unexpected fallback: %q", fallback)
	}

	encoded, err := EncodeBlocks(blocks)
	if err != nil {
		t.Fatalf("EncodeBlocks returned error: %v", err)
	}
	for _, unwanted := range []string{"🔴", "🟡", "🟢", "FP:"} {
		if strings.Contains(encoded, unwanted) {
			t.Fatalf("encoded blocks should not contain %q: %s", unwanted, encoded)
		}
	}
	if !strings.Contains(encoded, "*Alert: HighCPU*") {
		t.Fatalf("encoded blocks should include alert title: %s", encoded)
	}
	if !strings.Contains(encoded, "*Status*\\nOpen") {
		t.Fatalf("encoded blocks should include status field: %s", encoded)
	}
}

func TestEncodeBlocks_DoesNotSerializeInvalidDefaultStyle(t *testing.T) {
	alert := types.Alert{
		Status: "firing",
		Labels: map[string]string{
			"alertname": "TestAlert",
		},
		Annotations: map[string]string{
			"description": "Load is high",
		},
		Fingerprint: "fp-456",
	}

	blocks, _ := BuildAlertBlocks(alert)
	encoded, err := EncodeBlocks(blocks)
	if err != nil {
		t.Fatalf("EncodeBlocks returned error: %v", err)
	}
	if strings.Contains(encoded, `"style":"default"`) {
		t.Fatalf("encoded blocks should never contain default style: %s", encoded)
	}

	var decoded []map[string]any
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatalf("failed decoding encoded blocks: %v", err)
	}

	actions := mapActionElements(t, decoded)
	if got, want := len(actions), 2; got != want {
		t.Fatalf("unexpected encoded action count: got %d want %d", got, want)
	}
	if _, ok := actions[0]["style"]; ok {
		t.Fatalf("acknowledge button style should be omitted in encoded payload")
	}
	if got, ok := actions[1]["style"].(string); !ok || got != "primary" {
		t.Fatalf("resolve button style should be primary, got %#v", actions[1]["style"])
	}
}

func TestBuildAlertAttachments_UsesStatusColor(t *testing.T) {
	tests := []struct {
		name  string
		alert types.Alert
		want  string
	}{
		{
			name: "firing",
			alert: types.Alert{
				Status:      "firing",
				Labels:      map[string]string{"alertname": "HighCPU"},
				Annotations: map[string]string{"summary": "CPU is high"},
			},
			want: "#E01E5A",
		},
		{
			name: "acknowledged",
			alert: types.Alert{
				Status:       "firing",
				Acknowledged: true,
				Labels:       map[string]string{"alertname": "HighCPU"},
				Annotations:  map[string]string{"summary": "CPU is high"},
			},
			want: "#36C5F0",
		},
		{
			name: "resolved",
			alert: types.Alert{
				Status:      "resolved",
				Labels:      map[string]string{"alertname": "HighCPU"},
				Annotations: map[string]string{"summary": "CPU is high"},
			},
			want: "#2EB67D",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attachments, fallback := BuildAlertAttachments(tt.alert)
			if fallback == "" {
				t.Fatalf("fallback should not be empty")
			}
			if got, want := len(attachments), 1; got != want {
				t.Fatalf("unexpected attachment count: got %d want %d", got, want)
			}
			if got := attachments[0].Color; got != tt.want {
				t.Fatalf("unexpected attachment color: got %q want %q", got, tt.want)
			}
			if got := len(attachments[0].Blocks); got == 0 {
				t.Fatalf("attachment should include alert blocks")
			}
		})
	}
}

func TestBuildAlertAttachmentsUpdate_RemovesActionsAndKeepsStatusColor(t *testing.T) {
	alert := types.Alert{
		Status:       "firing",
		Acknowledged: true,
		Labels:       map[string]string{"alertname": "HighCPU"},
		Annotations:  map[string]string{"summary": "CPU is high"},
		Fingerprint:  "fp-update",
	}

	attachments, _ := BuildAlertAttachmentsUpdate(alert, "acknowledge", "alex")
	if got, want := len(attachments), 1; got != want {
		t.Fatalf("unexpected attachment count: got %d want %d", got, want)
	}
	if got, want := attachments[0].Color, "#36C5F0"; got != want {
		t.Fatalf("unexpected attachment color: got %q want %q", got, want)
	}
	for _, block := range attachments[0].Blocks {
		if block.Type == "actions" {
			t.Fatalf("updated alert attachment should not include action buttons")
		}
	}
}

func firstActionsElements(t *testing.T, blocks []Block) []BlockElement {
	t.Helper()
	for _, block := range blocks {
		if block.Type == "actions" {
			elements, ok := block.Elements.([]BlockElement)
			if !ok {
				t.Fatalf("actions block elements are not []BlockElement")
			}
			return elements
		}
	}
	t.Fatalf("actions block not found")
	return nil
}

func mapActionElements(t *testing.T, blocks []map[string]any) []map[string]any {
	t.Helper()
	for _, block := range blocks {
		if blockType, _ := block["type"].(string); blockType == "actions" {
			rawElements, ok := block["elements"].([]any)
			if !ok {
				t.Fatalf("actions elements missing or invalid type")
			}
			out := make([]map[string]any, 0, len(rawElements))
			for _, raw := range rawElements {
				element, ok := raw.(map[string]any)
				if !ok {
					t.Fatalf("action element has invalid type")
				}
				out = append(out, element)
			}
			return out
		}
	}
	t.Fatalf("actions block not found in encoded payload")
	return nil
}
