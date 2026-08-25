import { computed, ref, shallowRef, type Ref } from "vue";
import {
  api,
  type AgentTokenRow,
  type CoordinationTask,
  type CoordinationTaskCreate,
  type IncidentCoordinationMessage,
} from "@/lib/api";
import { getErrorMessage } from "@/lib/error";
import { useToast } from "@/lib/toast";
import { useStickToBottom } from "@/composables/useStickToBottom";
import { useTypingIndicator } from "@/composables/useTypingIndicator";
import { useUsers } from "@/composables/useUsers";
import { getAgentAvatarSrc } from "@/lib/agentAvatar";
import { MAX_THREAD_MESSAGES } from "@/lib/threadLimits";

type ThreadParticipant = {
  key: string;
  name: string;
  avatarSrc?: string;
};

/**
 * Owns the coordination stream on the incident detail page:
 *   - the chat messages + dispatchable tasks
 *   - the agent-typing indicator
 *   - the editor (text / kind / submitting) and the dispatch-task
 *     dialog (goal / kind / role / submitting)
 *   - the status-update feed
 *   - the participants computed list
 *
 * The page owns the layout + thread element; the composable owns
 * the SSE-driven state machine (loaders + reducer-shaped setters).
 */
export function useIncidentCoordination(incidentNumber: Ref<number>) {
  const { push } = useToast();
  const { loadUsers } = useUsers();

  const coordinationMessages = shallowRef<IncidentCoordinationMessage[]>([]);
  const coordinationTasks = shallowRef<CoordinationTask[]>([]);
  const statusUpdates = ref<IncidentCoordinationMessage[]>([]);
  const statusUpdatesLoading = ref(true);
  const statusUpdatesError = ref<string | null>(null);

  const coordinationText = ref("");
  const coordinationSubmitting = ref(false);
  const coordinationKind = ref<"chat" | "decision" | "action">("chat");

  const showDispatchTaskDialog = ref(false);
  const dispatchTaskGoal = ref("");
  const dispatchTaskKind = ref<CoordinationTaskCreate["kind"]>("investigate");
  const dispatchTaskRole = ref<CoordinationTaskCreate["assignee_role"]>("responder");
  const dispatchTaskSubmitting = ref(false);

  const coordinationThreadEl = ref<HTMLElement | null>(null);
  const { stickToBottom: stickCoordinationToBottom, scrollToBottom: scrollCoordinationToBottom } =
    useStickToBottom(coordinationThreadEl);

  const {
    typingSource: coordinationTypingSource,
    typingAgentType: coordinationTypingAgentType,
    isTyping: coordinationTyping,
    setTyping: setCoordinationTyping,
    clearTyping: clearCoordinationTyping,
  } = useTypingIndicator({ timeoutMs: 6000 });

  const agents = ref<AgentTokenRow[]>([]);

  function coordinationDisplayName(message: IncidentCoordinationMessage): string {
    if (message.actor_display_name?.trim()) return message.actor_display_name.trim();
    if (message.actor_type === "agent") return "Agent";
    if (message.actor_type === "system") return "System";
    if (message.source === "slack") return "Slack";
    if (message.source === "mattermost") return "Mattermost";
    return "User";
  }

  function addParticipant(
    map: Map<string, ThreadParticipant>,
    key: string | undefined,
    fallbackKey: string,
    name: string,
    avatarSrc?: string,
  ) {
    const normalizedName = name.trim() || "User";
    const normalizedKey = (key?.trim() || normalizedName).toLowerCase();
    if (!map.has(normalizedKey)) {
      map.set(normalizedKey, { key: fallbackKey, name: normalizedName, avatarSrc });
    }
  }

  const coordinationParticipants = computed<ThreadParticipant[]>(() => {
    const map = new Map<string, ThreadParticipant>();
    for (const message of coordinationMessages.value) {
      if (message.actor_type === "system") continue;
      const isAgent = message.actor_type === "agent";
      const agentType =
        isAgent && typeof message.metadata?.agent_type === "string"
          ? message.metadata.agent_type
          : undefined;
      const avatar = isAgent ? getAgentAvatarSrc(agentType) : undefined;
      addParticipant(
        map,
        message.actor_id ?? coordinationDisplayName(message),
        message.id,
        coordinationDisplayName(message),
        avatar,
      );
    }
    return [...map.values()];
  });

  function participantLabel(participants: ThreadParticipant[], fallback: string): string {
    if (participants.length === 0) return fallback;
    if (participants.length === 1) return participants[0].name;
    const others = participants.length - 1;
    return `${participants[0].name} and ${others} ${others === 1 ? "other" : "others"}`;
  }

  const coordinationParticipantLabel = computed(() =>
    participantLabel(coordinationParticipants.value, "No participants yet"),
  );

  async function loadCoordinationMessages() {
    try {
      coordinationMessages.value = await api.getIncidentCoordinationMessages(incidentNumber.value, {
        limit: 200,
      });
    } catch (err) {
      coordinationMessages.value = [];
      push(getErrorMessage(err, "Failed to load coordination messages"), "error");
    }
  }

  async function loadCoordinationTasks() {
    try {
      coordinationTasks.value = await api.getIncidentCoordinationTasks(incidentNumber.value);
    } catch (err) {
      coordinationTasks.value = [];
      push(getErrorMessage(err, "Failed to load coordination tasks"), "error");
    }
  }

  async function fetchStatusUpdates(silent = false) {
    if (!silent) statusUpdatesLoading.value = true;
    statusUpdatesError.value = null;
    try {
      statusUpdates.value = await api.getIncidentStatusUpdates(incidentNumber.value);
    } catch (err: unknown) {
      const msg = getErrorMessage(err, "Failed to load status updates");
      statusUpdatesError.value = msg;
      push(msg, "error");
    } finally {
      statusUpdatesLoading.value = false;
    }
  }

  function openDispatchTaskDialog() {
    dispatchTaskGoal.value = "";
    dispatchTaskKind.value = "investigate";
    dispatchTaskRole.value = "responder";
    showDispatchTaskDialog.value = true;
  }

  async function submitDispatchTask() {
    const goal = dispatchTaskGoal.value.trim();
    if (!goal || dispatchTaskSubmitting.value) return;
    dispatchTaskSubmitting.value = true;
    try {
      await api.createIncidentCoordinationTask(incidentNumber.value, {
        kind: dispatchTaskKind.value,
        assignee_role: dispatchTaskRole.value,
        goal,
      });
      showDispatchTaskDialog.value = false;
      await loadCoordinationTasks();
      push("Coordination task dispatched", "success");
    } catch (err) {
      push(getErrorMessage(err, "Failed to dispatch task"), "error");
    } finally {
      dispatchTaskSubmitting.value = false;
    }
  }

  async function cancelCoordinationTask(task: CoordinationTask) {
    try {
      await api.patchIncidentCoordinationTask(incidentNumber.value, task.id, {
        status: "cancelled",
      });
      await loadCoordinationTasks();
      push("Coordination task cancelled", "success");
    } catch (err) {
      push(getErrorMessage(err, "Failed to cancel task"), "error");
    }
  }

  async function loadMentionTargets(editorValue: unknown) {
    void editorValue;
    try {
      agents.value = await api.getAgentTokens();
    } catch (err) {
      agents.value = [];
      push(getErrorMessage(err, "Failed to load agents for mentions"), "error");
    }
    await loadUsers();
  }

  function extractCoordinationMentions(editorValue: unknown): string[] {
    const e = editorValue as { getMentionIds?: () => string[] } | null;
    return e?.getMentionIds?.() ?? [];
  }

  async function submitCoordinationMessage(internal = false, editorValue: unknown = null) {
    const body = coordinationText.value.trim();
    if (!body || coordinationSubmitting.value) return;
    coordinationSubmitting.value = true;
    try {
      const created = await api.addIncidentCoordinationMessage(incidentNumber.value, {
        kind: coordinationKind.value,
        body,
        internal,
        mentions: extractCoordinationMentions(editorValue),
      });
      coordinationMessages.value = [...coordinationMessages.value, created].slice(
        -MAX_THREAD_MESSAGES,
      );
      coordinationText.value = "";
      coordinationKind.value = "chat";
      await scrollCoordinationToBottom();
      const e = editorValue as { focus?: () => void } | null;
      e?.focus?.();
    } catch (err) {
      push(getErrorMessage(err, "Failed to send message"), "error");
    } finally {
      coordinationSubmitting.value = false;
    }
  }

  function reset() {
    coordinationMessages.value = [];
    coordinationTasks.value = [];
    statusUpdates.value = [];
    statusUpdatesError.value = null;
    statusUpdatesLoading.value = true;
    coordinationText.value = "";
    coordinationKind.value = "chat";
    clearCoordinationTyping();
  }

  return {
    coordinationMessages,
    coordinationTasks,
    statusUpdates,
    statusUpdatesLoading,
    statusUpdatesError,
    coordinationText,
    coordinationSubmitting,
    coordinationKind,
    showDispatchTaskDialog,
    dispatchTaskGoal,
    dispatchTaskKind,
    dispatchTaskRole,
    dispatchTaskSubmitting,
    coordinationThreadEl,
    coordinationTyping,
    coordinationTypingSource,
    coordinationTypingAgentType,
    setCoordinationTyping,
    clearCoordinationTyping,
    agents,
    coordinationParticipants,
    coordinationParticipantLabel,
    stickCoordinationToBottom,
    scrollCoordinationToBottom,
    loadCoordinationMessages,
    loadCoordinationTasks,
    fetchStatusUpdates,
    openDispatchTaskDialog,
    submitDispatchTask,
    cancelCoordinationTask,
    loadMentionTargets,
    submitCoordinationMessage,
    reset,
  };
}
