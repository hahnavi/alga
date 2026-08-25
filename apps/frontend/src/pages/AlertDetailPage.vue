<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import {
  computed,
  h,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  toRef,
  watch,
  type CSSProperties,
} from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import {
  Bot,
  CheckCircle,
  ChevronRight,
  CircleAlert,
  CircleDot,
  Clock,
  Copy,
  FileText,
  Flame,
  Lightbulb,
  Link2,
  MessageSquare,
  HatGlasses,
  Reply,
  ShieldCheck,
  X,
  Zap,
} from "@lucide/vue";
import {
  api,
  ApiError,
  type AlertEvent,
  type AlertRecord,
  type DeliveryTarget,
  type InvestigationUpdate,
  type OwnerThreadMessage,
  type OwnerThreadMessageSource,
  type AgentTokenRow,
} from "@/lib/api";
import { alertRecordSchema, validate } from "@/lib/validation";
import {
  alertSeverityLabel,
  incidentPriorityBorderColor,
  nonHeaderLabelEntries,
  severityBadgeClass,
} from "@/lib/alertLabels";
import {
  formatSlackChannelLabel,
  mattermostPostPermalink,
  slackMessageAppRedirectUrl,
} from "@/lib/providerLinks";
import { getProviderIconSrc } from "@/lib/providerIcon";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import AlertStatusBadge from "@/components/ui/AlertStatusBadge.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import KeyValueDisplay from "@/components/ui/KeyValueDisplay.vue";
import MarkdownRenderer from "@/components/ui/MarkdownRenderer.vue";
import ChatMessageRow from "@/components/ui/ChatMessageRow.vue";
import ChatDateSeparator from "@/components/ui/ChatDateSeparator.vue";
import ChatEditorBar from "@/components/ui/ChatEditorBar.vue";
import ChatTypingIndicator from "@/components/ui/ChatTypingIndicator.vue";
import MarkdownEditor from "@/components/ui/MarkdownEditor.vue";
import MessageContextMenu, { type MessageAction } from "@/components/ui/MessageContextMenu.vue";
import TypingIndicator from "@/components/ui/TypingIndicator.vue";
import DeletedBadge from "@/components/ui/DeletedBadge.vue";
import { useToast } from "@/lib/toast";
import { usePageHeader } from "@/composables/usePageHeader";
import { createSearchActionButton } from "@/lib/pageHeader";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { useDelete } from "@/composables/useDelete";
import AlertActionsMenu from "@/components/ui/AlertActionsMenu.vue";
import AlertDetailsSidebar from "@/components/AlertDetailsSidebar.vue";
import { useAlertDetailData } from "@/composables/useAlertDetailData";
import { useAlertSidebarState } from "@/composables/useAlertSidebarState";
import { useLoadIntegrations } from "@/composables/useLoadIntegrations";
import { useUsers } from "@/composables/useUsers";
import { useChatSearch } from "@/composables/useChatSearch";
import { useChatThread } from "@/composables/useChatThread";
import { useSSE } from "@/composables/useSSE";
import { useTypingIndicator } from "@/composables/useTypingIndicator";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { useClipboard } from "@/composables/useClipboard";
import { getAgentAvatarSrc } from "@/lib/agentAvatar";
import { resolveDisplayName } from "@/lib/userDisplay";
import { formatTimeFull, formatDateSeparator, dateSeparatorKey } from "@/lib/time";
import { messagePermalink as chatMessagePermalink } from "@/lib/chatMessage";

defineOptions({ name: "AlertDetailPage" });

const isDeleted = computed(() => !!alert.value?.deleted_at);
const route = useRoute();
const router = useRouter();
const { push } = useToast();
const { canWrite: canWriteAlerts, canDelete: canDeleteAlerts } = useEntityPermissions("alerts");
const { canWrite: canWriteIncidents } = useEntityPermissions("incidents");

const alertNumber = computed(() => Number(route.params.alertNumber));

const {
  alert,
  investigation: alertInvestigation,
  relatedAlerts,
  relatedIncident,
  loading,
  error,
  load,
  loadRelated,
  silentReload,
} = useAlertDetailData(alertNumber);

const { users, loadUsers } = useUsers();

const investigationOutcome = computed(() => alertInvestigation.value?.summary);
const sidebarAssignee = computed<{ name: string; isAgent: boolean; agentType?: string } | null>(
  () => {
    const inv = alertInvestigation.value;
    if (!inv) return null;
    if (inv.assignee_type === "user") {
      const name = inv.assignee_id
        ? resolveDisplayName({ userId: inv.assignee_id, users: users.value, fallback: "" })
        : "";
      return name ? { name, isAgent: false } : null;
    }
    const name = inv.agent_name?.trim();
    if (!name) return null;
    return { name, isAgent: true, agentType: inv.agent_type };
  },
);
const hasInvestigationDetails = computed(() => {
  const inv = alertInvestigation.value;
  return Boolean(
    investigationOutcome.value ||
    (inv?.findings && inv.findings.length > 0) ||
    (inv?.evidence && inv.evidence.length > 0),
  );
});
const hasMeaningfulInvestigation = computed(() => {
  const inv = alertInvestigation.value;
  if (!inv) return false;
  return Boolean(inv.agent_name?.trim()) || hasInvestigationDetails.value;
});
const promotedIncidentID = computed(() => alertInvestigation.value?.promoted_incident_id ?? null);
const investigationPromoted = computed(() =>
  Boolean(promotedIncidentID.value || relatedIncident.value),
);
const promotedIncidentRoute = computed(() => {
  const num = relatedIncident.value?.incident_number;
  if (num) return `/incidents/${num}`;
  const id = promotedIncidentID.value;
  return id ? `/incidents/${id}` : null;
});

const {
  mainWidth,
  nudgeMainWidth,
  resizingMain,
  startMainResize,
  showThread: showAlertThread,
  threadLayoutOpen,
  threadLeaving,
  open: openAlertThread,
  close: closeAlertThread,
  toggle: toggleAlertThread,
  onAfterLeave: onThreadSidebarAfterLeave,
} = useAlertSidebarState();

const canWriteAlert = canWriteAlerts;
const canCreateIncident = computed(() => canWriteIncidents.value && !relatedIncident.value);

const { submitting: ackLoading, withSubmit: withAcknowledge } = useFormSubmit();
const { submitting: resolveLoading, withSubmit: withResolve } = useFormSubmit();
const { submitting: reopenLoading, withSubmit: withReopen } = useFormSubmit();
const { submitting: createIncidentLoading, withSubmit: withCreateIncident } = useFormSubmit();
const showCreateIncidentConfirm = ref(false);

const { showDeleteConfirm, confirmDelete, doDelete } = useDelete<AlertRecord>(async () => {
  await api.deleteAlert(alertNumber.value);
  router.push("/alerts");
}, "Alert");

const deliveryTargets = computed<DeliveryTarget[]>(() => {
  return alert.value?.delivery_targets ?? [];
});

type ResolvedDeliveryTarget = {
  channel: string;
  channelLabel: string;
  icon: string;
  href: string | null;
};

const resolvedDeliveryTargets = computed<ResolvedDeliveryTarget[]>(() =>
  deliveryTargets.value.map((dt) => ({
    channel: dt.channel,
    channelLabel: deliveryChannelLabel(dt),
    icon: deliveryIcon(dt),
    href: deliveryThreadHref(dt),
  })),
);

function openDeliveryThreadFromResolved(dt: ResolvedDeliveryTarget) {
  if (dt.href) window.open(dt.href, "_blank", "noopener,noreferrer");
}

const chatThreadEl = ref<HTMLDivElement | null>(null);
const chatDraft = ref("");
const chatReplyingTo = ref<OwnerThreadMessage | null>(null);
const chatSending = ref(false);

const agents = ref<AgentTokenRow[]>([]);
const editorRef = ref<InstanceType<typeof MarkdownEditor> | null>(null);
const { copyToClipboard } = useClipboard();

const chatContextMenu = ref<{
  message: OwnerThreadMessage;
  position: { clientX: number; clientY: number };
} | null>(null);

function onChatMessageContextMenu(payload: { id: string; clientX: number; clientY: number }) {
  const message = (thread.messages.value ?? []).find((m) => m.id === payload.id);
  if (!message) return;
  chatContextMenu.value = {
    message,
    position: { clientX: payload.clientX, clientY: payload.clientY },
  };
}

function closeChatContextMenu() {
  chatContextMenu.value = null;
}

const canShowChatWriteActions = computed(() => canWriteAlerts.value && !isDeleted.value);

const chatContextMenuActions = computed<MessageAction[]>(() => {
  const open = chatContextMenu.value;
  if (!open) return [];
  const actions: MessageAction[] = [];
  if (canShowChatWriteActions.value) {
    actions.push({
      key: "reply",
      label: "Reply",
      icon: Reply,
      onSelect: () => startChatReply(open.message),
    });
  }
  actions.push({
    key: "copy-text",
    label: "Copy text",
    icon: Copy,
    onSelect: () => copyToClipboard(open.message.message, "Message copied"),
  });
  actions.push({
    key: "copy-link",
    label: "Copy link",
    icon: Link2,
    onSelect: () => copyToClipboard(chatMessagePermalink(open.message.id), "Link copied"),
  });
  return actions;
});

