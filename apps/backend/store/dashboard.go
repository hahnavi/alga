package store

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"alga/ent"
	"alga/ent/alert"
	"alga/ent/alertinvestigation"
	entincident "alga/ent/incident"
	"alga/logger"
	"alga/strutil"
)

type DashboardStore interface {
	GetStats(ctx context.Context) (*DashboardStats, error)
	GetTopAlerts(ctx context.Context, since time.Time, limit int) ([]TopAlertItem, error)
	GetRecentInvestigations(ctx context.Context, since time.Time, limit int) ([]RecentInvestigationItem, error)
	GetActiveInvestigations(ctx context.Context, limit int) ([]RecentInvestigationItem, error)
	GetAlertDataForSummary(ctx context.Context, since time.Time) (*SummaryData, error)
}

type DashboardStats struct {
	Alerts               DashboardAlertStats       `json:"alerts"`
	AlertsBySeverity     []SeverityBucket          `json:"alerts_by_severity"`
	AlertTrend           []DailyAlertCount         `json:"alert_trend"`
	Investigations       DashboardInvestigation    `json:"investigations"`
	TopAlerts24h         []TopAlertItem            `json:"top_alerts_24h"`
	RecentInvestigations []RecentInvestigationItem `json:"recent_investigations"`
	ActiveInvestigations []RecentInvestigationItem `json:"active_investigations"`
	Incidents            DashboardIncidentStats    `json:"incidents"`
	ActiveIncidents      []ActiveIncidentItem      `json:"active_incidents"`
	Services             DashboardServiceStats     `json:"services"`
	SLAStats             DashboardSLAStats         `json:"sla_stats"`
}

