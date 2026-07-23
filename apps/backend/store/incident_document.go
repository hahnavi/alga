package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	entincident "alga/ent/incident"
	entdoc "alga/ent/incidentdocument"

	"alga/ics"
)

type IncidentDocumentRecord struct {
	ID             uuid.UUID  `json:"id"`
	IncidentNumber int64      `json:"incident_number"`
	Section        string     `json:"section"`
	Content        string     `json:"content"`
	Version        int        `json:"version"`
	UpdatedBy      *uuid.UUID `json:"updated_by,omitempty"`
	UpdatedAt      string     `json:"updated_at"`
}

var ErrDocumentVersionConflict = errors.New("document version conflict")

type IncidentDocumentStore interface {
	GetAllSections(ctx context.Context, incidentNumber int64) ([]IncidentDocumentRecord, error)
	GetSection(ctx context.Context, incidentNumber int64, section ics.DocumentSection) (*IncidentDocumentRecord, error)
	UpsertSection(ctx context.Context, incidentNumber int64, section ics.DocumentSection, content string, version int, userID uuid.UUID) (*IncidentDocumentRecord, error)
	InitializeDocument(ctx context.Context, incidentNumber int64, sections map[ics.DocumentSection]string) error
}

type pgIncidentDocumentStore struct {
	pgStoreBase
}

func newPGIncidentDocumentStore(client *ent.Client) IncidentDocumentStore {
	return &pgIncidentDocumentStore{pgStoreBase{client: client}}
}

func (s *pgIncidentDocumentStore) GetAllSections(ctx context.Context, incidentNumber int64) ([]IncidentDocumentRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	docs, err := s.client.IncidentDocument.Query().
		Where(
			entdoc.HasIncidentWith(entincident.IncidentNumber(incidentNumber)),
		).
		Order(ent.Asc(entdoc.FieldSection)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query document sections: %w", err)
	}

	records := make([]IncidentDocumentRecord, 0, len(docs))
	for _, d := range docs {
		records = append(records, s.toRecord(d, incidentNumber))
	}
	return records, nil
}

func (s *pgIncidentDocumentStore) GetSection(ctx context.Context, incidentNumber int64, section ics.DocumentSection) (*IncidentDocumentRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	d, err := s.client.IncidentDocument.Query().
		Where(
			entdoc.HasIncidentWith(entincident.IncidentNumber(incidentNumber)),
			entdoc.SectionEQ(string(section)),
		).
		Only(ctx)
	if err != nil {
		return handleQueryErr[*IncidentDocumentRecord](err, "incident document section")
	}

	rec := s.toRecord(d, incidentNumber)
	return &rec, nil
}

func (s *pgIncidentDocumentStore) UpsertSection(ctx context.Context, incidentNumber int64, section ics.DocumentSection, content string, version int, userID uuid.UUID) (*IncidentDocumentRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.client.Incident.Query().
		Where(entincident.IncidentNumber(incidentNumber), entincident.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return nil, fmt.Errorf("failed to find incident: %w", err)
	}

	existing, err := s.client.IncidentDocument.Query().
		Where(
			entdoc.HasIncidentWith(entincident.IncidentNumber(incidentNumber)),
			entdoc.SectionEQ(string(section)),
		).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, fmt.Errorf("failed to query existing section: %w", err)
	}

	if existing != nil {
		if existing.Version != version {
			return nil, ErrDocumentVersionConflict
		}

		update := s.client.IncidentDocument.UpdateOneID(existing.ID).
			SetContent(content).
			SetVersion(version + 1).
			SetUpdatedAt(time.Now().UTC())
		if userID != uuid.Nil {
			update.SetUpdatedByID(userID)
		} else {
			update.ClearUpdatedBy()
		}
		updated, err := update.Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to update document section: %w", err)
		}

		rec := s.toRecord(updated, incidentNumber)
		return &rec, nil
	}

	create := s.client.IncidentDocument.Create().
		SetSection(string(section)).
		SetContent(content).
		SetVersion(1).
		SetIncidentID(inc.ID).
		SetUpdatedAt(time.Now().UTC())
	if userID != uuid.Nil {
		create.SetUpdatedByID(userID)
	}
	created, err := create.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create document section: %w", err)
	}

	rec := s.toRecord(created, incidentNumber)
	return &rec, nil
}

func (s *pgIncidentDocumentStore) InitializeDocument(ctx context.Context, incidentNumber int64, sections map[ics.DocumentSection]string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.client.Incident.Query().
		Where(entincident.IncidentNumber(incidentNumber), entincident.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return fmt.Errorf("failed to find incident: %w", err)
	}

	builders := make([]*ent.IncidentDocumentCreate, 0, len(sections))
	for sec, content := range sections {
		b := s.client.IncidentDocument.Create().
			SetSection(string(sec)).
			SetContent(content).
			SetVersion(1).
			SetIncidentID(inc.ID).
			SetUpdatedAt(time.Now().UTC())
		builders = append(builders, b)
	}

	_, err = s.client.IncidentDocument.CreateBulk(builders...).Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize document sections: %w", err)
	}
	return nil
}

func (s *pgIncidentDocumentStore) toRecord(d *ent.IncidentDocument, incidentNumber int64) IncidentDocumentRecord {
	rec := IncidentDocumentRecord{
		ID:             d.ID,
		IncidentNumber: incidentNumber,
		Section:        d.Section,
		Content:        d.Content,
		Version:        d.Version,
		UpdatedAt:      d.UpdatedAt.Format(time.RFC3339),
	}

	if u := d.Edges.UpdatedBy; u != nil {
		rec.UpdatedBy = &u.ID
	}

	return rec
}
