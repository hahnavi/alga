<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ArrowDown, ArrowUp, Plus, Trash2, Pencil } from "@lucide/vue";
import { api, type PlaybookRecord, type PlaybookStepRecord } from "@/lib/api";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { useDelete } from "@/composables/useDelete";
import { useAsyncData } from "@/composables/useAsyncData";
import { useFormSubmit } from "@/composables/useFormSubmit";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import Textarea from "@/components/ui/Textarea.vue";
import Card from "@/components/ui/Card.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import PlaybookFormModal from "@/components/playbook/PlaybookFormModal.vue";
import { usePageHeader } from "@/composables/usePageHeader";
import type { HeaderBadge } from "@/lib/pageHeader";
import { HEADER_ICON_BTN_CLASS } from "@/lib/uiClasses";
import { formatTime } from "@/lib/time";

defineOptions({ name: "PlaybookDetailPage" });

const route = useRoute();
const router = useRouter();
const playbookId = computed(() => route.params.id as string);

const {
  data: playbook,
  loading,
  error,
  reload: load,
} = useAsyncData(async () => api.getPlaybook(playbookId.value), "Failed to load playbook");
const showEditModal = ref(false);
const showAddStep = ref(false);
const editingStepId = ref<string | null>(null);

const {
  submitting: addStepSubmitting,
  formError: addStepError,
  withSubmit: withAddStep,
} = useFormSubmit();
const {
  submitting: editStepSubmitting,
  formError: editStepError,
  withSubmit: withEditStep,
} = useFormSubmit();

const newStepTitle = ref("");
const newStepDesc = ref("");
const newStepDuration = ref("");
const newStepCommand = ref("");

const editStepTitle = ref("");
const editStepDesc = ref("");
const editStepDuration = ref("");
const editStepCommand = ref("");

const { canWrite, canDelete } = useEntityPermissions("playbooks");

const { deleteTarget, showDeleteConfirm, confirmDelete, doDelete } = useDelete<PlaybookRecord>(
  async (pb) => {
    await api.deletePlaybook(pb.id);
    router.push("/playbooks");
  },
  "Playbook",
);

usePageHeader(() => {
  if (!playbook.value) return null;
  const badges: HeaderBadge[] = [
    {
      text: playbook.value.kind,
      cssClass:
        playbook.value.kind === "mitigation"
          ? "bg-[var(--bg-code)] text-orange-500"
          : "bg-[var(--bg-code)] text-blue-500",
    },
  ];
  const actions: ReturnType<typeof h>[] = [];
  if (canWrite.value) {
    actions.push(
      h(
        "button",
        {
          type: "button",
          class: HEADER_ICON_BTN_CLASS,
          "aria-label": "Edit playbook",
          title: "Edit playbook",
          onClick: () => {
            showEditModal.value = true;
          },
        },
        [h(Pencil, { class: "h-4 w-4", "aria-hidden": "true" })],
      ),
    );
  }
  if (canDelete.value) {
    actions.push(
      h(
        "button",
        {
          type: "button",
          class:
            "flex h-9 w-9 shrink-0 cursor-pointer items-center justify-center rounded-md text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-error)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]",
          "aria-label": "Delete playbook",
          title: "Delete playbook",
          onClick: () => {
            if (playbook.value) confirmDelete(playbook.value);
          },
        },
        [h(Trash2, { class: "h-4 w-4", "aria-hidden": "true" })],
      ),
    );
  }
  return { title: playbook.value.title, badges, options: { actions } };
});

function resetAddStep() {
  newStepTitle.value = "";
  newStepDesc.value = "";
  newStepDuration.value = "";
  newStepCommand.value = "";
  addStepError.value = "";
}

function startEditStep(step: PlaybookStepRecord) {
  editingStepId.value = step.id;
  editStepTitle.value = step.title;
  editStepDesc.value = step.description ?? "";
  editStepDuration.value = step.expected_duration ?? "";
  editStepCommand.value = step.command ?? "";
  editStepError.value = "";
}

function cancelEditStep() {
  editingStepId.value = null;
  editStepError.value = "";
}

async function handleAddStep() {
  if (!newStepTitle.value.trim()) {
    addStepError.value = "Step title is required.";
    return;
  }
  await withAddStep(async () => {
    await api.addPlaybookStep(playbookId.value, {
      title: newStepTitle.value.trim(),
      description: newStepDesc.value.trim() || undefined,
      expected_duration: newStepDuration.value.trim() || undefined,
      command: newStepCommand.value.trim() || undefined,
    });
    showAddStep.value = false;
    resetAddStep();
    await load();
  }, "Step added");
}

