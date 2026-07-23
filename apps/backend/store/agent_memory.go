package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/agentmemory"
	"alga/logger"
)

const (
	MemoryTypeFact      = "fact"
	MemoryTypePattern   = "pattern"
	MemoryTypeProcedure = "procedure"
)

func IsValidMemoryType(s string) bool {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case MemoryTypeFact, MemoryTypePattern, MemoryTypeProcedure:
		return true
	default:
		return false
	}
}

type AgentMemoryRecord struct {
	ID              uuid.UUID         `json:"id"`
	Content         string            `json:"content"`
	MemoryType      string            `json:"memory_type"`
	Hash            string            `json:"hash"`
	Embedding       []float32         `json:"embedding,omitempty"`
	AgentID         *uuid.UUID        `json:"agent_id,omitempty"`
	AgentName       string            `json:"agent_name,omitempty"`
	AgentType       string            `json:"agent_type,omitempty"`
	InvestigationID string            `json:"investigation_id,omitempty"`
	CorrelationKey  string            `json:"correlation_key,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Entities        []string          `json:"entities,omitempty"`
	Metadata        map[string]any    `json:"metadata,omitempty"`
	Confidence      *float64          `json:"confidence,omitempty"`
	AccessCount     int               `json:"access_count"`
	ExpiresAt       *time.Time        `json:"expires_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type ScoredMemory struct {
	AgentMemoryRecord
	Score float64 `json:"score"`
}

type MemoryFilters struct {
	AgentID         *uuid.UUID
	MemoryType      *string
	InvestigationID *string
	CorrelationKey  *string
	MinConfidence   *float64
	Query           string
	Limit           int
	Offset          int
}

type AgentMemoryStore interface {
	Create(ctx context.Context, mem *AgentMemoryRecord) (*AgentMemoryRecord, error)
	Get(ctx context.Context, id uuid.UUID) (*AgentMemoryRecord, error)
	Update(ctx context.Context, id uuid.UUID, content string, embedding []float32) (*AgentMemoryRecord, error)
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, f MemoryFilters) ([]AgentMemoryRecord, int, error)
	Search(ctx context.Context, embedding []float32, topK int, f MemoryFilters) ([]ScoredMemory, error)
	SearchByText(ctx context.Context, query string, topK int, f MemoryFilters) ([]ScoredMemory, error)
	IncrementAccess(ctx context.Context, ids []uuid.UUID) error
	DeleteExpired(ctx context.Context) (int, error)
	ExistsByHash(ctx context.Context, hash string) (bool, error)
	FindByInvestigation(ctx context.Context, investigationID string) ([]AgentMemoryRecord, error)
}

type pgAgentMemoryStore struct {
	pgStoreBase
	db *sql.DB
}

func newPGAgentMemoryStore(client *ent.Client, db *sql.DB) AgentMemoryStore {
	return &pgAgentMemoryStore{pgStoreBase{client: client}, db}
}

func (s *pgAgentMemoryStore) Create(ctx context.Context, mem *AgentMemoryRecord) (*AgentMemoryRecord, error) {
	if mem == nil {
		return nil, errors.New("nil memory record")
	}
	if strings.TrimSpace(mem.Content) == "" {
		return nil, errors.New("content is required")
	}
	if strings.TrimSpace(mem.Hash) == "" {
		return nil, errors.New("hash is required")
	}
	mem.MemoryType = strings.ToLower(strings.TrimSpace(mem.MemoryType))
	if mem.MemoryType == "" {
		mem.MemoryType = MemoryTypeFact
	}
	if !IsValidMemoryType(mem.MemoryType) {
		return nil, fmt.Errorf("invalid memory_type %q", mem.MemoryType)
	}
	if mem.Labels == nil {
		mem.Labels = map[string]string{}
	}
	if mem.Entities == nil {
		mem.Entities = []string{}
	}
	if mem.Metadata == nil {
		mem.Metadata = map[string]any{}
	}

	now := time.Now().UTC()
	mem.CreatedAt = now
	mem.UpdatedAt = now

	b := s.client.AgentMemory.Create().
		SetContent(mem.Content).
		SetMemoryType(mem.MemoryType).
		SetHash(mem.Hash).
		SetAgentName(mem.AgentName).
		SetAgentType(mem.AgentType).
		SetInvestigationID(mem.InvestigationID).
		SetCorrelationKey(mem.CorrelationKey).
		SetLabels(mem.Labels).
		SetEntities(mem.Entities).
		SetMetadata(mem.Metadata).
		SetAccessCount(0).
		SetCreatedAt(now).
		SetUpdatedAt(now)

	if mem.Embedding != nil {
		b.SetEmbedding(mem.Embedding)
	}
	if mem.AgentID != nil {
		b.SetAgentID(*mem.AgentID)
	}
	if mem.Confidence != nil {
		b.SetConfidence(*mem.Confidence)
	}
	if mem.ExpiresAt != nil {
		b.SetExpiresAt(*mem.ExpiresAt)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent memory: %w", err)
	}

	mem.ID = saved.ID

	if len(mem.Embedding) > 0 && s.db != nil {
		vecJSON, err := json.Marshal(mem.Embedding)
		if err != nil {
			logger.Warn("agent memory: failed to marshal embedding vector", "memory_id", mem.ID, "error", err)
		} else if _, err := s.db.ExecContext(ctx,
			`UPDATE agent_memories SET vec = $1::vector WHERE id = $2`,
			string(vecJSON), mem.ID,
		); err != nil {
			logger.Warn("agent memory: failed to persist embedding vector", "memory_id", mem.ID, "error", err)
		}
	}

	return mem, nil
}

