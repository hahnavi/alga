<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import {
  computed,
  onActivated,
  onBeforeUnmount,
  onDeactivated,
  onMounted,
  ref,
  watch,
  type ComponentPublicInstance,
  type Ref,
} from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  Plus,
  ArrowLeft,
  CheckCircle2,
  CircleDashed,
  Check,
  Clock,
  Edit3,
  Eye,
  FileText,
  ListChecks,
  Save,
  ShieldCheck,
  AlertTriangle,
  Activity,
  Trash2,
  X,
} from "@lucide/vue";
import {
  api,
  type PostMortemRecord,
  type PostMortemTimelineEntry,
  type ActionItemRecord,
} from "@/lib/api";
import { postMortemStatusBadgeClass } from "@/lib/alertLabels";
import { formatTime } from "@/lib/time";
import { getScrollContainer } from "@/lib/scrollContainer";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { useToast } from "@/lib/toast";
import { useDelete } from "@/composables/useDelete";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { useAsyncData } from "@/composables/useAsyncData";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import { usePageHeader } from "@/composables/usePageHeader";
import type { HeaderBadge } from "@/lib/pageHeader";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import Input from "@/components/ui/Input.vue";
import DatePicker from "@/components/ui/DatePicker.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Checkbox from "@/components/ui/Checkbox.vue";
import Select from "@/components/ui/Select.vue";
import Modal from "@/components/ui/Modal.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import MarkdownRenderer from "@/components/ui/MarkdownRenderer.vue";
import ActionItemRow from "@/components/postmortem/ActionItemRow.vue";
import SectionEditor from "@/components/postmortem/SectionEditor.vue";

defineOptions({ name: "PostMortemPage" });

type SectionKey =
  | "summary"
  | "impact"
  | "root_cause"
  | "contributing_factors"
  | "lessons_learned"
  | "what_went_well"
  | "what_went_wrong"
  | "blameless";

type SectionDef = {
  key: SectionKey;
  num: string;
  label: string;
  hint: string;
  placeholder: string;
};

const SECTIONS: SectionDef[] = [
  {
    key: "summary",
    num: "01",
    label: "Summary",
    hint: "A short, factual account of what happened and why this review exists.",
    placeholder: "What happened, when, and for how long?",
  },
  {
    key: "impact",
    num: "02",
    label: "Impact",
    hint: "Who and what was affected — customers, services, data, revenue, teams.",
    placeholder: "Describe the blast radius: users affected, SLO burn, revenue, support load…",
  },
  {
    key: "root_cause",
    num: "03",
    label: "Root Cause",
    hint: "The underlying system or process failure, not the proximate trigger.",
    placeholder: "Walk through the causal chain down to the underlying cause…",
  },
  {
    key: "contributing_factors",
    num: "04",
    label: "Contributing Factors",
    hint: "Secondary conditions that made the incident possible or worse.",
    placeholder:
      "List each factor on its own line, e.g.\n- Monitoring gap on queue depth\n- Runbook out of date",
  },
  {
    key: "lessons_learned",
    num: "05",
    label: "Lessons Learned",
    hint: "What this incident taught us about the system and how we run it.",
    placeholder: "What did we learn that we didn't know before?",
  },
  {
    key: "what_went_well",
    num: "06",
    label: "What Went Well",
    hint: "Response strengths worth keeping and reinforcing.",
    placeholder: "Detection, coordination, tooling, people — what worked?",
  },
  {
    key: "what_went_wrong",
    num: "07",
    label: "Needs Improvement",
    hint: "Gaps in detection, response, tooling, or process.",
    placeholder: "Where did detection, response, or communication fall short?",
  },
  {
    key: "blameless",
    num: "08",
    label: "Blameless Review",
    hint: "Confirm the review focuses on systems and processes, not people.",
    placeholder: "Optional notes on how blamelessness was upheld…",
  },
];

const route = useRoute();
const router = useRouter();
const { push } = useToast();

const incidentNumber = computed(() => Number(route.params.incident_number));
const notFound = ref(false);
const actionItems = ref<ActionItemRecord[]>([]);
const { submitting: saving, formError, withSubmit } = useFormSubmit();
const actionLoading = ref(false);
const isEditing = ref(false);
const {
  data: postMortem,
  loading,
  error,
  reload: load,
} = useAsyncData(async () => {
  try {
    const pm = await api.getPostMortem(incidentNumber.value);
    // The backend returns a null payload (not a 404) when no post-mortem
    // exists for the incident yet. Treat that the same as not-found so we
    // render the create empty-state instead of crashing in syncForm.
    if (!pm) {
      notFound.value = true;
      return null;
    }
    syncForm(pm);
    notFound.value = false;
    return pm;
  } catch (err) {
    if (isNotFoundError(err)) {
      notFound.value = true;
      return null;
    }
    throw err;
  }
}, "Failed to load post-mortem");

