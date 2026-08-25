import { ref, type Ref } from "vue";
import { api, type IncidentDocumentSection, type IncidentRecord } from "@/lib/api";
import { getErrorMessage } from "@/lib/error";
import { useToast } from "@/lib/toast";

/**
 * Owns the four editable document surfaces on the incident detail
 * page: summary (a top-level field on the incident row), plus the
 * three document sections (impact assessment, root cause, and
 * resolution). Each section tracks its own `content` / `version` /
 * `editing` / `saving` refs and supports an optimistic edit/save
 * round-trip.
 *
 * The composable also owns the `unescapeDocContent` helper used to
 * turn the literal `\n` / `\t` sequences agents may have written
 * into real whitespace before rendering.
 *
 * The page still binds the editor inputs and the start/save
 * triggers — this composable just centralizes the state and the
 * `api.*` calls.
 */
export function useIncidentDocumentSections(
  incidentNumber: Ref<number>,
  incident: Ref<IncidentRecord | null>,
  setIncident: (next: IncidentRecord) => void,
) {
  const { push } = useToast();

  const documentSections = ref<IncidentDocumentSection[]>([]);
  const impactContent = ref("");
  const impactVersion = ref(0);
  const impactEditing = ref(false);
  const impactSaving = ref(false);

  const rootCauseContent = ref("");
  const rootCauseVersion = ref(0);
  const rootCauseEditing = ref(false);
  const rootCauseSaving = ref(false);

  const resolutionContent = ref("");
  const resolutionVersion = ref(0);
  const resolutionEditing = ref(false);
  const resolutionSaving = ref(false);

  const summaryContent = ref("");
  const summaryEditing = ref(false);
  const summarySaving = ref(false);

  function unescapeDocContent(content: string): string {
    return content.replace(/\\n/g, "\n").replace(/\\t/g, "\t");
  }

  async function load() {
    try {
      documentSections.value = await api.getIncidentDocument(incidentNumber.value);
    } catch (err) {
      documentSections.value = [];
      impactContent.value = "";
      impactVersion.value = 0;
      rootCauseContent.value = "";
      rootCauseVersion.value = 0;
      resolutionContent.value = "";
      resolutionVersion.value = 0;
      push(getErrorMessage(err, "Failed to load document sections"), "error");
      return;
    }
    const impact = documentSections.value.find((s) => s.section === "impact_assessment");
    impactContent.value = unescapeDocContent(impact?.content ?? "");
    impactVersion.value = impact?.version ?? 0;

    const rc = documentSections.value.find((s) => s.section === "root_cause");
    rootCauseContent.value = unescapeDocContent(rc?.content ?? "");
    rootCauseVersion.value = rc?.version ?? 0;

    const res = documentSections.value.find((s) => s.section === "resolution");
    resolutionContent.value = unescapeDocContent(res?.content ?? "");
    resolutionVersion.value = res?.version ?? 0;
  }

  function startEditSummary() {
    summaryContent.value = unescapeDocContent(incident.value?.summary ?? "");
    summaryEditing.value = true;
  }

  async function saveSummary() {
    if (!incident.value || summarySaving.value) return;
    summarySaving.value = true;
    try {
      const updated = await api.patchIncident(incident.value.incident_number, {
        summary: summaryContent.value,
      });
      setIncident(updated);
      summaryEditing.value = false;
      push("Summary saved", "success");
    } catch (err) {
      push(getErrorMessage(err, "Failed to save summary"), "error");
    } finally {
      summarySaving.value = false;
    }
  }

  function startEditImpact() {
    const impact = documentSections.value.find((s) => s.section === "impact_assessment");
    impactContent.value = unescapeDocContent(impact?.content ?? "");
    impactEditing.value = true;
  }

  function startEditRootCause() {
    const rc = documentSections.value.find((s) => s.section === "root_cause");
    rootCauseContent.value = unescapeDocContent(rc?.content ?? "");
    rootCauseEditing.value = true;
  }

  function startEditResolution() {
    const res = documentSections.value.find((s) => s.section === "resolution");
    resolutionContent.value = unescapeDocContent(res?.content ?? "");
    resolutionEditing.value = true;
  }

  async function saveImpact() {
    if (!incident.value || impactSaving.value) return;
    impactSaving.value = true;
    try {
      const updated = await api.updateIncidentDocumentSection(
        incident.value.incident_number,
        "impact_assessment",
        { content: impactContent.value, version: impactVersion.value },
      );
      impactVersion.value = updated.version;
      impactContent.value = updated.content;
      impactEditing.value = false;
      push("Impact assessment saved", "success");
    } catch (err) {
      push(getErrorMessage(err, "Failed to save"), "error");
    } finally {
      impactSaving.value = false;
    }
  }

  async function saveRootCause() {
    if (!incident.value || rootCauseSaving.value) return;
    rootCauseSaving.value = true;
    try {
      const updated = await api.updateIncidentDocumentSection(
        incident.value.incident_number,
        "root_cause",
        { content: rootCauseContent.value, version: rootCauseVersion.value },
      );
      rootCauseVersion.value = updated.version;
      rootCauseContent.value = updated.content;
      rootCauseEditing.value = false;
      push("Root cause saved", "success");
    } catch (err) {
      push(getErrorMessage(err, "Failed to save"), "error");
    } finally {
      rootCauseSaving.value = false;
    }
  }

  async function saveResolution() {
    if (!incident.value || resolutionSaving.value) return;
    resolutionSaving.value = true;
    try {
      const updated = await api.updateIncidentDocumentSection(
        incident.value.incident_number,
        "resolution",
        { content: resolutionContent.value, version: resolutionVersion.value },
      );
      resolutionVersion.value = updated.version;
      resolutionContent.value = updated.content;
      resolutionEditing.value = false;
      push("Resolution saved", "success");
    } catch (err) {
      push(getErrorMessage(err, "Failed to save"), "error");
    } finally {
      resolutionSaving.value = false;
    }
  }

  function reset() {
    documentSections.value = [];
    impactContent.value = "";
    impactVersion.value = 0;
    impactEditing.value = false;
    rootCauseContent.value = "";
    rootCauseVersion.value = 0;
    rootCauseEditing.value = false;
    resolutionContent.value = "";
    resolutionVersion.value = 0;
    resolutionEditing.value = false;
    summaryContent.value = "";
    summaryEditing.value = false;
  }

  return {
    documentSections,
    impactContent,
    impactVersion,
    impactEditing,
    impactSaving,
    rootCauseContent,
    rootCauseVersion,
    rootCauseEditing,
    rootCauseSaving,
    resolutionContent,
    resolutionVersion,
    resolutionEditing,
    resolutionSaving,
    summaryContent,
    summaryEditing,
    summarySaving,
    unescapeDocContent,
    load,
    startEditSummary,
    saveSummary,
    startEditImpact,
    startEditRootCause,
    startEditResolution,
    saveImpact,
    saveRootCause,
    saveResolution,
    reset,
  };
}
