package models

// SystemConfig is a singleton configuration row.
type SystemConfig struct {
	BaseModel `bun:"table:system_config"`

	Config map[string]any `bun:"config,type:jsonb"`
}