const title = ref("");
const summary = ref("");
const rootCause = ref("");
const factorsMarkdown = ref("");
const impact = ref("");
const lessonsLearned = ref("");
const whatWentWell = ref("");
const whatWentWrong = ref("");
const blamelessConfirmed = ref(false);
const blamelessNotes = ref("");

const showActionItemModal = ref(false);
const newActionItem = ref({
  description: "",
  priority: "medium" as const,
  due_date: "",
  type: "investigate" as const,
});

const { canWrite, canDelete } = useEntityPermissions("postmortems");
const isDraft = computed(() => postMortem.value?.status === "draft");
const canEditDraft = computed(() => isDraft.value && canWrite.value);
const isEditable = computed(() => isEditing.value && canEditDraft.value);
const completedActionItems = computed(
  () => actionItems.value.filter((item) => item.status === "completed").length,
);
const openActionItems = computed(
  () =>
    actionItems.value.filter((item) => item.status === "open" || item.status === "in_progress")
      .length,
);

function isNotFoundError(err: unknown): boolean {
  const msg = getErrorMessage(err, "").toLowerCase();
  return msg === "404" || msg.startsWith("404 ") || msg.endsWith(" 404");
}

function factorsToMarkdown(factors: string[] | null | undefined): string {
  return (factors ?? []).map((f) => `- ${f}`).join("\n");
}

function markdownToFactors(md: string): string[] {
  const lines = md
    .split("\n")
    .map((r) => r.trim())
    .filter((l) => l.length > 0);
  // Only fall back to comma-splitting when the whole input is free-form prose
  // with no list markers at all. Otherwise a non-list line is kept whole so
  // prose sentences aren't shredded at every comma.
  const hasListMarker = lines.some((l) => /^[-*+]\s+/.test(l) || /^\d+[.)]\s+/.test(l));
  const items: string[] = [];
  for (const line of lines) {
    const bullet = line.match(/^[-*+]\s+(.*)$/);
    if (bullet) {
      if (bullet[1].trim()) items.push(bullet[1].trim());
      continue;
    }
    const numbered = line.match(/^\d+[.)]\s+(.*)$/);
    if (numbered) {
      if (numbered[1].trim()) items.push(numbered[1].trim());
      continue;
    }
    if (hasListMarker) {
      items.push(line);
    } else {
      for (const part of line.split(",")) {
        if (part.trim()) items.push(part.trim());
      }
    }
  }
  return items;
}

function syncForm(pm: PostMortemRecord) {
  title.value = pm.title ?? "";
  summary.value = pm.summary ?? "";
  rootCause.value = pm.root_cause ?? "";
  factorsMarkdown.value = factorsToMarkdown(pm.contributing_factors);
  impact.value = pm.impact ?? "";
  lessonsLearned.value = pm.lessons_learned ?? "";
  whatWentWell.value = pm.what_went_well ?? "";
  whatWentWrong.value = pm.what_went_wrong ?? "";
  blamelessConfirmed.value = pm.blameless_confirmed ?? false;
  blamelessNotes.value = pm.blameless_notes ?? "";
}

const sectionTextRefs: Record<SectionKey, Ref<string>> = {
  summary,
  impact,
  root_cause: rootCause,
  contributing_factors: factorsMarkdown,
  lessons_learned: lessonsLearned,
  what_went_well: whatWentWell,
  what_went_wrong: whatWentWrong,
  blameless: blamelessNotes,
};

function sectionValue(key: SectionKey): string {
  return sectionTextRefs[key].value;
}

function setSectionValue(key: SectionKey, value: string) {
  sectionTextRefs[key].value = value;
}

function isSectionFilled(key: SectionKey): boolean {
  if (key === "blameless") return blamelessConfirmed.value || !!blamelessNotes.value.trim();
  return !!sectionValue(key).trim();
}

const filledCount = computed(() => SECTIONS.filter((s) => isSectionFilled(s.key)).length);

const isDirty = computed(() => {
  const pm = postMortem.value;
  if (!pm) return false;
  return (
    title.value !== (pm.title ?? "") ||
    summary.value !== (pm.summary ?? "") ||
    rootCause.value !== (pm.root_cause ?? "") ||
    markdownToFactors(factorsMarkdown.value).join("\n") !==
      (pm.contributing_factors ?? []).join("\n") ||
    impact.value !== (pm.impact ?? "") ||
    lessonsLearned.value !== (pm.lessons_learned ?? "") ||
    whatWentWell.value !== (pm.what_went_well ?? "") ||
    whatWentWrong.value !== (pm.what_went_wrong ?? "") ||
    blamelessConfirmed.value !== (pm.blameless_confirmed ?? false) ||
    blamelessNotes.value !== (pm.blameless_notes ?? "")
  );
});

