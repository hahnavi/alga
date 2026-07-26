package store

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/agentmemory"
	"alga/ent/alertinvestigation"
	"alga/ent/alertinvestigationalert"
	"alga/ent/alertinvestigationevent"
	"alga/ent/alertinvestigationupdateentry"
	"alga/ent/incidentinvestigation"
	"alga/ent/incidentinvestigationupdateentry"
	"alga/ent/investigationthread"
	"alga/ent/investigationthreadmessage"
)

// hardDeleteAlertCascade removes every investigation artifact linked to the
// alert (regardless of investigation status): the alert_investigations rows
// and their events/updates/join rows, agent memories scoped to those
// investigations, and the alert-owned investigation thread + its messages.
// It matches investigations by the alert's unique identity (id / alert_number)
// only — never by fingerprint, which can be shared across sibling alerts.
// It must run inside the alert delete tx so the tombstone set and the child
// cleanup commit atomically.
func hardDeleteAlertCascade(ctx context.Context, tx *ent.Tx, a *ent.Alert) error {
	if a == nil {
		return nil
	}

	invs, err := tx.Client().AlertInvestigation.Query().
		Where(alertinvestigation.HasAlertsWith(alertinvestigationalert.Or(
			alertinvestigationalert.AlertIDEQ(a.ID),
			alertinvestigationalert.AlertNumber(a.AlertNumber),
		))).
		Select(alertinvestigation.FieldID, alertinvestigation.FieldAlertInvestigationID).
		All(ctx)
	if err != nil {
		return fmt.Errorf("query linked alert investigations: %w", err)
	}
	if len(invs) == 0 {
		return deleteOwnerThreadInTx(ctx, tx, ThreadOwnerAlert, strconv.FormatInt(a.AlertNumber, 10))
	}

	invUUIDs := make([]uuid.UUID, 0, len(invs))
	invStrIDs := make([]string, 0, len(invs))
	for _, inv := range invs {
		invUUIDs = append(invUUIDs, inv.ID)
		invStrIDs = append(invStrIDs, inv.AlertInvestigationID)
	}

	if _, err := tx.Client().AlertInvestigationAlert.Delete().
		Where(alertinvestigationalert.AlertInvestigationIDIn(invUUIDs...)).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete alert investigation alerts: %w", err)
	}
	if _, err := tx.Client().AlertInvestigationUpdateEntry.Delete().
		Where(alertinvestigationupdateentry.AlertInvestigationIDIn(invUUIDs...)).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete alert investigation updates: %w", err)
	}
	if _, err := tx.Client().AlertInvestigationEvent.Delete().
		Where(alertinvestigationevent.AlertInvestigationIDIn(invUUIDs...)).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete alert investigation events: %w", err)
	}
	if _, err := tx.Client().AlertInvestigation.Delete().
		Where(alertinvestigation.IDIn(invUUIDs...)).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete alert investigations: %w", err)
	}

	// Agent memories are scoped by the investigation's string business id. This
	// assumes alert-investigation ids are distinct from incident-investigation
	// ids (they are generated as UUIDs / prefixed human ids), so no cross-type
	// collision is expected.
	if _, err := tx.Client().AgentMemory.Delete().
		Where(agentmemory.InvestigationIDIn(invStrIDs...)).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete agent memories: %w", err)
	}

	return deleteOwnerThreadInTx(ctx, tx, ThreadOwnerAlert, strconv.FormatInt(a.AlertNumber, 10))
}

// hardDeleteIncidentCascade removes every investigation artifact for the
// incident (regardless of status): the incident_investigations rows + their
// updates, agent memories scoped to those investigations, and the
// incident-owned investigation thread + its messages. source_alert_investigation_id
// back-refs on alert investigations are SET NULL via the existing FK, so they
// need no handling here. It must run inside the incident delete tx.
func hardDeleteIncidentCascade(ctx context.Context, tx *ent.Tx, inc *ent.Incident) error {
	if inc == nil {
		return nil
	}
	invs, err := tx.Client().IncidentInvestigation.Query().
		Where(incidentinvestigation.IncidentIDEQ(inc.ID)).
		Select(incidentinvestigation.FieldID, incidentinvestigation.FieldIncidentInvestigationID).
		All(ctx)
	if err != nil {
		return fmt.Errorf("query incident investigations: %w", err)
	}
	if len(invs) > 0 {
		invUUIDs := make([]uuid.UUID, 0, len(invs))
		invStrIDs := make([]string, 0, len(invs))
		for _, inv := range invs {
			invUUIDs = append(invUUIDs, inv.ID)
			invStrIDs = append(invStrIDs, inv.IncidentInvestigationID)
		}
		if _, err := tx.Client().IncidentInvestigationUpdateEntry.Delete().
			Where(incidentinvestigationupdateentry.IncidentInvestigationIDIn(invUUIDs...)).
			Exec(ctx); err != nil {
			return fmt.Errorf("delete incident investigation updates: %w", err)
		}
		if _, err := tx.Client().IncidentInvestigation.Delete().
			Where(incidentinvestigation.IDIn(invUUIDs...)).
			Exec(ctx); err != nil {
			return fmt.Errorf("delete incident investigations: %w", err)
		}
		if _, err := tx.Client().AgentMemory.Delete().
			Where(agentmemory.InvestigationIDIn(invStrIDs...)).
			Exec(ctx); err != nil {
			return fmt.Errorf("delete agent memories: %w", err)
		}
	}
	return deleteOwnerThreadInTx(ctx, tx, ThreadOwnerIncidentInvestigation, strconv.FormatInt(inc.IncidentNumber, 10))
}

// deleteOwnerThreadInTx deletes the polymorphic owner thread (and its messages)
// for (ownerType, ownerID). ownerID is the entity NUMBER as a string for both
// alert-owned and incident-owned threads.
func deleteOwnerThreadInTx(ctx context.Context, tx *ent.Tx, ownerType, ownerID string) error {
	threadIDs, err := tx.Client().InvestigationThread.Query().
		Where(
			investigationthread.OwnerTypeEQ(ownerType),
			investigationthread.OwnerIDEQ(ownerID),
		).
		IDs(ctx)
	if err != nil {
		return fmt.Errorf("query owner thread: %w", err)
	}
	if len(threadIDs) == 0 {
		return nil
	}
	if _, err := tx.Client().InvestigationThreadMessage.Delete().
		Where(investigationthreadmessage.ThreadIDIn(threadIDs...)).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete thread messages: %w", err)
	}
	if _, err := tx.Client().InvestigationThread.Delete().
		Where(investigationthread.IDIn(threadIDs...)).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete owner thread: %w", err)
	}
	return nil
}
