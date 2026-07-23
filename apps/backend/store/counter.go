package store

import (
	"context"

	"alga/ent"
)

// CounterStore provides a monotonically-increasing integer counter for
// generating sequential numbers (e.g. triage_number).
type CounterStore interface {
	Next(ctx context.Context, name string) (int64, error)
}

type pgCounterStore struct {
	pgStoreBase
}

func newPGCounterStore(client *ent.Client) CounterStore {
	return &pgCounterStore{pgStoreBase{client: client}}
}

func (s *pgCounterStore) Next(ctx context.Context, name string) (int64, error) {
	return nextPgCounter(ctx, s.client, name)
}