const timelineEntries = computed<PostMortemTimelineEntry[]>(() => {
  const tl = postMortem.value?.timeline;
  if (!tl || !Array.isArray(tl)) return [];
  return tl;
});

function timelineEventColor(event: string): string {
  if (event.includes("resolved") || event.includes("closed")) {
    return "bg-green-500";
  }
  if (event.includes("mitigated")) {
    return "bg-blue-500";
  }
  if (event.includes("triaged") || event.includes("started") || event.includes("created")) {
    return "bg-orange-500";
  }
  if (event === "status_update") {
    return "bg-[var(--accent)]";
  }
  return "bg-[var(--text-muted)]";
}

function formatTimelineTime(ts: string): string {
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return ts;
  return formatTime(d.toISOString());
}

function statusBadgeClass(status: string): string {
  return `badge ${postMortemStatusBadgeClass(status)}`;
}

function statusLabel(status: string): string {
  return status.replace("_", " ");
}

function statusIcon(status: string) {
  switch (status) {
    case "draft":
      return FileText;
    case "in_review":
      return Eye;
    case "approved":
      return ShieldCheck;
    case "published":
      return CheckCircle2;
    default:
      return FileText;
  }
}

function statusDescription(status: string): string {
  switch (status) {
    case "draft":
      return "This post-mortem is being drafted. Edit the fields below and save your progress.";
    case "in_review":
      return "This post-mortem has been submitted for review and is awaiting approval.";
    case "approved":
      return "This post-mortem has been approved and is ready to be published.";
    case "published":
      return "This post-mortem has been published and is visible to the organization.";
    default:
      return "";
  }
}

usePageHeader(() => {
  const pm = postMortem.value;
  if (pm) {
    const badges: HeaderBadge[] = [
      { text: statusLabel(pm.status), cssClass: statusBadgeClass(pm.status) },
    ];
    return { title: pm.title || "Post-Mortem", badges };
  }
  return { title: "Post-Mortem" };
});

async function loadActionItems() {
  try {
    actionItems.value = await api.getActionItems(incidentNumber.value);
  } catch {
    actionItems.value = [];
  }
}

async function handleCreate() {
  actionLoading.value = true;
  try {
    const pm = await api.createPostMortem(incidentNumber.value, {});
    postMortem.value = pm;
    syncForm(pm);
    notFound.value = false;
    isEditing.value = true;
    push("Post-mortem created", "success");
  } catch (err) {
    push(getErrorMessage(err, "Failed to create post-mortem"), "error");
  } finally {
    actionLoading.value = false;
  }
}

async function handleSave() {
  if (!postMortem.value) return;
  await withSubmit(async () => {
    const pm = await api.updatePostMortem(incidentNumber.value, {
      title: title.value,
      summary: summary.value,
      root_cause: rootCause.value,
      contributing_factors: markdownToFactors(factorsMarkdown.value),
      impact: impact.value,
      lessons_learned: lessonsLearned.value,
      what_went_well: whatWentWell.value,
      what_went_wrong: whatWentWrong.value,
      blameless_confirmed: blamelessConfirmed.value,
      blameless_notes: blamelessNotes.value,
    });
    postMortem.value = pm;
    syncForm(pm);
    isEditing.value = false;
  }, "Post-mortem saved");
}

function startEditing() {
  if (!postMortem.value || !canEditDraft.value) return;
  syncForm(postMortem.value);
  isEditing.value = true;
}

function cancelEditing() {
  if (postMortem.value) syncForm(postMortem.value);
  isEditing.value = false;
}

async function handleSubmitForReview() {
  actionLoading.value = true;
  try {
    const pm = await api.submitPostMortemForReview(incidentNumber.value);
    postMortem.value = pm;
    push("Submitted for review", "success");
  } catch (err) {
    push(getErrorMessage(err, "Failed to submit"), "error");
  } finally {
    actionLoading.value = false;
  }
}

async function handleApprove() {
  actionLoading.value = true;
  try {
    const pm = await api.approvePostMortem(incidentNumber.value);
    postMortem.value = pm;
    push("Post-mortem approved", "success");
  } catch (err) {
    push(getErrorMessage(err, "Failed to approve"), "error");
  } finally {
    actionLoading.value = false;
  }
}

async function handlePublish() {
  actionLoading.value = true;
  try {
    const pm = await api.publishPostMortem(incidentNumber.value);
    postMortem.value = pm;
    push("Post-mortem published", "success");
  } catch (err) {
    push(getErrorMessage(err, "Failed to publish"), "error");
  } finally {
    actionLoading.value = false;
  }
}

