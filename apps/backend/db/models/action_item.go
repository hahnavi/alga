package models

import (
	"time"

	"github.com/google/uuid"
)

type ActionItem struct {
	BaseModel

	PostMortemID uuid.UUID  `bun:"post_mortem_id,notnull"`
	Description  string     `bun:"description,notnull"`
	Type         string     `bun:"type,notnull,default:'investigate'"`
	AssigneeName string     `bun:"assignee_name"`
	AssigneeID   *uuid.UUID `bun:"assignee_id"`
	Status       string     `bun:"status,notnull,default:'open'"`
	Priority     string     `bun:"priority,notnull,default:'medium'"`
	DueDate      *time.Time `bun:"due_date"`
}

func (*ActionItem) TableName() string { return "action_items" }
