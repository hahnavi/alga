<script setup lang="ts">
import { computed, h, onMounted, onBeforeUnmount, ref, watch, type CSSProperties } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import {
  ChevronRight,
  CircleAlert,
  CircleDot,
  Clock,
  ExternalLink,
  FileText,
  HatGlasses,
  Pencil,
  X,
  Plus,
  Unlink,
  ChevronDown,
  ChevronUp,
  BookOpen,
  Save,
  MessageSquare,
  ShieldAlert,
  Wrench,
} from "@lucide/vue";
import { useSSE } from "@/composables/useSSE";
import { useEscapeKey } from "@/composables/useEscapeKey";
import { useResizableMain } from "@/composables/useResizableMain";
import { useUsers } from "@/composables/useUsers";
import { useIncidentDetailData } from "@/composables/useIncidentDetailData";
import { useIncidentDocumentSections } from "@/composables/useIncidentDocumentSections";
import { useIncidentCoordination } from "@/composables/useIncidentCoordination";
import { useIncidentEditor } from "@/composables/useIncidentEditor";
import { useIncidentThread } from "@/composables/useIncidentThread";
import { getAgentAvatarSrc, getAgentBrandIconSrc } from "@/lib/agentAvatar";
import { getProviderIconSrc } from "@/lib/providerIcon";
import TypingIndicator from "@/components/ui/TypingIndicator.vue";
import ChatTypingIndicator from "@/components/ui/ChatTypingIndicator.vue";
import { useTypingIndicator } from "@/composables/useTypingIndicator";
import { api, type IncidentRecord, type AlertRecord, type OwnerThreadMessage } from "@/lib/api";
import {
  alertSeverityLabel,
  postMortemStatusBadgeClass,
  incidentStatusBadgeClass,
  incidentStatusLabel,
  severityBorderColor,
} from "@/lib/alertLabels";
import { formatTime, formatTimeFull } from "@/lib/time";
import { CARD_ICON_BTN_CLASS } from "@/lib/uiClasses";
import IncidentActionsMenu from "@/components/incident/IncidentActionsMenu.vue";
import IncidentTimeline from "@/components/incident/IncidentTimeline.vue";
import IncidentCoordinationStream from "@/components/incident/IncidentCoordinationStream.vue";
import StatusUpdateFeed from "@/components/incident/StatusUpdateFeed.vue";
import CoordinationTaskBoard from "@/components/incident/CoordinationTaskBoard.vue";
import ICSRoleBoard from "@/components/incident/ICSRoleBoard.vue";
import OwnerThreadPanel from "@/components/thread/OwnerThreadPanel.vue";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import NumberInput from "@/components/ui/NumberInput.vue";
import Textarea from "@/components/ui/Textarea.vue";
import Select from "@/components/ui/Select.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import AlertStatusBadge from "@/components/ui/AlertStatusBadge.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Modal from "@/components/ui/Modal.vue";
import MarkdownRenderer from "@/components/ui/MarkdownRenderer.vue";
import MarkdownEditor from "@/components/ui/MarkdownEditor.vue";
import ChatEditorBar from "@/components/ui/ChatEditorBar.vue";
import DeletedBadge from "@/components/ui/DeletedBadge.vue";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { useDelete } from "@/composables/useDelete";
import { useDocumentTitle } from "@/composables/useDocumentTitle";
import { usePageHeader } from "@/composables/usePageHeader";

defineOptions({ name: "IncidentDetailPage" });

const route = useRoute();
const router = useRouter();

const googleMeetBrandIcon = getAgentBrandIconSrc("google_meet") ?? "";

const incidentNumber = computed(() => Number(route.params.incident_number));
const { canRead: canReadPostmortem, canWrite: canWritePostmortem } =
  useEntityPermissions("postmortems");

const data = useIncidentDetailData(incidentNumber, {
  canCreatePostMortem: canWritePostmortem,
});

const incident = data.incident;
const timeline = data.timeline;
const alerts = data.alerts;
const icsRoles = data.icsRoles;
const loading = data.loading;
const error = data.error;
const mitigationPlaybooks = data.mitigationPlaybooks;

const docs = useIncidentDocumentSections(incidentNumber, incident, data.setIncident);
const coord = useIncidentCoordination(incidentNumber);

const { canWrite, canDelete, canCommand } = useEntityPermissions("incidents");

const editor = useIncidentEditor(
  incidentNumber,
  incident,
  data.setIncident,
  loadIncident,
  () => data.loadAlerts(),
  data.removeLinkedAlert,
);

const thread = useIncidentThread(incidentNumber, { scheduleReload });

async function loadIncident() {
  // The detail-data composable owns the main load + timeline, alerts,
  // ICS roles, mitigation playbooks, and post-mortem status side-loads.
  // The remaining side-loads (document sections, coordination,
  // thread, status updates) live in their own composables / helpers
  // and are awaited here so the page can render a complete view in
  // one shot.
  await data.load();
  if (!incident.value) return;
  await Promise.all([
    docs.load(),
    coord.loadCoordinationMessages(),
    coord.loadCoordinationTasks(),
    thread.loadIncidentThread(),
    coord.fetchStatusUpdates(true),
  ]);
}

useDocumentTitle(() =>
  incident.value ? `Incident #${incident.value.incident_number}` : "Incident",
);
const isDeleted = computed(() => !!incident.value?.deleted_at);
const { mainWidth, nudgeMainWidth, resizingMain, startMainResize } = useResizableMain();
let reloadDebounce: number | null = null;