function startChatReply(msg: OwnerThreadMessage) {
  chatReplyingTo.value = msg;
  nextTick(() => editorRef.value?.focus());
}

function cancelChatReply() {
  chatReplyingTo.value = null;
}

function chatReplyContextFor(message: OwnerThreadMessage): {
  replyToText: string;
  replyToAuthor: string;
} {
  const qid = message.reply_to_message_id;
  if (!qid) return { replyToText: "", replyToAuthor: "" };
  const found = (thread.messages.value ?? []).find((m) => m.id === qid);
  if (!found) return { replyToText: "", replyToAuthor: "" };
  return { replyToText: found.message, replyToAuthor: chatDisplayName(found) };
}

async function loadMentionTargets() {
  try {
    agents.value = await api.getAgentTokens();
  } catch {
    agents.value = [];
  }
  await loadUsers();
}

function extractMentions(): string[] {
  return editorRef.value?.getMentionIds() ?? [];
}

const {
  isTyping: agentTyping,
  setTyping: setAgentTyping,
  clearTyping: clearAgentTyping,
} = useTypingIndicator({ timeoutMs: 6000 });

// Hybrid chat: owner_thread_* events (matched by alertNumber) and
// investigation_update / investigation_draft / investigation_typing
// events (matched by alertInvestigationId) all land in the same
// messages / drafts arrays. The useChatThread composable owns the
// reducer + SSE event sources; this page just provides the
// scope-specific extractors.
const thread = useChatThread<OwnerThreadMessage>({
  scope: "alert",
  targetId: toRef(() => String(alertNumber.value)),
  fetchThread: async (id) => {
    try {
      const fresh = await api.getAlertThread(Number(id));
      return fresh.messages ?? [];
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) return [];
      throw err;
    }
  },
  extractMessage: (data) => (data as { message?: OwnerThreadMessage }).message,
  extractEdit: (data) => {
    const d = data as { message_id?: string; message?: string; edited?: boolean };
    if (!d.message_id || typeof d.message !== "string") return null;
    return { id: d.message_id, message: d.message, edited: d.edited };
  },
  extractDelete: (data) => {
    const d = data as { message_id?: string };
    return d.message_id ? { id: d.message_id } : null;
  },
  extractDraft: (data) => {
    const d = data as {
      draft_id?: string;
      message?: string;
      source?: OwnerThreadMessageSource;
    };
    if (!d.draft_id) return null;
    return { draft_id: d.draft_id, message: d.message ?? "", source: d.source };
  },
  extractTyping: (data) => {
    const d = data as { source?: string };
    return { source: d.source ?? "agent" };
  },
  buildDraft: (draftId, message, source, now) => ({
    id: `draft-${draftId}`,
    draft_id: draftId,
    draft: true,
    type: "comment" as const,
    source: source as OwnerThreadMessageSource,
    message,
    username: alertInvestigation.value?.agent_name ?? undefined,
    created_at: now,
    updated_at: now,
  }),
  isAgentMessage: (msg) => msg.source === "agent",
  onTyping: (source) => {
    setAgentTyping(source);
    void nextTick().then(scrollChatToBottom);
  },
  onTypingStop: () => clearAgentTyping(),
  additionalSources: [
    {
      matchEvent: (data) => {
        const d = data as { alert_investigation_id?: string } | null;
        return (
          !!d &&
          !!d.alert_investigation_id &&
          d.alert_investigation_id === alertInvestigation.value?.alert_investigation_id
        );
      },
      events: {
        message: "investigation_update",
        edited: "investigation_update_edited",
        deleted: "investigation_update_deleted",
        typing: "investigation_typing",
        typingStop: "investigation_typing_stop",
        draft: "investigation_draft",
      },
      extractMessage: (data) => {
        const d = data as { update?: InvestigationUpdate };
        if (!d.update) return undefined;
        const u = d.update;
        if (u.internal) return undefined;
        return {
          id: u.id,
          type: u.type === "comment" ? "comment" : "progress",
          source: u.source as OwnerThreadMessageSource,
          message: u.message,
          edited: u.edited,
          user_id: u.user_id,
          username: u.username ?? undefined,
          created_at: u.created_at,
          updated_at: u.created_at,
        };
      },
      extractEdit: (data) => {
        const d = data as { update_id?: string; update?: InvestigationUpdate };
        if (!d.update_id || !d.update) return null;
        return {
          id: d.update_id,
          message: d.update.message,
          edited: d.update.edited ?? true,
        };
      },
      extractDelete: (data) => {
        const d = data as { update_id?: string };
        return d.update_id ? { id: d.update_id } : null;
      },
      extractDraft: (data) => {
        const d = data as {
          draft_id?: string;
          message?: string;
          source?: OwnerThreadMessageSource;
        };
        if (!d.draft_id) return null;
        return { draft_id: d.draft_id, message: d.message ?? "", source: d.source };
      },
      extractTyping: (data) => {
        const d = data as { source?: string };
        return { source: d.source ?? "agent" };
      },
    },
  ],
});

const chatMessages = thread.messages;
const agentDrafts = thread.drafts;

const {
  query: searchQuery,
  hasQuery: searchHasQuery,
  searchHighlight,
  openSearch: searchOpen,
} = useChatSearch(
  chatMessages,
  (m: OwnerThreadMessage) => m.id,
  (m: OwnerThreadMessage) => m.message,
);

const totalChatCount = computed(() => chatMessages.value.length);

type ChatParticipant = {
  key: string;
  name: string;
  avatarSrc?: string;
};

const chatParticipants = computed<ChatParticipant[]>(() => {
  const map = new Map<string, ChatParticipant>();
  for (const message of chatMessages.value) {
    if (message.source === "system") continue;
    const isAgent = message.source === "agent";
    const name = chatDisplayName(message);
    const rawKey = (message.user_id ?? message.username ?? name).trim();
    const key = rawKey.toLowerCase() || name.toLowerCase();
    if (!map.has(key)) {
      map.set(key, {
        key,
        name,
        avatarSrc: isAgent ? chatAvatarSrc(message) : undefined,
      });
    }
  }
  return [...map.values()];
});

const chatParticipantLabel = computed(() => {
  const participants = chatParticipants.value;
  if (participants.length > 0) {
    if (participants.length === 1) return participants[0].name;
    const others = participants.length - 1;
    return `${participants[0].name} and ${others} ${others === 1 ? "other" : "others"}`;
  }
  const inv = alertInvestigation.value;
  if (!inv) return "No investigation";
  const name = inv.agent_name?.trim();
  if (name) return name;
  if (!hasInvestigationDetails.value) return "No investigation";
  const status = inv.status.charAt(0).toUpperCase() + inv.status.slice(1);
  return `Investigation ${status}`;
});

type ChatThreadItem =
  | { kind: "date"; key: string; label: string }
  | { kind: "msg"; message: OwnerThreadMessage }
  | { kind: "draft"; message: OwnerThreadMessage & { draft_id: string; draft: true } };

const chatThreadItems = computed((): ChatThreadItem[] => {
  const items: ChatThreadItem[] = [];
  let lastDate = "";
  const addItem = (m: OwnerThreadMessage, kind: "msg" | "draft") => {
    const dateStr = dateSeparatorKey(m.created_at);
    if (dateStr !== lastDate) {
      items.push({
        kind: "date",
        key: `sep-${dateStr}`,
        label: formatDateSeparator(m.created_at),
      });
      lastDate = dateStr;
    }
    if (kind === "draft") {
      items.push({
        kind,
        message: m as OwnerThreadMessage & { draft_id: string; draft: true },
      });
    } else {
      items.push({ kind, message: m });
    }
  };
  for (const m of chatMessages.value) {
    addItem(m, "msg");
  }
  for (const m of agentDrafts.value) {
    addItem(m, "draft");
  }
  return items;
});

const pageShellClass = computed(() => {
  if (showAlertThread.value || threadLeaving.value) {
    return "flex min-h-full flex-col gap-0 px-4 md:px-6 lg:grid lg:min-h-full lg:grid-cols-[clamp(480px,calc(var(--detail-main-width)_-_80px),calc(100vw-48px-24px-360px))_minmax(360px,1fr)] lg:items-start lg:gap-6 lg:[transition:--detail-main-width_220ms_cubic-bezier(0.16,1,0.3,1)]";
  }
  if (threadLayoutOpen.value) {
    return "flex min-h-full flex-col gap-0 px-4 md:px-6 lg:min-h-full";
  }
  return "flex h-full flex-col gap-0 px-4 md:px-6 lg:h-full";
});

const pageContentClass = computed(() => {
  if (showAlertThread.value || threadLeaving.value) {
    return "relative flex min-w-0 flex-col";
  }
  if (threadLayoutOpen.value) {
    return "relative flex min-w-0 flex-1 flex-col";
  }
  return "relative flex min-w-0 flex-1 flex-col lg:grid lg:grid-cols-[clamp(480px,var(--detail-main-width),calc(100%_-_24px_-_320px))_minmax(320px,1fr)] lg:items-start lg:gap-6";
});

