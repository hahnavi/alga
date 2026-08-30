import { reactive, ref, type Ref } from "vue";
import {
  api,
  type AlertRecord,
  type ImpactLevel,
  type IncidentCascadeSummary,
  type IncidentRecord,
  type IncidentPriority,
  type Severity,
} from "@/lib/api";
import { getErrorMessage } from "@/lib/error";
import { useToast, type ToastKind } from "@/lib/toast";

type ActionSuccess = { message: string; kind: ToastKind };

/**
 * Owns every "edit" surface on the incident detail page:
 *   - the edit-incident dialog (title / description / severity /
 *     impact / priority + submit)
 *   - the workflow actions (acknowledge / mitigate / resolve /
 *     close / reopen / cancel / promote / escalate)
 *   - the Slack channel create / unlink flows
 *   - the Google Meet create / unlink flows
 *   - the link-alert dialog
 *   - the add-timeline-entry dialog
 *
 * The composable centralizes the `actionLoading` flag (used by
 * `updatePageHeader` to disable the actions menu), the
 * `escalating` flag, the integrations probe (Slack / Google Meet
 * configured state), and all the dialog state.
 */
export function useIncidentEditor(
  incidentNumber: Ref<number>,
  incident: Ref<IncidentRecord | null>,
  setIncident: (next: IncidentRecord) => void,
  reload: () => Promise<void>,
  dataReload: () => Promise<void>,
  removeLinkedAlert: (num: number) => void,
) {
  const { push } = useToast();

  // Edit-incident dialog.
  const showEditDialog = ref(false);
  const editTitle = ref("");
  const editDescription = ref("");
  const editSeverity = ref<Severity | "">("");
  const editImpact = ref<ImpactLevel | "">("");
  const editPriority = ref<IncidentPriority | "">("");
  const editSubmitting = ref(false);
  const editError = ref("");

  // Workflow action state.
  const actionLoading = ref(false);
  const escalating = ref(false);

  // Link-alert dialog.
  const showLinkAlertDialog = ref(false);
  const linkAlertNumber = ref("");
  const linkAlertSubmitting = ref(false);
  const linkAlertError = ref("");

  const showUnlinkChannelConfirm = ref(false);

  const unlinkAlertTarget = ref<AlertRecord | null>(null);
  const unlinkAlertSubmitting = ref(false);

  // Add-timeline-entry dialog.
  const showAddTimelineDialog = ref(false);
  const timelineMessage = ref("");
  const timelineEventType = ref("manual");
  const timelineSubmitting = ref(false);
  const timelineError = ref("");

  // Integrations probe.
  const isSlackConfigured = ref(false);
  const creatingChannel = ref(false);
  const isGoogleMeetConfigured = ref(false);
  const creatingMeet = ref(false);

  function cascadeToast(verb: string, cascade: IncidentCascadeSummary): ActionSuccess {
    const { resolved, skipped, failed } = cascade;
    if (resolved === 0 && skipped === 0 && failed === 0) {
      return { message: `Incident ${verb.toLowerCase()}`, kind: "success" };
    }
    if (failed > 0) {
      return {
        message: `${verb} incident; ${resolved} alert(s) resolved, ${failed} failed`,
        kind: "info",
      };
    }
    if (resolved > 0) {
      return { message: `${verb} incident + ${resolved} linked alert(s)`, kind: "success" };
    }
    return {
      message: `Incident ${verb.toLowerCase()}; ${skipped} linked alert(s) already resolved`,
      kind: "success",
    };
  }

  async function executeAction(
    action: () => Promise<IncidentRecord>,
    success?: () => ActionSuccess,
  ) {
    actionLoading.value = true;
    try {
      const updated = await action();
      setIncident(updated);
      const result: ActionSuccess = success
        ? success()
        : { message: "Incident updated", kind: "success" };
      push(result.message, result.kind);
    } catch (err) {
      push(getErrorMessage(err, "Action failed"), "error");
    } finally {
      actionLoading.value = false;
    }
  }

  function acknowledge() {
    return executeAction(() => api.acknowledgeIncident(incidentNumber.value));
  }

  function mitigate() {
    return executeAction(() => api.mitigateIncident(incidentNumber.value));
  }

  function resolve() {
    let cascade: IncidentCascadeSummary = { resolved: 0, skipped: 0, failed: 0 };
    return executeAction(
      async () => {
        const res = await api.resolveIncident(incidentNumber.value);
        cascade = res.cascade;
        try {
          await api.addIncidentThreadMessage(incidentNumber.value, { message: "/stop" });
        } catch {
          // best-effort
        }
        return res.incident;
      },
      () => cascadeToast("Resolved", cascade),
    );
  }

  function close() {
    let cascade: IncidentCascadeSummary = { resolved: 0, skipped: 0, failed: 0 };
    let pmMissing = false;
    return executeAction(
      async () => {
        const res = await api.closeIncidentWithPMWarning(incidentNumber.value);
        cascade = res.cascade;
        pmMissing = res.postMortemMissing;
        return res.incident;
      },
      () => {
        if (pmMissing) {
          // The close succeeded; the warning is advisory, so surface it as a
          // distinct toast rather than folding it into the success message.
          push("Closed without an approved post-mortem — consider writing one", "info");
        }
        return cascadeToast("Closed", cascade);
      },
    );
  }

  function reopen() {
    return executeAction(() => api.reopenIncident(incidentNumber.value));
  }

  function cancel() {
    return executeAction(() => api.cancelIncident(incidentNumber.value));
  }

  function promoteToActive() {
    return executeAction(() => api.promoteIncident(incidentNumber.value));
  }

  async function escalateIncident() {
    if (!incident.value || escalating.value) return;
    escalating.value = true;
    try {
      await api.escalateIncident(incident.value.incident_number);
      push("Incident escalated", "success");
      await reload();
    } catch (err) {
      push(getErrorMessage(err, "Failed to escalate"), "error");
    } finally {
      escalating.value = false;
    }
  }

  async function createChannel() {
    if (!incident.value) return;
    creatingChannel.value = true;
    try {
      const updated = await api.createIncidentSlackChannel(incident.value.incident_number);
      setIncident(updated);
      push("Slack channel created", "success");
    } catch (e: unknown) {
      push(getErrorMessage(e, "Failed to create Slack channel"), "error");
    } finally {
      creatingChannel.value = false;
    }
  }

  async function createMeet() {
    if (!incident.value) return;
    creatingMeet.value = true;
    try {
      const updated = await api.createIncidentGoogleMeet(incident.value.incident_number);
      setIncident(updated);
      push("Google Meet created", "success");
    } catch (e: unknown) {
      push(getErrorMessage(e, "Failed to create Google Meet"), "error");
    } finally {
      creatingMeet.value = false;
    }
  }

  async function unlinkMeet() {
    if (!incident.value) return;
    try {
      const updated = await api.unlinkIncidentGoogleMeet(incident.value.incident_number);
      setIncident(updated);
      push("Google Meet unlinked", "success");
    } catch (e: unknown) {
      push(getErrorMessage(e, "Failed to unlink Google Meet"), "error");
    }
  }

  function unlinkChannel() {
    if (!incident.value) return;
    showUnlinkChannelConfirm.value = true;
  }

  async function confirmUnlinkChannel() {
    if (!incident.value) return;
    try {
      const updated = await api.deleteIncidentSlackChannel(incident.value.incident_number);
      setIncident(updated);
      push("Slack channel unlinked", "success");
    } catch (e: unknown) {
      push(getErrorMessage(e, "Failed to unlink Slack channel"), "error");
    }
  }

  function openEditDialog() {
    if (!incident.value) return;
    editTitle.value = incident.value.title;
    editDescription.value = incident.value.description;
    editSeverity.value = incident.value.severity;
    editImpact.value = incident.value.impact_level;
    editPriority.value = incident.value.priority;
    editError.value = "";
    editSubmitting.value = false;
    showEditDialog.value = true;
  }

  async function submitEdit() {
    if (!incident.value || editSubmitting.value) return;
    const title = editTitle.value.trim();
    if (!title) {
      editError.value = "Title is required.";
      return;
    }
    editSubmitting.value = true;
    editError.value = "";
    try {
      const body: Parameters<typeof api.patchIncident>[1] = { title };
      const desc = editDescription.value.trim();
      if (desc !== incident.value.description) body.description = desc;
      if (editSeverity.value && editSeverity.value !== incident.value.severity)
        body.severity = editSeverity.value as Severity;
      if (editImpact.value && editImpact.value !== incident.value.impact_level)
        body.impact_level = editImpact.value as ImpactLevel;
      if (editPriority.value && editPriority.value !== incident.value.priority)
        body.priority = editPriority.value as IncidentPriority;
      const updated = await api.patchIncident(incident.value.incident_number, body);
      setIncident(updated);
      showEditDialog.value = false;
      push("Incident updated", "success");
    } catch (err) {
      editError.value = getErrorMessage(err, "Failed to update incident");
    } finally {
      editSubmitting.value = false;
    }
  }

  function openLinkAlertDialog() {
    linkAlertNumber.value = "";
    linkAlertError.value = "";
    linkAlertSubmitting.value = false;
    showLinkAlertDialog.value = true;
  }

  async function submitLinkAlert() {
    if (!incident.value || linkAlertSubmitting.value) return;
    const num = parseInt(linkAlertNumber.value.trim(), 10);
    if (Number.isNaN(num) || num < 1) {
      linkAlertError.value = "Valid alert number is required.";
      return;
    }
    linkAlertSubmitting.value = true;
    linkAlertError.value = "";
    try {
      await api.linkAlertToIncident(incident.value.incident_number, num);
      showLinkAlertDialog.value = false;
      push("Alert linked", "success");
      await dataReload();
    } catch (err) {
      linkAlertError.value = getErrorMessage(err, "Failed to link alert");
    } finally {
      linkAlertSubmitting.value = false;
    }
  }

  async function confirmUnlinkAlert() {
    const target = unlinkAlertTarget.value;
    if (!target || !incident.value || unlinkAlertSubmitting.value) return;
    const num = target.alert_number;
    if (!num) {
      push("Cannot unlink alert without alert number", "error");
      unlinkAlertTarget.value = null;
      return;
    }
    unlinkAlertSubmitting.value = true;
    try {
      await api.unlinkAlertFromIncident(incident.value.incident_number, num);
      removeLinkedAlert(num);
      push("Alert unlinked", "success");
    } catch (err) {
      push(getErrorMessage(err, "Failed to unlink alert"), "error");
    } finally {
      unlinkAlertSubmitting.value = false;
      unlinkAlertTarget.value = null;
    }
  }

  function openAddTimelineDialog() {
    timelineMessage.value = "";
    timelineEventType.value = "manual";
    timelineError.value = "";
    timelineSubmitting.value = false;
    showAddTimelineDialog.value = true;
  }

  /**
   * Returns the created entry so the caller can hand it to
   * `useIncidentDetailData.addTimelineEntry` (which owns the
   * timeline reducer state).
   */
  async function submitTimelineEntry() {
    if (!incident.value || timelineSubmitting.value) return null;
    const msg = timelineMessage.value.trim();
    if (!msg) {
      timelineError.value = "Message is required.";
      return null;
    }
    timelineSubmitting.value = true;
    timelineError.value = "";
    try {
      const entry = await api.addIncidentTimelineEntry(incident.value.incident_number, {
        event_type: timelineEventType.value,
        message: msg,
      });
      showAddTimelineDialog.value = false;
      push("Timeline entry added", "success");
      return entry;
    } catch (err) {
      timelineError.value = getErrorMessage(err, "Failed to add timeline entry");
      return null;
    } finally {
      timelineSubmitting.value = false;
    }
  }

  async function probeIntegrations() {
    try {
      const integrations = await api.getIntegrations();
      isSlackConfigured.value = !!integrations?.slack?.provider_enabled;
      isGoogleMeetConfigured.value = !!integrations?.google_meet?.enabled;
    } catch {
      // best-effort: integrations probe is optional
    }
  }

  return reactive({
    showEditDialog,
    editTitle,
    editDescription,
    editSeverity,
    editImpact,
    editPriority,
    editSubmitting,
    editError,
    actionLoading,
    escalating,
    showLinkAlertDialog,
    linkAlertNumber,
    linkAlertSubmitting,
    linkAlertError,
    showUnlinkChannelConfirm,
    unlinkAlertTarget,
    unlinkAlertSubmitting,
    showAddTimelineDialog,
    timelineMessage,
    timelineEventType,
    timelineSubmitting,
    timelineError,
    isSlackConfigured,
    creatingChannel,
    isGoogleMeetConfigured,
    creatingMeet,
    acknowledge,
    mitigate,
    resolve,
    close,
    reopen,
    cancel,
    promoteToActive,
    escalateIncident,
    createChannel,
    createMeet,
    unlinkMeet,
    unlinkChannel,
    confirmUnlinkChannel,
    openEditDialog,
    submitEdit,
    openLinkAlertDialog,
    submitLinkAlert,
    confirmUnlinkAlert,
    openAddTimelineDialog,
    submitTimelineEntry,
    probeIntegrations,
  });
}
