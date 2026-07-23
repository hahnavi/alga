package incmetrics

import (
	"slices"
	"strings"
	"time"
)

type Metrics struct {
	MTTA          float64            `json:"mtta_minutes"`
	MTTR          float64            `json:"mttr_minutes"`
	MTTM          float64            `json:"mttm_minutes"`
	TotalCreated  int64              `json:"total_created"`
	TotalResolved int64              `json:"total_resolved"`
	BySeverity    map[string]CountMT `json:"by_severity"`
	ByPriority    map[string]CountMT `json:"by_priority"`
	ByService     map[string]CountMT `json:"by_service"`
	SLACompliance SLAComplianceStats `json:"sla_compliance"`
	Trend         []DailyMetric      `json:"trend"`
}

type CountMT struct {
	Count int64   `json:"count"`
	MTTA  float64 `json:"mtta_minutes"`
	MTTR  float64 `json:"mttr_minutes"`
}

type SLAComplianceStats struct {
	ResponseSLACompliance float64 `json:"response_sla_compliance_pct"`
	ResolveSLACompliance  float64 `json:"resolve_sla_compliance_pct"`
	ResponseBreaches      int64   `json:"response_breaches"`
	ResolveBreaches       int64   `json:"resolve_breaches"`
	TotalWithSLA          int64   `json:"total_with_sla"`
}

type DailyMetric struct {
	Date     string  `json:"date"`
	Created  int64   `json:"created"`
	Resolved int64   `json:"resolved"`
	MTTAMin  float64 `json:"mtta_minutes"`
	MTTRMin  float64 `json:"mttr_minutes"`
}

type IncidentData struct {
	CreatedAt         time.Time
	AcknowledgedAt    *time.Time
	MitigatedAt       *time.Time
	ResolvedAt        *time.Time
	Severity          string
	Priority          string
	ServiceID         string
	SLATargetRespond  *time.Time
	SLATargetResolve  *time.Time
	SLAAcknowledgedAt *time.Time
	SLAResolvedAt     *time.Time
}

