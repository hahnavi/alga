package models

import (
	"github.com/google/uuid"
)

type OnCallSchedule struct {
	BaseModel

	TeamID *uuid.UUID `bun:"team_id"`
}