async function handleAddActionItem() {
  if (!newActionItem.value.description.trim()) return;
  actionLoading.value = true;
  try {
    const item = await api.createActionItem(incidentNumber.value, {
      description: newActionItem.value.description.trim(),
      priority: newActionItem.value.priority,
      type: newActionItem.value.type,
      due_date: newActionItem.value.due_date || undefined,
    });
    actionItems.value.push(item);
    showActionItemModal.value = false;
    newActionItem.value = {
      description: "",
      priority: "medium",
      due_date: "",
      type: "investigate",
    };
    push("Action item added", "success");
  } catch (err) {
    push(getErrorMessage(err, "Failed to add action item"), "error");
  } finally {
    actionLoading.value = false;
  }
}

async function handleUpdateActionItem(item: ActionItemRecord, data: Partial<ActionItemRecord>) {
  try {
    const updated = await api.updateActionItem(incidentNumber.value, item.id, data);
    const idx = actionItems.value.findIndex((a) => a.id === item.id);
    if (idx !== -1) actionItems.value[idx] = updated;
    push("Action item updated", "success");
  } catch (err) {
    push(getErrorMessage(err, "Failed to update action item"), "error");
  }
}

async function handleDeleteActionItem(item: ActionItemRecord) {
  try {
    await api.deleteActionItem(incidentNumber.value, item.id);
    actionItems.value = actionItems.value.filter((a) => a.id !== item.id);
    push("Action item deleted", "success");
  } catch (err) {
    push(getErrorMessage(err, "Failed to delete action item"), "error");
  }
}

const { showDeleteConfirm, confirmDelete, doDelete } = useDelete<PostMortemRecord>(async () => {
  await api.deletePostMortem(incidentNumber.value);
  router.push(`/incidents/${incidentNumber.value}`);
}, "Post-mortem");

function goBackToIncident() {
  router.push(`/incidents/${incidentNumber.value}`);
}

const sectionEls = new Map<SectionKey, HTMLElement>();
const activeSection = ref<SectionKey>("summary");

function setSectionRef(key: SectionKey) {
  return (el: Element | ComponentPublicInstance | null) => {
    if (el instanceof HTMLElement) sectionEls.set(key, el);
    else sectionEls.delete(key);
  };
}

function scrollToSection(key: SectionKey) {
  sectionEls.get(key)?.scrollIntoView({ behavior: "smooth", block: "start" });
}

let spy: IntersectionObserver | null = null;

function setupScrollSpy() {
  cleanupScrollSpy();
  if (!postMortem.value) return;
  const root = getScrollContainer();
  spy = new IntersectionObserver(
    (entries) => {
      let topmost: { key: SectionKey; top: number } | null = null;
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        const key = (entry.target as HTMLElement).dataset.sectionKey as SectionKey | undefined;
        if (!key) continue;
        const top = entry.boundingClientRect.top;
        if (topmost === null || top < topmost.top) {
          topmost = { key, top };
        }
      }
      if (topmost) activeSection.value = topmost.key;
    },
    { root, rootMargin: "0px 0px -70% 0px", threshold: 0 },
  );
  for (const el of sectionEls.values()) spy.observe(el);
}

function cleanupScrollSpy() {
  spy?.disconnect();
  spy = null;
}

watch([isEditable, postMortem], () => {
  requestAnimationFrame(setupScrollSpy);
});

watch(isEditable, (editing) => {
  if (!editing) return;
  requestAnimationFrame(() => {
    getScrollContainer()?.scrollTo({ top: 0, behavior: "smooth" });
  });
});

onBeforeUnmount(cleanupScrollSpy);

onMounted(() => {
  if (!Number.isFinite(incidentNumber.value)) return;
  load();
  loadActionItems();
});

