package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"alga/logger"
	"alga/memory"
	"alga/rbac"
	"alga/store"
)

type DailySummaryResponse struct {
	Summary     string `json:"summary"`
	GeneratedAt string `json:"generated_at"`
	Period      string `json:"period"`
	Available   bool   `json:"available"`
	Failed      bool   `json:"failed,omitempty"`
	Error       string `json:"error,omitempty"`
}

type cachedSummary struct {
	response DailySummaryResponse
	date     string
}

type DailySummaryScheduler struct {
	server *Server
	mu     sync.RWMutex
	cached *cachedSummary
	stopCh chan struct{}
	wg     sync.WaitGroup
}

func NewDailySummaryScheduler(server *Server) *DailySummaryScheduler {
	return &DailySummaryScheduler{
		server: server,
		stopCh: make(chan struct{}),
	}
}

func (d *DailySummaryScheduler) Start() {
	d.wg.Add(1)
	go d.run()
}

func (d *DailySummaryScheduler) Stop() {
	close(d.stopCh)
	d.wg.Wait()
}

func (d *DailySummaryScheduler) run() {
	defer d.wg.Done()

	d.generateIfStale(context.Background())

	for {
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
		delay := next.Sub(now)
		if delay < time.Minute {
			delay = time.Minute
		}

		timer := time.NewTimer(delay)
		select {
		case <-d.stopCh:
			timer.Stop()
			return
		case <-timer.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("daily summary tick panicked", "component", "dashboard_summary", "tick", "generate", "panic", r, "stack", string(debug.Stack()))
					}
				}()
				d.generateIfStale(context.Background())
			}()
		}
	}
}

func (d *DailySummaryScheduler) generateIfStale(ctx context.Context) {
	today := time.Now().UTC().Format("2006-01-02")
	d.mu.RLock()
	if d.cached != nil && d.cached.date == today && d.cached.response.Available {
		d.mu.RUnlock()
		return
	}
	d.mu.RUnlock()

	d.generate(ctx)
}

func (d *DailySummaryScheduler) generate(ctx context.Context) {
	resp := d.doGenerate(ctx)

	today := time.Now().UTC().Format("2006-01-02")
	d.mu.Lock()
	d.cached = &cachedSummary{response: resp, date: today}
	d.mu.Unlock()
}

