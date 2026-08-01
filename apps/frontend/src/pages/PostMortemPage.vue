<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { computed, onActivated, onDeactivated, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  Plus,
  ArrowLeft,
  CheckCircle2,
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
import Textarea from "@/components/ui/Textarea.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Checkbox from "@/components/ui/Checkbox.vue";
import Select from "@/components/ui/Select.vue";
import Modal from "@/components/ui/Modal.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import ActionItemRow from "@/components/postmortem/ActionItemRow.vue";

defineOptions({ name: "PostMortemPage" });

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
const factorsCsv = ref("");
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

function syncForm(pm: PostMortemRecord) {
  title.value = pm.title ?? "";
  summary.value = pm.summary ?? "";
  rootCause.value = pm.root_cause ?? "";
  factorsCsv.value = (pm.contributing_factors ?? []).join(", ");
  impact.value = pm.impact ?? "";
  lessonsLearned.value = pm.lessons_learned ?? "";
  whatWentWell.value = pm.what_went_well ?? "";
  whatWentWrong.value = pm.what_went_wrong ?? "";
  blamelessConfirmed.value = pm.blameless_confirmed ?? false;
  blamelessNotes.value = pm.blameless_notes ?? "";
}

function displayText(value: string | undefined | null): string {
  const trimmed = value?.trim();
  return trimmed || "Not documented yet.";
}

function isEmptyText(value: string | undefined | null): boolean {
  return !value?.trim();
}

function completionLabel(): string {
  if (actionItems.value.length === 0) return "No action items";
  return `${completedActionItems.value}/${actionItems.value.length} completed`;
}

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
    const contributing_factors = factorsCsv.value
      .split(",")
      .map((f) => f.trim())
      .filter(Boolean);
    const pm = await api.updatePostMortem(incidentNumber.value, {
      title: title.value,
      summary: summary.value,
      root_cause: rootCause.value,
      contributing_factors,
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
            {{ completionLabel() }}
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

      <!-- Report sections -->
      <Card v-if="!isEditable">
        <div class="space-y-6">
          <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h3 class="text-sm font-semibold text-[var(--text-primary)]">Report</h3>
              <p class="mt-1 text-xs text-[var(--text-muted)]">
                Narrative, impact, cause analysis, and follow-up learnings.
              </p>
            </div>
            <Button v-if="canEditDraft" variant="outline" size="sm" @click="startEditing">
              <Edit3 class="h-3.5 w-3.5" />
              Edit
            </Button>
          </div>

          <h2 v-if="postMortem.title" class="text-lg font-semibold text-[var(--text-primary)]">
            {{ postMortem.title }}
          </h2>

          <div class="grid gap-6 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
            <div class="space-y-5">
              <section>
                <h4 class="mb-2 text-xs font-semibold uppercase text-[var(--text-muted)]">
                  Summary
                </h4>
                <p
                  class="whitespace-pre-wrap text-sm leading-6 text-[var(--text-primary)]"
                  :class="{ 'text-[var(--text-muted)]': isEmptyText(postMortem.summary) }"
                >
                  {{ displayText(postMortem.summary) }}
                </p>
              </section>
              <section>
                <h4 class="mb-2 text-xs font-semibold uppercase text-[var(--text-muted)]">
                  Impact
                </h4>
                <p
                  class="whitespace-pre-wrap text-sm leading-6 text-[var(--text-primary)]"
                  :class="{ 'text-[var(--text-muted)]': isEmptyText(postMortem.impact) }"
                >
                  {{ displayText(postMortem.impact) }}
                </p>
              </section>
              <section>
                <h4 class="mb-2 text-xs font-semibold uppercase text-[var(--text-muted)]">
                  Root cause
                </h4>
                <p
                  class="whitespace-pre-wrap text-sm leading-6 text-[var(--text-primary)]"
                  :class="{ 'text-[var(--text-muted)]': isEmptyText(postMortem.root_cause) }"
                >
                  {{ displayText(postMortem.root_cause) }}
                </p>
              </section>
            </div>

            <div class="space-y-5">
              <section>
                <h4 class="mb-2 text-xs font-semibold uppercase text-[var(--text-muted)]">
                  Contributing factors
                </h4>
                <div v-if="postMortem.contributing_factors?.length" class="flex flex-wrap gap-2">
                  <span
                    v-for="factor in postMortem.contributing_factors"
                    :key="factor"
                    class="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-2 py-1 text-xs text-[var(--text-secondary)]"
                  >
                    {{ factor }}
                  </span>
                </div>
                <p v-else class="text-sm text-[var(--text-muted)]">Not documented yet.</p>
              </section>
              <section>
                <h4 class="mb-2 text-xs font-semibold uppercase text-[var(--text-muted)]">
                  Lessons learned
                </h4>
                <p
                  class="whitespace-pre-wrap text-sm leading-6 text-[var(--text-primary)]"
                  :class="{ 'text-[var(--text-muted)]': isEmptyText(postMortem.lessons_learned) }"
                >
                  {{ displayText(postMortem.lessons_learned) }}
                </p>
              </section>
              <section class="grid gap-4 sm:grid-cols-2 lg:grid-cols-1">
                <div>
                  <h4 class="mb-2 text-xs font-semibold uppercase text-[var(--text-muted)]">
                    Went well
                  </h4>
                  <p
                    class="whitespace-pre-wrap text-sm leading-6 text-[var(--text-primary)]"
                    :class="{ 'text-[var(--text-muted)]': isEmptyText(postMortem.what_went_well) }"
                  >
                    {{ displayText(postMortem.what_went_well) }}
                  </p>
                </div>
                <div>
                  <h4 class="mb-2 text-xs font-semibold uppercase text-[var(--text-muted)]">
                    Needs improvement
                  </h4>
                  <p
                    class="whitespace-pre-wrap text-sm leading-6 text-[var(--text-primary)]"
                    :class="{ 'text-[var(--text-muted)]': isEmptyText(postMortem.what_went_wrong) }"
                  >
                    {{ displayText(postMortem.what_went_wrong) }}
                  </p>
                </div>
              </section>
              <section>
                <h4 class="mb-2 text-xs font-semibold uppercase text-[var(--text-muted)]">
                  Blameless notes
                </h4>
                <p
                  class="whitespace-pre-wrap text-sm leading-6 text-[var(--text-primary)]"
                  :class="{ 'text-[var(--text-muted)]': isEmptyText(postMortem.blameless_notes) }"
                >
                  {{ displayText(postMortem.blameless_notes) }}
                </p>
              </section>
            </div>
          </div>
        </div>
      </Card>

      <Card v-else>
        <div class="space-y-6">
          <div class="space-y-4">
            <h3 class="text-sm font-semibold text-[var(--text-primary)]">Overview</h3>
            <div>
              <FormLabel for="pm-title" required>Title</FormLabel>
              <Input
                id="pm-title"
                v-model="title"
                :disabled="!isEditable"
                placeholder="Post-mortem title"
              />
            </div>
            <div>
              <FormLabel for="pm-summary">Summary</FormLabel>
              <Textarea
                id="pm-summary"
                v-model="summary"
                :disabled="!isEditable"
                rows="3"
                placeholder="Brief summary of the incident and post-mortem"
              />
            </div>
            <div>
              <FormLabel for="pm-impact">Impact</FormLabel>
              <Textarea
                id="pm-impact"
                v-model="impact"
                :disabled="!isEditable"
                rows="3"
                placeholder="What was the impact of this incident?"
              />
            </div>
          </div>

          <div class="border-t border-[var(--border-primary)] pt-4">
            <h3 class="mb-4 text-sm font-semibold text-[var(--text-primary)]">Analysis</h3>
            <div class="space-y-4">
              <div>
                <FormLabel for="pm-root-cause">Root Cause</FormLabel>
                <Textarea
                  id="pm-root-cause"
                  v-model="rootCause"
                  :disabled="!isEditable"
                  rows="4"
                  placeholder="What was the root cause of this incident?"
                />
              </div>
              <div>
                <FormLabel for="pm-factors">Contributing Factors</FormLabel>
                <Input
                  id="pm-factors"
                  v-model="factorsCsv"
                  :disabled="!isEditable"
                  placeholder="comma, separated, factors"
                />
              </div>
            </div>
          </div>

          <div class="border-t border-[var(--border-primary)] pt-4">
            <h3 class="mb-4 text-sm font-semibold text-[var(--text-primary)]">Lessons</h3>
            <div class="space-y-4">
              <div>
                <FormLabel for="pm-lessons">Lessons Learned</FormLabel>
                <Textarea
                  id="pm-lessons"
                  v-model="lessonsLearned"
                  :disabled="!isEditable"
                  rows="3"
                  placeholder="What did we learn from this incident?"
                />
              </div>
              <div class="grid gap-4 sm:grid-cols-2">
                <div>
                  <FormLabel for="pm-went-well">Went Well</FormLabel>
                  <Textarea
                    id="pm-went-well"
                    v-model="whatWentWell"
                    :disabled="!isEditable"
                    rows="3"
                    placeholder="What went well?"
                  />
                </div>
                <div>
                  <FormLabel for="pm-went-wrong">Needs Improvement</FormLabel>
                  <Textarea
                    id="pm-went-wrong"
                    v-model="whatWentWrong"
                    :disabled="!isEditable"
                    rows="3"
                    placeholder="What could have gone better?"
                  />
                </div>
              </div>
            </div>
          </div>

          <div class="border-t border-[var(--border-primary)] pt-4">
            <h3 class="mb-4 text-sm font-semibold text-[var(--text-primary)]">Blameless Review</h3>
            <div class="space-y-2">
              <label
                class="flex items-start gap-3 cursor-pointer"
                :class="{ 'pointer-events-none opacity-60': !isEditable }"
              >
                <Checkbox
                  id="pm-blameless"
                  v-model="blamelessConfirmed"
                  :disabled="!isEditable"
                  class="mt-0.5"
                />
                <span class="text-sm text-[var(--text-primary)]">
                  I confirm this post-mortem focuses on systems and processes, not people
                </span>
              </label>
              <Textarea
                v-if="blamelessConfirmed"
                v-model="blamelessNotes"
                :disabled="!isEditable"
                rows="2"
                placeholder="Optional notes on blamelessness..."
              />
            </div>
          </div>

          <div
            v-if="isEditable"
            class="flex items-center justify-between border-t border-[var(--border-primary)] pt-4"
          >
            <p class="text-xs text-[var(--text-muted)]">
              Save your changes before submitting for review.
            </p>
            <Button :loading="saving" @click="handleSave">Save Draft</Button>
          </div>
        </div>
      </Card>

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
