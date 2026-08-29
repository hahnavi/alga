import { computed, ref, shallowRef, type Ref } from "vue";
import { api, type AgentTokenRow, type IncidentCoordinationMessage } from "@/lib/api";
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
 *   - the chat messages
 *   - the agent-typing indicator
 *   - the editor (text / kind / submitting)
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
  const statusUpdates = ref<IncidentCoordinationMessage[]>([]);
  const statusUpdatesLoading = ref(true);
  const statusUpdatesError = ref<string | null>(null);

  const coordinationText = ref("");
  const coordinationSubmitting = ref(false);
  const coordinationKind = ref<"chat" | "decision" | "action">("chat");

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
      const messages = await api.getIncidentCoordinationMessages(incidentNumber.value, {
        limit: 200,
      });
      // The API returns newest-first (spec 05 coordination R1); the chat view
      // renders chronological, so flip the page before storing. New messages
      // are appended at the end by submit and SSE reloads.
      coordinationMessages.value = [...messages].reverse();
    } catch (err) {
      coordinationMessages.value = [];
      push(getErrorMessage(err, "Failed to load coordination messages"), "error");
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
    statusUpdates.value = [];
    statusUpdatesError.value = null;
    statusUpdatesLoading.value = true;
    coordinationText.value = "";
    coordinationKind.value = "chat";
    clearCoordinationTyping();
  }

  return {
    coordinationMessages,
    statusUpdates,
    statusUpdatesLoading,
    statusUpdatesError,
    coordinationText,
    coordinationSubmitting,
    coordinationKind,
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
    fetchStatusUpdates,
    loadMentionTargets,
    submitCoordinationMessage,
    reset,
  };
}
