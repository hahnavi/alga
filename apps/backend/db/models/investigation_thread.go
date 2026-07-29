package models

type InvestigationThread struct {
	BaseModel

	ThreadID  string `bun:"thread_id,notnull,unique"`
	OwnerType string `bun:"owner_type,notnull"`
	OwnerID   string `bun:"owner_id,notnull"`
}