const shellAsideStyle = computed<CSSProperties>(() => {
  const drawerVisible = showAlertThread.value || threadLeaving.value || threadLayoutOpen.value;
  return {
    "--detail-main-width": `${mainWidth.value}px`,
    "--shell-aside-width": drawerVisible ? "28rem" : "24rem",
  };
});

const mainSectionClass = computed(() =>
  threadLayoutOpen.value ? "flex min-w-0 flex-col" : "relative flex min-w-0 flex-1 flex-col",
);

const alertContentClass = computed(() =>
  showAlertThread.value || threadLeaving.value
    ? "min-w-0 space-y-4 py-4 pr-0 lg:pt-6 lg:pb-2"
    : threadLayoutOpen.value
      ? "min-w-0 space-y-4 py-4 pr-0 lg:py-6"
      : "main-scroll min-w-0 flex-1 space-y-4 py-4 pr-0 lg:py-6",
);

const defaultSidebarClass = computed(() =>
  showAlertThread.value || threadLeaving.value
    ? "min-w-0 space-y-4 pt-4 pb-20 md:pb-4 lg:overflow-y-auto lg:pt-2 lg:pb-6"
    : threadLayoutOpen.value
      ? "min-w-0 space-y-4 pt-4 pb-20 md:pb-4 lg:overflow-y-auto lg:py-6"
      : "min-w-0 space-y-4 pt-4 pb-20 md:pb-4 lg:overflow-y-auto lg:py-6",
);

// Sidebar state (openAlertThread / closeAlertThread / toggleAlertThread /
// onThreadSidebarAfterLeave / Escape binding) is owned by useAlertSidebarState.
// The same applies to silentReload and loadRelated, owned by useAlertDetailData.

const { mattermostBaseUrl, mattermostTeam, load: loadIntegrations } = useLoadIntegrations();

let reloadTimer: number | null = null;
let typingPostTimer: number | null = null;

function scheduleReload() {
  if (reloadTimer != null) return;
  reloadTimer = setTimeout(() => {
    reloadTimer = null;
    void Promise.all([silentReload(), loadRelated(), thread.reload()]);
  }, 1500);
}

function scheduleTypingNotify() {
  if (typingPostTimer) clearTimeout(typingPostTimer);
  typingPostTimer = setTimeout(() => {
    typingPostTimer = null;
    void api.postAlertThreadTyping(alertNumber.value).catch(() => {
      /* intentional: typing indicator is best-effort */
    });
  }, 600);
}

function handleAlertSSE(data: unknown) {
  // `data` is the wire payload. Validate shape with the alert schema so a
  // malformed event (e.g. partial push from the server) is dropped instead
  // of causing a needless reload. The zod-inferred type is slightly looser
  // than `AlertRecord` (e.g. `agent_type` is a free-form string); the cast
  // narrows the runtime-validated value to the consumer's expected type.
  let rec: AlertRecord;
  try {
    rec = validate(alertRecordSchema, data) as AlertRecord;
  } catch {
    return;
  }
  if (rec.alert_number != null && rec.alert_number !== alertNumber.value) return;
  scheduleReload();
}

function scrollChatToBottom() {
  const el = chatThreadEl.value;
  if (!el) return;
  requestAnimationFrame(() => {
    el.scrollTop = el.scrollHeight;
  });
}

// useChatThread owns the chat reducer + all chat-related SSE event
// sources (owner_thread_* matched by alertNumber, and
// investigation_update / investigation_draft / investigation_typing
// events matched by alertInvestigationId). The remaining SSE handlers
// here are pure reload triggers for the alert row + sidebar.
const sse = useSSE("/api/v1/events", {
  alert_updated: handleAlertSSE,
  investigation_created: () => scheduleReload(),
  investigation_started: () => scheduleReload(),
  investigation_status_changed: () => scheduleReload(),
  investigation_complete: () => scheduleReload(),
  investigation_patch: () => scheduleReload(),
});
const sseState = sse.state;

async function acknowledge() {
  if (!alert.value) return;
  await withAcknowledge(async () => {
    alert.value = await api.acknowledgeAlert(alertNumber.value);
  }, "Alert acknowledged");
}

async function resolveAlert() {
  if (!alert.value) return;
  await withResolve(async () => {
    alert.value = await api.resolveAlert(alertNumber.value);
    if (alertInvestigation.value) {
      try {
        await api.addAlertThreadMessage(alertNumber.value, { message: "/stop" });
      } catch {
        // best-effort
      }
    }
  }, "Alert marked resolved");
}

async function reopenAlert() {
  if (!alert.value) return;
  await withReopen(async () => {
    alert.value = await api.reopenAlert(alertNumber.value);
  }, "Alert reopened");
}

const { submitting: investigateLoading, withSubmit: withInvestigate } = useFormSubmit();

async function triggerInvestigation() {
  if (!alert.value) return;
  await withInvestigate(async () => {
    const data = await api.investigateAlert(alertNumber.value);
    alert.value = data.alert;
    alertInvestigation.value = data.alert_investigation ?? null;
    await thread.reload();
    openAlertThread();
  }, "Investigation started");
}

async function handleAssignInvestigation(assigneeType: "user" | "agent", assigneeId?: string) {
  const inv = alertInvestigation.value;
  if (!inv) return;
  try {
    const updated = await api.assignAlertInvestigation(
      inv.alert_investigation_id,
      assigneeType,
      assigneeId,
    );
    alertInvestigation.value = updated;
    push(assigneeType === "user" ? "Assigned to user" : "Reassigned to agent pool", "success");
  } catch (err) {
    push(getErrorMessage(err, "Failed to assign investigation"), "error");
  }
}

function mapAlertSeverityToIncident(sev: string | null): "critical" | "high" | "warning" | "info" {
  if (!sev) return "warning";
  const l = sev.toLowerCase();
  if (l.includes("critical") || l.includes("fatal")) return "critical";
  if (l.includes("high") || l.includes("error") || l.includes("urgent")) return "high";
  if (l.includes("warn") || l.includes("medium")) return "warning";
  if (l.includes("info") || l.includes("low")) return "info";
  return "warning";
}

async function createIncidentFromAlert() {
  if (!alert.value || createIncidentLoading.value) return;
  showCreateIncidentConfirm.value = false;
  await withCreateIncident(async () => {
    const labels = alert.value!.labels ?? {};
    const annotations = alert.value!.annotations ?? {};
    const title = (labels.alertname ?? `Alert #${alertNumber.value}`).trim();
    const description =
      investigationOutcome.value?.summary?.trim() ||
      annotations.summary?.trim() ||
      annotations.description?.trim() ||
      undefined;
    const severity = mapAlertSeverityToIncident(alertSeverityLabel(labels));
    const created = await api.createIncident({
      title,
      description,
      severity,
      alert_numbers: [alertNumber.value],
    });
    relatedIncident.value = {
      incident_number: created.incident_number,
      title: created.title,
      status: created.status,
      severity: created.severity,
      priority: created.priority,
    };
  }, "Incident created");
}

function requestCreateIncidentFromAlert() {
  if (!alert.value || createIncidentLoading.value) return;
  showCreateIncidentConfirm.value = true;
}

async function sendChatMessage() {
  const text = chatDraft.value.trim();
  if (!text || chatSending.value) return;
  chatSending.value = true;
  try {
    const result = await api.addAlertThreadMessage(alertNumber.value, {
      message: text,
      type: "comment",
      mentions: extractMentions(),
      reply_to_message_id: chatReplyingTo.value?.id,
    });
    chatDraft.value = "";
    chatReplyingTo.value = null;
    // The server returns the full thread; sync our reducer state to it
    // without going through SSE (which would race the addMessage dedup).
    if (result?.messages) {
      thread.messages.value = result.messages;
    }
    await nextTick();
    scrollChatToBottom();
    editorRef.value?.focus();
  } catch (err) {
    push(getErrorMessage(err, "Failed to send message"), "error");
  } finally {
    chatSending.value = false;
  }
}

function mattermostLink(postId: string): string {
  return mattermostPostPermalink(mattermostBaseUrl.value, postId, mattermostTeam.value);
}

function deliveryThreadHref(dt: DeliveryTarget): string | null {
  if (!dt.post_id) return null;
  const p = dt.provider.toLowerCase();
  if (p === "mattermost" && mattermostBaseUrl.value) {
    return mattermostLink(dt.post_id);
  }
  if (p === "slack") {
    return slackMessageAppRedirectUrl(dt.channel, dt.post_id);
  }
  return null;
}

function deliveryChannelLabel(dt: DeliveryTarget): string {
  if (dt.channel_name) {
    return dt.channel_name;
  }
  if (dt.provider.toLowerCase() === "slack") {
    return formatSlackChannelLabel(dt.channel);
  }
  return dt.channel;
}

function deliveryIcon(dt: DeliveryTarget): string {
  return getProviderIconSrc(dt.provider);
}

type TimelineEntry = {
  type: string;
  timestamp: string;
  label: string;
  subline: string;
  dotClass: string;
  lineClass: string;
  iconClass: string;
  icon: typeof Flame;
};

function eventIcon(t: string): typeof Flame {
  if (t === "resolved" || t === "reopened") return CheckCircle;
  if (t === "acked") return ShieldCheck;
  return Flame;
}

