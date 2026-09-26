<script setup lang="ts">
import { computed, h, onMounted, onBeforeUnmount, ref, watch, type CSSProperties } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import {
  CircleAlert,
  CircleDot,
  Clock,
  ExternalLink,
  FileText,
  HatGlasses,
  ShieldAlert,
  Wrench,
  X,
  Plus,
  Unlink,
  ChevronDown,
  ChevronUp,
  BookOpen,
  MessageSquare,
} from "@lucide/vue";
import { useSSE } from "@/composables/useSSE";
import { useEscapeKey } from "@/composables/useEscapeKey";
import { useResizableMain } from "@/composables/useResizableMain";
import { useUsersIfPermitted } from "@/composables/useUsers";
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
import { api, type IncidentRecord, type AlertRecord } from "@/lib/api";
import {
  alertSeverityLabel,
  postMortemStatusBadgeClass,
  postMortemStatusLabel,
  incidentStatusBadgeClass,
  incidentStatusLabel,
  severityBorderColor,
} from "@/lib/alertLabels";
import {
  isForIncident,
  isOwnerThreadEvent,
  ownerThreadParticipants,
  participantLabel,
  visibleTimelineEntries,
} from "@/lib/incidentEvents";
import { formatTime, formatTimeFull } from "@/lib/time";
import {
  incidentImpactBadgeClass,
  incidentPriorityBadgeClass,
  incidentSeverityBadgeClass,
} from "@/lib/uiClasses";
import IncidentActionsMenu from "@/components/incident/IncidentActionsMenu.vue";
import IncidentDocSectionCard from "@/components/incident/IncidentDocSectionCard.vue";
import IncidentThreadSummaryCard from "@/components/incident/IncidentThreadSummaryCard.vue";
import IncidentTimeline from "@/components/incident/IncidentTimeline.vue";
import IncidentCoordinationStream from "@/components/incident/IncidentCoordinationStream.vue";
import IncidentLinkAlertDialog from "@/components/incident/IncidentLinkAlertDialog.vue";
import StatusUpdateFeed from "@/components/incident/StatusUpdateFeed.vue";
import ICSRoleBoard from "@/components/incident/ICSRoleBoard.vue";
import OwnerThreadPanel from "@/components/thread/OwnerThreadPanel.vue";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import Textarea from "@/components/ui/Textarea.vue";
import Select from "@/components/ui/Select.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
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
const { canRead: canReadPlaybooks } = useEntityPermissions("playbooks");
const { canWrite, canDelete, canCommand } = useEntityPermissions("incidents");

