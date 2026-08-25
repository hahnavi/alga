import { ref, type Ref } from "vue";
import { useRouter } from "vue-router";
import {
  api,
  type AlertRecord,
  type ICSRoleRecord,
  type IncidentRecord,
  type IncidentTimelineRecord,
  type PlaybookRecord,
} from "@/lib/api";
import { getErrorMessage } from "@/lib/error";
import { useToast } from "@/lib/toast";

/**
 * Owns the load pipeline for the incident detail page: the incident
 * row + its side-loads (timeline, alerts, ICS roles, mitigation
 * playbooks, post-mortem status).
 *
 * Returns the data as flat refs so the page can use them without
 * unwrapping an extra layer. The side-load functions are exposed
 * because the page also calls them in response to SSE events
 * (e.g. `ics_role_assigned` → `loadICSRoles`).
 *
 * `load()` runs the full pipeline (incident + side-loads). It
 * uses a seq counter so a stale fetch never lands after a newer
 * one (e.g. when the incident number changes mid-flight).
 */
export function useIncidentDetailData(
  incidentNumber: Ref<number>,
  options: { canCreatePostMortem?: Ref<boolean> } = {},
) {
  const { push } = useToast();
  const router = useRouter();

  const incident = ref<IncidentRecord | null>(null);
  const timeline = ref<IncidentTimelineRecord[]>([]);
  const alerts = ref<AlertRecord[]>([]);
  const icsRoles = ref<ICSRoleRecord[]>([]);
  const mitigationPlaybooks = ref<PlaybookRecord[]>([]);
  const postMortemStatus = ref<string | null>(null);
  const postMortemTitle = ref("");
  const postMortemOpening = ref(false);
  const loading = ref(true);
  const error = ref("");

  let loadSeq = 0;

  async function loadICSRoles() {
    try {
      icsRoles.value = await api.getICSRoles(incidentNumber.value);
    } catch (err) {
      icsRoles.value = [];
      push(getErrorMessage(err, "Failed to load ICS roles"), "error");
    }
  }

  async function loadAlerts() {
    try {
      alerts.value = await api.getIncidentAlerts(incidentNumber.value);
    } catch (err) {
      alerts.value = [];
      push(getErrorMessage(err, "Failed to load linked alerts"), "error");
    }
  }

  async function loadMitigationPlaybooks(inc?: IncidentRecord) {
    const target = inc ?? incident.value;
    if (!target?.service_id) {
      mitigationPlaybooks.value = [];
      return;
    }
    try {
      const result = await api.listPlaybooks({
        kind: "mitigation",
        service_id: target.service_id,
      });
      mitigationPlaybooks.value = result.items ?? [];
    } catch (err) {
      mitigationPlaybooks.value = [];
      push(getErrorMessage(err, "Failed to load mitigation playbooks"), "error");
    }
  }

  async function loadPostMortemStatus() {
    if (!incident.value) {
      postMortemStatus.value = null;
      postMortemTitle.value = "";
      return;
    }
    try {
      const pm = await api.getPostMortem(incident.value.incident_number);
      postMortemStatus.value = pm?.status ?? null;
      postMortemTitle.value = pm?.title ?? "";
    } catch (err) {
      postMortemStatus.value = null;
      postMortemTitle.value = "";
      push(getErrorMessage(err, "Failed to load post-mortem status"), "error");
    }
  }

  async function openPostMortem() {
    if (!incident.value || postMortemOpening.value) return;
    if (postMortemStatus.value) {
      router.push(`/incidents/${incident.value.incident_number}/post-mortem`);
      return;
    }
    if (options.canCreatePostMortem && !options.canCreatePostMortem.value) return;
    postMortemOpening.value = true;
    try {
      const pm = await api.createPostMortem(incident.value.incident_number, {});
      postMortemStatus.value = pm.status;
      push("Post-mortem created", "success");
      router.push(`/incidents/${incident.value.incident_number}/post-mortem`);
    } catch (err) {
      push(getErrorMessage(err, "Failed to create post-mortem"), "error");
    } finally {
      postMortemOpening.value = false;
    }
  }

  async function load() {
    const seq = ++loadSeq;
    loading.value = true;
    error.value = "";
    try {
      const data = await api.getIncident(incidentNumber.value);
      if (seq !== loadSeq) return;
      incident.value = data;
      timeline.value = data.timeline ?? [];
      // Side-loads are awaited only for the critical ones (alerts,
      // ICS roles) so a single failure doesn't block the main load;
      // playbooks and post-mortem status are fire-and-forget.
      await Promise.all([loadAlerts(), loadICSRoles()]);
      loadMitigationPlaybooks(data);
      loadPostMortemStatus();
    } catch (err) {
      if (seq !== loadSeq) return;
      const msg = getErrorMessage(err, "Failed to load incident");
      error.value = msg;
      push(msg, "error");
    } finally {
      if (seq === loadSeq) {
        loading.value = false;
      }
    }
  }

  function addTimelineEntry(entry: IncidentTimelineRecord) {
    timeline.value = [...timeline.value, entry];
  }

  function removeLinkedAlert(alertNumber: number) {
    alerts.value = alerts.value.filter((a) => a.alert_number !== alertNumber);
  }

  function setIncident(next: IncidentRecord) {
    incident.value = next;
  }

  function reset() {
    incident.value = null;
    timeline.value = [];
    alerts.value = [];
    icsRoles.value = [];
    mitigationPlaybooks.value = [];
    postMortemStatus.value = null;
    postMortemTitle.value = "";
    loadSeq++;
  }

  return {
    incident,
    timeline,
    alerts,
    icsRoles,
    mitigationPlaybooks,
    postMortemStatus,
    postMortemTitle,
    postMortemOpening,
    loading,
    error,
    load,
    loadICSRoles,
    loadAlerts,
    loadMitigationPlaybooks,
    loadPostMortemStatus,
    openPostMortem,
    addTimelineEntry,
    removeLinkedAlert,
    setIncident,
    reset,
  };
}
