package slack

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"alga/types"
)

type Block struct {
	Type      string        `json:"type"`
	Text      *TextObject   `json:"text,omitempty"`
	Fields    []*TextObject `json:"fields,omitempty"`
	Elements  any           `json:"elements,omitempty"`
	Accessory *BlockElement `json:"accessory,omitempty"`
}

type TextObject struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Emoji bool   `json:"emoji,omitempty"`
}

type BlockElement struct {
	Type     string         `json:"type"`
	Text     *TextObject    `json:"text,omitempty"`
	ActionID string         `json:"action_id,omitempty"`
	Value    string         `json:"value,omitempty"`
	Style    string         `json:"style,omitempty"`
	URL      string         `json:"url,omitempty"`
	Elements []BlockElement `json:"elements,omitempty"`
}

type Attachment struct {
	Color    string  `json:"color,omitempty"`
	Fallback string  `json:"fallback,omitempty"`
	Blocks   []Block `json:"blocks,omitempty"`
}

func textObject(textType, text string) *TextObject {
	return &TextObject{Type: textType, Text: text}
}

func plainText(text string) *TextObject {
	return textObject("plain_text", text)
}

func mrkdwnText(text string) *TextObject {
	return textObject("mrkdwn", text)
}

func sectionBlock(text string) Block {
	return Block{Type: "section", Text: mrkdwnText(text)}
}

func sectionBlockWithFields(fields ...*TextObject) Block {
	return Block{Type: "section", Fields: fields}
}

func actionsBlock(elements ...BlockElement) Block {
	return Block{Type: "actions", Elements: elements}
}

func contextBlock(elements ...*TextObject) Block {
	return Block{Type: "context", Elements: elements}
}

func buttonElement(text, actionID, value, style string) BlockElement {
	return BlockElement{
		Type:     "button",
		Text:     plainText(text),
		ActionID: actionID,
		Value:    value,
		Style:    style,
	}
}

func alertStatusColor(alert types.Alert) string {
	switch {
	case alert.Status == "resolved":
		return "#2EB67D"
	case alert.Acknowledged:
		return "#36C5F0"
	default:
		return "#E01E5A"
	}
}

func alertDisplayName(alert types.Alert) string {
	if alert.Labels != nil {
		if name := strings.TrimSpace(alert.Labels["alertname"]); name != "" {
			return name
		}
	}
	if alert.Annotations != nil {
		if summary := strings.TrimSpace(alert.Annotations["summary"]); summary != "" {
			return summary
		}
	}
	return "Alert"
}

func alertStatusLabel(alert types.Alert) string {
	switch {
	case alert.Status == "resolved":
		return "Resolved"
	case alert.Acknowledged:
		return "Acknowledged"
	default:
		return "Open"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func appendField(fields []*TextObject, label, value string) []*TextObject {
	if value == "" {
		return fields
	}
	return append(fields, mrkdwnText(fmt.Sprintf("*%s*\n%s", label, value)))
}

func formatAlertTime(value string) string {
	if value == "" || value == "0001-01-01T00:00:00Z" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return ""
	}
	return t.Format("2006-01-02 15:04 MST")
}

func formattedLabelPairs(labels map[string]string) []string {
	if len(labels) == 0 {
		return nil
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		switch key {
		case "alertname", "fingerprint", "severity", "priority", "service", "app", "application", "instance", "job", "namespace":
			continue
		default:
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)

	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, fmt.Sprintf("`%s=%s`", key, labels[key]))
	}
	return pairs
}

func formattedValue(alert types.Alert) string {
	if strings.TrimSpace(alert.ValueString) != "" {
		return alert.ValueString
	}
	if len(alert.Values) == 0 {
		return ""
	}

	keys := make([]string, 0, len(alert.Values))
	for key := range alert.Values {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, alert.Values[key]))
	}
	return strings.Join(parts, ", ")
}

