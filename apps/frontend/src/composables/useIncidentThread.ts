import { computed, reactive, shallowRef, type Ref } from "vue";
import { api, type OwnerThread, type OwnerThreadMessage } from "@/lib/api";
import { getErrorMessage } from "@/lib/error";
import { useToast } from "@/lib/toast";
import { MAX_THREAD_MESSAGES } from "@/lib/threadLimits";

type OwnerThreadKind = "incident_inv" | "incident_coord";

/**
 * Owns the incident "investigation" owner-thread state: the initial load,
 * the SSE reducer for live message upsert / edit / delete, and the
 * investigation lifecycle events that trigger a refresh. The coordination
 * typing branch of the `owner_thread_typing` events stays in the page
 * because it is shared with the coordination thread.
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

  function isRelevantOwnerThreadEvent(data: unknown, kind: OwnerThreadKind): boolean {
    const d = data as { owner_type?: string; owner_id?: string };
    return d.owner_type === kind && String(d.owner_id) === String(incidentNumber.value);
  }

  async function loadIncidentThread() {
    try {
      const fresh = await api.getIncidentThread(incidentNumber.value);
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

  const handlers = {
    investigation_created: () => {
      void loadIncidentThread();
      opts.scheduleReload();
    },
    investigation_started: () => {
      void loadIncidentThread();
      opts.scheduleReload();
    },
    investigation_update: () => {
      void loadIncidentThread();
      opts.scheduleReload();
    },
    investigation_status_changed: () => {
      void loadIncidentThread();
      opts.scheduleReload();
    },
    investigation_complete: () => {
      void loadIncidentThread();
      opts.scheduleReload();
    },
    investigation_patch: () => {
      void loadIncidentThread();
      opts.scheduleReload();
    },
    owner_thread_message: (data: unknown) => {
      if (!isRelevantOwnerThreadEvent(data, "incident_inv")) return;
      const d = data as { message?: OwnerThreadMessage };
      if (d.message) handleLiveThreadMessage(d.message);
      opts.scheduleReload();
    },
    owner_thread_message_edited: (data: unknown) => {
      if (!isRelevantOwnerThreadEvent(data, "incident_inv")) return;
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
      if (!isRelevantOwnerThreadEvent(data, "incident_inv")) return;
      const d = data as { message_id?: string };
      if (!d.message_id || !incidentThread.value) return;
      incidentThread.value = {
        ...incidentThread.value,
        messages: (incidentThread.value.messages ?? []).filter((m) => m.id !== d.message_id),
      };
    },
  };

  return reactive({
    incidentThread,
    incidentThreadMessageCount,
    loadIncidentThread,
    setThread,
    handlers,
  });
}
