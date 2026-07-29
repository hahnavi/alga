package models

import (
	"time"

	"github.com/google/uuid"
)

type KnowledgeNote struct {
	BaseModel

	Kind                  string           `bun:"kind,notnull"`
	Title                 string           `bun:"title,notnull"`
	BodyMarkdown          string           `bun:"body_markdown,notnull"`
	Tags                  []string         `bun:"tags,type:jsonb"`
	Selectors             []RouteCondition `bun:"selectors,type:jsonb"`
	AuthorID              *uuid.UUID       `bun:"author_id"`
	AuthorType            string           `bun:"author_type,notnull,default:'user'"`
	AuthorName            string           `bun:"author_name,default:''"`
	SourceInvestigationID string           `bun:"source_investigation_id,default:''"`
	Confidence            *float64         `bun:"confidence"`
	ExpiresAt             *time.Time       `bun:"expires_at"`
}

func (*KnowledgeNote) TableName() string { return "knowledge_notes" }