func (s *pgAgentMemoryStore) Get(ctx context.Context, id uuid.UUID) (*AgentMemoryRecord, error) {
	m, err := s.client.AgentMemory.Get(ctx, id)
	if err != nil {
		return handleQueryErr[*AgentMemoryRecord](err, "agent memory")
	}
	return pgMemoryToRecord(m), nil
}

func (s *pgAgentMemoryStore) Update(ctx context.Context, id uuid.UUID, content string, embedding []float32) (*AgentMemoryRecord, error) {
	b := s.client.AgentMemory.UpdateOneID(id).
		SetContent(content).
		SetUpdatedAt(time.Now().UTC())

	if embedding != nil {
		b.SetEmbedding(embedding)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errors.New("agent memory not found")
		}
		return nil, fmt.Errorf("failed to update agent memory: %w", err)
	}

	if len(embedding) > 0 && s.db != nil {
		vecJSON, err := json.Marshal(embedding)
		if err != nil {
			logger.Warn("agent memory: failed to marshal embedding vector", "memory_id", id, "error", err)
		} else if _, err := s.db.ExecContext(ctx,
			`UPDATE agent_memories SET vec = $1::vector WHERE id = $2`,
			string(vecJSON), id,
		); err != nil {
			logger.Warn("agent memory: failed to persist embedding vector", "memory_id", id, "error", err)
		}
	}

	return pgMemoryToRecord(saved), nil
}

func (s *pgAgentMemoryStore) Delete(ctx context.Context, id uuid.UUID) error {
	err := s.client.AgentMemory.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return errors.New("agent memory not found")
		}
		return fmt.Errorf("failed to delete agent memory: %w", err)
	}
	return nil
}

func (s *pgAgentMemoryStore) List(ctx context.Context, f MemoryFilters) ([]AgentMemoryRecord, int, error) {
	query := s.client.AgentMemory.Query()
	query = applyMemoryFilters(query, f)

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count agent memories: %w", err)
	}

	query = query.Order(ent.Desc(agentmemory.FieldCreatedAt))
	if f.Limit > 0 {
		query = query.Limit(f.Limit)
	}
	if f.Offset > 0 {
		query = query.Offset(f.Offset)
	}

	items, err := query.All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list agent memories: %w", err)
	}

	out := make([]AgentMemoryRecord, 0, len(items))
	for _, m := range items {
		out = append(out, *pgMemoryToRecord(m))
	}
	return out, total, nil
}

