package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"alga/store"
	"alga/valkey"
)

type mockDashboardStore struct {
	stats *store.DashboardStats
	err   error
	calls int
}

func (m *mockDashboardStore) GetStats(ctx context.Context) (*store.DashboardStats, error) {
	m.calls++
	return m.stats, m.err
}

func (m *mockDashboardStore) GetTopAlerts(ctx context.Context, since time.Time, limit int) ([]store.TopAlertItem, error) {
	return nil, nil
}

func (m *mockDashboardStore) GetRecentInvestigations(ctx context.Context, since time.Time, limit int) ([]store.RecentInvestigationItem, error) {
	return nil, nil
}

func (m *mockDashboardStore) GetActiveInvestigations(ctx context.Context, limit int) ([]store.RecentInvestigationItem, error) {
	return nil, nil
}

func (m *mockDashboardStore) GetAlertDataForSummary(ctx context.Context, since time.Time) (*store.SummaryData, error) {
	return nil, nil
}

func TestHandleDashboardStats_CacheMiss(t *testing.T) {
	t.Parallel()

	mockStore := &mockDashboardStore{
		stats: &store.DashboardStats{
			Alerts: store.DashboardAlertStats{Total: 42, Firing: 10, Resolved: 30, Unacknowledged: 5},
		},
	}

	s := &Server{
		dashboardStore: mockStore,
		cache:          valkey.NewCache(nil),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stats", nil)
	w := httptest.NewRecorder()

	s.handleDashboardStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var stats store.DashboardStats
	if err := decodeResponse(t, w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if stats.Alerts.Total != 42 {
		t.Fatalf("alerts.total = %d, want 42", stats.Alerts.Total)
	}
	if mockStore.calls != 1 {
		t.Fatalf("store calls = %d, want 1", mockStore.calls)
	}
}

func TestHandleDashboardStats_CacheHit(t *testing.T) {
	t.Parallel()

	mockStore := &mockDashboardStore{
		stats: &store.DashboardStats{
			Alerts: store.DashboardAlertStats{Total: 42},
		},
	}

	s := &Server{
		dashboardStore: mockStore,
		cache:          valkey.NewCache(nil),
	}

	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stats", nil)
	w1 := httptest.NewRecorder()
	s.handleDashboardStats(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("first call status = %d, want %d", w1.Code, http.StatusOK)
	}
	if mockStore.calls != 1 {
		t.Fatalf("first call store calls = %d, want 1", mockStore.calls)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stats", nil)
	w2 := httptest.NewRecorder()
	s.handleDashboardStats(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("second call status = %d, want %d", w2.Code, http.StatusOK)
	}
	if mockStore.calls != 2 {
		t.Fatalf("second call store calls = %d, want 2 (nil cache = no caching)", mockStore.calls)
	}
}