const conferenceHref = computed(() => {
  const raw = incident.value?.conference_url?.trim();
  if (!raw) return null;
  if (/^https?:\/\//i.test(raw)) return raw;
  try {
    const u = new URL(raw);
    return u.protocol === "http:" || u.protocol === "https:" ? u.href : null;
  } catch {
    return null;
  }
});

const {
  showDeleteConfirm,
  deleting,
  confirmDelete: confirmDeleteIncident,
  doDelete: doDeleteIncident,
} = useDelete<IncidentRecord>(async (item) => {
  await api.deleteIncident(item.incident_number);
  router.push("/incidents");
}, "Incident");

const unlinkAlertTarget = ref<AlertRecord | null>(null);
const timelineCollapsed = ref(true);

type IncidentSidebarThread = { kind: "coordination" } | { kind: "technical_thread" };
const activeSidebarThread = ref<IncidentSidebarThread | null>(null);
const threadLayoutOpen = ref(false);
const threadLeaving = ref(false);

const coordinationMessages = coord.coordinationMessages;
const coordinationTasks = coord.coordinationTasks;
const showDispatchTaskDialog = coord.showDispatchTaskDialog;
const dispatchTaskGoal = coord.dispatchTaskGoal;
const dispatchTaskKind = coord.dispatchTaskKind;
const dispatchTaskRole = coord.dispatchTaskRole;
const dispatchTaskSubmitting = coord.dispatchTaskSubmitting;
const coordinationTypingSource = coord.coordinationTypingSource;
const coordinationTypingAgentType = coord.coordinationTypingAgentType;
const coordinationTyping = coord.coordinationTyping;
const setCoordinationTyping = coord.setCoordinationTyping;
const clearCoordinationTyping = coord.clearCoordinationTyping;

const {
  isTyping: investigationTyping,
  setTyping: setInvestigationTyping,
  clearTyping: clearInvestigationTyping,
} = useTypingIndicator({ timeoutMs: 6000 });

type ThreadParticipant = {
  key: string;
  name: string;
  avatarSrc?: string;
};

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

function ownerThreadDisplayName(message: OwnerThreadMessage): string {
  if (message.username?.trim()) return message.username.trim();
  if (message.source === "agent") return "Agent";
  if (message.source === "system") return "System";
  if (message.source === "slack") return "Slack";
  if (message.source === "mattermost") return "Mattermost";
  return "User";
}

function participantInitial(name: string): string {
  return name.trim().charAt(0).toUpperCase() || "U";
}

function participantLabel(participants: ThreadParticipant[], fallback: string): string {
  if (participants.length === 0) return fallback;
  if (participants.length === 1) return participants[0].name;
  const others = participants.length - 1;
  return `${participants[0].name} and ${others} ${others === 1 ? "other" : "others"}`;
}

const coordinationParticipants = coord.coordinationParticipants;
const coordinationParticipantLabel = coord.coordinationParticipantLabel;

const investigationParticipants = computed(() => {
  const map = new Map<string, ThreadParticipant>();
  for (const message of thread.incidentThread?.messages ?? []) {
    const isAgent = message.source === "agent";
    const avatar = isAgent ? getAgentAvatarSrc(message.agent_type) : undefined;
    addParticipant(
      map,
      message.user_id ?? ownerThreadDisplayName(message),
      message.id,
      ownerThreadDisplayName(message),
      avatar,
    );
  }
  return [...map.values()];
});

const investigationParticipantLabel = computed(() =>
  participantLabel(investigationParticipants.value, "No participants yet"),
);

const expandedPlaybookId = ref<string | null>(null);

const coordinationText = coord.coordinationText;
const coordinationSubmitting = coord.coordinationSubmitting;
const coordinationKind = coord.coordinationKind;
const coordinationThreadEl = coord.coordinationThreadEl;
const stickCoordinationToBottom = coord.stickCoordinationToBottom;
const scrollCoordinationToBottom = coord.scrollCoordinationToBottom;
const agents = coord.agents;
const editorRef = ref<InstanceType<typeof MarkdownEditor> | null>(null);
const { users } = useUsers();
// Touch the page-local aliases so the typecheck sees them as used;
// the template references them via `ref="…"` and event handlers,
// but the script-only audit only counts script reads.
void coordinationKind;
void coordinationThreadEl;

const postMortemStatus = data.postMortemStatus;
const postMortemTitle = data.postMortemTitle;
const postMortemOpening = data.postMortemOpening;

const impactContent = docs.impactContent;
const impactEditing = docs.impactEditing;
const impactSaving = docs.impactSaving;
const rootCauseContent = docs.rootCauseContent;
const rootCauseEditing = docs.rootCauseEditing;
const rootCauseSaving = docs.rootCauseSaving;
const resolutionContent = docs.resolutionContent;
const resolutionEditing = docs.resolutionEditing;
const resolutionSaving = docs.resolutionSaving;
const summaryContent = docs.summaryContent;
const summaryEditing = docs.summaryEditing;
const summarySaving = docs.summarySaving;

const canCreatePostMortem = canWritePostmortem;

const statusUpdates = coord.statusUpdates;
const statusUpdatesLoading = coord.statusUpdatesLoading;
const statusUpdatesError = coord.statusUpdatesError;
const fetchStatusUpdates = coord.fetchStatusUpdates;

const canPostStatusUpdate = canCommand;

usePageHeader(() => {
  const inc = incident.value;
  if (!inc) return null;
  const actions: ReturnType<typeof h>[] = [];
  if (!inc.deleted_at) {
    actions.push(
      h(IncidentActionsMenu, {
        status: inc.status,
        loading: editor.actionLoading,
        canCommand: canCommand.value,
        canDelete: canDelete.value,
        escalating: editor.escalating,
        conferenceHref: conferenceHref.value,
        onEscalate: () => editor.escalateIncident(),
        onConference: () => {},
        onAcknowledge: () => editor.acknowledge(),
        onMitigate: () => editor.mitigate(),
        onResolve: () => editor.resolve(),
        onClose: () => editor.close(),
        onReopen: () => editor.reopen(),
        onCancel: () => editor.cancel(),
        onPromote: () => editor.promoteToActive(),
        onDelete: () => {
          if (incident.value) confirmDeleteIncident(incident.value);
        },
        onEdit: () => editor.openEditDialog(),
      }),
    );
  }
  return {
    title: inc.title,
    options: {
      titlePrefix: `#${inc.incident_number}`,
      actions,
    },
  };
});

function resetIncidentState() {
  data.reset();
  docs.reset();
  coord.reset();
  thread.incidentThread = null;
  activeSidebarThread.value = null;
  threadLayoutOpen.value = false;
  threadLeaving.value = false;
  expandedPlaybookId.value = null;
  clearInvestigationTyping();
}

function togglePlaybook(id: string) {
  expandedPlaybookId.value = expandedPlaybookId.value === id ? null : id;
}

const pageShellClass = computed(() => {
  if (activeSidebarThread.value || threadLeaving.value) {
    return "flex min-h-full flex-col gap-0 px-4 md:px-6 lg:grid lg:min-h-full lg:grid-cols-[clamp(480px,calc(var(--detail-main-width)_-_80px),calc(100vw-48px-24px-360px))_minmax(360px,1fr)] lg:items-start lg:gap-6 lg:[transition:--detail-main-width_220ms_cubic-bezier(0.16,1,0.3,1)]";
  }
  if (threadLayoutOpen.value) {
    return "flex min-h-full flex-col gap-0 px-4 md:px-6 lg:min-h-full";
  }
  return "flex h-full flex-col gap-0 px-4 md:px-6 lg:h-full";
});

const pageContentClass = computed(() => {
  if (activeSidebarThread.value || threadLeaving.value) {
    return "relative flex min-w-0 flex-col";
  }
  if (threadLayoutOpen.value) {
    return "relative flex min-w-0 flex-1 flex-col";
  }
  return "relative flex min-w-0 flex-1 flex-col lg:grid lg:grid-cols-[clamp(480px,var(--detail-main-width),calc(100%_-_24px_-_320px))_minmax(320px,1fr)] lg:items-start lg:gap-6";
});

const mainContentClass = computed(() => {
  if (activeSidebarThread.value || threadLeaving.value) {
    return "min-w-0 space-y-4 py-4 pr-0 lg:pt-6 lg:pb-2";
  }
  if (threadLayoutOpen.value) {
    return "min-w-0 space-y-4 py-4 pr-0 lg:py-6";
  }
  return "relative min-w-0 space-y-4 py-4 pr-0 lg:py-6 lg:pr-1";
});

const detailsSidebarClass = computed(() => {
  if (activeSidebarThread.value || threadLeaving.value) {
    return "min-w-0 space-y-4 pt-4 pb-20 md:pb-4 lg:overflow-y-auto lg:pt-2 lg:pb-6";
  }
  return "min-w-0 space-y-4 pt-4 pb-20 md:pb-4 lg:overflow-y-auto lg:py-6";
});

const shellAsideStyle = computed<CSSProperties>(() => {
  return {
    "--detail-main-width": `${mainWidth.value}px`,
  };
});

function openCoordinationThread() {
  threadLayoutOpen.value = true;
  activeSidebarThread.value = { kind: "coordination" };
  void scrollCoordinationToBottom();
}

function openTechnicalThread() {
  threadLayoutOpen.value = true;
  activeSidebarThread.value = { kind: "technical_thread" };
}

function toggleSidebarThread(kind: IncidentSidebarThread["kind"]) {
  if (activeSidebarThread.value?.kind === kind) {
    closeSidebarThread();
    return;
  }
  if (kind === "coordination") {
    openCoordinationThread();
  } else {
    openTechnicalThread();
  }
}

function closeSidebarThread() {
  activeSidebarThread.value = null;
  threadLeaving.value = true;
}

function onThreadSidebarAfterLeave() {
  if (!activeSidebarThread.value) {
    threadLayoutOpen.value = false;
  }
  threadLeaving.value = false;
}

useEscapeKey(closeSidebarThread, () => activeSidebarThread.value !== null);

function scheduleReload() {
  if (reloadDebounce) clearTimeout(reloadDebounce);
  reloadDebounce = setTimeout(() => {
    reloadDebounce = null;
    loadIncident();
  }, 1500);
}

function isIncidentEvent(data: unknown): data is { incident_number: number | string } {
  return (
    typeof data === "object" &&
    data !== null &&
    "incident_number" in data &&
    (typeof (data as Record<string, unknown>).incident_number === "number" ||
      typeof (data as Record<string, unknown>).incident_number === "string")
  );
}

function onIncidentSSE(data: unknown, handler: () => void) {
  if (isIncidentEvent(data)) {
    const num =
      typeof data.incident_number === "number"
        ? data.incident_number
        : parseInt(data.incident_number, 10);
    if (num === incidentNumber.value) handler();
  }
}

type OwnerThreadKind = "incident_inv" | "incident_coord";

function isRelevantOwnerThreadEvent(data: unknown, kind: OwnerThreadKind): boolean {
  const d = data as { owner_type?: string; owner_id?: string };
  return d.owner_type === kind && String(d.owner_id) === String(incidentNumber.value);
}

const sse = useSSE(
  "/api/v1/events",
  {
    // Incident status transitions (triaging/promoted/etc.) arrive via the
    // single incident_updated event; dedicated per-status events don't exist.
    incident_updated: (data: unknown) => onIncidentSSE(data, scheduleReload),
    // Emitted by ICSWorker after provisioning completes.
    war_room_created: (data: unknown) => onIncidentSSE(data, scheduleReload),
    ics_role_assigned: (payload: unknown) => onIncidentSSE(payload, data.loadICSRoles),
    incident_coordination_message_created: (data: unknown) => {
      onIncidentSSE(data, async () => {
        await coord.loadCoordinationMessages();
        stickCoordinationToBottom();
        fetchStatusUpdates(true);
      });
    },
    coordination_task_created: (data: unknown) => onIncidentSSE(data, coord.loadCoordinationTasks),
    coordination_task_dispatched: (data: unknown) =>
      onIncidentSSE(data, coord.loadCoordinationTasks),
    coordination_task_claimed: (data: unknown) => onIncidentSSE(data, coord.loadCoordinationTasks),
    coordination_task_completed: (data: unknown) =>
      onIncidentSSE(data, coord.loadCoordinationTasks),
    coordination_task_failed: (data: unknown) => onIncidentSSE(data, coord.loadCoordinationTasks),
    coordination_task_cancelled: (data: unknown) =>
      onIncidentSSE(data, coord.loadCoordinationTasks),
    ...thread.handlers,
    owner_thread_typing: (data: unknown) => {
      const d = data as { source?: string; agent_type?: string };
      const source = d.source ?? "agent";
      const agentType = d.agent_type;
      if (isRelevantOwnerThreadEvent(data, "incident_coord")) {
        setCoordinationTyping(source, agentType);
      } else if (isRelevantOwnerThreadEvent(data, "incident_inv")) {
        setInvestigationTyping(source, agentType);
      }
    },
    owner_thread_typing_stop: (data: unknown) => {
      if (isRelevantOwnerThreadEvent(data, "incident_coord")) {
        clearCoordinationTyping();
      } else if (isRelevantOwnerThreadEvent(data, "incident_inv")) {
        clearInvestigationTyping();
      }
    },
    investigation_typing: (data: unknown) => {
      const d = data as { source?: string; agent_type?: string };
      setInvestigationTyping(d.source ?? "agent", d.agent_type);
    },
    investigation_typing_stop: () => {
      clearInvestigationTyping();
    },
  },
  {
    onReconnect: () => scheduleReload(),
  },
);
const sseState = sse.state;

onMounted(async () => {
  loadIncident();
  void coord.loadMentionTargets(editorRef.value);
  void editor.probeIntegrations();
});

// Navigating between incident detail routes (same component, different
// :incident_number param) reuses this instance — KeepAlive + RouterView
// skip onMounted. Reset stale state and reload for the new incident.
// Guard against NaN: when navigating away from a detail route the param
// disappears and Number(undefined) is NaN — KeepAlive keeps this watcher
// alive while deactivated, so it would otherwise fire a /incidents/NaN
// request on the way out.
watch(incidentNumber, (next, prev) => {
  if (!Number.isFinite(next)) return;
  if (prev !== undefined && next !== prev) {
    resetIncidentState();
    incident.value = null;
    loadIncident();
  }
});

const incidentStartedAtText = computed(() =>
  incident.value ? formatTimeFull(incident.value.created_at) : "",
);

const timelineEventCount = computed(
  () => timeline.value.filter((e) => e.event_type !== "investigation_created").length,
);

const priorityFilledBadgeCss = computed(() => {
  switch (incident.value?.priority) {
    case "P1":
      return "rounded bg-red-500 px-2 py-0.5 text-xs font-semibold text-white";
    case "P2":
      return "rounded bg-orange-500 px-2 py-0.5 text-xs font-semibold text-white";
    case "P3":
      return "rounded bg-amber-500 px-2 py-0.5 text-xs font-semibold text-white";
    case "P4":
      return "rounded bg-blue-500 px-2 py-0.5 text-xs font-semibold text-white";
    case "P5":
      return "rounded bg-slate-500 px-2 py-0.5 text-xs font-semibold text-white";
    default:
      return "rounded bg-[var(--bg-tertiary)] px-2 py-0.5 text-xs font-semibold text-[var(--text-primary)]";
  }
});

const severityFilledBadgeCss = computed(() => {
  switch (incident.value?.severity) {
    case "critical":
      return "rounded bg-red-500 px-2 py-0.5 text-xs font-semibold text-white";
    case "high":
      return "rounded bg-orange-500 px-2 py-0.5 text-xs font-semibold text-white";
    case "warning":
      return "rounded bg-amber-500 px-2 py-0.5 text-xs font-semibold text-white";
    case "info":
      return "rounded bg-sky-500 px-2 py-0.5 text-xs font-semibold text-white";
    default:
      return "rounded bg-[var(--bg-tertiary)] px-2 py-0.5 text-xs font-semibold text-[var(--text-primary)]";
  }
});

const impactFilledBadgeCss = computed(() => {
  switch (incident.value?.impact_level) {
    case "high":
      return "rounded bg-red-500 px-2 py-0.5 text-xs font-semibold text-white";
    case "medium":
      return "rounded bg-amber-500 px-2 py-0.5 text-xs font-semibold text-white";
    case "low":
      return "rounded bg-sky-500 px-2 py-0.5 text-xs font-semibold text-white";
    default:
      return "rounded bg-[var(--bg-tertiary)] px-2 py-0.5 text-xs font-semibold text-[var(--text-primary)]";
  }
});

onBeforeUnmount(() => {
  data.reset();
  if (reloadDebounce) {
    clearTimeout(reloadDebounce);
    reloadDebounce = null;
  }
});
</script>

<template>
  <div :class="pageShellClass" :style="shellAsideStyle">
    <div
      v-if="loading && !incident"
      :class="pageContentClass"
      aria-busy="true"
      aria-label="Loading incident"
    >
      <div :class="mainContentClass">
        <div class="space-y-1.5">
          <div class="h-3 w-48 animate-pulse rounded bg-[var(--skeleton-bg)]"></div>
          <div class="flex items-center gap-1.5">
            <div class="h-4 w-10 animate-pulse rounded bg-[var(--skeleton-bg)]"></div>
            <div class="h-4 w-10 animate-pulse rounded bg-[var(--skeleton-bg)]"></div>
            <div class="h-4 w-10 animate-pulse rounded bg-[var(--skeleton-bg)]"></div>
            <div class="ml-auto h-4 w-16 animate-pulse rounded bg-[var(--skeleton-bg)]"></div>
          </div>
        </div>
        <div
          class="animate-pulse rounded border border-[var(--border-primary)] bg-[var(--bg-card)] p-4 space-y-2"
        >
          <div class="h-3 w-full rounded bg-[var(--skeleton-bg)]"></div>
          <div class="h-3 w-5/6 rounded bg-[var(--skeleton-bg)]"></div>
          <div class="h-3 w-4/6 rounded bg-[var(--skeleton-bg)]"></div>
        </div>
        <div class="grid gap-3 md:grid-cols-2">
          <div
            v-for="i in 2"
            :key="i"
            class="animate-pulse rounded border border-[var(--border-primary)] bg-[var(--bg-card)] p-4 space-y-3"
          >
            <div class="h-4 w-32 rounded bg-[var(--skeleton-bg)]"></div>
            <div class="h-10 w-full rounded bg-[var(--skeleton-bg)]"></div>
          </div>
        </div>
        <div
          class="animate-pulse rounded border border-[var(--border-primary)] bg-[var(--bg-card)] p-4 space-y-3"
        >
          <div class="h-4 w-32 rounded bg-[var(--skeleton-bg)]"></div>
          <div class="space-y-2">
            <div class="h-3 w-full rounded bg-[var(--skeleton-bg)]"></div>
            <div class="h-3 w-2/3 rounded bg-[var(--skeleton-bg)]"></div>
          </div>
        </div>
        <div
          class="animate-pulse rounded border border-[var(--border-primary)] bg-[var(--bg-card)] p-4 space-y-2"
        >
          <div class="h-4 w-40 rounded bg-[var(--skeleton-bg)]"></div>
          <div class="h-16 w-full rounded bg-[var(--skeleton-bg)]"></div>
        </div>
        <div
          v-for="i in 4"
          :key="i"
          class="animate-pulse rounded border border-[var(--border-primary)] bg-[var(--bg-card)] p-4 space-y-3"
        >
          <div class="h-4 w-32 rounded bg-[var(--skeleton-bg)]"></div>
          <div class="space-y-2">
            <div class="h-3 w-full rounded bg-[var(--skeleton-bg)]"></div>
            <div class="h-3 w-5/6 rounded bg-[var(--skeleton-bg)]"></div>
            <div class="h-3 w-3/6 rounded bg-[var(--skeleton-bg)]"></div>
          </div>
        </div>
        <div
          class="animate-pulse rounded border border-[var(--border-primary)] bg-[var(--bg-card)] p-4 space-y-2"
        >
          <div class="h-4 w-24 rounded bg-[var(--skeleton-bg)]"></div>
          <div class="space-y-2">
            <div class="h-3 w-3/4 rounded bg-[var(--skeleton-bg)]"></div>
            <div class="h-3 w-2/3 rounded bg-[var(--skeleton-bg)]"></div>
            <div class="h-3 w-1/2 rounded bg-[var(--skeleton-bg)]"></div>
          </div>
        </div>
      </div>
      <div :class="detailsSidebarClass">
        <div
          class="animate-pulse rounded border border-[var(--border-primary)] bg-[var(--bg-card)] p-4 space-y-3"
        >
          <div class="h-4 w-32 rounded bg-[var(--skeleton-bg)]"></div>
          <div class="space-y-2">
            <div v-for="i in 4" :key="i" class="h-10 w-full rounded bg-[var(--skeleton-bg)]"></div>
          </div>
        </div>
        <div
          class="animate-pulse rounded border border-[var(--border-primary)] bg-[var(--bg-card)] p-4 space-y-3"
        >
          <div class="h-4 w-32 rounded bg-[var(--skeleton-bg)]"></div>
          <div class="space-y-2">
            <div v-for="i in 3" :key="i" class="h-8 w-full rounded bg-[var(--skeleton-bg)]"></div>
          </div>
        </div>
        <div
          class="animate-pulse rounded border border-[var(--border-primary)] bg-[var(--bg-card)] p-4 space-y-3"
        >
          <div class="h-4 w-24 rounded bg-[var(--skeleton-bg)]"></div>
          <div class="h-3 w-3/4 rounded bg-[var(--skeleton-bg)]"></div>
        </div>
      </div>
    </div>
    <ErrorBanner v-else-if="error && !incident" :message="error" />
    <template v-else-if="incident">
      <div :class="pageContentClass">
        <div
          v-if="activeSidebarThread || threadLeaving"
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

        <!-- Main content area -->
        <div :class="mainContentClass">
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
          <div
            v-if="isDeleted"
            class="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-2 text-sm text-[var(--text-muted)]"
            role="status"
          >
            This incident was deleted and is shown read-only.
          </div>
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

          <!-- Timestamp + priority/severity/impact badge + status badge row -->
          <div class="space-y-1.5 text-xs text-[var(--text-muted)]">
            <div class="flex items-center gap-1">
              <Clock class="h-3 w-3" aria-hidden="true" />
              <span>{{ incidentStartedAtText }}</span>
            </div>
            <div class="flex items-center">
              <div class="flex items-center gap-1.5">
                <span
                  v-if="incident.priority"
                  class="shrink-0 uppercase"
                  :class="priorityFilledBadgeCss"
                >
                  <span class="sr-only">Priority:</span>
                  {{ incident.priority }}
                </span>
                <span
                  v-if="incident.severity"
                  class="shrink-0 uppercase"
                  :class="severityFilledBadgeCss"
                >
                  <span class="sr-only">Severity:</span>
                  {{ incident.severity }}
                </span>
                <span
                  v-if="incident.impact_level"
                  class="shrink-0 uppercase"
                  :class="impactFilledBadgeCss"
                >
                  <span class="sr-only">Impact:</span>
                  {{ incident.impact_level }}
                </span>
              </div>
              <div class="ml-auto flex items-center gap-1.5">
                <span
                  class="shrink-0 uppercase"
                  :class="[incidentStatusBadgeClass(incident.status)]"
                  aria-live="polite"
                >
                  <span class="sr-only">Status:</span>
                  {{ incidentStatusLabel(incident.status) }}
                </span>
              </div>
            </div>
          </div>
          <Card v-if="incident.description || incident.tags?.length" class="text-sm">
            <MarkdownRenderer
              v-if="incident.description"
              :content="incident.description"
              class="text-sm text-[var(--text-secondary)]"
            />
            <div v-if="incident.tags?.length" class="mt-2 flex flex-wrap gap-1">
              <span
                v-for="tag in incident.tags"
                :key="tag"
                class="rounded-full bg-[var(--bg-secondary)] px-2 py-0.5 text-xs text-[var(--text-muted)]"
              >
                {{ tag }}
              </span>
            </div>
          </Card>

          <div class="grid gap-3 md:grid-cols-2">
            <div class="rounded border border-[var(--border-primary)] bg-[var(--bg-secondary)]">
              <div class="rounded-t">
                <div class="flex items-center justify-between gap-2 px-4 py-3">
                  <div class="flex min-w-0 items-center gap-2">
                    <h3 class="field-label mb-0">
                      <MessageSquare
                        class="inline h-4 w-4 align-text-bottom text-[var(--text-muted)]"
                      />
                      Coordination
                    </h3>
                  </div>
                </div>

                <button
                  type="button"
                  class="flex w-full cursor-pointer items-center justify-between gap-3 border-t border-[var(--border-primary)] px-4 py-3 text-left transition-colors hover:bg-[var(--btn-default-hover)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
                  :aria-expanded="activeSidebarThread?.kind === 'coordination'"
                  aria-controls="incident-thread-drawer"
                  @click="toggleSidebarThread('coordination')"
                >
                  <div class="flex min-w-0 items-center gap-2 text-sm">
                    <div class="flex -space-x-1.5">
                      <div
                        v-for="participant in coordinationParticipants.slice(0, 3)"
                        :key="participant.key"
                        class="flex h-5 w-5 items-center justify-center overflow-hidden rounded-full border border-[var(--bg-secondary)] bg-[var(--bg-tertiary)] text-[10px] font-semibold text-[var(--text-secondary)]"
                        :title="participant.name"
                      >
                        <img
                          v-if="participant.avatarSrc"
                          :src="participant.avatarSrc"
                          :alt="participant.name"
                          class="h-full w-full object-cover rounded-full"
                          loading="lazy"
                          decoding="async"
                        />
                        <span v-else>{{ participantInitial(participant.name) }}</span>
                      </div>
                      <div
                        v-if="coordinationParticipants.length === 0"
                        class="flex h-5 w-5 items-center justify-center rounded-full bg-[var(--bg-tertiary)] text-[var(--text-muted)]"
                      >
                        <MessageSquare class="h-3 w-3" />
                      </div>
                    </div>
                    <span class="truncate font-semibold text-[var(--text-secondary)]">
                      {{ coordinationParticipantLabel }}
                    </span>
                    <TypingIndicator v-if="coordinationTyping" class="shrink-0" />
                  </div>
                  <div class="flex shrink-0 items-center gap-2">
                    <span
                      v-if="coordinationMessages.length > 0"
                      class="flex items-center gap-1 text-xs text-[var(--text-muted)]"
                    >
                      <MessageSquare class="h-3 w-3" />
                      {{ coordinationMessages.length }}
                    </span>
                    <ChevronRight
                      class="h-5 w-5 text-[var(--text-muted)] transition-transform duration-200"
                      :class="activeSidebarThread?.kind === 'coordination' ? 'rotate-180' : ''"
                    />
                  </div>
                </button>
              </div>
            </div>

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
                  class="flex w-full cursor-pointer items-center justify-between gap-3 border-t border-[var(--border-primary)] px-4 py-3 text-left transition-colors hover:bg-[var(--btn-default-hover)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
                  :aria-expanded="activeSidebarThread?.kind === 'technical_thread'"
                  aria-controls="incident-thread-drawer"
                  @click="toggleSidebarThread('technical_thread')"
                >
                  <div class="flex min-w-0 items-center gap-2 text-sm">
                    <div class="flex -space-x-1.5">
                      <div
                        v-for="participant in investigationParticipants.slice(0, 3)"
                        :key="participant.key"
                        class="flex h-5 w-5 items-center justify-center overflow-hidden rounded-full border border-[var(--bg-secondary)] bg-[var(--bg-tertiary)] text-[10px] font-semibold text-[var(--text-secondary)]"
                        :title="participant.name"
                      >
                        <img
                          v-if="participant.avatarSrc"
                          :src="participant.avatarSrc"
                          :alt="participant.name"
                          class="h-full w-full object-cover rounded-full"
                          loading="lazy"
                          decoding="async"
                        />
                        <span v-else>{{ participantInitial(participant.name) }}</span>
                      </div>
                      <div
                        v-if="investigationParticipants.length === 0"
                        class="flex h-5 w-5 items-center justify-center rounded-full bg-[var(--bg-tertiary)] text-[var(--text-muted)]"
                      >
                        <HatGlasses class="h-3 w-3" />
                      </div>
                    </div>
                    <span class="truncate font-semibold text-[var(--text-secondary)]">
                      {{ investigationParticipantLabel }}
                    </span>
                    <TypingIndicator v-if="investigationTyping" class="shrink-0" />
                  </div>
                  <div class="flex shrink-0 items-center gap-2">
                    <span
                      v-if="thread.incidentThreadMessageCount > 0"
                      class="flex items-center gap-1 text-xs text-[var(--text-muted)]"
                    >
                      <MessageSquare class="h-3 w-3" />
                      {{ thread.incidentThreadMessageCount }}
                    </span>
                    <ChevronRight
                      class="h-5 w-5 text-[var(--text-muted)] transition-transform duration-200"
                      :class="activeSidebarThread?.kind === 'technical_thread' ? 'rotate-180' : ''"
                    />
                  </div>
                </button>
              </div>
            </div>
          </div>

          <StatusUpdateFeed
            :incident-id="String(incidentNumber)"
            :updates="statusUpdates"
            :can-post="canPostStatusUpdate"
            :loading="statusUpdatesLoading"
            :error="statusUpdatesError"
            :incident-status="incident?.status"
            @posted="fetchStatusUpdates(true)"
            @retry="() => fetchStatusUpdates()"
          />

          <CoordinationTaskBoard
            v-if="incident"
            :tasks="coordinationTasks"
            :can-command="canCommand && !isDeleted"
            @dispatch="coord.openDispatchTaskDialog"
            @cancel="coord.cancelCoordinationTask"
          />

          <!-- Summary -->
          <Card class="hover:shadow-md transition-all duration-300">
            <div
              class="mb-3 flex items-center justify-between gap-2 border-b border-[var(--border-primary)] pb-2"
            >
              <div class="flex items-center gap-2">
                <FileText class="h-4 w-4 text-[var(--text-secondary)]" />
                <h3 class="text-sm font-semibold text-[var(--text-primary)]">Summary</h3>
              </div>
              <button
                v-if="canWrite && !summaryEditing && !isDeleted"
                type="button"
                :class="CARD_ICON_BTN_CLASS"
                title="Edit summary"
                @click="docs.startEditSummary"
              >
                <Pencil class="h-3.5 w-3.5" />
              </button>
            </div>
            <MarkdownEditor
              v-if="summaryEditing"
              v-model="summaryContent"
              :disabled="summarySaving"
              :users="users"
              :agents="agents"
              :enable-internal-note="false"
              :show-send-button="false"
              placeholder="Write an executive summary (the cause, why it started, what it did, and status until recovery)... (markdown supported)"
            />
            <MarkdownRenderer
              v-else-if="summaryContent.trim()"
              :content="summaryContent"
              class="text-sm text-[var(--text-secondary)]"
            />
            <p v-else class="text-sm text-[var(--text-muted)]">No executive summary recorded.</p>
            <div v-if="summaryEditing" class="flex justify-end gap-2 mt-2">
              <Button
                variant="outline"
                size="sm"
                :disabled="summarySaving"
                @click="summaryEditing = false"
              >
                Cancel
              </Button>
              <Button size="sm" :disabled="summarySaving" @click="docs.saveSummary">
                <Save class="h-3.5 w-3.5" />
                {{ summarySaving ? "Saving..." : "Save" }}
              </Button>
            </div>
          </Card>

          <!-- Root Cause -->
          <Card class="hover:shadow-md transition-all duration-300">
            <div
              class="mb-3 flex items-center justify-between gap-2 border-b border-[var(--border-primary)] pb-2"
            >
              <div class="flex items-center gap-2">
                <ShieldAlert class="h-4 w-4 text-[var(--text-secondary)]" />
                <h3 class="text-sm font-semibold text-[var(--text-primary)]">Root Cause</h3>
              </div>
              <button
                v-if="canCommand && !rootCauseEditing && !isDeleted"
                type="button"
                :class="CARD_ICON_BTN_CLASS"
                title="Edit root cause"
                @click="docs.startEditRootCause"
              >
                <Pencil class="h-3.5 w-3.5" />
              </button>
            </div>
            <MarkdownEditor
              v-if="rootCauseEditing"
              v-model="rootCauseContent"
              :disabled="rootCauseSaving"
              :users="users"
              :agents="agents"
              :enable-internal-note="false"
              :show-send-button="false"
              placeholder="Describe the root cause of this incident... (markdown supported)"
            />
            <MarkdownRenderer
              v-else-if="rootCauseContent.trim()"
              :content="rootCauseContent"
              class="text-sm text-[var(--text-secondary)]"
            />
            <p v-else class="text-sm text-[var(--text-muted)]">No root cause recorded.</p>
            <div v-if="rootCauseEditing" class="flex justify-end gap-2 mt-2">
              <Button
                variant="outline"
                size="sm"
                :disabled="rootCauseSaving"
                @click="rootCauseEditing = false"
              >
                Cancel
              </Button>
              <Button size="sm" :disabled="rootCauseSaving" @click="docs.saveRootCause">
                <Save class="h-3.5 w-3.5" />
                {{ rootCauseSaving ? "Saving..." : "Save" }}
              </Button>
            </div>
          </Card>

          <!-- Resolution -->
          <Card class="hover:shadow-md transition-all duration-300">
            <div
              class="mb-3 flex items-center justify-between gap-2 border-b border-[var(--border-primary)] pb-2"
            >
              <div class="flex items-center gap-2">
                <Wrench class="h-4 w-4 text-[var(--text-secondary)]" />
                <h3 class="text-sm font-semibold text-[var(--text-primary)]">Resolution</h3>
              </div>
              <button
                v-if="canCommand && !resolutionEditing && !isDeleted"
                type="button"
                :class="CARD_ICON_BTN_CLASS"
                title="Edit resolution"
                @click="docs.startEditResolution"
              >
                <Pencil class="h-3.5 w-3.5" />
              </button>
            </div>
            <MarkdownEditor
              v-if="resolutionEditing"
              v-model="resolutionContent"
              :disabled="resolutionSaving"
              :users="users"
              :agents="agents"
              :enable-internal-note="false"
              :show-send-button="false"
              placeholder="Describe the resolution for this incident... (markdown supported)"
            />
            <MarkdownRenderer
              v-else-if="resolutionContent.trim()"
              :content="resolutionContent"
              class="text-sm text-[var(--text-secondary)]"
            />
            <p v-else class="text-sm text-[var(--text-muted)]">No resolution recorded.</p>
            <div v-if="resolutionEditing" class="flex justify-end gap-2 mt-2">
              <Button
                variant="outline"
                size="sm"
                :disabled="resolutionSaving"
                @click="resolutionEditing = false"
              >
                Cancel
              </Button>
              <Button size="sm" :disabled="resolutionSaving" @click="docs.saveResolution">
                <Save class="h-3.5 w-3.5" />
                {{ resolutionSaving ? "Saving..." : "Save" }}
              </Button>
            </div>
          </Card>

          <!-- Impact Assessment -->
          <Card class="hover:shadow-md transition-all duration-300">
            <div
              class="mb-3 flex items-center justify-between gap-2 border-b border-[var(--border-primary)] pb-2"
            >
              <div class="flex items-center gap-2">
                <CircleAlert class="h-4 w-4 text-[var(--text-secondary)]" />
                <h3 class="text-sm font-semibold text-[var(--text-primary)]">Impact Assessment</h3>
              </div>
              <button
                v-if="canCommand && !impactEditing && !isDeleted"
                type="button"
                :class="CARD_ICON_BTN_CLASS"
                title="Edit impact assessment"
                @click="docs.startEditImpact"
              >
                <Pencil class="h-3.5 w-3.5" />
              </button>
            </div>
            <MarkdownEditor
              v-if="impactEditing"
              v-model="impactContent"
              :disabled="impactSaving"
              :users="users"
              :agents="agents"
              :enable-internal-note="false"
              :show-send-button="false"
              placeholder="Describe the impact of this incident... (markdown supported)"
            />
            <MarkdownRenderer
              v-else-if="impactContent.trim()"
              :content="impactContent"
              class="text-sm text-[var(--text-secondary)]"
            />
            <p v-else class="text-sm text-[var(--text-muted)]">No impact assessment recorded.</p>
            <div v-if="impactEditing" class="flex justify-end gap-2 mt-2">
              <Button
                variant="outline"
                size="sm"
                :disabled="impactSaving"
                @click="impactEditing = false"
              >
                Cancel
              </Button>
              <Button size="sm" :disabled="impactSaving" @click="docs.saveImpact">
                <Save class="h-3.5 w-3.5" />
                {{ impactSaving ? "Saving..." : "Save" }}
              </Button>
            </div>
          </Card>

          <!-- Timeline -->
          <Card class="hover:shadow-md transition-all duration-300">
            <div
              class="flex items-center justify-between gap-2 pb-2"
              :class="
                timelineCollapsed
                  ? 'mb-0 border-b-0'
                  : 'mb-3 border-b border-[var(--border-primary)]'
              "
            >
              <button
                type="button"
                class="flex min-w-0 items-center gap-2 rounded-md text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
                :aria-expanded="!timelineCollapsed"
                aria-controls="incident-timeline-body"
                @click="timelineCollapsed = !timelineCollapsed"
              >
                <ChevronDown
                  class="h-4 w-4 shrink-0 text-[var(--text-muted)] transition-transform duration-200"
                  :class="timelineCollapsed ? '-rotate-90' : ''"
                />
                <Clock class="h-4 w-4 text-[var(--text-secondary)]" />
                <h3 class="text-sm font-semibold text-[var(--text-primary)]">Timeline</h3>
                <span
                  v-if="timelineEventCount"
                  class="rounded-full bg-[var(--bg-secondary)] px-1.5 py-0.5 text-[10px] font-semibold tabular-nums text-[var(--text-muted)]"
                >
                  {{ timelineEventCount }}
                </span>
              </button>
              <button
                v-if="canWrite && !isDeleted"
                type="button"
                class="flex h-8 w-8 shrink-0 cursor-pointer items-center justify-center rounded-md text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)]"
                title="Add timeline entry"
                @click="editor.openAddTimelineDialog"
              >
                <Plus class="h-3.5 w-3.5" />
              </button>
            </div>
            <div
              id="incident-timeline-body"
              class="grid transition-all duration-300 ease-out"
              :class="timelineCollapsed ? 'grid-rows-[0fr]' : 'grid-rows-[1fr]'"
            >
              <div class="overflow-hidden">
                <IncidentTimeline :entries="timeline" />
              </div>
            </div>
          </Card>
        </div>

        <!-- Details sidebar -->
        <div :class="detailsSidebarClass">
          <ICSRoleBoard
            :roles="icsRoles"
            :incident-id="String(incident.incident_number)"
            @reload-roles="data.loadICSRoles"
          />

          <!-- Mitigation playbooks -->
          <Card v-if="mitigationPlaybooks.length > 0">
            <div class="mb-3 flex items-center gap-2">
              <BookOpen class="h-4 w-4 text-[var(--text-muted)]" />
              <h3 class="text-sm font-medium text-[var(--text-primary)]">Mitigations</h3>
            </div>
            <div class="space-y-2">
              <div
                v-for="pb in mitigationPlaybooks"
                :key="pb.id"
                class="rounded-md border border-[var(--border-primary)]"
              >
                <button
                  type="button"
                  class="flex w-full cursor-pointer items-center justify-between px-3 py-2 text-left"
                  @click="togglePlaybook(pb.id)"
                >
                  <div>
                    <span class="text-sm font-medium text-[var(--text-primary)]">{{
                      pb.title
                    }}</span>
                    <span class="ml-2 text-xs text-[var(--text-muted)]">
                      {{ pb.steps?.length ?? 0 }} step{{ (pb.steps?.length ?? 0) !== 1 ? "s" : "" }}
                    </span>
                  </div>
                  <ChevronDown
                    v-if="expandedPlaybookId !== pb.id"
                    class="h-4 w-4 text-[var(--text-muted)]"
                  />
                  <ChevronUp v-else class="h-4 w-4 text-[var(--text-muted)]" />
                </button>
                <div
                  v-if="expandedPlaybookId === pb.id && pb.steps?.length"
                  class="border-t border-[var(--border-primary)] px-3 py-2"
                >
                  <ol class="space-y-3">
                    <li v-for="step in pb.steps" :key="step.id">
                      <div class="flex items-start gap-2">
                        <span
                          class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-[var(--bg-secondary)] text-xs font-medium text-[var(--text-muted)]"
                        >
                          {{ step.step_number }}
                        </span>
                        <div class="min-w-0 flex-1">
                          <p class="text-sm font-medium text-[var(--text-primary)]">
                            {{ step.title }}
                          </p>
                          <p
                            v-if="step.description"
                            class="mt-0.5 text-xs text-[var(--text-secondary)]"
                          >
                            {{ step.description }}
                          </p>
                          <code
                            v-if="step.command"
                            class="mt-1 block rounded bg-[var(--bg-secondary)] px-2 py-1 text-xs text-[var(--text-muted)]"
                            >{{ step.command }}</code
                          >
                        </div>
                      </div>
                    </li>
                  </ol>
                </div>
              </div>
            </div>
          </Card>

          <!-- Slack channel -->
          <Card v-if="editor.isSlackConfigured">
            <div class="mb-3 flex items-center justify-between gap-2">
              <h3 class="text-sm font-medium text-[var(--text-primary)]">Slack</h3>
            </div>
            <div class="flex items-center gap-2">
              <a
                v-if="incident.slack_channel_id"
                :href="`https://slack.com/app_redirect?channel=${encodeURIComponent(incident.slack_channel_id)}`"
                target="_blank"
                rel="noopener noreferrer"
                class="flex min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2 text-left text-sm text-[var(--text-primary)] transition-colors hover:bg-[var(--btn-default-hover)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
                :title="`Open #${incident.slack_channel_name}`"
              >
                <img
                  :src="getProviderIconSrc('slack')"
                  alt=""
                  class="h-5 w-5 shrink-0 rounded-sm"
                  loading="lazy"
                  decoding="async"
                />
                <span class="min-w-0 flex-1 truncate font-medium">
                  #{{ incident.slack_channel_name }}
                </span>
                <ExternalLink class="ml-auto h-3.5 w-3.5 shrink-0 text-[var(--text-muted)]" />
              </a>
              <button
                v-else-if="canCommand && !isDeleted"
                type="button"
                class="flex min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2 text-left text-sm text-[var(--text-primary)] transition-colors hover:bg-[var(--btn-default-hover)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)] disabled:cursor-not-allowed disabled:opacity-50"
                :disabled="editor.creatingChannel"
                @click="editor.createChannel"
              >
                <img
                  :src="getProviderIconSrc('slack')"
                  alt=""
                  class="h-5 w-5 shrink-0 rounded-sm"
                  loading="lazy"
                  decoding="async"
                />
                <span class="min-w-0 flex-1 truncate font-medium">
                  {{ editor.creatingChannel ? "Creating..." : "Create Slack Channel" }}
                </span>
              </button>
              <button
                v-if="incident.slack_channel_id && canCommand && !isDeleted"
                type="button"
                class="inline-flex h-9 w-9 shrink-0 cursor-pointer items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)] transition-colors hover:bg-[var(--btn-default-hover)] hover:text-[var(--text-primary)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
                title="Unlink Slack channel"
                @click="editor.unlinkChannel"
              >
                <X class="h-3.5 w-3.5" />
              </button>
            </div>
          </Card>

          <!-- Google Meet -->
          <Card v-if="editor.isGoogleMeetConfigured">
            <div class="mb-3 flex items-center justify-between gap-2">
              <h3 class="text-sm font-medium text-[var(--text-primary)]">Google Meet</h3>
            </div>
            <div class="flex items-center gap-2">
              <a
                v-if="incident.google_meet_space_name"
                :href="incident.conference_url"
                target="_blank"
                rel="noopener noreferrer"
                class="flex min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2 text-left text-sm text-[var(--text-primary)] transition-colors hover:bg-[var(--btn-default-hover)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
                title="Join Google Meet"
              >
                <img
                  :src="googleMeetBrandIcon"
                  alt=""
                  class="h-5 w-5 shrink-0 rounded-sm"
                  loading="lazy"
                  decoding="async"
                />
                <span class="min-w-0 flex-1 truncate font-medium">Join Google Meet</span>
                <ExternalLink class="ml-auto h-3.5 w-3.5 shrink-0 text-[var(--text-muted)]" />
              </a>
              <button
                v-else-if="canCommand && !isDeleted"
                type="button"
                class="flex min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2 text-left text-sm text-[var(--text-primary)] transition-colors hover:bg-[var(--btn-default-hover)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)] disabled:cursor-not-allowed disabled:opacity-50"
                :disabled="editor.creatingMeet"
                @click="editor.createMeet"
              >
                <img
                  :src="googleMeetBrandIcon"
                  alt=""
                  class="h-5 w-5 shrink-0 rounded-sm"
                  loading="lazy"
                  decoding="async"
                />
                <span class="min-w-0 flex-1 truncate font-medium">
                  {{ editor.creatingMeet ? "Creating..." : "Create Google Meet" }}
                </span>
              </button>
              <button
                v-if="incident.google_meet_space_name && canCommand && !isDeleted"
                type="button"
                class="inline-flex h-9 w-9 shrink-0 cursor-pointer items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)] transition-colors hover:bg-[var(--btn-default-hover)] hover:text-[var(--text-primary)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
                title="Unlink Google Meet"
                @click="editor.unlinkMeet"
              >
                <X class="h-3.5 w-3.5" />
              </button>
            </div>
          </Card>

          <!-- SLA targets -->
          <Card v-if="incident.sla_target_respond_at || incident.sla_target_resolve_at">
            <h3 class="field-label mb-3">SLA</h3>
            <div class="space-y-1 text-xs text-[var(--text-muted)]">
              <p v-if="incident.sla_target_respond_at">
                Respond by: {{ formatTime(incident.sla_target_respond_at) }}
                <span v-if="incident.sla_acknowledged_at" class="text-[var(--text-success)]"
                  >(met)</span
                >
              </p>
              <p v-if="incident.sla_target_resolve_at">
                Resolve by: {{ formatTime(incident.sla_target_resolve_at) }}
                <span v-if="incident.sla_resolved_at" class="text-[var(--text-success)]"
                  >(met)</span
                >
              </p>
            </div>
          </Card>

          <!-- Linked Alerts -->
          <Card>
            <div class="mb-3 flex items-center justify-between gap-2">
              <h3 class="text-sm font-medium text-[var(--text-primary)]">Linked Alerts</h3>
              <Button
                v-if="canWrite && !isDeleted"
                variant="outline"
                size="sm"
                @click="editor.openLinkAlertDialog"
              >
                <Plus class="h-3.5 w-3.5" />
                Link
              </Button>
            </div>
            <div v-if="alerts.length === 0" class="text-xs text-[var(--text-muted)]">
              No linked alerts.
            </div>
            <div v-else class="space-y-2">
              <div
                v-for="alert in alerts"
                :key="alert.alert_number ?? alert.fingerprint"
                class="flex items-center gap-3 rounded-md border border-[var(--border-primary)] px-3 py-2 transition-colors"
                :class="
                  alert.deleted_at
                    ? 'cursor-default opacity-50 italic'
                    : 'cursor-pointer hover:bg-[var(--bg-secondary)]'
                "
              >
                <component
                  :is="alert.deleted_at ? 'div' : RouterLink"
                  :to="alert.deleted_at ? undefined : `/alerts/${alert.alert_number}`"
                  class="flex min-w-0 flex-1 items-center gap-3"
                >
                  <CircleDot
                    class="h-4 w-4 shrink-0"
                    :style="{ color: severityBorderColor(alertSeverityLabel(alert.labels)) }"
                  />
                  <div class="min-w-0 flex-1">
                    <span
                      v-if="alert.alert_number != null && alert.alert_number > 0"
                      class="font-mono text-xs text-[var(--text-muted)] mr-1.5"
                    >
                      #{{ alert.alert_number }}
                    </span>
                    <span class="text-sm text-[var(--text-primary)]">{{
                      alert.labels?.alertname || alert.fingerprint
                    }}</span>
                  </div>
                  <AlertStatusBadge :status="alert.status" class="shrink-0" />
                  <DeletedBadge
                    v-if="alert.deleted_at"
                    class="shrink-0"
                    title="This alert was deleted"
                  />
                </component>
                <button
                  v-if="canWrite && alert.alert_number && !alert.deleted_at"
                  type="button"
                  class="shrink-0 inline-flex h-7 w-7 cursor-pointer items-center justify-center rounded-md text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)]"
                  title="Unlink alert"
                  @click="unlinkAlertTarget = alert"
                >
                  <Unlink class="h-3.5 w-3.5" />
                </button>
              </div>
            </div>
          </Card>

          <!-- Post-Mortem -->
          <Card v-if="canReadPostmortem">
            <div class="mb-3 flex items-center justify-between gap-2">
              <h3 class="text-sm font-medium text-[var(--text-primary)]">Post-Mortem</h3>
              <Button
                v-if="postMortemStatus || canCreatePostMortem"
                variant="outline"
                size="sm"
                :disabled="postMortemOpening"
                @click="data.openPostMortem"
              >
                <FileText class="h-3.5 w-3.5" />
                {{ postMortemOpening ? "Opening..." : postMortemStatus ? "View" : "Create" }}
              </Button>
            </div>
            <div v-if="postMortemStatus" class="space-y-1.5">
              <button
                v-if="postMortemTitle"
                type="button"
                class="block w-full text-left text-sm font-medium text-[var(--text-primary)] transition-colors hover:text-[var(--accent)]"
                :title="postMortemTitle"
                @click="data.openPostMortem"
              >
                {{ postMortemTitle }}
              </button>
              <div class="flex items-center gap-2">
                <span :class="['badge', postMortemStatusBadgeClass(postMortemStatus)]">
                  {{ postMortemStatus.replaceAll("_", " ") }}
                </span>
              </div>
            </div>
            <p v-else class="text-xs text-[var(--text-muted)]">No post-mortem created yet.</p>
          </Card>
        </div>
      </div>

      <Transition name="investigation-sidebar" @after-leave="onThreadSidebarAfterLeave">
        <aside
          v-if="activeSidebarThread"
          id="incident-thread-drawer"
          class="fixed inset-x-0 top-0 z-50 flex h-dvh min-w-0 flex-col overflow-hidden border border-[var(--border-primary)] bg-[var(--bg-primary)] shadow-sm lg:sticky lg:inset-auto lg:top-0 lg:z-auto lg:col-start-2 lg:h-[calc(100vh-3.5rem)] lg:max-h-[calc(100vh-3.5rem)] lg:min-h-0 lg:self-start lg:rounded-none lg:-mr-6"
          role="complementary"
          :aria-label="
            activeSidebarThread?.kind === 'coordination'
              ? 'Coordination thread'
              : 'Incident investigation thread'
          "
        >
          <OwnerThreadPanel
            v-if="activeSidebarThread?.kind === 'technical_thread' && incident"
            owner-type="incident_inv"
            :owner-id="String(incident.incident_number)"
            title="Investigation"
            :can-write="canWrite"
            :users="users"
            :agents="agents"
            empty-title="No investigation thread yet"
            empty-description="Investigation notes and responder messages will appear here once the thread starts."
            class="min-h-0 flex-1 overflow-hidden"
            @close="closeSidebarThread"
            @updated="thread.setThread"
          />

          <div
            v-else-if="activeSidebarThread?.kind === 'coordination'"
            class="flex min-h-0 flex-1 flex-col"
          >
            <header
              class="flex shrink-0 items-center justify-between gap-3 border-b border-[var(--border-primary)] px-4 py-2"
            >
              <div class="flex min-w-0 items-center gap-2">
                <h2 id="incident-coordination-drawer-title" class="field-label mb-0">
                  <MessageSquare
                    class="inline h-4 w-4 align-text-bottom text-[var(--text-muted)]"
                  />
                  COORDINATION
                </h2>
                <TypingIndicator v-if="coordinationTyping" />
              </div>
              <button
                type="button"
                class="flex h-7 w-7 shrink-0 cursor-pointer items-center justify-center rounded-md text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
                aria-label="Close coordination thread"
                @click="closeSidebarThread"
              >
                <X class="h-4 w-4" />
              </button>
            </header>
            <div
              ref="coordinationThreadEl"
              class="min-h-0 flex-1 overflow-y-auto px-3 pt-2 pb-28 md:pb-2"
            >
              <IncidentCoordinationStream :messages="coordinationMessages" />
              <ChatTypingIndicator
                v-if="coordinationTyping"
                :avatar-src="getAgentAvatarSrc(coordinationTypingAgentType ?? undefined)"
                avatar-bg="bg-transparent"
                :avatar-title="coordinationTypingSource ?? 'Agent'"
                :display-name="coordinationTypingSource ?? 'Agent'"
                class="mt-1"
              />
            </div>
            <ChatEditorBar v-if="canWrite && !isDeleted" class="!mt-0 !pt-0">
              <MarkdownEditor
                ref="editorRef"
                v-model="coordinationText"
                :disabled="coordinationSubmitting || !canWrite"
                :users="users"
                :agents="agents"
                placeholder="Coordinate the incident..."
                @submit="coord.submitCoordinationMessage(false, editorRef ?? null)"
                @submit-internal="coord.submitCoordinationMessage(true, editorRef ?? null)"
              />
            </ChatEditorBar>
          </div>
        </aside>
      </Transition>

      <Modal
        :open="editor.showEditDialog"
        title="Edit incident"
        max-width="xl"
        :prevent-close="editor.editSubmitting"
        @update:open="!$event && (editor.showEditDialog = false)"
        @close="editor.showEditDialog = false"
      >
        <form class="space-y-4" @submit.prevent="editor.submitEdit">
          <ErrorBanner :message="editor.editError" />
          <div>
            <label
              for="edit-incident-title-input"
              class="mb-1.5 block text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]"
              >Title <span class="text-[var(--text-error)]">*</span></label
            >
            <Input
              id="edit-incident-title-input"
              v-model="editor.editTitle"
              required
              :disabled="editor.editSubmitting"
            />
          </div>
          <div>
            <label
              for="edit-incident-desc"
              class="mb-1.5 block text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]"
              >Description</label
            >
            <Textarea
              id="edit-incident-desc"
              v-model="editor.editDescription"
              rows="3"
              class="min-h-[4.5rem] w-full resize-y"
              :disabled="editor.editSubmitting"
            />
          </div>
          <div>
            <label
              for="edit-incident-severity"
              class="mb-1.5 block text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]"
              >Severity</label
            >
            <Select
              id="edit-incident-severity"
              v-model="editor.editSeverity"
              class="w-full"
              :disabled="editor.editSubmitting"
            >
              <option value="critical">Critical</option>
              <option value="high">High</option>
              <option value="warning">Warning</option>
              <option value="info">Info</option>
            </Select>
          </div>
          <div>
            <label
              for="edit-incident-impact"
              class="mb-1.5 block text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]"
              >Impact</label
            >
            <Select
              id="edit-incident-impact"
              v-model="editor.editImpact"
              class="w-full"
              :disabled="editor.editSubmitting"
            >
              <option value="">No impact</option>
              <option value="high">High</option>
              <option value="medium">Medium</option>
              <option value="low">Low</option>
            </Select>
          </div>
          <div>
            <label
              for="edit-incident-priority"
              class="mb-1.5 block text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]"
              >Priority</label
            >
            <Select
              id="edit-incident-priority"
              v-model="editor.editPriority"
              class="w-full"
              :disabled="editor.editSubmitting"
            >
              <option value="">No priority</option>
              <option value="P1">P1 — Critical</option>
              <option value="P2">P2 — High</option>
              <option value="P3">P3 — Medium</option>
              <option value="P4">P4 — Low</option>
              <option value="P5">P5 — Minimal</option>
            </Select>
          </div>
        </form>
        <template #footer>
          <Button
            variant="outline"
            :disabled="editor.editSubmitting"
            @click="editor.showEditDialog = false"
            >Cancel</Button
          >
          <Button variant="primary" :loading="editor.editSubmitting" @click="editor.submitEdit">
            Save
          </Button>
        </template>
      </Modal>

      <Modal
        :open="editor.showLinkAlertDialog"
        title="Link Alert"
        max-width="lg"
        :prevent-close="editor.linkAlertSubmitting"
        @update:open="!$event && (editor.showLinkAlertDialog = false)"
        @close="editor.showLinkAlertDialog = false"
      >
        <form class="space-y-4" @submit.prevent="editor.submitLinkAlert">
          <ErrorBanner :message="editor.linkAlertError" />
          <div>
            <label
              for="link-alert-number"
              class="mb-1.5 block text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]"
              >Alert Number <span class="text-[var(--text-error)]">*</span></label
            >
            <NumberInput
              id="link-alert-number"
              v-model="editor.linkAlertNumber"
              required
              :disabled="editor.linkAlertSubmitting"
              placeholder="e.g. 42"
            />
          </div>
        </form>
        <template #footer>
          <Button
            variant="outline"
            :disabled="editor.linkAlertSubmitting"
            @click="editor.showLinkAlertDialog = false"
            >Cancel</Button
          >
          <Button
            variant="primary"
            :loading="editor.linkAlertSubmitting"
            @click="editor.submitLinkAlert"
          >
            Link
          </Button>
        </template>
      </Modal>

      <Modal
        :open="editor.showAddTimelineDialog"
        title="Add Timeline Entry"
        max-width="lg"
        :prevent-close="editor.timelineSubmitting"
        @update:open="!$event && (editor.showAddTimelineDialog = false)"
        @close="editor.showAddTimelineDialog = false"
      >
        <form class="space-y-4" @submit.prevent="editor.submitTimelineEntry">
          <ErrorBanner :message="editor.timelineError" />
          <div>
            <label
              for="timeline-event-type"
              class="mb-1.5 block text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]"
              >Event Type</label
            >
            <Select
              id="timeline-event-type"
              v-model="editor.timelineEventType"
              class="w-full"
              :disabled="editor.timelineSubmitting"
            >
              <option value="manual">Manual</option>
              <option value="comment">Comment</option>
              <option value="status_changed">Status Changed</option>
              <option value="note">Note</option>
            </Select>
          </div>
          <div>
            <label
              for="timeline-message"
              class="mb-1.5 block text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]"
              >Message <span class="text-[var(--text-error)]">*</span></label
            >
            <Textarea
              id="timeline-message"
              v-model="editor.timelineMessage"
              rows="3"
              class="min-h-[4.5rem] w-full resize-y"
              :disabled="editor.timelineSubmitting"
              placeholder="What happened?"
              required
            />
          </div>
        </form>
        <template #footer>
          <Button
            variant="outline"
            :disabled="editor.timelineSubmitting"
            @click="editor.showAddTimelineDialog = false"
            >Cancel</Button
          >
          <Button
            variant="primary"
            :loading="editor.timelineSubmitting"
            @click="editor.submitTimelineEntry"
          >
            Add
          </Button>
        </template>
      </Modal>

      <!-- Dispatch coordination task -->
      <Modal
        :open="showDispatchTaskDialog"
        title="Dispatch coordination task"
        max-width="lg"
        confirm-label="Dispatch"
        :loading="dispatchTaskSubmitting"
        @update:open="!$event && (showDispatchTaskDialog = false)"
        @close="showDispatchTaskDialog = false"
        @confirm="coord.submitDispatchTask"
      >
        <div class="flex flex-col gap-3">
          <div>
            <label class="field-label mb-1 block" for="dispatch-task-goal">Goal</label>
            <Textarea
              id="dispatch-task-goal"
              v-model="dispatchTaskGoal"
              :disabled="dispatchTaskSubmitting"
              placeholder="Describe what needs to be done..."
              :rows="3"
            />
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="field-label mb-1 block" for="dispatch-task-kind">Kind</label>
              <Select id="dispatch-task-kind" v-model="dispatchTaskKind">
                <option value="investigate">Investigate</option>
                <option value="communicate">Communicate</option>
                <option value="verify">Verify</option>
                <option value="mitigate">Mitigate</option>
                <option value="synthesize">Synthesize</option>
              </Select>
            </div>
            <div>
              <label class="field-label mb-1 block" for="dispatch-task-role">Assignee role</label>
              <Select id="dispatch-task-role" v-model="dispatchTaskRole">
                <option value="commander">Commander</option>
                <option value="communicator">Communicator</option>
                <option value="responder">Responder</option>
              </Select>
            </div>
          </div>
        </div>
      </Modal>

      <!-- Confirm dialogs -->
      <ConfirmDialog
        v-model:open="showDeleteConfirm"
        title="Delete Incident"
        message="Are you sure you want to delete this incident? This action cannot be undone."
        confirm-label="Delete"
        destructive
        :loading="deleting"
        @confirm="doDeleteIncident"
      />
      <ConfirmDialog
        v-model:open="editor.showUnlinkChannelConfirm"
        title="Unlink Slack Channel"
        message="Unlink the Slack channel from this incident? This won't delete or archive the channel in Slack."
        confirm-label="Unlink"
        destructive
        @confirm="editor.confirmUnlinkChannel"
      />
      <ConfirmDialog
        :open="unlinkAlertTarget !== null"
        title="Unlink Alert"
        message="Remove this alert from the incident?"
        confirm-label="Unlink"
        destructive
        :loading="editor.unlinkAlertSubmitting"
        @update:open="
          (v: boolean) => {
            if (!v) unlinkAlertTarget = null;
          }
        "
        @confirm="editor.confirmUnlinkAlert"
      />
    </template>
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

:deep(.rounded-lg) {
  border-radius: 0.375rem;
}
:deep(.rounded-t-lg) {
  border-top-left-radius: 0.375rem;
  border-top-right-radius: 0.375rem;
}
</style>
