import { computed, reactive, shallowRef, type Ref } from "vue";
import { api, type OwnerThread, type OwnerThreadMessage } from "@/lib/api";
import { getErrorMessage } from "@/lib/error";
import { useToast } from "@/lib/toast";
import { MAX_THREAD_MESSAGES } from "@/lib/threadLimits";
import { isIncidentInvestigationEvent, isOwnerThreadEvent } from "@/lib/incidentEvents";

/**
 * Owns the incident "investigation" owner-thread state: the initial load,
 * the SSE reducer for live message upsert / edit / delete, and the
 * investigation lifecycle events that trigger a refresh.
 *
 * Every SSE handler here is scoped: incident investigations carry
 * `incident_number` in the payload, while alert investigations carry
 * `alert_investigation_id` — those never belong to this thread and are
 * filtered out so unrelated alert activity doesn't reload the page.
 */
export function useIncidentThread(
  incidentNumber: Ref<number>,
  opts: {
    scheduleReload: () => void;
  },
) {
  const { push } = useToast();
  const incidentThread = shallowRef<OwnerThread | null>(null);
  const incidentThreadMessageCount = computed(() => incidentThread.value?.messages?.length ?? 0);

  let loadSeq = 0;

  async function loadIncidentThread() {
    const seq = ++loadSeq;
    try {
      const fresh = await api.getIncidentThread(incidentNumber.value);
      if (seq !== loadSeq) return;
      if (incidentThread.value && fresh) {
        const freshIds = new Set((fresh.messages ?? []).map((m) => m.id));
        const preserved = (incidentThread.value.messages ?? []).filter((m) => !freshIds.has(m.id));
        incidentThread.value = {
          ...fresh,
          messages: [...(fresh.messages ?? []), ...preserved],
        };
      } else {
        incidentThread.value = fresh;
      }
    } catch (err) {
      if (seq !== loadSeq) return;
      incidentThread.value = null;
      push(getErrorMessage(err, "Failed to load incident thread"), "error");
    }
  }

  function handleLiveThreadMessage(msg: OwnerThreadMessage) {
    if (!incidentThread.value) {
      void loadIncidentThread();
      return;
    }
    const messages = incidentThread.value.messages ?? [];
    if (messages.some((m) => m.id === msg.id)) return;
    incidentThread.value = {
      ...incidentThread.value,
      messages: [...messages, msg].slice(-MAX_THREAD_MESSAGES),
    };
  }

  function setThread(t: OwnerThread | null) {
    incidentThread.value = t;
  }

  /** Reloads the thread only when the event belongs to this incident. */
  function onIncidentInvestigationEvent(data: unknown) {
    if (!isIncidentInvestigationEvent(data, incidentNumber.value)) return;
    void loadIncidentThread();
    opts.scheduleReload();
  }

  const handlers = {
    // The incident-scoped investigation create/update family. The backend
    // publishes the record itself (`IncidentInvestigationRecord` with
    // `incident_number`), plus a slim map variant for `investigation_created`.
    investigation_created: onIncidentInvestigationEvent,
    investigation_updated: onIncidentInvestigationEvent,
    investigation_complete: onIncidentInvestigationEvent,
    investigation_status_changed: onIncidentInvestigationEvent,
    // Alert-investigation lifecycle: kept for the `owner_thread_message`
    // reducer below, which is separately scoped by owner_type/owner_id.
    owner_thread_message: (data: unknown) => {
      if (!isOwnerThreadEvent(data, "incident_inv", incidentNumber.value)) return;
      const d = data as { message?: OwnerThreadMessage };
      if (d.message) handleLiveThreadMessage(d.message);
    },
    owner_thread_message_edited: (data: unknown) => {
      if (!isOwnerThreadEvent(data, "incident_inv", incidentNumber.value)) return;
      const d = data as { message_id?: string; message?: string; edited?: boolean };
      if (!d.message_id || !incidentThread.value) return;
      const msgs = incidentThread.value.messages ?? [];
      const idx = msgs.findIndex((m) => m.id === d.message_id);
      if (idx >= 0 && typeof d.message === "string") {
        const updated = { ...msgs[idx], message: d.message, edited: d.edited ?? false };
        incidentThread.value = {
          ...incidentThread.value,
          messages: [...msgs.slice(0, idx), updated, ...msgs.slice(idx + 1)],
        };
      }
    },
    owner_thread_message_deleted: (data: unknown) => {
      if (!isOwnerThreadEvent(data, "incident_inv", incidentNumber.value)) return;
      const d = data as { message_id?: string };
      if (!d.message_id || !incidentThread.value) return;
      incidentThread.value = {
        ...incidentThread.value,
        messages: (incidentThread.value.messages ?? []).filter((m) => m.id !== d.message_id),
      };
    },
  };

  function reset() {
    loadSeq++;
    incidentThread.value = null;
  }

  return reactive({
    incidentThread,
    incidentThreadMessageCount,
    loadIncidentThread,
    setThread,
    reset,
    handlers,
  });
}
