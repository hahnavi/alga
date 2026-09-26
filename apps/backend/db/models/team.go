package models

type Team struct {
	BaseModel `bun:"table:teams"`

	Name        string `bun:"name,notnull,unique"`
	Description string `bun:"description,notnull,default:''"`
}