async function handleEditStep(stepId: string) {
  if (!editStepTitle.value.trim()) {
    editStepError.value = "Step title is required.";
    return;
  }
  await withEditStep(async () => {
    await api.updatePlaybookStep(playbookId.value, stepId, {
      title: editStepTitle.value.trim(),
      description: editStepDesc.value.trim() || undefined,
      expected_duration: editStepDuration.value.trim() || undefined,
      command: editStepCommand.value.trim() || undefined,
    });
    editingStepId.value = null;
    await load();
  }, "Step updated");
}

async function handleDeleteStep(stepId: string) {
  try {
    await api.deletePlaybookStep(playbookId.value, stepId);
    await load();
  } catch {
    // error handled by load
  }
}

async function handleReorder(stepId: string, direction: "up" | "down") {
  const steps = playbook.value?.steps;
  if (!steps || steps.length < 2) return;
  const idx = steps.findIndex((s) => s.id === stepId);
  if (idx < 0) return;
  const swapIdx = direction === "up" ? idx - 1 : idx + 1;
  if (swapIdx < 0 || swapIdx >= steps.length) return;
  const order = steps.map((s, i) => {
    let num = s.step_number;
    if (i === idx) num = steps[swapIdx].step_number;
    else if (i === swapIdx) num = steps[idx].step_number;
    return { id: s.id, step_number: num };
  });
  try {
    await api.reorderPlaybookSteps(playbookId.value, order);
    await load();
  } catch {
    // error handled by load
  }
}

onMounted(async () => {
  await load();
});

