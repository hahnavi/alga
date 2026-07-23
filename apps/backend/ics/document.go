package ics

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type DocumentManager struct {
	docStore DocumentStore
}

func NewDocumentManager(docStore DocumentStore) *DocumentManager {
	return &DocumentManager{docStore: docStore}
}

func (m *DocumentManager) GetAllSections(ctx context.Context, incidentNumber int64) ([]DocumentRecord, error) {
	return m.docStore.GetAllSections(ctx, incidentNumber)
}

func (m *DocumentManager) UpdateSection(ctx context.Context, incidentNumber int64, section DocumentSection, content string, version int, userID uuid.UUID) (*DocumentRecord, error) {
	if !ValidDocumentSection(section) {
		return nil, fmt.Errorf("invalid document section: %s", section)
	}
	return m.docStore.UpsertSection(ctx, incidentNumber, section, content, version, userID)
}

func (m *DocumentManager) InitializeForIncident(ctx context.Context, incidentNumber int64, triageReport map[string]any) error {
	sections := map[DocumentSection]string{
		SectionCurrentStatus:    "Triaging — IC is assessing scope",
		SectionImpactAssessment: "",
		SectionActionsTaken:     "",
		SectionOpenQuestions:    "",
		SectionResources:        "",
		SectionTimelineSummary:  "",
	}
	if triageReport != nil {
		if severity, ok := triageReport["severity"].(string); ok && severity != "" {
			sections[SectionImpactAssessment] = fmt.Sprintf("Severity: %s", severity)
		}
	}
	return m.docStore.InitializeDocument(ctx, incidentNumber, sections)
}
