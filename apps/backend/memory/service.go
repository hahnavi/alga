package memory

import (
	"context"
	"time"

	"github.com/google/uuid"

	"alga/store"
)

type Service interface {
	ExtractFromInvestigation(ctx context.Context, inv *store.AlertInvestigationRecord) error
	Search(ctx context.Context, query string, labels map[string]string, topK int) ([]store.ScoredMemory, error)
	CreateMemory(ctx context.Context, input CreateMemoryInput) (*store.AgentMemoryRecord, error)
	Get(ctx context.Context, id uuid.UUID) (*store.AgentMemoryRecord, error)
	Update(ctx context.Context, id uuid.UUID, content string) (*store.AgentMemoryRecord, error)
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, f store.MemoryFilters) ([]store.AgentMemoryRecord, int, error)
	DeleteExpired(ctx context.Context) (int, error)
}

type CreateMemoryInput struct {
	Content         string
	MemoryType      string
	AgentID         *uuid.UUID
	AgentName       string
	AgentType       string
	InvestigationID string
	CorrelationKey  string
	Labels          map[string]string
	Confidence      *float64
	ExpiresAt       *time.Time
}