func BuildAlertBlocks(alert types.Alert) ([]Block, string) {
	alertName := alertDisplayName(alert)
	statusLabel := alertStatusLabel(alert)

	fallback := fmt.Sprintf("[%s] %s", statusLabel, alertName)

	var blocks []Block

	blocks = append(blocks, sectionBlock(fmt.Sprintf("*Alert: %s*", alertName)))

	if desc, ok := alert.Annotations["description"]; ok && strings.TrimSpace(desc) != "" {
		blocks = append(blocks, sectionBlock(desc))
	} else if summary, ok := alert.Annotations["summary"]; ok && strings.TrimSpace(summary) != "" && summary != alertName {
		blocks = append(blocks, sectionBlock(summary))
	}

	var fields []*TextObject
	fields = appendField(fields, "Status", statusLabel)
	fields = appendField(fields, "Severity", firstNonEmpty(alert.Labels["severity"], alert.Labels["priority"]))
	fields = appendField(fields, "Service", firstNonEmpty(alert.Labels["service"], alert.Labels["app"], alert.Labels["application"]))
	fields = appendField(fields, "Source", firstNonEmpty(alert.Labels["instance"], alert.Labels["job"], alert.Labels["namespace"]))
	fields = appendField(fields, "Started", formatAlertTime(alert.StartsAt))
	if alert.Status == "resolved" {
		fields = appendField(fields, "Resolved", formatAlertTime(alert.EndsAt))
	}

	if len(fields) > 10 {
		fields = fields[:10]
	}

	if len(fields) > 0 {
		blocks = append(blocks, sectionBlockWithFields(fields...))
	}

	if value := formattedValue(alert); value != "" {
		blocks = append(blocks, sectionBlock(fmt.Sprintf("*Value*\n`%s`", value)))
	}

	if labelPairs := formattedLabelPairs(alert.Labels); len(labelPairs) > 0 {
		if len(labelPairs) > 8 {
			labelPairs = labelPairs[:8]
		}
		blocks = append(blocks, contextBlock(mrkdwnText("Labels: "+strings.Join(labelPairs, " "))))
	}

	if alert.Status != "resolved" {
		var actionButtons []BlockElement
		if !alert.Acknowledged {
			actionButtons = append(actionButtons, buttonElement("Acknowledge", "acknowledge", alert.Fingerprint, ""))
		}
		actionButtons = append(actionButtons, buttonElement("Resolve", "resolve", alert.Fingerprint, "primary"))
		if len(actionButtons) > 0 {
			blocks = append(blocks, actionsBlock(actionButtons...))
		}
	}

	var linkElements []*TextObject
	if alert.GeneratorURL != "" {
		linkElements = append(linkElements, mrkdwnText(fmt.Sprintf("<%s|%s>", alert.GeneratorURL, "Grafana")))
	}
	if alert.SilenceURL != "" {
		linkElements = append(linkElements, mrkdwnText(fmt.Sprintf("<%s|%s>", alert.SilenceURL, "Silence")))
	}
	if runbookURL, ok := alert.Annotations["runbook_url"]; ok && runbookURL != "" {
		linkElements = append(linkElements, mrkdwnText(fmt.Sprintf("<%s|%s>", runbookURL, "Runbook")))
	}
	if alert.DashboardURL != "" {
		linkElements = append(linkElements, mrkdwnText(fmt.Sprintf("<%s|%s>", alert.DashboardURL, "Dashboard")))
	}
	if alert.PanelURL != "" {
		linkElements = append(linkElements, mrkdwnText(fmt.Sprintf("<%s|%s>", alert.PanelURL, "Panel")))
	}

	if len(linkElements) > 0 {
		blocks = append(blocks, contextBlock(linkElements...))
	}

	return blocks, fallback
}

func BuildAlertAttachments(alert types.Alert) ([]Attachment, string) {
	blocks, fallback := BuildAlertBlocks(alert)
	return []Attachment{
		{
			Color:    alertStatusColor(alert),
			Fallback: fallback,
			Blocks:   blocks,
		},
	}, fallback
}

func BuildAlertBlocksUpdate(alert types.Alert, action string, userName string) ([]Block, string) {
	blocks, fallback := BuildAlertBlocks(alert)

	var updatedBlocks []Block
	for _, b := range blocks {
		if b.Type == "actions" {
			continue
		}
		updatedBlocks = append(updatedBlocks, b)
	}

	var statusText string
	switch action {
	case "acknowledge":
		statusText = fmt.Sprintf("_Acknowledged by @%s_", userName)
	case "resolve":
		statusText = fmt.Sprintf("_Resolved by @%s_", userName)
	default:
		return updatedBlocks, fallback
	}

	updatedBlocks = append(updatedBlocks, contextBlock(mrkdwnText(statusText)))

	return updatedBlocks, fallback
}

func BuildAlertAttachmentsUpdate(alert types.Alert, action string, userName string) ([]Attachment, string) {
	blocks, fallback := BuildAlertBlocksUpdate(alert, action, userName)
	return []Attachment{
		{
			Color:    alertStatusColor(alert),
			Fallback: fallback,
			Blocks:   blocks,
		},
	}, fallback
}

func EncodeBlocks(blocks []Block) (string, error) {
	if len(blocks) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(blocks)
	if err != nil {
		return "", fmt.Errorf("failed to marshal blocks: %w", err)
	}
	return string(data), nil
}

func EncodeAttachments(attachments []Attachment) (string, error) {
	if len(attachments) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(attachments)
	if err != nil {
		return "", fmt.Errorf("failed to marshal attachments: %w", err)
	}
	return string(data), nil
}
