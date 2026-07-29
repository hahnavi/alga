package models

import (
	"time"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"
)

type AgentMemory struct {
	BaseModel

	Content         string            `bun:"content,notnull"`
	MemoryType      string            `bun:"memory_type,notnull,default:'fact'"`
	Hash            string            `bun:"hash,notnull,unique"`
	Embedding       []float32         `bun:"embedding,type:jsonb"`
	Vec             pgvector.Vector   `bun:"vec,type:vector(1536)"`
	AgentID         *uuid.UUID        `bun:"agent_id"`
	AgentName       string            `bun:"agent_name"`
	AgentType       string            `bun:"agent_type"`
	InvestigationID string            `bun:"investigation_id"`
	CorrelationKey  string            `bun:"correlation_key"`
	Labels          map[string]string `bun:"labels,type:jsonb"`
	Entities        []string          `bun:"entities,type:jsonb"`
	Metadata        map[string]any    `bun:"metadata,type:jsonb"`
	Confidence      *float64          `bun:"confidence"`
	AccessCount     int               `bun:"access_count,notnull,default:0"`
	ExpiresAt       *time.Time        `bun:"expires_at"`
}

func (*AgentMemory) TableName() string { return "agent_memories" }