type TopAlertItem struct {
	AlertName string            `json:"alert_name"`
	Count     int64             `json:"count"`
	Severity  string            `json:"severity"`
	Status    string            `json:"status"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type RecentInvestigationItem struct {
	InvestigationID string `json:"investigation_id"`
	Status          string `json:"status"`
	AlertName       string `json:"alert_name"`
	AgentName       string `json:"agent_name,omitempty"`
	CorrelationKey  string `json:"correlation_key"`
	CreatedAt       string `json:"created_at"`
	Summary         string `json:"summary,omitempty"`
}

type SummaryData struct {
	AlertsCreated  int64                     `json:"alerts_created"`
	AlertsResolved int64                     `json:"alerts_resolved"`
	AlertsFiring   int64                     `json:"alerts_firing"`
	TopAlerts      []TopAlertItem            `json:"top_alerts"`
	Investigations []RecentInvestigationItem `json:"investigations"`
	AlertsBySev    map[string]int64          `json:"alerts_by_severity"`
}

type DashboardAlertStats struct {
	Total          int64 `json:"total"`
	Firing         int64 `json:"firing"`
	Resolved       int64 `json:"resolved"`
	Unacknowledged int64 `json:"unacknowledged"`
}

type SeverityBucket struct {
	Severity string `json:"severity"`
	Count    int64  `json:"count"`
}

type DailyAlertCount struct {
	Date     string `json:"date"`
	Created  int64  `json:"created"`
	Resolved int64  `json:"resolved"`
}

type DashboardInvestigation struct {
	Total          int64   `json:"total"`
	Pending        int64   `json:"pending"`
	Investigating  int64   `json:"investigating"`
	Complete       int64   `json:"complete"`
	Failed         int64   `json:"failed"`
	Cancelled      int64   `json:"cancelled"`
	TimedOut       int64   `json:"timed_out"`
	CompletionRate float64 `json:"completion_rate"`
}

type DashboardIncidentStats struct {
	Total      int64            `json:"total"`
	Active     int64            `json:"active"`
	Mitigated  int64            `json:"mitigated"`
	Resolved   int64            `json:"resolved"`
	BySeverity map[string]int64 `json:"by_severity"`
	ByPriority map[string]int64 `json:"incidents_by_priority"`
}

type ActiveIncidentItem struct {
	IncidentNumber int64  `json:"incident_number"`
	Title          string `json:"title"`
	Severity       string `json:"severity"`
	Status         string `json:"status"`
	ServiceName    string `json:"service_name,omitempty"`
	CommanderName  string `json:"commander_name,omitempty"`
	CreatedAt      string `json:"created_at"`
}

type DashboardServiceStats struct {
	Total    int64            `json:"total"`
	ByStatus map[string]int64 `json:"by_status"`
}

type DashboardSLAStats struct {
	ResponseBreaches int64   `json:"response_breaches"`
	ResolveBreaches  int64   `json:"resolve_breaches"`
	CompliancePct    float64 `json:"compliance_pct"`
}

type pgDashboardStore struct {
	pgStoreBase
}

func newPGDashboardStore(client *ent.Client) DashboardStore {
	return &pgDashboardStore{pgStoreBase{client: client}}
}

func (s *pgDashboardStore) GetStats(ctx context.Context) (*DashboardStats, error) {
	stats := &DashboardStats{}

	alertStats, err := s.getAlertStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("alert stats: %w", err)
	}
	stats.Alerts = *alertStats

	severityBuckets, err := s.getAlertsBySeverity(ctx)
	if err != nil {
		return nil, fmt.Errorf("alerts by severity: %w", err)
	}
	stats.AlertsBySeverity = severityBuckets

	trend, err := s.getAlertTrend(ctx)
	if err != nil {
		return nil, fmt.Errorf("alert trend: %w", err)
	}
	stats.AlertTrend = trend

	invStats, err := s.getInvestigationStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("investigation stats: %w", err)
	}
	stats.Investigations = *invStats

	since24h := time.Now().Add(-24 * time.Hour)

	topAlerts, err := s.GetTopAlerts(ctx, since24h, 10)
	if err != nil {
		return nil, fmt.Errorf("top alerts: %w", err)
	}
	stats.TopAlerts24h = topAlerts

	recentInvs, err := s.GetRecentInvestigations(ctx, since24h, 10)
	if err != nil {
		return nil, fmt.Errorf("recent investigations: %w", err)
	}
	stats.RecentInvestigations = recentInvs

	activeInvs, err := s.GetActiveInvestigations(ctx, 10)
	if err != nil {
		return nil, fmt.Errorf("active investigations: %w", err)
	}
	stats.ActiveInvestigations = activeInvs

	incStats, err := s.getIncidentStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("incident stats: %w", err)
	}
	stats.Incidents = *incStats

	activeIncs, err := s.getActiveIncidents(ctx)
	if err != nil {
		return nil, fmt.Errorf("active incidents: %w", err)
	}
	stats.ActiveIncidents = activeIncs

	svcStats, err := s.getServiceStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("service stats: %w", err)
	}
	stats.Services = *svcStats

	slaStats, err := s.getSLAStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("sla stats: %w", err)
	}
	stats.SLAStats = *slaStats

	return stats, nil
}

func (s *pgDashboardStore) getAlertStats(ctx context.Context) (*DashboardAlertStats, error) {
	total, err := s.client.Alert.Query().Where(alert.DeletedAtIsNil()).Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count total: %w", err)
	}

	firing, err := s.client.Alert.Query().Where(alert.Status("firing"), alert.DeletedAtIsNil()).Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count firing: %w", err)
	}

	resolved, err := s.client.Alert.Query().Where(alert.Status("resolved"), alert.DeletedAtIsNil()).Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count resolved: %w", err)
	}

	unacknowledged, err := s.client.Alert.Query().Where(alert.Status("firing"), alert.Acknowledged(false), alert.DeletedAtIsNil()).Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count unacknowledged: %w", err)
	}

	return &DashboardAlertStats{
		Total:          int64(total),
		Firing:         int64(firing),
		Resolved:       int64(resolved),
		Unacknowledged: int64(unacknowledged),
	}, nil
}

func (s *pgDashboardStore) getAlertsBySeverity(ctx context.Context) ([]SeverityBucket, error) {
	allAlerts, err := s.client.Alert.Query().Where(alert.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query alerts: %w", err)
	}

	counts := make(map[string]int64)
	for _, a := range allAlerts {
		sev := extractSeverity(a.Labels)
		counts[sev]++
	}

	buckets := make([]SeverityBucket, 0, len(counts))
	for sev, count := range counts {
		buckets = append(buckets, SeverityBucket{Severity: sev, Count: count})
	}
	slices.SortFunc(buckets, func(a, b SeverityBucket) int {
		return cmp.Compare(b.Count, a.Count)
	})
	return buckets, nil
}

func extractSeverity(labels map[string]string) string {
	for _, key := range []string{"severity", "priority", "Severity", "Priority", "level"} {
		if v, ok := labels[key]; ok && v != "" {
			return v
		}
	}
	return "info"
}

func (s *pgDashboardStore) getAlertTrend(ctx context.Context) ([]DailyAlertCount, error) {
	cutoff := time.Now().AddDate(0, 0, -30)

	allAlerts, err := s.client.Alert.Query().
		Where(alert.CreatedAtGTE(cutoff), alert.DeletedAtIsNil()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query alerts for trend: %w", err)
	}

	createdMap := make(map[string]int64)
	for _, a := range allAlerts {
		day := a.CreatedAt.Format("2006-01-02")
		createdMap[day]++
	}

	resolvedAlerts, err := s.client.Alert.Query().
		Where(alert.Status("resolved"), alert.UpdatedAtGTE(cutoff), alert.DeletedAtIsNil()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query resolved alerts for trend: %w", err)
	}

	resolvedMap := make(map[string]int64)
	for _, a := range resolvedAlerts {
		day := a.UpdatedAt.Format("2006-01-02")
		resolvedMap[day]++
	}

	allDates := make(map[string]struct{})
	for d := range createdMap {
		allDates[d] = struct{}{}
	}
	for d := range resolvedMap {
		allDates[d] = struct{}{}
	}

	sortedDates := make([]string, 0, len(allDates))
	for d := range allDates {
		sortedDates = append(sortedDates, d)
	}
	slices.Sort(sortedDates)

	trend := make([]DailyAlertCount, 0, len(sortedDates))
	for _, date := range sortedDates {
		trend = append(trend, DailyAlertCount{
			Date:     date,
			Created:  createdMap[date],
			Resolved: resolvedMap[date],
		})
	}
	return trend, nil
}

func (s *pgDashboardStore) getInvestigationStats(ctx context.Context) (*DashboardInvestigation, error) {
	total, err := s.client.AlertInvestigation.Query().Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count total: %w", err)
	}

	pending, err := s.client.AlertInvestigation.Query().Where(alertinvestigation.Status("pending")).Count(ctx)
	if err != nil {
		logger.WarnCtx(ctx, "failed to count pending investigations", "component", "store", "error", err)
	}
	investigating, err := s.client.AlertInvestigation.Query().Where(alertinvestigation.Status("investigating")).Count(ctx)
	if err != nil {
		logger.WarnCtx(ctx, "failed to count investigating investigations", "component", "store", "error", err)
	}
	complete, err := s.client.AlertInvestigation.Query().Where(alertinvestigation.Status("complete")).Count(ctx)
	if err != nil {
		logger.WarnCtx(ctx, "failed to count complete investigations", "component", "store", "error", err)
	}
	failedCount, err := s.client.AlertInvestigation.Query().Where(alertinvestigation.Status("failed")).Count(ctx)
	if err != nil {
		logger.WarnCtx(ctx, "failed to count failed investigations", "component", "store", "error", err)
	}
	cancelled, err := s.client.AlertInvestigation.Query().Where(alertinvestigation.Status("cancelled")).Count(ctx)
	if err != nil {
		logger.WarnCtx(ctx, "failed to count cancelled investigations", "component", "store", "error", err)
	}
	timedOut, err := s.client.AlertInvestigation.Query().Where(alertinvestigation.Status("timed_out")).Count(ctx)
	if err != nil {
		logger.WarnCtx(ctx, "failed to count timed out investigations", "component", "store", "error", err)
	}

	stats := &DashboardInvestigation{
		Total:         int64(total),
		Pending:       int64(pending),
		Investigating: int64(investigating),
		Complete:      int64(complete),
		Failed:        int64(failedCount),
		Cancelled:     int64(cancelled),
		TimedOut:      int64(timedOut),
	}

	finished := stats.Complete + stats.Failed + stats.Cancelled + stats.TimedOut
	if finished > 0 {
		stats.CompletionRate = float64(stats.Complete) / float64(finished) * 100
	}

	return stats, nil
}

func (s *pgDashboardStore) getIncidentStats(ctx context.Context) (*DashboardIncidentStats, error) {
	total, err := s.client.Incident.Query().Where(entincident.DeletedAtIsNil()).Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count total incidents: %w", err)
	}

	active, err := s.client.Incident.Query().
		Where(entincident.StatusIn("detected", "triaging", "active"), entincident.DeletedAtIsNil()).
		Count(ctx)
	if err != nil {
		logger.WarnCtx(ctx, "failed to count active incidents", "component", "store", "error", err)
	}
	mitigated, err := s.client.Incident.Query().
		Where(entincident.Status("mitigated"), entincident.DeletedAtIsNil()).
		Count(ctx)
	if err != nil {
		logger.WarnCtx(ctx, "failed to count mitigated incidents", "component", "store", "error", err)
	}
	resolved, err := s.client.Incident.Query().
		Where(entincident.StatusIn("resolved", "closed"), entincident.DeletedAtIsNil()).
		Count(ctx)
	if err != nil {
		logger.WarnCtx(ctx, "failed to count resolved incidents", "component", "store", "error", err)
	}

	allIncs, err := s.client.Incident.Query().Where(entincident.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query incidents for severity: %w", err)
	}
	bySeverity := make(map[string]int64)
	byPriority := make(map[string]int64)
	for _, inc := range allIncs {
		bySeverity[inc.Severity]++
		p := inc.Priority
		if p == "" {
			p = "P5"
		}
		byPriority[p]++
	}

	return &DashboardIncidentStats{
		Total:      int64(total),
		Active:     int64(active),
		Mitigated:  int64(mitigated),
		Resolved:   int64(resolved),
		BySeverity: bySeverity,
		ByPriority: byPriority,
	}, nil
}

func (s *pgDashboardStore) getActiveIncidents(ctx context.Context) ([]ActiveIncidentItem, error) {
	incs, err := s.client.Incident.Query().
		Where(entincident.StatusIn("active", "mitigated"), entincident.DeletedAtIsNil()).
		Order(ent.Desc(entincident.FieldCreatedAt)).
		Limit(10).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query active incidents: %w", err)
	}

	serviceCache := make(map[string]string)
	userCache := make(map[string]string)

	items := make([]ActiveIncidentItem, 0, len(incs))
	for _, inc := range incs {
		item := ActiveIncidentItem{
			IncidentNumber: inc.IncidentNumber,
			Title:          inc.Title,
			Severity:       inc.Severity,
			Status:         inc.Status,
			CreatedAt:      inc.CreatedAt.Format(time.RFC3339),
		}

		if inc.ServiceID != nil {
			sid := inc.ServiceID.String()
			if name, ok := serviceCache[sid]; ok {
				item.ServiceName = name
			} else {
				svc, serr := s.client.Service.Get(ctx, *inc.ServiceID)
				if serr == nil {
					name := svc.Name
					if svc.DisplayName != "" {
						name = svc.DisplayName
					}
					serviceCache[sid] = name
					item.ServiceName = name
				}
			}
		}

		if inc.CommanderID != nil {
			cid := inc.CommanderID.String()
			if name, ok := userCache[cid]; ok {
				item.CommanderName = name
			} else {
				u, uerr := s.client.User.Get(ctx, *inc.CommanderID)
				if uerr == nil {
					name := u.Email
					if u.FullName != "" {
						name = u.FullName
					}
					userCache[cid] = name
					item.CommanderName = name
				}
			}
		}

		items = append(items, item)
	}
	return items, nil
}

func (s *pgDashboardStore) getServiceStats(ctx context.Context) (*DashboardServiceStats, error) {
	services, err := s.client.Service.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query services: %w", err)
	}

	byStatus := make(map[string]int64)
	for _, svc := range services {
		byStatus[svc.Status]++
	}

	return &DashboardServiceStats{
		Total:    int64(len(services)),
		ByStatus: byStatus,
	}, nil
}

func (s *pgDashboardStore) getSLAStats(ctx context.Context) (*DashboardSLAStats, error) {
	now := time.Now()

	respBreaches, err := s.client.Incident.Query().
		Where(
			entincident.SLATargetRespondAtLT(now),
			entincident.SLAAcknowledgedAtIsNil(),
			entincident.DeletedAtIsNil(),
		).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count response breaches: %w", err)
	}

	resolveBreaches, err := s.client.Incident.Query().
		Where(
			entincident.SLATargetResolveAtLT(now),
			entincident.SLAResolvedAtIsNil(),
			entincident.DeletedAtIsNil(),
		).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count resolve breaches: %w", err)
	}

	totalWithSLA, err := s.client.Incident.Query().
		Where(
			entincident.SLATargetRespondAtNotNil(),
			entincident.DeletedAtIsNil(),
		).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count incidents with sla: %w", err)
	}

	compliance := 100.0
	if totalWithSLA > 0 {
		totalBreaches := respBreaches + resolveBreaches
		totalChecks := totalWithSLA * 2
		compliance = float64(totalChecks-totalBreaches) / float64(totalChecks) * 100
		compliance = max(compliance, 0)
	}

	return &DashboardSLAStats{
		ResponseBreaches: int64(respBreaches),
		ResolveBreaches:  int64(resolveBreaches),
		CompliancePct:    compliance,
	}, nil
}

func (s *pgDashboardStore) GetTopAlerts(ctx context.Context, since time.Time, limit int) ([]TopAlertItem, error) {
	alerts, err := s.client.Alert.Query().
		Where(alert.CreatedAtGTE(since), alert.DeletedAtIsNil()).
		Order(ent.Desc(alert.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query alerts: %w", err)
	}

	type key struct {
		name string
		sev  string
	}
	counts := make(map[key]*TopAlertItem)
	for _, a := range alerts {
		name := a.Labels["alertname"]
		if name == "" {
			name = strutil.Prefix(a.Fingerprint, 8)
		}
		sev := extractSeverity(a.Labels)
		k := key{name: name, sev: sev}
		if existing, ok := counts[k]; ok {
			existing.Count++
			if a.Status == "firing" {
				existing.Status = "firing"
			}
		} else {
			item := &TopAlertItem{
				AlertName: name,
				Count:     1,
				Severity:  sev,
				Status:    a.Status,
				Labels:    a.Labels,
			}
			counts[k] = item
		}
	}

	items := make([]TopAlertItem, 0, len(counts))
	for _, v := range counts {
		items = append(items, *v)
	}
	slices.SortFunc(items, func(a, b TopAlertItem) int {
		return cmp.Compare(b.Count, a.Count)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *pgDashboardStore) GetRecentInvestigations(ctx context.Context, since time.Time, limit int) ([]RecentInvestigationItem, error) {
	invs, err := s.client.AlertInvestigation.Query().
		Where(alertinvestigation.CreatedAtGTE(since)).
		Order(ent.Desc(alertinvestigation.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query investigations: %w", err)
	}
	return s.buildRecentInvestigationItems(ctx, invs)
}

func (s *pgDashboardStore) GetActiveInvestigations(ctx context.Context, limit int) ([]RecentInvestigationItem, error) {
	invs, err := s.client.AlertInvestigation.Query().
		Where(alertinvestigation.StatusIn("pending", "investigating", "assigned", "paused")).
		Order(ent.Desc(alertinvestigation.FieldUpdatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query active investigations: %w", err)
	}
	return s.buildRecentInvestigationItems(ctx, invs)
}

func (s *pgDashboardStore) buildRecentInvestigationItems(ctx context.Context, invs []*ent.AlertInvestigation) ([]RecentInvestigationItem, error) {
	items := make([]RecentInvestigationItem, 0, len(invs))
	for _, inv := range invs {
		alertName := inv.CorrelationKey
		linkedAlerts, err := inv.QueryAlerts().Limit(1).All(ctx)
		if err == nil && len(linkedAlerts) > 0 {
			if name := linkedAlerts[0].Labels["alertname"]; name != "" {
				alertName = name
			}
		}
		summary := ""
		if inv.Summary != nil {
			summary = inv.Summary.Summary
		}
		items = append(items, RecentInvestigationItem{
			InvestigationID: inv.AlertInvestigationID,
			Status:          inv.Status,
			AlertName:       alertName,
			AgentName:       inv.AgentName,
			CorrelationKey:  inv.CorrelationKey,
			CreatedAt:       inv.CreatedAt.Format(time.RFC3339),
			Summary:         summary,
		})
	}
	return items, nil
}

func (s *pgDashboardStore) GetAlertDataForSummary(ctx context.Context, since time.Time) (*SummaryData, error) {
	alerts, err := s.client.Alert.Query().
		Where(alert.CreatedAtGTE(since), alert.DeletedAtIsNil()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query alerts: %w", err)
	}

	resolved, err := s.client.Alert.Query().
		Where(alert.Status("resolved"), alert.UpdatedAtGTE(since), alert.DeletedAtIsNil()).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count resolved: %w", err)
	}

	firing, err := s.client.Alert.Query().
		Where(alert.Status("firing"), alert.DeletedAtIsNil()).Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count firing: %w", err)
	}

	data := &SummaryData{
		AlertsCreated:  int64(len(alerts)),
		AlertsResolved: int64(resolved),
		AlertsFiring:   int64(firing),
		AlertsBySev:    make(map[string]int64),
	}

	for _, a := range alerts {
		sev := extractSeverity(a.Labels)
		data.AlertsBySev[sev]++
	}

	topAlerts, err := s.GetTopAlerts(ctx, since, 5)
	if err == nil {
		data.TopAlerts = topAlerts
	}

	invs, err := s.GetRecentInvestigations(ctx, since, 10)
	if err == nil {
		data.Investigations = invs
	}

	return data, nil
}
