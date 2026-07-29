package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type PostMortem struct {
	bun.BaseModel `bun:"table:post_mortems"`

	ID                  uuid.UUID        `bun:"id,pk"`
	IncidentID          uuid.UUID        `bun:"incident_id,notnull,unique"`
	Title               string           `bun:"title,notnull,default:''"`
	Status              string           `bun:"status,notnull,default:'draft'"`
	Summary             string           `bun:"summary,notnull,default:''"`
	Timeline            []map[string]any `bun:"timeline,type:jsonb"`
	RootCause           string           `bun:"root_cause,notnull,default:''"`
	ContributingFactors []string         `bun:"contributing_factors,type:jsonb"`
	Impact              string           `bun:"impact,notnull,default:''"`
	LessonsLearned      string           `bun:"lessons_learned,notnull,default:''"`
	WhatWentWell        string           `bun:"what_went_well,notnull,default:''"`
	WhatWentWrong       string           `bun:"what_went_wrong,notnull,default:''"`
	BlamelessConfirmed  bool             `bun:"blameless_confirmed,notnull,default:false"`
	BlamelessNotes      string           `bun:"blameless_notes,notnull,default:''"`
	ApprovedByID        *uuid.UUID       `bun:"approved_by_id"`
	PublishedAt         *time.Time       `bun:"published_at"`
	CreatedAt           time.Time        `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt           time.Time        `bun:"updated_at,notnull,default:current_timestamp"`
}

func (*PostMortem) TableName() string { return "post_mortems" }
