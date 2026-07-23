package knowledge

import (
	"context"
	"slices"

	"alga/store"
)

// NotesFinder returns the applicable organizational knowledge notes for an
// investigation (runbooks, known_issues, service_owners, facts).
type NotesFinder interface {
	Find(ctx context.Context, inv *store.AlertInvestigationRecord, limit int) []NoteEntry
}

// knowledgeStoreNotesFinder is the default NotesFinder backed by
// store.KnowledgeStore.Match.
type knowledgeStoreNotesFinder struct {
	store store.KnowledgeStore
}

// NewNotesFinder returns a NotesFinder backed by the given KnowledgeStore.
func NewNotesFinder(s store.KnowledgeStore) NotesFinder {
	return &knowledgeStoreNotesFinder{store: s}
}

// Find returns up to limit notes whose selectors match the investigation's
// primary labels.
func (f *knowledgeStoreNotesFinder) Find(ctx context.Context, inv *store.AlertInvestigationRecord, limit int) []NoteEntry {
	if f == nil || f.store == nil || inv == nil || limit <= 0 {
		return nil
	}
	notes, err := f.store.Match(ctx, primaryLabels(inv), limit)
	if err != nil || len(notes) == 0 {
		return nil
	}
	out := make([]NoteEntry, 0, len(notes))
	for _, n := range notes {
		out = append(out, NoteEntry{
			ID:    n.ID.String(),
			Kind:  n.Kind,
			Title: n.Title,
			Body:  n.BodyMarkdown,
			Tags:  slices.Clone(n.Tags),
		})
	}
	return out
}