function eventColors(t: string): { dot: string; line: string; icon: string } {
  if (t === "resolved")
    return { dot: "bg-[var(--text-success)]", line: "", icon: "text-[var(--text-success)]" };
  if (t === "acked")
    return {
      dot: "bg-[var(--accent)]",
      line: "border-[var(--accent)]",
      icon: "text-[var(--accent)]",
    };
  if (t === "reopened" || t === "refired")
    return {
      dot: "bg-[var(--text-badge-orange)]",
      line: "border-[var(--text-badge-orange)]",
      icon: "text-[var(--text-badge-orange)]",
    };
  return {
    dot: "bg-[var(--text-badge-firing)]",
    line: "border-[var(--text-badge-firing)]",
    icon: "text-[var(--text-muted)]",
  };
}

function primaryEventLabel(ev: AlertEvent): string {
  switch (ev.type) {
    case "fired":
      return "Fired";
    case "acked":
      return "Acknowledged";
    case "resolved":
      return "Resolved";
    case "refired":
      return "Firing again";
    case "reopened":
      return "Reopened";
    default:
      return ev.type;
  }
}

function eventActorLabel(ev: AlertEvent): string | undefined {
  const name = ev.actor_display_name?.trim();
  if (name) return name;
  const u = ev.actor_username?.trim();
  return u || undefined;
}

function eventSubline(ev: AlertEvent): string {
  const who = eventActorLabel(ev);
  if (who) {
    if (ev.type === "acked") return `by ${who}`;
    if (
      ev.type === "resolved" &&
      (ev.source === "user" || ev.source === "agent" || ev.source === "incident_cascade")
    )
      return `by ${who}`;
    if (ev.type === "reopened") return `by ${who}`;
    if (ev.type === "fired" && (ev.source === "user" || ev.source === "agent")) return `by ${who}`;
  }
  if (ev.type === "acked") {
    if (ev.source === "user") return "by Console user";
  }
  if (ev.type === "resolved") {
    if (ev.source === "grafana") return "by Grafana";
    if (ev.source === "system") return "by System / silenced";
    if (ev.source === "user" && !who) return "by Console user";
    if (ev.source === "incident_cascade") return "by Incident resolution";
  }
  if (ev.type === "refired") return "by Grafana";
  if (ev.type === "fired") {
    if (ev.source === "grafana") return "by Grafana";
    if (ev.source === "user" && !who) return "by Console user";
  }
  return "";
}

const timeline = computed<TimelineEntry[]>(() => {
  if (!alert.value) return [];

  const entries: TimelineEntry[] = [];
  const a = alert.value;
  const events = a.events ?? [];

  for (const raw of events) {
    const ev = raw.type === "acknowledged" ? { ...raw, type: "acked" } : raw;
    const { dot, line, icon } = eventColors(ev.type);
    entries.push({
      type: ev.type,
      timestamp: ev.timestamp,
      label: primaryEventLabel(ev),
      subline: eventSubline(ev),
      dotClass: dot,
      lineClass: line,
      iconClass: icon,
      icon: eventIcon(ev.type),
    });
  }

  entries.sort((x, y) => new Date(y.timestamp).getTime() - new Date(x.timestamp).getTime());
  return entries;
});

const showAckButton = computed(
  () => alert.value?.status === "firing" && !alert.value?.acknowledged,
);

const ANNOTATION_CARD_KEYS = new Set(["summary", "description", "runbook_url"]);

const annotationSummaryText = computed(() => alert.value?.annotations?.summary?.trim() ?? "");

const annotationDescriptionText = computed(
  () => alert.value?.annotations?.description?.trim() ?? "",
);