func ComputeMetrics(incidents []IncidentData, trendDays int) *Metrics {
	m := &Metrics{
		BySeverity: make(map[string]CountMT),
		ByPriority: make(map[string]CountMT),
		ByService:  make(map[string]CountMT),
	}

	now := time.Now().UTC()

	if len(incidents) == 0 {
		return m
	}

	var totalAckDur, totalResolveDur, totalMitDur time.Duration
	var ackCount, resolveCount, mitCount int64
	var slaTotal, slaResolveTotal, slaRespBreaches, slaResolvBreaches int64

	sevData := make(map[string][]IncidentData)
	svcData := make(map[string][]IncidentData)

	for _, inc := range incidents {
		m.TotalCreated++
		if inc.ResolvedAt != nil {
			m.TotalResolved++
		}

		if inc.SLATargetRespond != nil {
			slaTotal++
			if inc.SLAAcknowledgedAt == nil || inc.SLAAcknowledgedAt.After(*inc.SLATargetRespond) {
				slaRespBreaches++
			}
		}
		if inc.SLATargetResolve != nil {
			slaResolveTotal++
			if inc.SLAResolvedAt != nil {
				if inc.SLAResolvedAt.After(*inc.SLATargetResolve) {
					slaResolvBreaches++
				}
			} else if now.After(*inc.SLATargetResolve) {
				slaResolvBreaches++
			}
		}

		if inc.SLAAcknowledgedAt != nil {
			dur := inc.SLAAcknowledgedAt.Sub(inc.CreatedAt)
			totalAckDur += dur
			ackCount++
		}

		if inc.ResolvedAt != nil {
			dur := inc.ResolvedAt.Sub(inc.CreatedAt)
			totalResolveDur += dur
			resolveCount++
		}

		if inc.MitigatedAt != nil {
			dur := inc.MitigatedAt.Sub(inc.CreatedAt)
			totalMitDur += dur
			mitCount++
		}

		sevData[inc.Severity] = append(sevData[inc.Severity], inc)
		if inc.ServiceID != "" {
			svcData[inc.ServiceID] = append(svcData[inc.ServiceID], inc)
		}
	}

	if ackCount > 0 {
		m.MTTA = totalAckDur.Minutes() / float64(ackCount)
	}
	if resolveCount > 0 {
		m.MTTR = totalResolveDur.Minutes() / float64(resolveCount)
	}
	if mitCount > 0 {
		m.MTTM = totalMitDur.Minutes() / float64(mitCount)
	}

	for sev, data := range sevData {
		m.BySeverity[sev] = computeCountMT(data)
	}

	prioData := make(map[string][]IncidentData)
	for _, inc := range incidents {
		p := inc.Priority
		if p == "" {
			p = "P5"
		}
		prioData[p] = append(prioData[p], inc)
	}
	for p, data := range prioData {
		m.ByPriority[p] = computeCountMT(data)
	}

	for svc, data := range svcData {
		m.ByService[svc] = computeCountMT(data)
	}

	if slaTotal > 0 {
		m.SLACompliance = SLAComplianceStats{
			ResponseSLACompliance: pct(slaTotal-slaRespBreaches, slaTotal),
			ResolveSLACompliance:  pct(slaResolveTotal-slaResolvBreaches, slaResolveTotal),
			ResponseBreaches:      slaRespBreaches,
			ResolveBreaches:       slaResolvBreaches,
			TotalWithSLA:          slaTotal,
		}
	}

	m.Trend = make([]DailyMetric, 0)

	if trendDays > 0 {
		dayMap := make(map[string]*DailyMetric)
		for _, inc := range incidents {
			dateStr := inc.CreatedAt.Format("2006-01-02")
			dm, ok := dayMap[dateStr]
			if !ok {
				dm = &DailyMetric{Date: dateStr}
				dayMap[dateStr] = dm
			}
			dm.Created++
			if inc.ResolvedAt != nil {
				dm.Resolved++
				dur := inc.ResolvedAt.Sub(inc.CreatedAt)
				dm.MTTRMin += dur.Minutes()
			}
			if inc.SLAAcknowledgedAt != nil {
				dur := inc.SLAAcknowledgedAt.Sub(inc.CreatedAt)
				dm.MTTAMin += dur.Minutes()
			}
		}

		for _, dm := range dayMap {
			if dm.Resolved > 0 {
				dm.MTTRMin = dm.MTTRMin / float64(dm.Resolved)
			}
			if dm.Created > 0 && dm.MTTAMin > 0 {
				ackCount := int64(0)
				for _, inc := range incidents {
					if inc.CreatedAt.Format("2006-01-02") == dm.Date && inc.SLAAcknowledgedAt != nil {
						ackCount++
					}
				}
				if ackCount > 0 {
					dm.MTTAMin = dm.MTTAMin / float64(ackCount)
				}
			}
			m.Trend = append(m.Trend, *dm)
		}
		slices.SortFunc(m.Trend, func(a, b DailyMetric) int {
			return strings.Compare(a.Date, b.Date)
		})
	}

	return m
}

func computeCountMT(incidents []IncidentData) CountMT {
	var c CountMT
	var totalAck, totalResolve time.Duration
	var ackN, resolveN int64

	c.Count = int64(len(incidents))
	for _, inc := range incidents {
		if inc.SLAAcknowledgedAt != nil {
			totalAck += inc.SLAAcknowledgedAt.Sub(inc.CreatedAt)
			ackN++
		}
		if inc.ResolvedAt != nil {
			totalResolve += inc.ResolvedAt.Sub(inc.CreatedAt)
			resolveN++
		}
	}
	if ackN > 0 {
		c.MTTA = totalAck.Minutes() / float64(ackN)
	}
	if resolveN > 0 {
		c.MTTR = totalResolve.Minutes() / float64(resolveN)
	}
	return c
}

func pct(num, total int64) float64 {
	if total == 0 {
		return 100.0
	}
	return float64(num) / float64(total) * 100.0
}
