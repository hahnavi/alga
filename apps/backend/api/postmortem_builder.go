package api

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"alga/ics"
	"alga/logger"
	"alga/store"
)

// postMortemDraftDeps bundles the stores that buildPostMortemDraft consults to
// assemble a richly populated post-mortem draft. Every field is optional — a
// nil store simply means the corresponding data is omitted. This lets both the
// API resolve handler and the agent resolve tool share identical enrichment
// logic regardless of which stores are wired.
type postMortemDraftDeps struct {
	documentStore     store.IncidentDocumentStore
	coordinationStore store.IncidentCoordinationStore
	incidentStore     store.IncidentStore
	alertStore        store.Store
}

// pmTimelineEntry is an intermediate representation used while merging
// lifecycle timestamps, timeline records, and status updates into the final
// chronologically-sorted timeline payload.
type pmTimelineEntry struct {
	timestamp   time.Time
	event       string
	description string
	actor       string
}

// buildPostMortemDraft assembles a post-mortem draft record pre-populated with
// as much incident context as is available. It is the single source of truth
// for auto-created post-mortem content — used by both the operator API resolve
// handler (Server.ensurePostMortemDraft) and the agent resolve tool
// (AgentToolExecutor.ensurePostMortem) so content is consistent across paths.
//
// The function never returns an error: a post-mortem is always produced. If an
// individual store call fails the gap is logged and the remaining fields are
// still filled. Required document sections (root_cause, impact_assessment,
// resolution) are guaranteed to be populated by the resolution gate, so the
// resulting draft is actionable rather than a blank stub.
func buildPostMortemDraft(ctx context.Context, deps postMortemDraftDeps, incident *store.IncidentRecord, reason string) *store.PostMortemRecord {
	rec := &store.PostMortemRecord{
		IncidentID: incident.ID,
		Status:     "draft",
	}

	sections := gatherDocSections(ctx, deps.documentStore, incident.IncidentNumber)
	statusUpdates := gatherStatusUpdates(ctx, deps.coordinationStore, incident.IncidentNumber)
	alertNames := gatherAlertNames(ctx, deps.alertStore, incident.IncidentNumber)
	timelineEntries := gatherTimeline(ctx, deps.incidentStore, incident.IncidentNumber)

	rec.Summary = buildPMSummary(incident, reason, alertNames)
	rec.Title = buildPMTitle(incident)
	rec.RootCause = sections[string(ics.SectionRootCause)]
	rec.Impact = buildPMImpact(incident, sections)
	rec.WhatWentWrong = buildPMWhatWentWrong(sections)
	rec.LessonsLearned = sections[string(ics.SectionResolution)]
	rec.ContributingFactors = buildPMContributingFactors(incident, alertNames)
	rec.Timeline = buildPMTimeline(incident, timelineEntries, statusUpdates)

	return rec
}

// gatherDocSections loads all incident document sections into a section→content
// map. Returns an empty map (not nil) when the store is unavailable or empty.
func gatherDocSections(ctx context.Context, ds store.IncidentDocumentStore, incidentNumber int64) map[string]string {
	out := map[string]string{}
	if ds == nil {
		return out
	}
	sections, err := ds.GetAllSections(ctx, incidentNumber)
	if err != nil {
		logger.WarnCtx(ctx, "failed to load incident document sections for post-mortem", "incident_number", incidentNumber, "error", err)
		return out
	}
	for _, s := range sections {
		out[s.Section] = strings.TrimSpace(s.Content)
	}
	return out
}

// gatherStatusUpdates returns public status updates (newest-last) for the
// incident. Returns an empty slice when the store is unavailable.
func gatherStatusUpdates(ctx context.Context, cs store.IncidentCoordinationStore, incidentNumber int64) []store.IncidentCoordinationMessageRecord {
	if cs == nil {
		return nil
	}
	updates, err := cs.ListMessagesByKind(ctx, incidentNumber, store.IncidentCoordinationKindStatusUpdate, 100, 0)
	if err != nil {
		logger.WarnCtx(ctx, "failed to load status updates for post-mortem", "incident_number", incidentNumber, "error", err)
		return nil
	}
	return updates
}