func (d *DailySummaryScheduler) doGenerate(ctx context.Context) DailySummaryResponse {
	if d.server.summaryLLM == nil {
		return DailySummaryResponse{
			Available: false,
			Period:    "24h",
		}
	}

	if !d.server.requireDashboardStore() {
		return DailySummaryResponse{
			Available: false,
			Failed:    true,
			Error:     "dashboard store not available",
			Period:    "24h",
		}
	}

	innerCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	since := time.Now().UTC().Add(-24 * time.Hour)
	data, err := d.server.dashboardStore.GetAlertDataForSummary(innerCtx, since)
	if err != nil {
		logger.ErrorCtx(ctx, "daily summary: gather data", "error", err)
		return DailySummaryResponse{
			Available: false,
			Failed:    true,
			Error:     "failed to gather alert data",
			Period:    "24h",
		}
	}

	prompt := buildSummaryPrompt(data, since)
	summary, err := d.server.summaryLLM.Generate(innerCtx, []memory.Message{
		{Role: "system", Content: summarySystemPrompt()},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		logger.ErrorCtx(ctx, "daily summary: LLM generate", "error", err)
		return DailySummaryResponse{
			Available: false,
			Failed:    true,
			Error:     "failed to generate summary",
			Period:    "24h",
		}
	}

	var summaryText string
	var raw map[string]any
	if err := json.Unmarshal([]byte(summary), &raw); err == nil {
		if text, ok := raw["summary"].(string); ok {
			summaryText = text
		}
	}
	if summaryText == "" {
		summaryText = summary
	}

	return DailySummaryResponse{
		Summary:     summaryText,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Period:      "24h",
		Available:   true,
	}
}

func (d *DailySummaryScheduler) Get() *DailySummaryResponse {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.cached == nil {
		return nil
	}
	resp := d.cached.response
	return &resp
}

func (s *Server) requireDashboardStore() bool {
	return s.dashboardStore != nil
}

func (s *Server) handleDailySummary(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetDailySummary(w, r)
	case http.MethodPost:
		// POST forces an LLM regeneration: a cost-incurring mutation that must
		// not be available to read-only principals just because the route gate
		// is the shared DashboardRead.
		if !s.checkPermission(w, r, rbac.SystemConfigWrite) {
			return
		}
		s.handlePostDailySummary(w, r)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleGetDailySummary(w http.ResponseWriter, _ *http.Request) {
	if s.summaryScheduler == nil {
		writeData(w, http.StatusOK, DailySummaryResponse{
			Available: false,
			Period:    "24h",
		})
		return
	}

	cached := s.summaryScheduler.Get()
	if cached == nil {
		writeData(w, http.StatusOK, DailySummaryResponse{
			Available: false,
			Period:    "24h",
		})
		return
	}
	writeData(w, http.StatusOK, *cached)
}

func (s *Server) handlePostDailySummary(w http.ResponseWriter, _ *http.Request) {
	if s.summaryScheduler == nil {
		if s.summaryLLM == nil {
			writeData(w, http.StatusOK, DailySummaryResponse{
				Available: false,
				Period:    "24h",
			})
			return
		}
		writeData(w, http.StatusOK, DailySummaryResponse{
			Available: false,
			Failed:    true,
			Error:     "summary scheduler not initialized",
			Period:    "24h",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	s.summaryScheduler.generate(ctx)

	cached := s.summaryScheduler.Get()
	if cached == nil {
		writeData(w, http.StatusOK, DailySummaryResponse{
			Available: false,
			Failed:    true,
			Error:     "generation produced no result",
			Period:    "24h",
		})
		return
	}
	writeData(w, http.StatusOK, *cached)
}

func summarySystemPrompt() string {
	return `You are an SRE operations analyst generating a daily shift handoff report. 
Write a clear, structured summary in Markdown format. Include:
1. **Overview** - One paragraph summarizing the alert landscape in the past 24 hours
2. **Critical Items** - Any ongoing critical or unresolved alerts
3. **Top Alerting Patterns** - The most frequent alerts and what they may indicate
4. **Investigation Outcomes** - Summary of investigation results and resolutions
5. **Action Items** - Recommended follow-up actions based on the data

Keep the summary concise but actionable. Use bullet points for lists. Return your response as a JSON object with a single "summary" key containing the Markdown text.`
}

func buildSummaryPrompt(data *store.SummaryData, since time.Time) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Generate a daily operations summary for the past 24 hours (since %s).\n\n", since.Format(time.RFC1123))
	fmt.Fprintf(&b, "## Alert Statistics\n")
	fmt.Fprintf(&b, "- New alerts: %d\n", data.AlertsCreated)
	fmt.Fprintf(&b, "- Resolved alerts: %d\n", data.AlertsResolved)
	fmt.Fprintf(&b, "- Currently firing: %d\n", data.AlertsFiring)

	if len(data.AlertsBySev) > 0 {
		fmt.Fprintf(&b, "\n### Alerts by Severity\n")
		for sev, count := range data.AlertsBySev {
			fmt.Fprintf(&b, "- %s: %d\n", sev, count)
		}
	}

	if len(data.TopAlerts) > 0 {
		fmt.Fprintf(&b, "\n### Top Recurring Alerts\n")
		for i, a := range data.TopAlerts {
			fmt.Fprintf(&b, "%d. **%s** (%s, %s) - %d occurrences\n", i+1, a.AlertName, a.Severity, a.Status, a.Count)
		}
	}

	if len(data.Investigations) > 0 {
		fmt.Fprintf(&b, "\n### Investigations\n")
		for _, inv := range data.Investigations {
			fmt.Fprintf(&b, "- **%s** [%s]", inv.AlertName, inv.Status)
			if inv.AgentName != "" {
				fmt.Fprintf(&b, " (Agent: %s)", inv.AgentName)
			}
			if inv.Summary != "" {
				fmt.Fprintf(&b, " - %s", inv.Summary)
			}
			fmt.Fprintf(&b, "\n")
		}
	}

	return b.String()
}
