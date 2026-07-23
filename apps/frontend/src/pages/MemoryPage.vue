<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Bot, Brain } from "@lucide/vue";
import { api, type AgentMemoryRecord, type AgentMemoryInput } from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Input from "@/components/ui/Input.vue";
import NumberInput from "@/components/ui/NumberInput.vue";
import Textarea from "@/components/ui/Textarea.vue";
import Select from "@/components/ui/Select.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import Modal from "@/components/ui/Modal.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import DateTimePicker from "@/components/ui/DateTimePicker.vue";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { formatTime, localDatetimeToRFC3339 } from "@/lib/time";
import { useDelete } from "@/composables/useDelete";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { useListPage } from "@/composables/useListPage";

defineOptions({ name: "MemoryPage" });

type MemoryForm = {
  content: string;
  memory_type: string;
  correlation_key: string;
  labelsCsv: string;
  confidence: string;
  expires_at: string;
};

const filterType = ref("");
const filterText = ref("");
const currentOffset = ref(0);
const pageSize = 50;

const editing = ref<AgentMemoryRecord | null>(null);
const creating = ref(false);
const form = ref<MemoryForm>(blankForm());

const { submitting: saving, formError, withSubmit } = useFormSubmit();

const { deleteTarget, showDeleteConfirm, confirmDelete, doDelete } = useDelete<AgentMemoryRecord>(
  async (m) => {
    await api.deleteMemory(m.id);
    await load();
  },
  "Memory",
);

const editorOpen = computed(() => creating.value || editing.value !== null);

const { canWrite, canDelete } = useEntityPermissions("memories");

const {
  items: memories,
  total,
  loading,
  error,
  reload: load,
} = useListPage<AgentMemoryRecord>({
  fetch: () =>
    api.getMemories({
      q: filterText.value || undefined,
      memory_type: filterType.value || undefined,
      limit: pageSize,
      offset: currentOffset.value,
    }),
  entityName: "memories",
});

const hasNextPage = computed(() => currentOffset.value + pageSize < total.value);
const hasPrevPage = computed(() => currentOffset.value > 0);

function blankForm(): MemoryForm {
  return {
    content: "",
    memory_type: "fact",
    correlation_key: "",
    labelsCsv: "",
    confidence: "",
    expires_at: "",
  };
}

function formFromMemory(m: AgentMemoryRecord): MemoryForm {
  return {
    content: m.content,
    memory_type: m.memory_type,
    correlation_key: m.correlation_key ?? "",
    labelsCsv: m.labels
      ? Object.entries(m.labels)
          .map(([k, v]) => `${k}=${v}`)
          .join(", ")
      : "",
    confidence: m.confidence != null ? String(m.confidence) : "",
    expires_at: m.expires_at ? m.expires_at.slice(0, 16) : "",
  };
}

function toPayload(f: MemoryForm): AgentMemoryInput {
  const payload: AgentMemoryInput = {
    content: f.content.trim(),
    memory_type: f.memory_type,
  };
  if (f.correlation_key.trim()) payload.correlation_key = f.correlation_key.trim();
  if (f.labelsCsv.trim()) {
    const labels: Record<string, string> = {};
    for (const pair of f.labelsCsv.split(",")) {
      const eq = pair.indexOf("=");
      if (eq > 0) {
        labels[pair.slice(0, eq).trim()] = pair.slice(eq + 1).trim();
      }
    }
    if (Object.keys(labels).length) payload.labels = labels;
  }
  if (f.confidence.trim()) {
    const v = parseFloat(f.confidence);
    if (!Number.isNaN(v)) payload.confidence = v;
  }
  if (f.expires_at) {
    const iso = localDatetimeToRFC3339(f.expires_at);
    if (iso) payload.expires_at = iso;
  }
  return payload;
}

function resetAndLoad() {
  currentOffset.value = 0;
  load();
}

function openCreate() {
  form.value = blankForm();
  creating.value = true;
  editing.value = null;
  formError.value = "";
}

function openEdit(m: AgentMemoryRecord) {
  form.value = formFromMemory(m);
  editing.value = m;
  creating.value = false;
  formError.value = "";
}

function closeEditor() {
  creating.value = false;
  editing.value = null;
  formError.value = "";
}

