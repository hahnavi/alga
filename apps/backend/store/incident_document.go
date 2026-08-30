package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
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

func newPGIncidentDocumentStore(db *bun.DB) IncidentDocumentStore {
	return &pgIncidentDocumentStore{pgStoreBase{db: db}}
}

func (s *pgIncidentDocumentStore) findIncidentByNumber(ctx context.Context, incidentNumber int64) (*models.Incident, error) {
	var inc models.Incident
	err := s.db.NewSelect().Model(&inc).
		Where("incident_number = ?", incidentNumber).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("incident not found: %w", ErrIncidentNotFound)
		}
		return nil, fmt.Errorf("failed to find incident: %w", err)
	}
	return &inc, nil
}

func (s *pgIncidentDocumentStore) GetAllSections(ctx context.Context, incidentNumber int64) ([]IncidentDocumentRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.findIncidentByNumber(ctx, incidentNumber)
	if err != nil {
		return nil, err
	}

	var docs []models.IncidentDocument
	err = s.db.NewSelect().Model(&docs).
		Where("incident_id = ?", inc.ID).
		OrderExpr("section ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query document sections: %w", err)
	}

	records := make([]IncidentDocumentRecord, 0, len(docs))
	for _, d := range docs {
		records = append(records, s.toRecord(&d, incidentNumber))
	}
	return records, nil
}

func (s *pgIncidentDocumentStore) GetSection(ctx context.Context, incidentNumber int64, section ics.DocumentSection) (*IncidentDocumentRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.findIncidentByNumber(ctx, incidentNumber)
	if err != nil {
		return nil, err
	}

	var d models.IncidentDocument
	err = s.db.NewSelect().Model(&d).
		Where("incident_id = ?", inc.ID).
		Where("section = ?", string(section)).
		Scan(ctx)
	if err != nil {
		return handleQueryErr[*IncidentDocumentRecord](err, "incident document section")
	}

	rec := s.toRecord(&d, incidentNumber)
	return &rec, nil
}

func (s *pgIncidentDocumentStore) UpsertSection(ctx context.Context, incidentNumber int64, section ics.DocumentSection, content string, version int, userID uuid.UUID) (*IncidentDocumentRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.findIncidentByNumber(ctx, incidentNumber)
	if err != nil {
		return nil, err
	}

	var existing models.IncidentDocument
	err = s.db.NewSelect().Model(&existing).
		Where("incident_id = ?", inc.ID).
		Where("section = ?", string(section)).
		Scan(ctx)
	if err != nil && !isNotFound(err) {
		return nil, fmt.Errorf("failed to query existing section: %w", err)
	}

	now := time.Now().UTC()

	if err == nil {
		// Existing document found - update it
		if existing.Version != version {
			return nil, ErrDocumentVersionConflict
		}

		upd := s.db.NewUpdate().Model((*models.IncidentDocument)(nil)).
			Set("content = ?", content).
			Set("version = ?", version+1).
			Set("updated_at = ?", now).
			Where("id = ?", existing.ID)
		if userID != uuid.Nil {
			upd = upd.Set("updated_by_id = ?", userID)
		} else {
			upd = upd.Set("updated_by_id = NULL")
		}
		_, err = upd.Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to update document section: %w", err)
		}

		var updated models.IncidentDocument
		err = s.db.NewSelect().Model(&updated).Where("id = ?", existing.ID).Scan(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to reload document section: %w", err)
		}
		rec := s.toRecord(&updated, incidentNumber)
		return &rec, nil
	}

	// No existing document - create it
	m := &models.IncidentDocument{
		ID:         models.NewUUID(),
		Section:    string(section),
		Content:    content,
		Version:    1,
		IncidentID: inc.ID,
		UpdatedAt:  now,
	}
	if userID != uuid.Nil {
		m.UpdatedByID = &userID
	}

	_, err = s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create document section: %w", err)
	}

	rec := s.toRecord(m, incidentNumber)
	return &rec, nil
}

func (s *pgIncidentDocumentStore) InitializeDocument(ctx context.Context, incidentNumber int64, sections map[ics.DocumentSection]string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	inc, err := s.findIncidentByNumber(ctx, incidentNumber)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	docs := make([]models.IncidentDocument, 0, len(sections))
	for sec, content := range sections {
		docs = append(docs, models.IncidentDocument{
			ID:         models.NewUUID(),
			Section:    string(sec),
			Content:    content,
			Version:    1,
			IncidentID: inc.ID,
			UpdatedAt:  now,
		})
	}

	if len(docs) > 0 {
		// Idempotent per spec R17: sections that already exist (e.g. begin-triage
		// ran before war-room provisioning) are left untouched instead of
		// failing on the (incident_id, section) unique index.
		if _, err = s.db.NewInsert().Model(&docs).
			On("CONFLICT (incident_id, section) DO NOTHING").
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to initialize document sections: %w", err)
		}
	}
	return nil
}

func (s *pgIncidentDocumentStore) toRecord(d *models.IncidentDocument, incidentNumber int64) IncidentDocumentRecord {
	rec := IncidentDocumentRecord{
		ID:             d.ID,
		IncidentNumber: incidentNumber,
		Section:        d.Section,
		Content:        d.Content,
		Version:        d.Version,
		UpdatedAt:      d.UpdatedAt.Format(time.RFC3339),
	}

	if d.UpdatedByID != nil {
		rec.UpdatedBy = d.UpdatedByID
	}

	return rec
}
