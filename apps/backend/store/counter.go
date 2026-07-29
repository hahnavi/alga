package store

import (
	"context"

	"github.com/uptrace/bun"
)

// CounterStore provides a monotonically-increasing integer counter for
// generating sequential numbers (e.g. triage_number).
type CounterStore interface {
	Next(ctx context.Context, name string) (int64, error)
}

type pgCounterStore struct {
	pgStoreBase
}

func newPGCounterStore(db *bun.DB) CounterStore {
	return &pgCounterStore{pgStoreBase{db: db}}
}

func (s *pgCounterStore) Next(ctx context.Context, name string) (int64, error) {
	return nextPgCounter(ctx, s.db, name)
}