watch(playbookId, async () => {
  await load();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <ErrorBanner :message="error" />
    <LoadingSpinner v-if="loading" centered label="Loading playbook..." />

    <template v-if="!loading && playbook">
      <p v-if="playbook.summary" class="text-sm text-[var(--text-secondary)]">
        {{ playbook.summary }}
      </p>

      <div v-if="playbook.tags?.length" class="flex flex-wrap gap-1">
        <span
          v-for="tag in playbook.tags"
          :key="tag"
          class="rounded bg-[var(--bg-code)] px-1.5 py-0.5 text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]"
        >
          {{ tag }}
        </span>
      </div>

      <div
        v-if="playbook.label_selectors?.length"
        class="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-card)] px-4 py-3"
      >
        <p class="text-xs font-medium text-[var(--text-muted)]">Label Selectors</p>
        <p
          v-for="(sel, i) in playbook.label_selectors"
          :key="i"
          class="font-mono text-xs text-[var(--text-secondary)]"
        >
          {{ sel.key }} {{ sel.op }} {{ sel.value }}
        </p>
      </div>

      <div>
        <div class="mb-3 flex items-center justify-between">
          <h2 class="text-base font-medium text-[var(--text-primary)]">Steps</h2>
          <Button
            v-if="canWrite"
            size="sm"
            @click="
              showAddStep = true;
              resetAddStep();
            "
          >
            <Plus class="h-3 w-3" />
            Add Step
          </Button>
        </div>

        <div v-if="!playbook.steps?.length" class="text-sm text-[var(--text-muted)]">
          No steps yet.
        </div>

        <div v-else class="space-y-2">
          <Card v-for="(step, idx) in playbook.steps" :key="step.id">
            <div v-if="editingStepId === step.id" class="space-y-3">
              <div>
                <FormLabel>Title</FormLabel>
                <Input v-model="editStepTitle" placeholder="Step title" />
              </div>
              <div>
                <FormLabel>Description</FormLabel>
                <Textarea v-model="editStepDesc" rows="2" />
              </div>
              <div class="grid gap-3 md:grid-cols-2">
                <div>
                  <FormLabel>Expected Duration</FormLabel>
                  <Input v-model="editStepDuration" placeholder="e.g. 5m" />
                </div>
                <div>
                  <FormLabel>Command</FormLabel>
                  <Input v-model="editStepCommand" placeholder="Optional command" />
                </div>
              </div>
              <ErrorBanner :message="editStepError" />
              <div class="flex gap-2">
                <Button :loading="editStepSubmitting" size="sm" @click="handleEditStep(step.id)">
                  Save
                </Button>
                <Button variant="outline" size="sm" @click="cancelEditStep">Cancel</Button>
              </div>
            </div>
            <div v-else class="flex items-start gap-3">
              <span
                class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-[var(--bg-code)] text-xs font-bold text-[var(--text-muted)]"
              >
                {{ step.step_number }}
              </span>
              <div class="min-w-0 flex-1 space-y-1">
                <p class="text-sm font-medium text-[var(--text-primary)]">{{ step.title }}</p>
                <p
                  v-if="step.description"
                  class="whitespace-pre-wrap text-xs text-[var(--text-secondary)]"
                >
                  {{ step.description }}
                </p>
                <div class="flex flex-wrap gap-2 text-xs text-[var(--text-muted)]">
                  <span v-if="step.expected_duration">Duration: {{ step.expected_duration }}</span>
                  <span v-if="step.command" class="font-mono">`{{ step.command }}`</span>
                </div>
              </div>
              <div v-if="canWrite" class="flex shrink-0 gap-1">
                <button
                  type="button"
                  :disabled="idx === 0"
                  class="cursor-pointer text-[var(--text-muted)] hover:text-[var(--text-primary)] disabled:opacity-30"
                  title="Move up"
                  @click="handleReorder(step.id, 'up')"
                >
                  <ArrowUp class="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  :disabled="idx === (playbook.steps?.length ?? 0) - 1"
                  class="cursor-pointer text-[var(--text-muted)] hover:text-[var(--text-primary)] disabled:opacity-30"
                  title="Move down"
                  @click="handleReorder(step.id, 'down')"
                >
                  <ArrowDown class="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  class="cursor-pointer text-[var(--text-muted)] hover:text-[var(--text-primary)]"
                  title="Edit step"
                  @click="startEditStep(step)"
                >
                  <Pencil class="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  class="cursor-pointer text-[var(--text-muted)] hover:text-[var(--text-error)]"
                  title="Delete step"
                  @click="handleDeleteStep(step.id)"
                >
                  <Trash2 class="h-3.5 w-3.5" />
                </button>
              </div>
            </div>
          </Card>
        </div>
      </div>

      <div class="text-xs text-[var(--text-muted)]">
        Created {{ formatTime(playbook.created_at) }} · Updated
        {{ formatTime(playbook.updated_at) }}
      </div>
    </template>

    <PlaybookFormModal
      v-if="playbook"
      :show="showEditModal"
      :playbook="playbook"
      @close="showEditModal = false"
      @saved="
        showEditModal = false;
        load();
      "
    />

    <ConfirmDialog
      v-model:open="showDeleteConfirm"
      title="Delete Playbook"
      :message="`Delete '${deleteTarget?.title ?? ''}'? This cannot be undone.`"
      confirm-label="Delete"
      :destructive="true"
      @confirm="doDelete"
    />

    <div
      v-if="showAddStep"
      class="fixed inset-0 z-40 flex items-center justify-center bg-black/40"
      @click.self="showAddStep = false"
    >
      <div
        class="w-full max-w-lg rounded-lg border border-[var(--border-primary)] bg-[var(--bg-card)] p-6 shadow-xl"
        @click.stop
      >
        <h3 class="mb-4 text-base font-semibold text-[var(--text-primary)]">Add Step</h3>
        <div class="space-y-3">
          <div>
            <FormLabel for="step-title">Title</FormLabel>
            <Input id="step-title" v-model="newStepTitle" placeholder="Step title" required />
          </div>
          <div>
            <FormLabel for="step-desc">Description</FormLabel>
            <Textarea id="step-desc" v-model="newStepDesc" rows="2" />
          </div>
          <div class="grid gap-3 md:grid-cols-2">
            <div>
              <FormLabel for="step-duration">Expected Duration</FormLabel>
              <Input id="step-duration" v-model="newStepDuration" placeholder="e.g. 5m" />
            </div>
            <div>
              <FormLabel for="step-command">Command</FormLabel>
              <Input id="step-command" v-model="newStepCommand" placeholder="Optional command" />
            </div>
          </div>
          <ErrorBanner :message="addStepError" />
          <div class="flex justify-end gap-2">
            <Button variant="outline" @click="showAddStep = false">Cancel</Button>
            <Button :loading="addStepSubmitting" @click="handleAddStep">Add Step</Button>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