func (s *pgAgentMemoryStore) Search(ctx context.Context, embedding []float32, topK int, f MemoryFilters) ([]ScoredMemory, error) {
	if s.db == nil || len(embedding) == 0 {
		return nil, nil
	}
	if topK <= 0 {
		topK = 10
	}

	whereClause, whereArgs := buildMemoryWhereClauses(f, 2)
	query := `
		SELECT id, content, memory_type, hash, agent_id, agent_name, agent_type,
		       investigation_id, correlation_key, labels, entities, metadata,
		       confidence, access_count, expires_at, created_at, updated_at,
		       1 - (vec <=> $1::vector) AS similarity
		FROM agent_memories
		WHERE vec IS NOT NULL AND (expires_at IS NULL OR expires_at > NOW())` + whereClause + `
		ORDER BY vec <=> $1::vector
		LIMIT $2`

	vecJSON, _ := json.Marshal(embedding)
	args := append([]any{string(vecJSON), topK}, whereArgs...)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("vector search agent memories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanScoredMemories(rows)
}

func (s *pgAgentMemoryStore) SearchByText(ctx context.Context, query string, topK int, f MemoryFilters) ([]ScoredMemory, error) {
	if s.db == nil || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if topK <= 0 {
		topK = 10
	}

	f.Query = strings.TrimSpace(query)
	whereClause, whereArgs := buildMemoryWhereClauses(f, 2)

	q := `
		SELECT id, content, memory_type, hash, agent_id, agent_name, agent_type,
		       investigation_id, correlation_key, labels, entities, metadata,
		       confidence, access_count, expires_at, created_at, updated_at,
		       ts_rank(to_tsvector('english', content), plainto_tsquery('english', $1)) AS similarity
		FROM agent_memories
		WHERE to_tsvector('english', content) @@ plainto_tsquery('english', $1)` + whereClause + `
		ORDER BY similarity DESC
		LIMIT $2`

	args := append([]any{query, topK}, whereArgs...)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("text search agent memories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanScoredMemories(rows)
}

func (s *pgAgentMemoryStore) IncrementAccess(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.client.AgentMemory.Update().
		Where(agentmemory.IDIn(ids...)).
		AddAccessCount(1).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("increment access count: %w", err)
	}
	return nil
}

func (s *pgAgentMemoryStore) DeleteExpired(ctx context.Context) (int, error) {
	now := time.Now().UTC()
	n, err := s.client.AgentMemory.Delete().
		Where(
			agentmemory.ExpiresAtNotNil(),
			agentmemory.ExpiresAtLTE(now),
		).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired memories: %w", err)
	}
	return n, nil
}

func (s *pgAgentMemoryStore) ExistsByHash(ctx context.Context, hash string) (bool, error) {
	exists, err := s.client.AgentMemory.Query().
		Where(agentmemory.Hash(hash)).
		Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("check memory hash: %w", err)
	}
	return exists, nil
}