// KeepAlive keeps this page (and this watcher) alive after navigating away.
// Without the deactivated/NaN guards, browsing to another incident's detail or
// list page mutates :incident_number and triggers a background post-mortem load
// for an incident that may have none — the null payload then crashes syncForm
// and surfaces an error toast on the way out. Mirror the IncidentDetailPage /
// IncidentsPage watcher guards.
let isDeactivated = false;
onActivated(() => {
  const wasDeactivated = isDeactivated;
  isDeactivated = false;
  if (wasDeactivated && Number.isFinite(incidentNumber.value)) {
    load();
    loadActionItems();
  }
});
onDeactivated(() => {
  isDeactivated = true;
});
watch(incidentNumber, (next, prev) => {
  if (isDeactivated) return;
  if (!Number.isFinite(next)) return;
  if (prev !== undefined && next !== prev) {
    load();
    loadActionItems();
  }
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <LoadingSpinner v-if="loading" centered />
    <ErrorBanner v-else-if="error" :message="error" />

    <Card v-else-if="notFound" class="text-center">
      <EmptyState message="No post-mortem has been created for this incident yet.">
        <template #icon>
          <FileText class="mb-2 h-8 w-8 opacity-30" />
        </template>
        <template #footer>
          <div class="mt-4 flex items-center justify-center gap-3">
            <Button variant="outline" @click="goBackToIncident">
              <ArrowLeft class="h-3.5 w-3.5" />
              Back to incident
            </Button>
            <Button v-if="canWrite" :loading="actionLoading" @click="handleCreate">
              <Plus class="h-3.5 w-3.5" />
              Create Post-Mortem
            </Button>
          </div>
        </template>
      </EmptyState>
    </Card>

    <template v-else-if="postMortem">
      <!-- Status banner -->
      <Card>
        <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div class="flex items-start gap-3">
            <div
              class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg"
              :class="{
                'bg-[var(--bg-secondary)]': postMortem.status === 'draft',
                'bg-yellow-500/10': postMortem.status === 'in_review',
                'bg-blue-500/10': postMortem.status === 'approved',
                'bg-green-500/10': postMortem.status === 'published',
              }"
            >
              <component
                :is="statusIcon(postMortem.status)"
                class="h-5 w-5"
                :class="{
                  'text-[var(--text-muted)]': postMortem.status === 'draft',
                  'text-yellow-600 dark:text-yellow-400': postMortem.status === 'in_review',
                  'text-blue-600 dark:text-blue-400': postMortem.status === 'approved',
                  'text-green-600 dark:text-green-400': postMortem.status === 'published',
                }"
              />
            </div>
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <span class="text-sm font-medium text-[var(--text-primary)]">
                  {{ statusLabel(postMortem.status) }}
                </span>
                <span :class="statusBadgeClass(postMortem.status)">
                  {{ statusLabel(postMortem.status) }}
                </span>
              </div>
              <p class="mt-0.5 text-xs text-[var(--text-muted)]">
                {{ statusDescription(postMortem.status) }}
              </p>
            </div>
          </div>
          <div class="flex shrink-0 flex-wrap items-center gap-2">
            <Button variant="outline" size="sm" @click="goBackToIncident">
              <ArrowLeft class="h-3.5 w-3.5" />
              Incident
            </Button>
            <template v-if="isEditable">
              <Button variant="outline" size="sm" @click="cancelEditing">
                <X class="h-3.5 w-3.5" />
                Cancel
              </Button>
              <Button :loading="saving" size="sm" @click="handleSave">
                <Save class="h-3.5 w-3.5" />
                Save Draft
              </Button>
            </template>
            <Button v-else-if="canEditDraft" variant="outline" size="sm" @click="startEditing">
              <Edit3 class="h-3.5 w-3.5" />
              Edit Draft
            </Button>
            <Button
              v-if="canWrite && postMortem.status === 'draft' && !isEditable"
              :loading="actionLoading"
              size="sm"
              @click="handleSubmitForReview"
            >
              Submit for Review
            </Button>
            <Button
              v-if="canWrite && postMortem.status === 'in_review'"
              :loading="actionLoading"
              size="sm"
              @click="handleApprove"
            >
              Approve
            </Button>
            <Button
              v-if="canWrite && postMortem.status === 'approved'"
              :loading="actionLoading"
              size="sm"
              @click="handlePublish"
            >
              Publish
            </Button>
            <Button
              v-if="canDelete && !isEditable"
              variant="destructive"
              size="sm"
              @click="confirmDelete(postMortem)"
            >
              <Trash2 class="h-3.5 w-3.5" />
              Delete
            </Button>
          </div>
        </div>
        <div
          class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 border-t border-[var(--border-primary)] pt-3 text-xs text-[var(--text-muted)]"
        >
          <span class="flex items-center gap-1">
            <Clock class="h-3 w-3" />
            Created {{ formatTime(postMortem.created_at) }}
          </span>
          <span v-if="postMortem.published_at" class="flex items-center gap-1">
            <CheckCircle2 class="h-3 w-3" />
            Published {{ formatTime(postMortem.published_at) }}
          </span>
          <span class="flex items-center gap-1">
            <ListChecks class="h-3 w-3" />
            {{
              actionItems.length === 0
                ? "No action items"
                : `${completedActionItems}/${actionItems.length} completed`
            }}
          </span>
        </div>
      </Card>

      <!-- Incident context -->
      <Card v-if="postMortem.incident_title || postMortem.incident_number">
        <div class="flex items-center gap-3">
          <div
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-orange-500/10"
          >
            <AlertTriangle class="h-4 w-4 text-orange-600 dark:text-orange-400" />
          </div>
          <div class="min-w-0 flex-1">
            <p class="text-sm font-medium text-[var(--text-primary)]">
              {{ postMortem.incident_title || `Incident #${postMortem.incident_number}` }}
            </p>
            <div
              class="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-[var(--text-muted)]"
            >
              <span v-if="postMortem.incident_number"
                >Incident #{{ postMortem.incident_number }}</span
              >
              <span v-if="postMortem.incident_severity"
                >Severity: {{ postMortem.incident_severity }}</span
              >
              <span>Post-mortem for this incident</span>
            </div>
          </div>
        </div>
      </Card>

      <div class="grid gap-3 sm:grid-cols-3">
        <Card>
          <p class="text-xs text-[var(--text-muted)]">Open action items</p>
          <p class="mt-1 text-lg font-semibold text-[var(--text-primary)]">{{ openActionItems }}</p>
        </Card>
        <Card>
          <p class="text-xs text-[var(--text-muted)]">Completed action items</p>
          <p class="mt-1 text-lg font-semibold text-[var(--text-primary)]">
            {{ completedActionItems }}
          </p>
        </Card>
        <Card>
          <p class="text-xs text-[var(--text-muted)]">Blameless review</p>
          <p class="mt-1 text-lg font-semibold text-[var(--text-primary)]">
            {{ postMortem.blameless_confirmed ? "Confirmed" : "Pending" }}
          </p>
        </Card>
      </div>

      <ErrorBanner v-if="isEditable && formError" :message="formError" />

      <!-- Document -->
      <Card class="overflow-visible">
        <div class="grid gap-6 lg:grid-cols-[210px_minmax(0,1fr)] lg:gap-10">
          <!-- Section outline -->
          <aside class="hidden lg:block">
            <div class="sticky top-4">
              <p
                class="text-[11px] font-semibold uppercase tracking-wider text-[var(--text-muted)]"
              >
                On this page
              </p>
              <nav class="mt-3 flex flex-col" aria-label="Post-mortem sections">
                <button
                  v-for="s in SECTIONS"
                  :key="s.key"
                  type="button"
                  class="group flex items-center gap-2 rounded-md py-1.5 pl-3 pr-2 text-left text-[13px] transition-colors"
                  :class="
                    activeSection === s.key
                      ? 'bg-[var(--bg-secondary)] font-medium text-[var(--text-primary)]'
                      : 'text-[var(--text-muted)] hover:bg-[var(--bg-secondary)] hover:text-[var(--text-secondary)]'
                  "
                  :aria-current="activeSection === s.key ? 'true' : undefined"
                  @click="scrollToSection(s.key)"
                >
                  <CheckCircle2
                    v-if="isSectionFilled(s.key)"
                    class="h-3.5 w-3.5 shrink-0 text-[var(--accent)]"
                  />
                  <CircleDashed v-else class="h-3.5 w-3.5 shrink-0 opacity-50" />
                  <span class="truncate">{{ s.label }}</span>
                </button>
              </nav>
              <div class="mt-4 border-t border-[var(--border-primary)] pt-3">
                <div class="flex items-center justify-between text-[11px] text-[var(--text-muted)]">
                  <span>Sections filled</span>
                  <span class="font-medium text-[var(--text-secondary)]"
                    >{{ filledCount }}/{{ SECTIONS.length }}</span
                  >
                </div>
                <div class="mt-1.5 h-1 overflow-hidden rounded-full bg-[var(--bg-secondary)]">
                  <div
                    class="h-full rounded-full bg-[var(--accent)] transition-[width] duration-300"
                    :style="{ width: `${(filledCount / SECTIONS.length) * 100}%` }"
                  />
                </div>
              </div>
            </div>
          </aside>

          <!-- Document body -->
          <div class="min-w-0">
            <div class="max-w-3xl">
              <p
                class="text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--text-muted)]"
              >
                Post-mortem · Incident
                <template v-if="postMortem.incident_number"
                  >#{{ postMortem.incident_number }}</template
                >
              </p>

              <h2
                v-if="!isEditable"
                class="mt-2 text-2xl font-bold leading-snug tracking-tight text-[var(--text-primary)] md:text-[28px]"
              >
                {{ postMortem.title || "Untitled post-mortem" }}
              </h2>
              <Input
                v-else
                v-model="title"
                aria-label="Post-mortem title"
                placeholder="Untitled post-mortem"
                class="mt-2 border-transparent bg-transparent px-0 text-2xl font-bold tracking-tight shadow-none focus:border-transparent focus:shadow-none md:text-[28px]"
              />

              <div
                class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--text-muted)]"
              >
                <span :class="statusBadgeClass(postMortem.status)">
                  {{ statusLabel(postMortem.status) }}
                </span>
                <span class="flex items-center gap-1">
                  <Clock class="h-3 w-3" />
                  Created {{ formatTime(postMortem.created_at) }}
                </span>
                <span v-if="postMortem.published_at" class="flex items-center gap-1">
                  <CheckCircle2 class="h-3 w-3" />
                  Published {{ formatTime(postMortem.published_at) }}
                </span>
              </div>

              <div class="my-6 border-t border-[var(--border-primary)]" />

              <div class="space-y-10">
                <section
                  v-for="s in SECTIONS"
                  :id="`pm-section-${s.key}`"
                  :key="s.key"
                  :ref="setSectionRef(s.key)"
                  :data-section-key="s.key"
                  class="scroll-mt-6"
                >
                  <div class="flex items-baseline gap-3">
                    <span
                      class="font-mono text-xs font-semibold tabular-nums"
                      :class="
                        isSectionFilled(s.key) ? 'text-[var(--accent)]' : 'text-[var(--text-muted)]'
                      "
                      >{{ s.num }}</span
                    >
                    <div class="min-w-0">
                      <h3 class="text-base font-semibold text-[var(--text-primary)]">
                        {{ s.label }}
                      </h3>
                      <p class="mt-0.5 text-xs text-[var(--text-muted)]">{{ s.hint }}</p>
                    </div>
                  </div>

                  <div class="mt-3 border-l-2 border-[var(--border-primary)] pl-4 md:pl-5">
                    <template v-if="!isEditable">
                      <template v-if="s.key === 'blameless'">
                        <span
                          class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium"
                          :class="
                            postMortem.blameless_confirmed
                              ? 'border-green-500/30 bg-green-500/10 text-green-600 dark:text-green-400'
                              : 'border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)]'
                          "
                        >
                          <CheckCircle2 v-if="postMortem.blameless_confirmed" class="h-3.5 w-3.5" />
                          <CircleDashed v-else class="h-3.5 w-3.5" />
                          {{
                            postMortem.blameless_confirmed
                              ? "Blamelessness confirmed"
                              : "Not confirmed yet"
                          }}
                        </span>
                        <MarkdownRenderer
                          v-if="postMortem.blameless_notes?.trim()"
                          class="mt-3 text-sm"
                          :content="postMortem.blameless_notes"
                        />
                        <p v-else class="mt-3 text-sm italic text-[var(--text-muted)]">
                          No notes recorded.
                        </p>
                      </template>
                      <template v-else-if="s.key === 'contributing_factors'">
                        <MarkdownRenderer
                          v-if="postMortem.contributing_factors?.length"
                          class="text-sm"
                          :content="factorsToMarkdown(postMortem.contributing_factors)"
                        />
                        <p v-else class="text-sm italic text-[var(--text-muted)]">
                          Not documented yet.
                        </p>
                      </template>
                      <template v-else>
                        <MarkdownRenderer
                          v-if="sectionValue(s.key).trim()"
                          class="text-sm"
                          :content="sectionValue(s.key)"
                        />
                        <p v-else class="text-sm italic text-[var(--text-muted)]">
                          Not documented yet.
                        </p>
                      </template>
                    </template>

                    <template v-else>
                      <template v-if="s.key === 'blameless'">
                        <label class="mb-3 flex cursor-pointer items-start gap-3">
                          <Checkbox v-model="blamelessConfirmed" class="mt-0.5" />
                          <span class="text-sm text-[var(--text-primary)]">
                            I confirm this post-mortem focuses on systems and processes, not people
                          </span>
                        </label>
                        <SectionEditor
                          :model-value="blamelessNotes"
                          :placeholder="s.placeholder"
                          min-height="90px"
                          @update:model-value="setSectionValue('blameless', $event)"
                        />
                      </template>
                      <SectionEditor
                        v-else
                        :model-value="sectionValue(s.key)"
                        :placeholder="s.placeholder"
                        :min-height="s.key === 'contributing_factors' ? '110px' : '140px'"
                        @update:model-value="setSectionValue(s.key, $event)"
                      />
                    </template>
                  </div>
                </section>
              </div>
            </div>
          </div>
        </div>
      </Card>

      <!-- Sticky save bar while editing -->
      <div
        v-if="isEditable"
        class="sticky bottom-3 z-20 flex items-center justify-between gap-3 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-card)]/95 px-4 py-2.5 shadow-lg backdrop-blur"
      >
        <div class="flex min-w-0 items-center gap-2 text-xs">
          <template v-if="isDirty">
            <span class="h-2 w-2 shrink-0 animate-pulse rounded-full bg-amber-500" />
            <span class="font-medium text-[var(--text-secondary)]">Unsaved changes</span>
          </template>
          <template v-else>
            <Check class="h-3.5 w-3.5 shrink-0 text-green-600 dark:text-green-400" />
            <span class="text-[var(--text-muted)]">All changes saved</span>
          </template>
          <span class="hidden truncate text-[var(--text-muted)] sm:inline"
            >· Markdown supported in every section</span
          >
        </div>
        <div class="flex shrink-0 items-center gap-2">
          <Button variant="outline" size="sm" @click="cancelEditing">
            <X class="h-3.5 w-3.5" />
            Cancel
          </Button>
          <Button :loading="saving" size="sm" @click="handleSave">
            <Save class="h-3.5 w-3.5" />
            Save Draft
          </Button>
        </div>
      </div>

      <!-- Incident timeline -->
      <Card v-if="timelineEntries.length > 0">
        <div class="flex items-center gap-2">
          <Activity class="h-4 w-4 text-[var(--text-muted)]" />
          <h3 class="text-sm font-semibold text-[var(--text-primary)]">Incident Timeline</h3>
          <span class="text-xs text-[var(--text-muted)]">
            ({{ timelineEntries.length }} events)
          </span>
        </div>
        <p class="mt-1 mb-4 text-xs text-[var(--text-muted)]">
          Auto-generated from incident lifecycle, status updates, and coordination events.
        </p>
        <ol class="relative ml-3 border-l border-[var(--border-primary)]">
          <li
            v-for="(entry, idx) in timelineEntries"
            :key="idx"
            class="relative mb-4 ml-5 last:mb-0"
          >
            <span
              class="absolute -left-[26px] top-0.5 h-3 w-3 rounded-full ring-2 ring-[var(--bg-primary)]"
              :class="timelineEventColor(entry.event)"
            />
            <div class="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
              <span class="text-xs font-medium text-[var(--text-muted)]">
                {{ formatTimelineTime(entry.timestamp) }}
              </span>
              <span
                class="rounded px-1.5 py-0.5 text-[10px] font-medium uppercase"
                :class="{
                  'bg-[var(--bg-secondary)] text-[var(--text-muted)]':
                    entry.event !== 'status_update',
                  'bg-[var(--accent)]/10 text-[var(--accent)]': entry.event === 'status_update',
                }"
              >
                {{ entry.event.replace(/_/g, " ") }}
              </span>
            </div>
            <p class="mt-0.5 whitespace-pre-wrap text-sm leading-6 text-[var(--text-primary)]">
              {{ entry.description }}
            </p>
          </li>
        </ol>
      </Card>

      <!-- Action items -->
      <Card>
        <div class="flex items-center justify-between">
          <h3 class="text-sm font-semibold text-[var(--text-primary)]">Action Items</h3>
          <Button v-if="canWrite" size="sm" @click="showActionItemModal = true">
            <Plus class="h-3 w-3" />
            Add Action Item
          </Button>
        </div>

        <div v-if="actionItems.length === 0" class="mt-3 text-xs text-[var(--text-muted)]">
          No action items yet.
        </div>

        <div v-else class="mt-3 overflow-x-auto">
          <table class="w-full text-left">
            <thead>
              <tr
                class="border-b border-[var(--border-primary)] text-xs font-medium text-[var(--text-muted)]"
              >
                <th class="px-3 py-2">Description</th>
                <th class="px-3 py-2">Type</th>
                <th class="px-3 py-2">Priority</th>
                <th class="px-3 py-2">Status</th>
                <th class="px-3 py-2">Due Date</th>
                <th class="px-3 py-2">Assignee</th>
                <th class="px-3 py-2"></th>
              </tr>
            </thead>
            <tbody>
              <ActionItemRow
                v-for="item in actionItems"
                :key="item.id"
                :item="item"
                @update="handleUpdateActionItem(item, $event)"
                @delete="handleDeleteActionItem(item)"
              />
            </tbody>
          </table>
        </div>
      </Card>
    </template>

    <Modal
      :open="showActionItemModal"
      title="Add Action Item"
      :loading="actionLoading"
      confirm-label="Add"
      @update:open="showActionItemModal = $event"
      @confirm="handleAddActionItem"
    >
      <div class="space-y-4">
        <div>
          <FormLabel for="ai-desc" required>Description</FormLabel>
          <Input
            id="ai-desc"
            v-model="newActionItem.description"
            placeholder="Action item description"
          />
        </div>
        <div class="grid grid-cols-2 gap-4">
          <div>
            <FormLabel for="ai-priority">Priority</FormLabel>
            <Select id="ai-priority" v-model="newActionItem.priority">
              <option value="high">high</option>
              <option value="medium">medium</option>
              <option value="low">low</option>
            </Select>
          </div>
          <div>
            <FormLabel for="ai-type">Type</FormLabel>
            <Select id="ai-type" v-model="newActionItem.type">
              <option value="prevent">prevent</option>
              <option value="mitigate">mitigate</option>
              <option value="detect">detect</option>
              <option value="investigate">investigate</option>
            </Select>
          </div>
        </div>
        <div>
          <FormLabel for="ai-due">Due Date</FormLabel>
          <DatePicker id="ai-due" v-model="newActionItem.due_date" placeholder="Pick due date" />
        </div>
      </div>
    </Modal>

    <ConfirmDialog
      v-model:open="showDeleteConfirm"
      title="Delete post-mortem"
      message="Delete this post-mortem and all its action items? This cannot be undone."
      confirm-label="Delete"
      :destructive="true"
      @confirm="doDelete"
    />
  </section>
</template>