const runbookHref = computed(() => {
  const raw = alert.value?.annotations?.runbook_url?.trim();
  if (!raw) return null;
  if (/^https?:\/\//i.test(raw)) return raw;
  try {
    const u = new URL(raw);
    return u.protocol === "http:" || u.protocol === "https:" ? u.href : null;
  } catch {
    return null;
  }
});

const otherAnnotations = computed(() => {
  const ann = alert.value?.annotations;
  if (!ann) return [] as { key: string; value: string }[];
  return Object.entries(ann)
    .filter(([k]) => !ANNOTATION_CARD_KEYS.has(k))
    .map(([key, value]) => ({ key, value: String(value) }));
});

const otherAnnotationsMap = computed(() => {
  const map: Record<string, string> = {};
  for (const a of otherAnnotations.value) {
    map[a.key] = a.value;
  }
  return map;
});

const alertValuesMap = computed(() => {
  const map: Record<string, string> = {};
  const vals = alert.value?.values;
  if (vals) {
    for (const [k, v] of Object.entries(vals)) {
      map[k] = String(v);
    }
  }
  return map;
});

const showAnnotationsCard = computed(
  () =>
    Boolean(annotationSummaryText.value) ||
    Boolean(annotationDescriptionText.value) ||
    otherAnnotations.value.length > 0,
);

const labelsShownBelowHeader = computed(() => {
  return nonHeaderLabelEntries(alert.value?.labels);
});

const workflowStatus = computed(() => (alert.value?.status === "resolved" ? "resolved" : "open"));

const statusWorkflowBusy = computed(() => resolveLoading.value || reopenLoading.value);

const statusBadgeLabel = computed(() =>
  workflowStatus.value === "resolved" ? "RESOLVED" : "OPEN",
);

const statusBadgeStyleClass = computed(() =>
  workflowStatus.value === "resolved" ? "badge-green" : "badge-red",
);

const severityLabel = computed(() => alertSeverityLabel(alert.value?.labels));

const severityFilledBadgeCss = computed(() => {
  switch (severityLabel.value?.toLowerCase()) {
    case "critical":
      return "rounded bg-red-500 px-2 py-0.5 text-xs font-semibold text-white";
    case "warning":
      return "rounded bg-amber-500 px-2 py-0.5 text-xs font-semibold text-white";
    case "info":
      return "rounded bg-sky-500 px-2 py-0.5 text-xs font-semibold text-white";
    default:
      return "rounded bg-[var(--bg-tertiary)] px-2 py-0.5 text-xs font-semibold text-[var(--text-primary)]";
  }
});

const alertStartedAtText = computed(() =>
  alert.value ? formatTimeFull(alert.value.starts_at) : "",
);

const severityDisplayText = computed(() => severityLabel.value?.toUpperCase() ?? "");

const deleteConfirmMessage = computed(() => {
  const a = alert.value;
  if (!a) {
    return "Are you sure you want to delete this alert? This action cannot be undone.";
  }
  const raw = (a.labels?.alertname ?? "this alert").trim() || "this alert";
  const name = raw.replace(/\p{Cc}/gu, "");
  const id = a.alert_number != null && a.alert_number > 0 ? ` (#${a.alert_number})` : "";
  return `Are you sure you want to delete '${name}'${id}? This action cannot be undone.`;
});

const createIncidentConfirmMessage = computed(() => {
  const raw = (alert.value?.labels?.alertname ?? `Alert #${alertNumber.value}`).trim();
  const name = (raw || `Alert #${alertNumber.value}`).replace(/\p{Cc}/gu, "");
  return `Create an incident from '${name}'? This will link the alert, start incident automation, and queue agent investigation work.`;
});

function onWorkflowStatusChange(next: string) {
  if (!alert.value || next === workflowStatus.value) return;
  if (next === "resolved") {
    void resolveAlert();
  } else if (next === "open") {
    void reopenAlert();
  }
}

function onDeleteFromHeader() {
  if (alert.value) confirmDelete(alert.value);
}

// Indicator chip class per chat source. Rendered as a non-rail dot in
// ChatMessageRow's meta line — see lib/chatMessage.ts for the shared helper.
function chatSourceIndicatorClass(source: string): string {
  switch (source) {
    case "agent":
      return "bg-purple-500/15 text-purple-700 dark:text-purple-300";
    case "system":
      return "bg-blue-500/15 text-blue-700 dark:text-blue-300";
    case "mattermost":
      return "bg-indigo-500/15 text-indigo-700 dark:text-indigo-300";
    case "slack":
      return "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300";
    default:
      return "bg-[var(--bg-online)]";
  }
}

function chatSourceAvatarBg(source: string): string {
  switch (source) {
    case "agent":
      return "bg-transparent";
    case "system":
      return "bg-blue-100 dark:bg-blue-900/30";
    default:
      return "bg-[var(--bg-secondary)]";
  }
}

function chatAvatarSrc(msg: OwnerThreadMessage): string | undefined {
  if (msg.source === "agent") {
    return getAgentAvatarSrc(alertInvestigation.value?.agent_type);
  }
  return undefined;
}

function chatAvatarLetter(msg: OwnerThreadMessage): string {
  return chatDisplayName(msg).charAt(0).toUpperCase();
}

function chatDisplayName(msg: OwnerThreadMessage): string {
  if (msg.username) return msg.username;
  if (msg.source === "agent") return alertInvestigation.value?.agent_name ?? "Agent";
  if (msg.source === "system") return "System";
  return "User";
}

onBeforeUnmount(() => {
  clearAgentTyping();
  if (reloadTimer != null) {
    clearTimeout(reloadTimer);
    reloadTimer = null;
  }
  if (typingPostTimer != null) {
    clearTimeout(typingPostTimer);
    typingPostTimer = null;
  }
  thread.clearAll();
});

// Navigating between alert detail routes (same component, different
// :alertNumber param) reuses this instance — KeepAlive + RouterView
// skip onMounted. Reset stale state, null the entity so the skeleton
// shows immediately, and reload for the new alert. Guard against NaN:
// when navigating away from a detail route the param disappears and
// Number(undefined) is NaN — KeepAlive keeps this watcher alive while
// deactivated, so it would otherwise fire a /alerts/NaN request.
function resetAlertState() {
  alert.value = null;
  alertInvestigation.value = null;
  relatedAlerts.value = [];
  relatedIncident.value = null;
  thread.clearAll();
  chatReplyingTo.value = null;
}

watch(alertNumber, (next, prev) => {
  if (!Number.isFinite(next)) return;
  if (prev !== undefined && next !== prev) {
    resetAlertState();
    void load();
    void thread.reload();
  }
});

watch(chatDraft, () => {
  if (chatDraft.value.trim()) scheduleTypingNotify();
});

usePageHeader(() => {
  const a = alert.value;
  if (!a) return null;
  const raw = (a.labels?.alertname ?? "Alert").trim() || "Alert";
  const name = raw.replace(/\p{Cc}/gu, "");
  const idPrefix = a.alert_number != null && a.alert_number > 0 ? `#${a.alert_number}` : undefined;
  const actions: ReturnType<typeof h>[] = [];
  if (showAlertThread.value || threadLayoutOpen.value) {
    actions.push(createSearchActionButton(() => searchOpen()));
  }
  if (
    !isDeleted.value &&
    (!showAckButton.value ||
      canWriteAlerts.value ||
      canDeleteAlerts.value ||
      canCreateIncident.value)
  ) {
    actions.push(
      h(AlertActionsMenu, {
        workflowStatus: workflowStatus.value,
        statusBusy: statusWorkflowBusy.value,
        canDelete: canDeleteAlerts.value,
        canCreateIncident: canCreateIncident.value,
        showAckButton: showAckButton.value,
        onResolve: () => onWorkflowStatusChange("resolved"),
        onReopen: () => onWorkflowStatusChange("open"),
        onDelete: onDeleteFromHeader,
        onCreateIncident: requestCreateIncidentFromAlert,
      }),
    );
  }
  return { title: name, options: { titlePrefix: idPrefix, actions } };
});

onMounted(async () => {
  await Promise.all([loadIntegrations(), load(), loadMentionTargets(), thread.reload()]);
});
</script>

<template>
  <div :class="pageShellClass" :style="shellAsideStyle">
    <div :class="pageContentClass">
      <div
        v-if="showAlertThread || threadLeaving"
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize main content"
        tabindex="0"
        class="group absolute -right-6 top-0 z-20 hidden h-full w-6 cursor-col-resize focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)] lg:block"
        :class="resizingMain ? 'lg:select-none' : ''"
        @pointerdown="startMainResize"
        @keydown.left.prevent="nudgeMainWidth('narrower')"
        @keydown.right.prevent="nudgeMainWidth('wider')"
      >
        <div
          class="mx-auto h-full w-px bg-[var(--border-primary)] opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100"
        />
      </div>

      <section :class="mainSectionClass">
        <div
          v-if="!threadLayoutOpen"
          role="separator"
          aria-orientation="vertical"
          aria-label="Resize main content"
          tabindex="0"
          class="group absolute -right-6 top-0 z-20 hidden h-full w-6 cursor-col-resize focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)] lg:block"
          :class="resizingMain ? 'lg:select-none' : ''"
          @pointerdown="startMainResize"
          @keydown.left.prevent="nudgeMainWidth('narrower')"
          @keydown.right.prevent="nudgeMainWidth('wider')"
        >
          <div
            class="mx-auto h-full w-px bg-[var(--border-primary)] opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100"
          />
        </div>

        <ErrorBanner :message="error" />

        <div
          v-if="sseState !== 'open'"
          class="flex items-center gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-400"
          role="status"
          aria-live="polite"
        >
          <CircleAlert class="h-4 w-4 shrink-0 animate-pulse" aria-hidden="true" />
          <span class="font-medium">
            {{ sseState === "reconnecting" ? "Reconnecting…" : "Connecting…" }}
          </span>
          <span class="text-amber-600/80 dark:text-amber-500/80">Live updates paused.</span>
        </div>

        <!-- Skeleton loading: main column -->
        <div
          v-if="loading && !alert"
          :class="alertContentClass"
          aria-busy="true"
          aria-label="Loading alert"
        >
          <!-- Timestamp + severity/status skeleton -->
          <div class="space-y-1.5">
            <div class="h-3 w-48 animate-pulse rounded bg-[var(--skeleton-bg)]"></div>
            <div class="flex items-center gap-1.5">
              <div class="h-4 w-16 animate-pulse rounded bg-[var(--skeleton-bg)]"></div>
              <div class="ml-auto h-4 w-20 animate-pulse rounded bg-[var(--skeleton-bg)]"></div>
            </div>
          </div>

          <!-- Label badges skeleton -->
          <div class="flex flex-wrap gap-2">
            <div class="h-5 w-24 animate-pulse rounded bg-[var(--skeleton-bg)]"></div>
            <div class="h-5 w-32 animate-pulse rounded bg-[var(--skeleton-bg)]"></div>
            <div class="h-5 w-20 animate-pulse rounded bg-[var(--skeleton-bg)]"></div>
          </div>

          <!-- Annotations card skeleton -->
          <div
            class="animate-pulse rounded border border-[var(--border-primary)] bg-[var(--bg-card)] p-4 space-y-2"
          >
            <div class="h-4 w-32 rounded bg-[var(--skeleton-bg)]"></div>
            <div class="h-3 w-full rounded bg-[var(--skeleton-bg)]"></div>
            <div class="h-3 w-5/6 rounded bg-[var(--skeleton-bg)]"></div>
            <div class="h-3 w-4/6 rounded bg-[var(--skeleton-bg)]"></div>
          </div>

          <!-- Values card skeleton -->
          <div
            class="animate-pulse rounded border border-[var(--border-primary)] bg-[var(--bg-card)] p-4 space-y-2"
          >
            <div class="h-4 w-20 rounded bg-[var(--skeleton-bg)]"></div>
            <div class="space-y-2">
              <div class="h-3 w-full rounded bg-[var(--skeleton-bg)]"></div>
              <div class="h-3 w-2/3 rounded bg-[var(--skeleton-bg)]"></div>
            </div>
          </div>

          <!-- Investigation section skeleton -->
          <div
            class="animate-pulse rounded border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-4 space-y-3"
          >
            <div class="flex items-center gap-2">
              <div class="h-4 w-24 rounded bg-[var(--skeleton-bg)]"></div>
            </div>
            <div class="border-t border-[var(--border-primary)] pt-3">
              <div class="flex items-center gap-2">
                <div class="h-6 w-6 animate-pulse rounded-full bg-[var(--skeleton-bg)]"></div>
                <div class="h-6 w-6 animate-pulse rounded-full bg-[var(--skeleton-bg)]"></div>
                <div class="h-4 w-32 animate-pulse rounded bg-[var(--skeleton-bg)]"></div>
              </div>
            </div>
            <div class="border-t border-[var(--border-primary)] pt-3 space-y-2">
              <div class="h-3 w-full rounded bg-[var(--skeleton-bg)]"></div>
              <div class="h-3 w-5/6 rounded bg-[var(--skeleton-bg)]"></div>
              <div class="h-3 w-3/6 rounded bg-[var(--skeleton-bg)]"></div>
            </div>
          </div>
        </div>

        <div v-if="!loading && alert" class="flex flex-1 flex-col gap-2 lg:min-h-0">
          <div
            v-if="isDeleted"
            class="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-2 text-sm text-[var(--text-muted)]"
            role="status"
          >
            This alert was deleted and is shown read-only.
          </div>
          <!-- Main content cards -->
          <div :class="alertContentClass">
            <!-- Timestamp + severity/status row -->
            <div class="space-y-1.5 text-xs text-[var(--text-muted)]">
              <div class="flex items-center gap-1">
                <Clock class="h-3 w-3" aria-hidden="true" />
                <span>{{ alertStartedAtText }}</span>
              </div>
              <div class="flex items-center">
                <div v-if="severityDisplayText" class="flex items-center gap-1.5">
                  <span class="shrink-0 uppercase" :class="severityFilledBadgeCss">
                    <span class="sr-only">Severity:</span>
                    {{ severityDisplayText }}
                  </span>
                </div>
                <div class="ml-auto flex items-center gap-1.5">
                  <template v-if="showAckButton && !isDeleted">
                    <Button @click="acknowledge" :disabled="ackLoading" size="sm">
                      <ShieldCheck class="h-4 w-4" />
                      {{ ackLoading ? "Acknowledging..." : "Acknowledge" }}
                    </Button>
                  </template>
                  <span
                    v-else
                    class="shrink-0 uppercase"
                    :class="[statusBadgeStyleClass, statusWorkflowBusy ? 'opacity-60' : '']"
                    aria-live="polite"
                    ><span class="sr-only">Status:</span>{{ statusBadgeLabel }}</span
                  >
                </div>
              </div>
            </div>

            <!-- Label badges -->
            <div v-if="labelsShownBelowHeader.length > 0" class="flex flex-wrap gap-2 text-xs">
              <span
                v-for="{ key, value } in labelsShownBelowHeader"
                :key="key"
                class="rounded border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-2 py-0.5"
              >
                <span class="text-[var(--text-muted)]">{{ key }}:</span> {{ value }}
              </span>
            </div>
            <!-- Annotations -->
            <Card v-if="showAnnotationsCard">
              <h3
                v-if="annotationSummaryText"
                class="text-base font-semibold leading-snug text-[var(--text-primary)]"
              >
                {{ annotationSummaryText }}
              </h3>
              <h3 v-else class="field-label">Annotations</h3>
              <p
                v-if="annotationDescriptionText"
                class="mt-2 whitespace-pre-wrap text-sm leading-relaxed text-[var(--text-secondary)]"
              >
                {{ annotationDescriptionText }}
              </p>
              <div
                v-if="otherAnnotations.length"
                :class="
                  annotationSummaryText || annotationDescriptionText
                    ? 'mt-4 border-t border-[var(--border-primary)] pt-4'
                    : 'mt-0'
                "
              >
                <div class="grid gap-2 text-sm">
                  <KeyValueDisplay :data="otherAnnotationsMap" />
                </div>
              </div>
            </Card>

            <!-- Values -->
            <Card v-if="alert.values && Object.keys(alert.values).length > 0">
              <h3 class="field-label mb-3">Values</h3>
              <div class="grid gap-2 text-sm">
                <KeyValueDisplay :data="alertValuesMap" />
              </div>
            </Card>

            <!-- Investigation section -->
            <div class="rounded border border-[var(--border-primary)] bg-[var(--bg-secondary)]">
              <div class="rounded-t">
                <div class="flex items-center justify-between gap-2 px-4 py-3">
                  <div class="flex min-w-0 items-center gap-2">
                    <h3 class="field-label mb-0">
                      <HatGlasses
                        class="inline h-4 w-4 align-text-bottom text-[var(--text-muted)]"
                      />
                      Investigation
                    </h3>
                  </div>
                </div>

                <button
                  type="button"
                  class="flex w-full items-center justify-between gap-3 border-t border-[var(--border-primary)] px-4 py-3 text-left transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
                  :class="
                    chatParticipants.length > 0 || hasMeaningfulInvestigation
                      ? 'cursor-pointer hover:bg-[var(--btn-default-hover)]'
                      : 'cursor-default'
                  "
                  :disabled="chatParticipants.length === 0 && !hasMeaningfulInvestigation"
                  :aria-expanded="showAlertThread"
                  aria-controls="alert-investigation-drawer"
                  @click="toggleAlertThread"
                >
                  <div class="flex min-w-0 items-center gap-2 text-sm">
                    <div class="flex -space-x-1.5">
                      <div
                        v-for="participant in chatParticipants.slice(0, 3)"
                        :key="participant.key"
                        class="flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden rounded-full border border-[var(--bg-secondary)] bg-[var(--bg-tertiary)] text-[10px] font-semibold text-[var(--text-secondary)]"
                        :title="participant.name"
                      >
                        <img
                          v-if="participant.avatarSrc"
                          :src="participant.avatarSrc"
                          :alt="participant.name"
                          class="h-full w-full rounded-full object-cover"
                          loading="lazy"
                          decoding="async"
                        />
                        <span v-else>{{ participant.name.charAt(0).toUpperCase() }}</span>
                      </div>
                      <div
                        v-if="chatParticipants.length === 0 && hasMeaningfulInvestigation"
                        class="flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden rounded-full bg-[var(--bg-secondary)]"
                      >
                        <img
                          :src="getAgentAvatarSrc(alertInvestigation?.agent_type)"
                          :alt="
                            alertInvestigation?.agent_type === 'openclaw' ? 'OpenClaw' : 'Hermes'
                          "
                          class="h-full w-full rounded-full object-cover"
                          loading="lazy"
                          decoding="async"
                        />
                      </div>
                      <div
                        v-else-if="chatParticipants.length === 0"
                        class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-[var(--bg-tertiary)] text-[var(--text-muted)]"
                      >
                        <MessageSquare class="h-3 w-3" />
                      </div>
                    </div>
                    <span class="truncate font-semibold text-[var(--text-secondary)]">
                      {{ chatParticipantLabel }}
                    </span>
                    <TypingIndicator v-if="agentTyping" class="shrink-0" />
                  </div>
                  <div class="flex shrink-0 items-center gap-2">
                    <span
                      v-if="totalChatCount > 0"
                      class="flex items-center gap-1 text-xs text-[var(--text-muted)]"
                    >
                      <MessageSquare class="h-3 w-3" />
                      {{ totalChatCount }}
                    </span>
                    <ChevronRight
                      v-if="chatParticipants.length > 0 || hasMeaningfulInvestigation"
                      class="h-5 w-5 text-[var(--text-muted)] transition-transform duration-200"
                      :class="showAlertThread ? 'rotate-180' : ''"
                    />
                  </div>
                </button>
              </div>

              <!-- Promoted to incident banner -->
              <div
                v-if="investigationPromoted"
                class="border-t border-[var(--border-primary)] px-4 py-3 text-sm text-[var(--text-secondary)]"
              >
                <div class="flex items-center gap-2">
                  <span class="font-medium text-[var(--text-primary)]">Handled by incident</span>
                  <DeletedBadge
                    v-if="relatedIncident?.deleted_at"
                    title="This incident was deleted"
                  />
                </div>
                <p class="mt-1">
                  This alert investigation was promoted. Use the linked incident for current status,
                  command decisions, and final resolution details.
                </p>
                <RouterLink
                  v-if="promotedIncidentRoute"
                  :to="promotedIncidentRoute"
                  :class="[
                    'mt-2 inline-flex text-[var(--accent)] hover:underline',
                    relatedIncident?.deleted_at ? 'opacity-50 italic' : '',
                  ]"
                >
                  Open incident
                </RouterLink>
              </div>

              <!-- Investigation details (when investigation exists) -->
              <div
                v-if="alertInvestigation && hasInvestigationDetails"
                class="border-t border-[var(--border-primary)] px-4 py-2.5"
              >
                <div
                  v-if="investigationPromoted"
                  class="mb-2 inline-flex items-center gap-1.5 rounded bg-[var(--bg-tertiary)] px-2 py-0.5 text-xs font-medium text-[var(--text-muted)]"
                >
                  Historical — superseded by incident
                </div>
                <div v-if="investigationOutcome" class="space-y-2 text-sm">
                  <div v-if="investigationOutcome.summary">
                    <span class="font-medium text-[var(--text-muted)]">Summary</span>
                    <p class="mt-0.5 whitespace-pre-wrap text-[var(--text-primary)]">
                      {{ investigationOutcome.summary }}
                    </p>
                  </div>
                  <div v-if="investigationOutcome.root_cause">
                    <span class="font-medium text-[var(--text-muted)]">Root Cause</span>
                    <MarkdownRenderer
                      :content="investigationOutcome.root_cause"
                      class="mt-0.5 text-[var(--text-primary)]"
                    />
                  </div>
                  <div v-if="investigationOutcome.resolution">
                    <span class="font-medium text-[var(--text-muted)]">Resolution</span>
                    <MarkdownRenderer
                      :content="investigationOutcome.resolution"
                      class="mt-0.5 text-[var(--text-primary)]"
                    />
                  </div>
                  <div
                    v-if="investigationOutcome.findings && investigationOutcome.findings.length > 0"
                  >
                    <span class="font-medium text-[var(--text-muted)]">Findings</span>
                    <ul class="mt-0.5 list-inside list-disc space-y-0.5 text-[var(--text-primary)]">
                      <li v-for="(f, i) in investigationOutcome.findings" :key="i">{{ f }}</li>
                    </ul>
                  </div>
                  <div
                    v-if="
                      investigationOutcome.recommended_actions &&
                      investigationOutcome.recommended_actions.length > 0
                    "
                  >
                    <span class="font-medium text-[var(--text-muted)]">Recommended Actions</span>
                    <ul class="mt-0.5 list-inside list-disc space-y-0.5 text-[var(--text-primary)]">
                      <li v-for="(a, i) in investigationOutcome.recommended_actions" :key="i">
                        {{ a }}
                      </li>
                    </ul>
                  </div>
                </div>

                <div
                  v-if="alertInvestigation.findings && alertInvestigation.findings.length > 0"
                  class="mt-2.5 border-t border-[var(--border-primary)] pt-2.5"
                >
                  <div class="flex items-center gap-1.5 text-sm">
                    <Lightbulb class="h-3.5 w-3.5 text-[var(--text-muted)]" />
                    <span class="font-medium text-[var(--text-muted)]">Structured Findings</span>
                  </div>
                  <div
                    v-for="(f, i) in alertInvestigation.findings"
                    :key="i"
                    class="mt-1.5 rounded border border-[var(--border-primary)] px-3 py-2 text-sm"
                  >
                    <div class="flex items-center gap-2">
                      <span class="text-[var(--text-primary)]">{{ f.title }}</span>
                      <span v-if="f.severity" :class="['badge', severityBadgeClass(f.severity)]">
                        {{ f.severity }}
                      </span>
                    </div>
                    <ul
                      v-if="f.evidence && f.evidence.length > 0"
                      class="mt-1 list-inside list-disc text-xs text-[var(--text-muted)]"
                    >
                      <li v-for="(e, j) in f.evidence" :key="j">{{ e }}</li>
                    </ul>
                  </div>
                </div>

                <div
                  v-if="alertInvestigation.evidence && alertInvestigation.evidence.length > 0"
                  class="mt-2.5 border-t border-[var(--border-primary)] pt-2.5"
                >
                  <div class="flex items-center gap-1.5 text-sm">
                    <FileText class="h-3.5 w-3.5 text-[var(--text-muted)]" />
                    <span class="font-medium text-[var(--text-muted)]">Evidence</span>
                  </div>
                  <div class="mt-1.5 space-y-1.5">
                    <div
                      v-for="(ev, i) in alertInvestigation.evidence"
                      :key="i"
                      class="rounded border border-[var(--border-primary)] px-3 py-2 text-sm"
                    >
                      <div class="flex items-center gap-2">
                        <span class="font-medium text-[var(--text-secondary)]">{{
                          ev.source
                        }}</span>
                        <span class="text-xs text-[var(--text-muted)]">{{ ev.type }}</span>
                      </div>
                      <p class="mt-0.5 whitespace-pre-wrap text-[var(--text-primary)]">
                        {{ ev.content }}
                      </p>
                    </div>
                  </div>
                </div>
              </div>

              <!-- Start investigation buttons -->
              <div
                v-if="!alertInvestigation && canWriteAlert && !isDeleted"
                class="border-t border-[var(--border-primary)] px-4 py-3"
              >
                <div class="flex flex-wrap items-center gap-2">
                  <Button size="sm" :loading="investigateLoading" @click="triggerInvestigation">
                    <Zap class="h-4 w-4" />
                    Start Agent Investigation
                  </Button>
                  <Button variant="outline" size="sm" @click="openAlertThread">
                    <MessageSquare class="h-4 w-4" />
                    Manual Thread
                  </Button>
                </div>
              </div>
            </div>
          </div>

          <aside
            v-if="threadLayoutOpen && !showAlertThread && !threadLeaving"
            class="min-w-0 space-y-4 pb-20 md:pb-4"
          >
            <AlertDetailsSidebar
              :runbook-href="runbookHref"
              :delivery-targets="resolvedDeliveryTargets"
              :timeline="timeline"
              :assignee="sidebarAssignee"
              :users="users"
              :can-assign="canWriteAlert && !isDeleted && !!alertInvestigation"
              :assignee-id="alertInvestigation?.assignee_id"
              @open-delivery-thread="openDeliveryThreadFromResolved"
              @assign="handleAssignInvestigation"
            >
              <template #after-notifications>
                <!-- Related Incident -->
                <Card v-if="relatedIncident">
                  <div class="mb-3 flex items-center gap-2">
                    <h3 class="field-label mb-0">Incident</h3>
                  </div>
                  <component
                    :is="relatedIncident.deleted_at ? 'div' : RouterLink"
                    :to="
                      relatedIncident.deleted_at
                        ? undefined
                        : `/incidents/${relatedIncident.incident_number}`
                    "
                    :class="[
                      'flex items-center gap-3 rounded-md border border-[var(--border-primary)] px-3 py-2 transition-colors',
                      relatedIncident.deleted_at
                        ? 'cursor-default opacity-50 italic'
                        : 'cursor-pointer hover:bg-[var(--bg-secondary)]',
                    ]"
                  >
                    <CircleDot
                      class="h-4 w-4 shrink-0"
                      :style="{ color: incidentPriorityBorderColor(relatedIncident.priority) }"
                    />
                    <div class="min-w-0 flex-1">
                      <span class="text-sm font-medium text-[var(--text-primary)]">
                        #{{ relatedIncident.incident_number }}
                        {{ relatedIncident.title }}
                      </span>
                    </div>
                    <DeletedBadge
                      v-if="relatedIncident.deleted_at"
                      class="shrink-0"
                      title="This incident was deleted"
                    />
                    <span
                      :class="[
                        'badge shrink-0',
                        relatedIncident.status === 'resolved' || relatedIncident.status === 'closed'
                          ? 'badge-green'
                          : relatedIncident.status === 'mitigated'
                            ? 'badge-yellow'
                            : 'badge-red',
                      ]"
                    >
                      {{ relatedIncident.status.replace("_", " ") }}
                    </span>
                  </component>
                </Card>
              </template>
            </AlertDetailsSidebar>

            <!-- Related Alerts -->
            <Card v-if="relatedAlerts.length > 0">
              <div class="mb-3">
                <h3 class="field-label mb-0">Related Alerts</h3>
                <p class="mt-0.5 text-xs text-[var(--text-muted)]">
                  Correlated alerts from the same investigation
                </p>
              </div>
              <div class="space-y-2">
                <RouterLink
                  v-for="ra in relatedAlerts"
                  :key="ra.fingerprint"
                  :to="ra.alert_number ? `/alerts/${ra.alert_number}` : `/alerts/${ra.fingerprint}`"
                  class="flex cursor-pointer items-center gap-3 rounded-md border border-[var(--border-primary)] px-3 py-2 transition-colors hover:bg-[var(--bg-secondary)]"
                >
                  <CircleDot class="h-4 w-4 shrink-0 text-[var(--text-muted)]" />
                  <div class="min-w-0 flex-1">
                    <span class="text-sm text-[var(--text-primary)]">
                      {{ ra.labels?.alertname || ra.fingerprint }}
                    </span>
                    <span v-if="ra.alert_number" class="ml-1 text-xs text-[var(--text-muted)]">
                      #{{ ra.alert_number }}
                    </span>
                  </div>
                  <AlertStatusBadge :status="ra.status" class="shrink-0" />
                </RouterLink>
              </div>
            </Card>
          </aside>
        </div>

        <ConfirmDialog
          v-model:open="showDeleteConfirm"
          title="Delete alert"
          :message="deleteConfirmMessage"
          confirm-label="Delete"
          :destructive="true"
          @confirm="doDelete"
        />

        <ConfirmDialog
          v-model:open="showCreateIncidentConfirm"
          title="Create incident"
          :message="createIncidentConfirmMessage"
          confirm-label="Create incident"
          :loading="createIncidentLoading"
          @confirm="createIncidentFromAlert"
        />
      </section>

      <!-- Skeleton loading: sidebar column -->
      <aside v-if="loading && !alert" :class="defaultSidebarClass" aria-hidden="true">
        <div
          class="animate-pulse rounded border border-[var(--border-primary)] bg-[var(--bg-card)] p-4 space-y-3"
        >
          <div class="h-4 w-32 rounded bg-[var(--skeleton-bg)]"></div>
          <div class="space-y-2">
            <div v-for="i in 3" :key="i" class="h-10 w-full rounded bg-[var(--skeleton-bg)]"></div>
          </div>
        </div>
        <div
          class="animate-pulse rounded border border-[var(--border-primary)] bg-[var(--bg-card)] p-4 space-y-3"
        >
          <div class="h-4 w-24 rounded bg-[var(--skeleton-bg)]"></div>
          <div class="h-3 w-3/4 rounded bg-[var(--skeleton-bg)]"></div>
        </div>
        <div
          class="animate-pulse rounded border border-[var(--border-primary)] bg-[var(--bg-card)] p-4 space-y-3"
        >
          <div class="h-4 w-28 rounded bg-[var(--skeleton-bg)]"></div>
          <div class="space-y-2">
            <div v-for="i in 2" :key="i" class="h-12 w-full rounded bg-[var(--skeleton-bg)]"></div>
          </div>
        </div>
      </aside>

      <aside
        v-if="!loading && alert && (!threadLayoutOpen || showAlertThread || threadLeaving)"
        :class="defaultSidebarClass"
      >
        <AlertDetailsSidebar
          :runbook-href="runbookHref"
          :delivery-targets="resolvedDeliveryTargets"
          :timeline="timeline"
          :assignee="sidebarAssignee"
          :users="users"
          :can-assign="canWriteAlert && !isDeleted && !!alertInvestigation"
          :assignee-id="alertInvestigation?.assignee_id"
          @open-delivery-thread="openDeliveryThreadFromResolved"
          @assign="handleAssignInvestigation"
        >
          <template #after-notifications>
            <!-- Related Incident -->
            <Card v-if="relatedIncident">
              <div class="mb-3 flex items-center gap-2">
                <h3 class="field-label mb-0">Incident</h3>
              </div>
              <component
                :is="relatedIncident.deleted_at ? 'div' : RouterLink"
                :to="
                  relatedIncident.deleted_at
                    ? undefined
                    : `/incidents/${relatedIncident.incident_number}`
                "
                :class="[
                  'flex items-center gap-3 rounded-md border border-[var(--border-primary)] px-3 py-2 transition-colors',
                  relatedIncident.deleted_at
                    ? 'cursor-default opacity-50 italic'
                    : 'cursor-pointer hover:bg-[var(--bg-secondary)]',
                ]"
              >
                <CircleDot
                  class="h-4 w-4 shrink-0"
                  :style="{ color: incidentPriorityBorderColor(relatedIncident.priority) }"
                />
                <div class="min-w-0 flex-1">
                  <span class="text-sm font-medium text-[var(--text-primary)]">
                    #{{ relatedIncident.incident_number }}
                    {{ relatedIncident.title }}
                  </span>
                </div>
                <DeletedBadge
                  v-if="relatedIncident.deleted_at"
                  class="shrink-0"
                  title="This incident was deleted"
                />
                <span
                  :class="[
                    'badge shrink-0',
                    relatedIncident.status === 'resolved' || relatedIncident.status === 'closed'
                      ? 'badge-green'
                      : relatedIncident.status === 'mitigated'
                        ? 'badge-yellow'
                        : 'badge-red',
                  ]"
                >
                  {{ relatedIncident.status.replace("_", " ") }}
                </span>
              </component>
            </Card>
          </template>
        </AlertDetailsSidebar>

        <!-- Related Alerts -->
        <Card v-if="relatedAlerts.length > 0">
          <div class="mb-3">
            <h3 class="field-label mb-0">Related Alerts</h3>
            <p class="mt-0.5 text-xs text-[var(--text-muted)]">
              Correlated alerts from the same investigation
            </p>
          </div>
          <div class="space-y-2">
            <RouterLink
              v-for="ra in relatedAlerts"
              :key="ra.fingerprint"
              :to="ra.alert_number ? `/alerts/${ra.alert_number}` : `/alerts/${ra.fingerprint}`"
              class="flex cursor-pointer items-center gap-3 rounded-md border border-[var(--border-primary)] px-3 py-2 transition-colors hover:bg-[var(--bg-secondary)]"
            >
              <CircleDot class="h-4 w-4 shrink-0 text-[var(--text-muted)]" />
              <div class="min-w-0 flex-1">
                <span class="text-sm text-[var(--text-primary)]">
                  {{ ra.labels?.alertname || ra.fingerprint }}
                </span>
                <span v-if="ra.alert_number" class="ml-1 text-xs text-[var(--text-muted)]">
                  #{{ ra.alert_number }}
                </span>
              </div>
              <AlertStatusBadge :status="ra.status" class="shrink-0" />
            </RouterLink>
          </div>
        </Card>
      </aside>
    </div>

    <Transition name="investigation-sidebar" @after-leave="onThreadSidebarAfterLeave">
      <aside
        v-if="showAlertThread"
        id="alert-investigation-drawer"
        class="fixed inset-x-0 top-0 z-50 flex h-dvh min-w-0 flex-col overflow-hidden border border-[var(--border-primary)] bg-[var(--bg-primary)] shadow-sm lg:sticky lg:inset-auto lg:top-0 lg:z-auto lg:col-start-2 lg:h-[calc(100vh-3.5rem)] lg:max-h-[calc(100vh-3.5rem)] lg:min-h-0 lg:self-start lg:rounded-none lg:-mr-6"
        role="complementary"
        aria-labelledby="alert-investigation-drawer-title"
      >
        <header
          class="flex shrink-0 items-center justify-between gap-3 border-b border-[var(--border-primary)] px-4 py-2"
        >
          <div class="flex min-w-0 items-center gap-2">
            <h2 id="alert-investigation-drawer-title" class="field-label mb-0">
              <HatGlasses class="inline h-4 w-4 align-text-bottom text-[var(--text-muted)]" />
              INVESTIGATION
            </h2>
            <TypingIndicator v-if="agentTyping" />
          </div>

          <button
            type="button"
            class="flex h-7 w-7 shrink-0 cursor-pointer items-center justify-center rounded-md text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
            aria-label="Close investigation thread"
            @click="closeAlertThread"
          >
            <X class="h-4 w-4" />
          </button>
        </header>

        <div
          ref="chatThreadEl"
          class="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-3 pt-2 pb-28 md:pb-2 [&]:[overflow-anchor:none]"
        >
          <template
            v-for="item in chatThreadItems"
            :key="item.kind === 'date' ? item.key : item.message.id"
          >
            <ChatDateSeparator v-if="item.kind === 'date'" :label="item.label" />
            <ChatMessageRow
              v-else
              :id="item.message.id"
              :indicator-class="chatSourceIndicatorClass(item.message.source)"
              :highlight-class="searchHighlight(item.message.id)"
              :avatar-src="chatAvatarSrc(item.message)"
              :avatar-letter="
                item.message.source !== 'agent' ? chatAvatarLetter(item.message) : undefined
              "
              :avatar-bg="chatSourceAvatarBg(item.message.source)"
              :avatar-title="chatDisplayName(item.message)"
              :display-name="chatDisplayName(item.message)"
              :created-at="item.message.created_at"
              :content="item.message.message"
              :internal="item.message.internal"
              :edited="item.message.edited"
              :search-query="searchHasQuery ? searchQuery : undefined"
              :reply-to-text="chatReplyContextFor(item.message).replyToText"
              :reply-to-author="chatReplyContextFor(item.message).replyToAuthor"
              @context-menu="onChatMessageContextMenu"
            >
              <template #meta-extras>
                <Bot
                  v-if="item.message.source === 'agent'"
                  class="h-3.5 w-3.5 text-purple-500"
                  title="Agent"
                />
              </template>
              <template #actions>
                <button
                  v-if="canWriteAlert && !isDeleted"
                  type="button"
                  class="rounded p-1 text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-hover)] hover:text-[var(--text-primary)]"
                  title="Reply"
                  :aria-label="'Reply to ' + chatDisplayName(item.message)"
                  @click="startChatReply(item.message)"
                >
                  <Reply class="h-4 w-4" />
                </button>
              </template>
            </ChatMessageRow>
          </template>

          <ChatTypingIndicator
            v-if="agentTyping"
            :avatar-src="getAgentAvatarSrc(alertInvestigation?.agent_type)"
            avatar-bg="bg-transparent"
            :avatar-title="alertInvestigation?.agent_name ?? 'Agent'"
            :display-name="alertInvestigation?.agent_name ?? 'Agent'"
            :dimmed="searchHasQuery"
          />

          <div
            v-if="chatMessages.length === 0 && !agentTyping"
            class="py-10 text-center text-sm text-[var(--text-muted)]"
          >
            No messages yet
          </div>
        </div>

        <div
          v-if="chatReplyingTo"
          class="flex items-center gap-2 rounded border border-[var(--border-primary)] bg-[var(--bg-muted)]/50 px-3 py-1.5 text-sm"
        >
          <Reply class="h-4 w-4 shrink-0 text-[var(--text-muted)]" />
          <div class="min-w-0 flex-1">
            <span class="font-medium text-[var(--text-secondary)]">{{
              chatDisplayName(chatReplyingTo)
            }}</span>
            <p class="line-clamp-1 text-xs text-[var(--text-muted)]">
              {{ chatReplyingTo.message }}
            </p>
          </div>
          <button
            type="button"
            class="rounded p-1 text-[var(--text-muted)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-primary)]"
            title="Cancel reply"
            aria-label="Cancel reply"
            @click="cancelChatReply"
          >
            <X class="h-4 w-4" />
          </button>
        </div>
        <ChatEditorBar v-if="canWriteAlert && !isDeleted" class="!mt-0 !pt-0">
          <MarkdownEditor
            ref="editorRef"
            v-model="chatDraft"
            :disabled="chatSending"
            :users="users"
            :agents="agents"
            placeholder="Message the agent..."
            @submit="sendChatMessage"
          />
        </ChatEditorBar>
        <MessageContextMenu
          :open="chatContextMenu !== null"
          :position="chatContextMenu?.position ?? null"
          :actions="chatContextMenuActions"
          :aria-label="'Message actions'"
          @close="closeChatContextMenu"
        />
      </aside>
    </Transition>
  </div>
</template>

<style scoped>
.investigation-sidebar-enter-active,
.investigation-sidebar-leave-active {
  transition:
    opacity 0.18s ease,
    transform 0.22s cubic-bezier(0.16, 1, 0.3, 1);
}

.investigation-sidebar-enter-from,
.investigation-sidebar-leave-to {
  opacity: 0;
  transform: translateX(1rem);
}

.main-scroll {
  overflow-y: auto;
  scrollbar-width: thin;
  scrollbar-color: transparent transparent;
}

.main-scroll:hover {
  scrollbar-color: var(--border-primary) transparent;
}

.main-scroll::-webkit-scrollbar {
  width: 6px;
}

.main-scroll::-webkit-scrollbar-track {
  background: transparent;
}

.main-scroll::-webkit-scrollbar-thumb {
  background-color: transparent;
  border-radius: 3px;
}

.main-scroll:hover::-webkit-scrollbar-thumb {
  background-color: var(--border-primary);
}

:deep(.rounded-lg) {
  border-radius: 0.375rem;
}
:deep(.rounded-t-lg) {
  border-top-left-radius: 0.375rem;
  border-top-right-radius: 0.375rem;
}
</style>