async function save() {
  if (!form.value.content.trim()) {
    formError.value = "Content is required.";
    return;
  }
  await withSubmit(
    async () => {
      if (editing.value) {
        await api.updateMemory(editing.value.id, form.value.content);
      } else {
        const payload = toPayload(form.value);
        await api.createMemory(payload);
      }
      closeEditor();
      await load();
    },
    editing.value ? "Memory updated" : "Memory created",
  );
}

function nextPage() {
  currentOffset.value += pageSize;
  load();
}

function prevPage() {
  currentOffset.value = Math.max(0, currentOffset.value - pageSize);
  load();
}

function typeBadgeClass(t: string): string {
  switch (t) {
    case "fact":
      return "border border-[var(--text-badge-info)] text-[var(--text-badge-info)]";
    case "pattern":
      return "border border-[var(--text-badge-warning)] text-[var(--text-badge-warning)]";
    case "procedure":
      return "border border-[var(--text-badge-resolved)] text-[var(--text-badge-resolved)]";
    default:
      return "border border-[var(--border-secondary)] text-[var(--text-muted)]";
  }
}
const { showSearch } = usePageHeaderActions({
  title: "Memory",
  titleIcon: Brain,
  searchInput: filterText,
  searchPlaceholder: "Search memories...",
  showAdd: canWrite,
  onAdd: openCreate,
  addLabel: "Add memory",
});

