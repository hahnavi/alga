package models

type EscalationPolicy struct {
	BaseModel

	Name        string                  `bun:"name,notnull,unique"`
	Description string                  `bun:"description"`
	RepeatCount int                     `bun:"repeat_count,notnull,default:3"`
	Levels      []EscalationLevelRecord `bun:"levels,type:jsonb,notnull,default:'[]'"`
}

func (*EscalationPolicy) TableName() string { return "escalation_policies" }