// gatherAlertNames resolves linked-alert fingerprints to human-readable alert
// names. Returns at most 15 distinct names.
func gatherAlertNames(ctx context.Context, as store.Store, incidentNumber int64) []string {
	if as == nil {
		return nil
	}
	fingerprints, err := as.GetAlertsByIncident(ctx, incidentNumber)
	if err != nil {
		logger.WarnCtx(ctx, "failed to load linked alerts for post-mortem", "incident_number", incidentNumber, "error", err)
		return nil
	}
	names := make([]string, 0, len(fingerprints))
	for _, fp := range fingerprints {
		if len(names) >= 15 {
			break
		}
		alert, gErr := as.GetByFingerprint(fp)
		if gErr != nil || alert == nil {
			continue
		}
		name := alert.Labels["alertname"]
		if name == "" {
			name = fp
		}
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// gatherTimeline returns the incident timeline entries (newest-last). Returns
// an empty slice when the store is unavailable.
func gatherTimeline(ctx context.Context, is store.IncidentStore, incidentNumber int64) []store.IncidentTimelineEntryRecord {
	if is == nil {
		return nil
	}
	entries, err := is.GetTimeline(ctx, incidentNumber)
	if err != nil {
		logger.WarnCtx(ctx, "failed to load incident timeline for post-mortem", "incident_number", incidentNumber, "error", err)
		return nil
	}
	return entries
}

// buildPMTitle returns the human-readable title for a post-mortem draft,
// derived from the incident. Falls back to a number-only title when the
// incident has no title set.
func buildPMTitle(incident *store.IncidentRecord) string {
	if title := strings.TrimSpace(incident.Title); title != "" {
		return fmt.Sprintf("Post-Mortem: %s", title)
	}
	return fmt.Sprintf("Post-Mortem: Incident #%d", incident.IncidentNumber)
}

// buildPMSummary assembles a multi-line summary block with incident metadata,
// duration, linked alerts, and the resolution reason.
func buildPMSummary(incident *store.IncidentRecord, reason string, alertNames []string) string {
	var b strings.Builder

	if title := strings.TrimSpace(incident.Title); title != "" {
		fmt.Fprintf(&b, "Incident #%d: %s", incident.IncidentNumber, title)
	} else {
		fmt.Fprintf(&b, "Incident #%d", incident.IncidentNumber)
	}

	metaParts := make([]string, 0, 3)
	if incident.Severity != "" {
		metaParts = append(metaParts, "Severity: "+incident.Severity)
	}
	if incident.ImpactLevel != "" {
		metaParts = append(metaParts, "Impact: "+incident.ImpactLevel)
	}
	if dur := incidentDuration(incident); dur != "" {
		metaParts = append(metaParts, "Duration: "+dur)
	}
	if len(metaParts) > 0 {
		fmt.Fprintf(&b, "\n%s", strings.Join(metaParts, " | "))
	}

	if incident.Status != "" {
		fmt.Fprintf(&b, "\nStatus: %s", incident.Status)
	}

	if len(alertNames) > 0 {
		joined := strings.Join(alertNames, ", ")
		if len(alertNames) == 1 {
			fmt.Fprintf(&b, "\nTriggered alert: %s", joined)
		} else {
			fmt.Fprintf(&b, "\nTriggered alerts: %s", joined)
		}
	}

	r := strings.TrimSpace(reason)
	if r == "" {
		r = strings.TrimSpace(incident.Summary)
	}
	if r != "" {
		fmt.Fprintf(&b, "\n\n%s", r)
	}

	return b.String()
}

// buildPMImpact returns the impact_assessment document section when available,
// otherwise falls back to a structured statement built from incident severity
// and impact level.
func buildPMImpact(incident *store.IncidentRecord, sections map[string]string) string {
	if content := sections[string(ics.SectionImpactAssessment)]; content != "" {
		return content
	}
	var parts []string
	if incident.Severity != "" {
		parts = append(parts, fmt.Sprintf("Severity: %s", incident.Severity))
	}
	if incident.ImpactLevel != "" {
		parts = append(parts, fmt.Sprintf("Impact level: %s", incident.ImpactLevel))
	}
	if incident.IncidentType != "" {
		parts = append(parts, fmt.Sprintf("Type: %s", incident.IncidentType))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Impact assessment not yet documented.\n\nIncident metadata: " + strings.Join(parts, ", ") + "."
}

// buildPMWhatWentWrong surfaces the actions_taken document section (the
// remediation steps) as the starting point for the "needs improvement" field.
// The author is expected to refine this into genuine reflective content.
func buildPMWhatWentWrong(sections map[string]string) string {
	actions := sections[string(ics.SectionActionsTaken)]
	if actions == "" {
		return ""
	}
	return "Remediation actions taken during incident response (review and refine):\n\n" + actions
}

// buildPMContributingFactors derives a concise list of factual contributing
// factors from incident metadata and linked alerts.
func buildPMContributingFactors(incident *store.IncidentRecord, alertNames []string) []string {
	factors := make([]string, 0, len(alertNames)+len(incident.Tags)+2)
	for _, name := range alertNames {
		factors = append(factors, "Triggered alert: "+name)
	}
	for _, tag := range incident.Tags {
		if tag = strings.TrimSpace(tag); tag != "" && !slices.Contains(factors, tag) {
			factors = append(factors, tag)
		}
	}
	if t := strings.TrimSpace(incident.IncidentType); t != "" {
		factors = append(factors, "Incident type: "+t)
	}
	return factors
}

// buildPMTimeline merges lifecycle milestones, timeline entries, and status
// updates into a single chronologically-sorted JSON-serialisable timeline.
func buildPMTimeline(incident *store.IncidentRecord, timelineEntries []store.IncidentTimelineEntryRecord, statusUpdates []store.IncidentCoordinationMessageRecord) []map[string]any {
	entries := make([]pmTimelineEntry, 0, len(timelineEntries)+len(statusUpdates)+5)

	addLifecycle := func(t *time.Time, event, description string) {
		if t != nil && !t.IsZero() {
			entries = append(entries, pmTimelineEntry{timestamp: *t, event: event, description: description, actor: "system"})
		}
	}
	addLifecycle(incident.StartedAt, "incident_started", "Incident started")
	if incident.StartedAt == nil || incident.StartedAt.IsZero() {
		addLifecycle(&incident.CreatedAt, "incident_created", "Incident created")
	}
	addLifecycle(incident.TriagedAt, "incident_triaged", "Incident triaged")
	addLifecycle(incident.MitigatedAt, "incident_mitigated", "Incident mitigated")
	addLifecycle(incident.ResolvedAt, "incident_resolved", "Incident resolved")

	for _, te := range timelineEntries {
		if te.EventType == "postmortem_created" {
			continue
		}
		desc := strings.TrimSpace(te.Message)
		if desc == "" {
			continue
		}
		actor := te.ActorType
		if actor == "" {
			actor = "system"
		}
		entries = append(entries, pmTimelineEntry{
			timestamp:   te.CreatedAt,
			event:       te.EventType,
			description: desc,
			actor:       actor,
		})
	}

	for _, su := range statusUpdates {
		body := strings.TrimSpace(su.Body)
		if body == "" {
			continue
		}
		level := ""
		if su.Metadata != nil {
			if v, ok := su.Metadata["status_level"].(string); ok {
				level = v
			}
		}
		event := "status_update"
		desc := body
		if level != "" {
			desc = fmt.Sprintf("[%s] %s", level, body)
		}
		actor := su.ActorType
		if actor == "" {
			actor = "system"
		}
		entries = append(entries, pmTimelineEntry{
			timestamp:   su.CreatedAt,
			event:       event,
			description: desc,
			actor:       actor,
		})
	}

	slices.SortFunc(entries, func(a, b pmTimelineEntry) int {
		return a.timestamp.Compare(b.timestamp)
	})

	out := make([]map[string]any, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		ts := e.timestamp.UTC().Format(time.RFC3339)
		dedupKey := ts + "|" + e.event + "|" + e.description
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true
		out = append(out, map[string]any{
			"timestamp":   ts,
			"event":       e.event,
			"description": e.description,
			"actor":       e.actor,
		})
	}
	return out
}

// incidentDuration computes a human-readable duration string from the incident
// lifecycle timestamps. Prefers resolved−started, then resolved−created.
func incidentDuration(incident *store.IncidentRecord) string {
	end := incident.ResolvedAt
	if end == nil || end.IsZero() {
		end = incident.ClosedAt
	}
	if end == nil || end.IsZero() {
		return ""
	}
	start := incident.StartedAt
	if start == nil || start.IsZero() {
		start = &incident.CreatedAt
	}
	if start == nil || start.IsZero() {
		return ""
	}
	return formatHumanDuration(end.Sub(*start))
}

// formatHumanDuration renders a duration as a compact "2h 15m" / "3d 4h" /
// "45m" / "30s" string.
func formatHumanDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	var parts []string
	switch {
	case days > 0:
		parts = append(parts, fmt.Sprintf("%dd", days))
		fallthrough
	case hours > 0:
		parts = append(parts, fmt.Sprintf("%dh", hours))
		if minutes > 0 {
			parts = append(parts, fmt.Sprintf("%dm", minutes))
		}
	case minutes > 0:
		parts = append(parts, fmt.Sprintf("%dm", minutes))
		if seconds > 0 && minutes < 5 {
			parts = append(parts, fmt.Sprintf("%ds", seconds))
		}
	default:
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	return strings.Join(parts, " ")
}