func (s *pgAgentMemoryStore) FindByInvestigation(ctx context.Context, investigationID string) ([]AgentMemoryRecord, error) {
	items, err := s.client.AgentMemory.Query().
		Where(agentmemory.InvestigationID(investigationID)).
		Order(ent.Desc(agentmemory.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("find memories by investigation: %w", err)
	}
	out := make([]AgentMemoryRecord, 0, len(items))
	for _, m := range items {
		out = append(out, *pgMemoryToRecord(m))
	}
	return out, nil
}

func applyMemoryFilters(query *ent.AgentMemoryQuery, f MemoryFilters) *ent.AgentMemoryQuery {
	if f.MemoryType != nil {
		query = query.Where(agentmemory.MemoryType(*f.MemoryType))
	}
	if f.AgentID != nil {
		query = query.Where(agentmemory.AgentID(*f.AgentID))
	}
	if f.InvestigationID != nil {
		query = query.Where(agentmemory.InvestigationID(*f.InvestigationID))
	}
	if f.CorrelationKey != nil {
		query = query.Where(agentmemory.CorrelationKey(*f.CorrelationKey))
	}
	if f.MinConfidence != nil {
		query = query.Where(agentmemory.ConfidenceGTE(*f.MinConfidence))
	}
	return query
}

func buildMemoryWhereClauses(f MemoryFilters, startParamIdx int) (string, []any) {
	var clauses []string
	var args []any
	idx := startParamIdx
	if f.MemoryType != nil && *f.MemoryType != "" {
		idx++
		clauses = append(clauses, fmt.Sprintf(" AND memory_type = $%d", idx))
		args = append(args, *f.MemoryType)
	}
	if f.AgentID != nil {
		idx++
		clauses = append(clauses, fmt.Sprintf(" AND agent_id = $%d", idx))
		args = append(args, f.AgentID.String())
	}
	if f.InvestigationID != nil && *f.InvestigationID != "" {
		idx++
		clauses = append(clauses, fmt.Sprintf(" AND investigation_id = $%d", idx))
		args = append(args, *f.InvestigationID)
	}
	if f.CorrelationKey != nil && *f.CorrelationKey != "" {
		idx++
		clauses = append(clauses, fmt.Sprintf(" AND correlation_key = $%d", idx))
		args = append(args, *f.CorrelationKey)
	}
	return strings.Join(clauses, ""), args
}

func scanScoredMemories(rows *sql.Rows) ([]ScoredMemory, error) {
	var out []ScoredMemory
	for rows.Next() {
		var (
			id              uuid.UUID
			content         string
			memoryType      string
			hash            string
			agentID         *uuid.UUID
			agentName       sql.NullString
			agentType       sql.NullString
			investigationID sql.NullString
			correlationKey  sql.NullString
			labelsJSON      sql.NullString
			entitiesJSON    sql.NullString
			metadataJSON    sql.NullString
			confidence      *float64
			accessCount     int
			expiresAt       *time.Time
			createdAt       time.Time
			updatedAt       time.Time
			similarity      float64
		)
		if err := rows.Scan(
			&id, &content, &memoryType, &hash, &agentID,
			&agentName, &agentType, &investigationID, &correlationKey,
			&labelsJSON, &entitiesJSON, &metadataJSON,
			&confidence, &accessCount, &expiresAt,
			&createdAt, &updatedAt, &similarity,
		); err != nil {
			return nil, fmt.Errorf("scan scored memory: %w", err)
		}

		var labels map[string]string
		if labelsJSON.Valid && labelsJSON.String != "" {
			_ = json.Unmarshal([]byte(labelsJSON.String), &labels)
		}
		if labels == nil {
			labels = map[string]string{}
		}

		var entities []string
		if entitiesJSON.Valid && entitiesJSON.String != "" {
			_ = json.Unmarshal([]byte(entitiesJSON.String), &entities)
		}
		if entities == nil {
			entities = []string{}
		}

		var metadata map[string]any
		if metadataJSON.Valid && metadataJSON.String != "" {
			_ = json.Unmarshal([]byte(metadataJSON.String), &metadata)
		}
		if metadata == nil {
			metadata = map[string]any{}
		}

		rec := AgentMemoryRecord{
			ID:              id,
			Content:         content,
			MemoryType:      memoryType,
			Hash:            hash,
			AgentID:         agentID,
			AgentName:       agentName.String,
			AgentType:       agentType.String,
			InvestigationID: investigationID.String,
			CorrelationKey:  correlationKey.String,
			Labels:          labels,
			Entities:        entities,
			Metadata:        metadata,
			Confidence:      confidence,
			AccessCount:     accessCount,
			ExpiresAt:       expiresAt,
			CreatedAt:       createdAt,
			UpdatedAt:       updatedAt,
		}
		out = append(out, ScoredMemory{AgentMemoryRecord: rec, Score: similarity})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan scored memories: %w", err)
	}
	return out, nil
}

func pgMemoryToRecord(m *ent.AgentMemory) *AgentMemoryRecord {
	var labels map[string]string
	if m.Labels != nil {
		labels = m.Labels
	} else {
		labels = map[string]string{}
	}

	var entities []string
	if m.Entities != nil {
		entities = m.Entities
	} else {
		entities = []string{}
	}

	var metadata map[string]any
	if m.Metadata != nil {
		metadata = m.Metadata
	} else {
		metadata = map[string]any{}
	}

	var embedding []float32
	if m.Embedding != nil {
		embedding = m.Embedding
	}

	return &AgentMemoryRecord{
		ID:              m.ID,
		Content:         m.Content,
		MemoryType:      m.MemoryType,
		Hash:            m.Hash,
		Embedding:       embedding,
		AgentID:         m.AgentID,
		AgentName:       m.AgentName,
		AgentType:       m.AgentType,
		InvestigationID: m.InvestigationID,
		CorrelationKey:  m.CorrelationKey,
		Labels:          labels,
		Entities:        entities,
		Metadata:        metadata,
		Confidence:      m.Confidence,
		AccessCount:     m.AccessCount,
		ExpiresAt:       m.ExpiresAt,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}