const data = useIncidentDetailData(incidentNumber, {
  canCreatePostMortem: canWritePostmortem,
  canReadPostMortem: canReadPostmortem,
  canReadPlaybooks: canReadPlaybooks,
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

const editor = useIncidentEditor(
  incidentNumber,
  incident,
  data.setIncident,
  loadIncident,
  () => data.loadAlerts(),
  data.removeLinkedAlert,
);

const thread = useIncidentThread(incidentNumber, { scheduleReload });

const linkedAlertNumbers = computed(() =>
  alerts.value.map((a) => a.alert_number).filter((n): n is number => typeof n === "number"),
);

// Mention targets: the agent list needs `tokens:manage` and the user list
// `users:manage` — both operator permissions an `incidents` viewer may lack,
// so each fetch is permission-gated (the composable gates agents, the
// `useUsersIfPermitted` helper gates users) to avoid error toasts on every
// visit. Mentions only matter for writers anyway.
const { users, loadUsers } = useUsersIfPermitted("users:manage");

async function loadMentionTargets() {
  const targets: Promise<unknown>[] = [coord.loadMentionTargets()];
  if (canWrite.value) targets.push(loadUsers());
  await Promise.all(targets);
}

async function loadIncident() {
  // The detail-data composable owns the main load + timeline, alerts,
  // ICS roles, mitigation playbooks, and post-mortem status side-loads.
  // The remaining side-loads (document sections, coordination,
  // thread, status updates) live in their own composables / helpers
  // and are awaited here so the page can render a complete view in
  // one shot. `docs.load()` must run after `data.load()` because it
  // seeds the summary display from the incident row.
  await data.load();
  if (!incident.value) return;
  await Promise.all([
    docs.load(),
    coord.loadCoordinationMessages(),
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

const coordinationParticipants = coord.coordinationParticipants;
const coordinationParticipantLabel = coord.coordinationParticipantLabel;

const investigationParticipants = computed(() =>
  ownerThreadParticipants(thread.incidentThread?.messages ?? []),
);

const investigationParticipantLabel = computed(() =>
  participantLabel(investigationParticipants.value, "No participants yet"),
);

const expandedPlaybookId = ref<string | null>(null);

const coordinationText = coord.coordinationText;
const coordinationSubmitting = coord.coordinationSubmitting;
const coordinationThreadEl = coord.coordinationThreadEl;
const stickCoordinationToBottom = coord.stickCoordinationToBottom;
const scrollCoordinationToBottom = coord.scrollCoordinationToBottom;
const agents = coord.agents;
const editorRef = ref<InstanceType<typeof MarkdownEditor> | null>(null);
// Referenced by the template via `ref="coordinationThreadEl"`; the script-only
// usage audit doesn't count template reads.
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
  thread.reset();
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

const sse = useSSE(
  "/api/v1/events",
  {
    // Incident status transitions (triaging/promoted/etc.) arrive via the
    // single incident_updated event; dedicated per-status events don't exist.
    incident_updated: (data: unknown) => {
      if (isForIncident(data, incidentNumber.value)) scheduleReload();
    },
    // Emitted by ICSWorker after provisioning completes.
    war_room_created: (data: unknown) => {
      if (isForIncident(data, incidentNumber.value)) scheduleReload();
    },
    // The scheduler publishes this with `incident_id` holding the incident
    // number, while the other incident events use `incident_number` — the
    // shared guard accepts both keys.
    ics_role_assigned: (payload: unknown) => {
      if (isForIncident(payload, incidentNumber.value)) void data.loadICSRoles();
    },
    incident_coordination_message_created: (data: unknown) => {
      if (!isForIncident(data, incidentNumber.value)) return;
      void (async () => {
        await coord.loadCoordinationMessages();
        stickCoordinationToBottom();
        fetchStatusUpdates(true);
      })();
    },
    ...thread.handlers,
    owner_thread_typing: (data: unknown) => {
      const d = data as { source?: string; agent_type?: string };
      const source = d.source ?? "agent";
      const agentType = d.agent_type;
      if (isOwnerThreadEvent(data, "incident_coord", incidentNumber.value)) {
        setCoordinationTyping(source, agentType);
      } else if (isOwnerThreadEvent(data, "incident_inv", incidentNumber.value)) {
        setInvestigationTyping(source, agentType);
      }
    },
    owner_thread_typing_stop: (data: unknown) => {
      if (isOwnerThreadEvent(data, "incident_coord", incidentNumber.value)) {
        clearCoordinationTyping();
      } else if (isOwnerThreadEvent(data, "incident_inv", incidentNumber.value)) {
        clearInvestigationTyping();
      }
    },
  },
  {
    onReconnect: () => scheduleReload(),
  },
);
const sseState = sse.state;

onMounted(() => {
  void loadIncident();
  void loadMentionTargets();
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

const timelineEventCount = computed(() => visibleTimelineEntries(timeline.value).length);

/** Closes the unlink-alert dialog after the confirm resolves. */
async function onConfirmUnlinkAlert() {
  const target = unlinkAlertTarget.value;
  if (!target) return;
  await editor.confirmUnlinkAlert(target);
  unlinkAlertTarget.value = null;
}

/** Appends the created entry without a full reload; see useIncidentEditor. */
async function onSubmitTimelineEntry() {
  const entry = await editor.submitTimelineEntry();
  if (entry) data.addTimelineEntry(entry);
}

onBeforeUnmount(() => {
  resetIncidentState();
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
                  :class="incidentPriorityBadgeClass(incident.priority)"
                >
                  <span class="sr-only">Priority:</span>
                  {{ incident.priority }}
                </span>
                <span
                  v-if="incident.severity"
                  class="shrink-0 uppercase"
                  :class="incidentSeverityBadgeClass(incident.severity)"
                >
                  <span class="sr-only">Severity:</span>
                  {{ incident.severity }}
                </span>
                <span
                  v-if="incident.impact_level"
                  class="shrink-0 uppercase"
                  :class="incidentImpactBadgeClass(incident.impact_level)"
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
            <IncidentThreadSummaryCard
              :icon="MessageSquare"
              title="Coordination"
              :participants="coordinationParticipants"
              :participant-label="coordinationParticipantLabel"
              :message-count="coordinationMessages.length"
              :expanded="activeSidebarThread?.kind === 'coordination'"
              :typing="coordinationTyping"
              @toggle="toggleSidebarThread('coordination')"
            />

            <IncidentThreadSummaryCard
              :icon="HatGlasses"
              title="Investigation"
              :participants="investigationParticipants"
              :participant-label="investigationParticipantLabel"
              :message-count="thread.incidentThreadMessageCount"
              :expanded="activeSidebarThread?.kind === 'technical_thread'"
              :typing="investigationTyping"
              :empty-participant-icon="HatGlasses"
              @toggle="toggleSidebarThread('technical_thread')"
            />
          </div>

          <StatusUpdateFeed
            :incident-id="String(incidentNumber)"
            :updates="statusUpdates"
            :can-post="canPostStatusUpdate"
            :loading="statusUpdatesLoading"
            :error="statusUpdatesError"
            :incident-status="incident?.status"
            :disabled="isDeleted"
            @posted="fetchStatusUpdates(true)"
            @retry="() => fetchStatusUpdates()"
          />

          <!-- Summary -->
          <IncidentDocSectionCard
            :icon="FileText"
            title="Summary"
            :content="summaryContent"
            :editing="summaryEditing"
            :saving="summarySaving"
            :can-edit="canWrite && !isDeleted"
            empty-text="No executive summary recorded."
            placeholder="Write an executive summary (the cause, why it started, what it did, and status until recovery)... (markdown supported)"
            :users="users"
            :agents="agents"
            @start-edit="docs.startEditSummary"
            @cancel-edit="summaryEditing = false"
            @save="docs.saveSummary"
            @update:content="summaryContent = $event"
          />

          <!-- Root Cause -->
          <IncidentDocSectionCard
            :icon="ShieldAlert"
            title="Root Cause"
            :content="rootCauseContent"
            :editing="rootCauseEditing"
            :saving="rootCauseSaving"
            :can-edit="canCommand && !isDeleted"
            empty-text="No root cause recorded."
            placeholder="Describe the root cause of this incident... (markdown supported)"
            :users="users"
            :agents="agents"
            @start-edit="docs.startEditRootCause"
            @cancel-edit="rootCauseEditing = false"
            @save="docs.saveRootCause"
            @update:content="rootCauseContent = $event"
          />

          <!-- Resolution -->
          <IncidentDocSectionCard
            :icon="Wrench"
            title="Resolution"
            :content="resolutionContent"
            :editing="resolutionEditing"
            :saving="resolutionSaving"
            :can-edit="canCommand && !isDeleted"
            empty-text="No resolution recorded."
            placeholder="Describe the resolution for this incident... (markdown supported)"
            :users="users"
            :agents="agents"
            @start-edit="docs.startEditResolution"
            @cancel-edit="resolutionEditing = false"
            @save="docs.saveResolution"
            @update:content="resolutionContent = $event"
          />

          <!-- Impact Assessment -->
          <IncidentDocSectionCard
            :icon="CircleAlert"
            title="Impact Assessment"
            :content="impactContent"
            :editing="impactEditing"
            :saving="impactSaving"
            :can-edit="canCommand && !isDeleted"
            empty-text="No impact assessment recorded."
            placeholder="Describe the impact of this incident... (markdown supported)"
            :users="users"
            :agents="agents"
            @start-edit="docs.startEditImpact"
            @cancel-edit="impactEditing = false"
            @save="docs.saveImpact"
            @update:content="impactContent = $event"
          />

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
                v-if="incident.google_meet_space_name && conferenceHref"
                :href="conferenceHref"
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
                    :style="{
                      color: severityBorderColor(alertSeverityLabel(alert.labels)),
                    }"
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
                  {{ postMortemStatusLabel(postMortemStatus) }}
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
            <FormLabel for="edit-incident-title-input" required>Title</FormLabel>
            <Input
              id="edit-incident-title-input"
              v-model="editor.editTitle"
              required
              :disabled="editor.editSubmitting"
            />
          </div>
          <div>
            <FormLabel for="edit-incident-desc">Description</FormLabel>
            <Textarea
              id="edit-incident-desc"
              v-model="editor.editDescription"
              rows="3"
              class="min-h-[4.5rem] w-full resize-y"
              :disabled="editor.editSubmitting"
            />
          </div>
          <div>
            <FormLabel for="edit-incident-severity">Severity</FormLabel>
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
            <FormLabel for="edit-incident-impact">Impact</FormLabel>
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
            <FormLabel for="edit-incident-priority">Priority</FormLabel>
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

      <IncidentLinkAlertDialog
        :open="editor.showLinkAlertDialog"
        :submitting="editor.linkAlertSubmitting"
        :error="editor.linkAlertError"
        :linked-alert-numbers="linkedAlertNumbers"
        @update:open="(v: boolean) => (editor.showLinkAlertDialog = v)"
        @pick-alert="(n: number) => (editor.linkAlertPickerStaged = n)"
        @submit="editor.submitStagedLink"
      />

      <Modal
        :open="editor.showAddTimelineDialog"
        title="Add Timeline Entry"
        max-width="lg"
        :prevent-close="editor.timelineSubmitting"
        @update:open="!$event && (editor.showAddTimelineDialog = false)"
        @close="editor.showAddTimelineDialog = false"
      >
        <form class="space-y-4" @submit.prevent="onSubmitTimelineEntry">
          <ErrorBanner :message="editor.timelineError" />
          <div>
            <FormLabel for="timeline-event-type">Event Type</FormLabel>
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
            <FormLabel for="timeline-message" required>Message</FormLabel>
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
            @click="onSubmitTimelineEntry"
          >
            Add
          </Button>
        </template>
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
        @confirm="onConfirmUnlinkAlert"
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
</style>