onMounted(() => {
  load();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <p class="text-sm text-[var(--text-muted)]">
      Agent shared memory stores facts, patterns, and procedures extracted from investigations.
      Memories are automatically injected into agent prompts during future investigations to improve
      resolution quality.
    </p>

    <div v-if="showSearch" class="flex flex-wrap items-end gap-2">
      <div class="flex flex-col gap-1">
        <FormLabel>Type</FormLabel>
        <Select v-model="filterType" class="min-w-32" @change="resetAndLoad">
          <option value="">All types</option>
          <option value="fact">fact</option>
          <option value="pattern">pattern</option>
          <option value="procedure">procedure</option>
        </Select>
      </div>
      <Button :loading="loading" @click="resetAndLoad">Filter</Button>
    </div>

    <ErrorBanner :message="error" />

    <p v-if="loading && memories.length === 0" class="text-sm text-[var(--text-muted)]">
      Loading memories...
    </p>

    <EmptyState v-if="!loading && memories.length === 0" message="No memories yet.">
      <template #icon>
        <Brain class="mb-2 h-6 w-6 opacity-40" />
      </template>
      <template v-if="canWrite" #footer>
        <p class="mt-1 text-xs">
          Add facts, patterns, and procedures that agents should recall during investigations.
        </p>
      </template>
    </EmptyState>

    <div class="space-y-3">
      <Card v-for="m in memories" :key="m.id">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0 flex-1 space-y-1">
            <div class="flex flex-wrap items-center gap-2">
              <span
                class="rounded px-1.5 py-0.5 text-xs font-semibold uppercase"
                :class="typeBadgeClass(m.memory_type)"
                >{{ m.memory_type }}</span
              >
              <span
                v-if="m.agent_type"
                class="inline-flex items-center gap-1 rounded border border-[var(--text-badge-pending)] px-1.5 py-0.5 text-xs font-medium text-[var(--text-badge-pending)]"
                :title="`Agent: ${m.agent_name ?? 'unknown'}`"
              >
                <Bot class="h-3 w-3" />
                {{ m.agent_name || m.agent_type }}
              </span>
              <span
                v-if="m.confidence != null"
                class="rounded bg-[var(--bg-code)] px-1.5 py-0.5 text-xs font-medium text-[var(--text-muted)]"
              >
                {{ (m.confidence * 100).toFixed(0) }}%
              </span>
              <span
                v-if="m.access_count > 0"
                class="rounded bg-[var(--bg-code)] px-1.5 py-0.5 text-xs text-[var(--text-muted)]"
                :title="`Accessed ${m.access_count} times by agents`"
              >
                {{ m.access_count }}x used
              </span>
            </div>
            <p class="whitespace-pre-wrap text-sm text-[var(--text-primary)]">{{ m.content }}</p>
            <div v-if="m.labels && Object.keys(m.labels).length" class="flex flex-wrap gap-1">
              <span
                v-for="(v, k) in m.labels"
                :key="k"
                class="rounded bg-[var(--bg-code)] px-1.5 py-0.5 text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]"
                >{{ k }}={{ v }}</span
              >
            </div>
            <div class="flex flex-wrap items-center gap-3 text-xs text-[var(--text-secondary)]">
              <span>{{ formatTime(m.created_at) }}</span>
              <span v-if="m.correlation_key" class="font-mono">key: {{ m.correlation_key }}</span>
              <span v-if="m.investigation_id" class="font-mono"
                >inv: {{ m.investigation_id.slice(0, 8) }}...</span
              >
              <span v-if="m.expires_at">expires {{ formatTime(m.expires_at!) }}</span>
            </div>
          </div>
          <div v-if="canWrite || canDelete" class="flex gap-2">
            <Button v-if="canWrite" size="sm" @click="openEdit(m)">Edit</Button>
            <Button v-if="canDelete" size="sm" variant="destructive" @click="confirmDelete(m)"
              >Delete</Button
            >
          </div>
        </div>
      </Card>
    </div>

    <div v-if="memories.length > 0" class="flex items-center justify-between">
      <p class="text-xs text-[var(--text-muted)]">
        {{ currentOffset + 1 }}–{{ Math.min(currentOffset + pageSize, total) }} of
        {{ total }}
      </p>
      <div class="flex gap-2">
        <Button size="sm" variant="outline" :disabled="!hasPrevPage" @click="prevPage">
          Previous
        </Button>
        <Button size="sm" variant="outline" :disabled="!hasNextPage" @click="nextPage">
          Next
        </Button>
      </div>
    </div>
  </section>

  <Modal
    v-model:open="editorOpen"
    :title="editing ? 'Edit memory' : 'New memory'"
    maxWidth="3xl"
    :showHeader="false"
    :showFooter="false"
  >
    <div class="mb-3 flex items-center justify-between">
      <h3 id="memory-editor-title" class="text-base font-semibold">
        {{ editing ? "Edit memory" : "New memory" }}
      </h3>
      <Button @click="closeEditor">Cancel</Button>
    </div>

    <div class="space-y-4">
      <div class="grid gap-3 md:grid-cols-[200px_1fr]">
        <div class="flex flex-col gap-1">
          <FormLabel for="memory-type">Type</FormLabel>
          <Select id="memory-type" v-model="form.memory_type">
            <option value="fact">fact</option>
            <option value="pattern">pattern</option>
            <option value="procedure">procedure</option>
          </Select>
        </div>
        <div class="flex flex-col gap-1">
          <FormLabel for="memory-corr-key">Correlation key (optional)</FormLabel>
          <Input
            id="memory-corr-key"
            v-model="form.correlation_key"
            placeholder="e.g. deployment:payments-api"
          />
        </div>
      </div>

      <div class="flex flex-col gap-1">
        <FormLabel for="memory-content">Content</FormLabel>
        <Textarea
          id="memory-content"
          v-model="form.content"
          rows="6"
          class="font-mono"
          placeholder="A self-contained fact, pattern, or procedure that agents should remember..."
        />
      </div>

      <div class="grid gap-3 md:grid-cols-2">
        <div class="flex flex-col gap-1">
          <FormLabel for="memory-labels">Labels (key=value, comma-separated)</FormLabel>
          <Input
            id="memory-labels"
            v-model="form.labelsCsv"
            placeholder="service=payments, team=platform"
          />
        </div>
        <div class="flex flex-col gap-1">
          <FormLabel for="memory-confidence">Confidence (0–1)</FormLabel>
          <NumberInput
            id="memory-confidence"
            v-model="form.confidence"
            min="0"
            max="1"
            step="0.1"
            placeholder="0.9"
          />
        </div>
      </div>

      <div class="flex flex-col gap-1">
        <FormLabel for="memory-expires">Expires at (optional)</FormLabel>
        <DateTimePicker
          id="memory-expires"
          v-model="form.expires_at"
          placeholder="Pick expiry date & time"
        />
      </div>

      <ErrorBanner :message="formError" />

      <div class="flex justify-end gap-2">
        <Button variant="outline" @click="closeEditor">Cancel</Button>
        <Button :loading="saving" @click="save">{{ editing ? "Save" : "Create" }}</Button>
      </div>
    </div>
  </Modal>

  <ConfirmDialog
    v-model:open="showDeleteConfirm"
    title="Delete memory"
    :message="`Delete this ${deleteTarget?.memory_type ?? 'memory'}? This cannot be undone.`"
    confirm-label="Delete"
    :destructive="true"
    @confirm="doDelete"
  />
</template>
