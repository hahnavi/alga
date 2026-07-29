package models

import "github.com/uptrace/bun"

type Counter struct {
	bun.BaseModel `bun:"table:counters"`

	ID  string `bun:"id,pk"`
	Seq int64  `bun:"seq,notnull,default:0"`
}

func (*Counter) TableName() string { return "counters" }
