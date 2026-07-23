<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { BookOpen, Bot, User as UserIcon, X, Filter, Tag } from "@lucide/vue";
import { api, type KnowledgeNote, type KnowledgeNoteInput, type RouteCondition } from "@/lib/api";
import {
  summarizeCondition,
  CONDITION_SOURCE_OPTIONS,
  CONDITION_OPERATOR_OPTIONS,
} from "@/lib/routeConditions";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Input from "@/components/ui/Input.vue";
import NumberInput from "@/components/ui/NumberInput.vue";
import Select from "@/components/ui/Select.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import Modal from "@/components/ui/Modal.vue";
import MarkdownEditor from "@/components/ui/MarkdownEditor.vue";
import DateTimePicker from "@/components/ui/DateTimePicker.vue";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { useDelete } from "@/composables/useDelete";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { useListPage } from "@/composables/useListPage";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { formatTime, localDatetimeToRFC3339 } from "@/lib/time";

defineOptions({ name: "KnowledgePage" });

type ConditionEntry = Omit<RouteCondition, "value"> & { value: string };

type NoteForm = {
  kind: string;
  title: string;
  body_markdown: string;
  tags: string[];
  selectors: ConditionEntry[];
  confidence: string;
  expires_at: string;
};

const filterKind = ref("");
const filterTag = ref("");
const filterText = ref("");

const editing = ref<KnowledgeNote | null>(null);
const creating = ref(false);
const form = ref<NoteForm>(blankForm());
const tagInput = ref("");
const { submitting: saving, formError, withSubmit } = useFormSubmit();

const { deleteTarget, showDeleteConfirm, confirmDelete, doDelete } = useDelete<KnowledgeNote>(
  async (note) => {
    await api.deleteKnowledgeNote(note.id);
    await load();
  },
  "Knowledge note",
);

const editorOpen = computed(() => creating.value || editing.value !== null);

const { canWrite, canDelete } = useEntityPermissions("knowledge");

const {
  items: notes,
  loading,
  error,
  reload: load,
} = useListPage<KnowledgeNote>({
  fetch: () =>
    api.getKnowledgeNotes({
      kind: filterKind.value || undefined,
      tag: filterTag.value || undefined,
      q: filterText.value || undefined,
      limit: 100,
    }),
  entityName: "knowledge notes",
});

function blankForm(): NoteForm {
  return {
    kind: "runbook",
    title: "",
    body_markdown: "",
    tags: [],
    selectors: [{ source: "labels", field: "alertname", operator: "exact", value: "" }],
    confidence: "",
    expires_at: "",
  };
}

function formFromNote(n: KnowledgeNote): NoteForm {
  return {
    kind: n.kind,
    title: n.title,
    body_markdown: n.body_markdown,
    tags: [...(n.tags ?? [])],
    selectors: (n.selectors ?? []).map((c) => ({
      source: c.source,
      field: c.field,
      operator: c.operator,
      value: c.value ?? "",
    })) || [{ source: "labels", field: "alertname", operator: "exact", value: "" }],
    confidence: n.confidence != null ? String(n.confidence) : "",
    expires_at: n.expires_at ? n.expires_at.slice(0, 16) : "",
  };
}

function toPayload(f: NoteForm): KnowledgeNoteInput {
  const selectors: RouteCondition[] = f.selectors
    .filter((c) => c.field.trim() !== "")
    .map((c) => ({
      source: c.source,
      field: c.field.trim(),
      operator: c.operator,
      value: c.value.trim(),
    }));
  const tags = f.tags.map((t) => t.trim()).filter(Boolean);
  const payload: KnowledgeNoteInput = {
    kind: f.kind.trim(),
    title: f.title.trim(),
    body_markdown: f.body_markdown,
    tags,
    selectors,
  };
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

function kindBadgeClass(kind: string): string {
  switch (kind) {
    case "runbook":
      return "badge-blue";
    case "known_issue":
      return "badge-orange";
    case "service_owner":
      return "badge-purple";
    case "fact":
      return "badge-muted";
    default:
      return "badge-muted";
  }
}

function openCreate() {
  form.value = blankForm();
  creating.value = true;
  editing.value = null;
  formError.value = "";
  tagInput.value = "";
}

function openEdit(n: KnowledgeNote) {
  form.value = formFromNote(n);
  editing.value = n;
  creating.value = false;
  formError.value = "";
  tagInput.value = "";
}

function closeEditor() {
  creating.value = false;
  editing.value = null;
  formError.value = "";
}

function addSelector() {
  form.value.selectors.push({ source: "labels", field: "", operator: "exact", value: "" });
}

function removeSelector(i: number) {
  form.value.selectors.splice(i, 1);
  if (form.value.selectors.length === 0) addSelector();
}

function commitTag() {
  const v = tagInput.value.trim();
  if (v && !form.value.tags.includes(v)) {
    form.value.tags.push(v);
  }
  tagInput.value = "";
}

function removeTag(i: number) {
  form.value.tags.splice(i, 1);
}

function removeLastTag() {
  if (tagInput.value === "" && form.value.tags.length > 0) {
    form.value.tags.pop();
  }
}

async function save() {
  formError.value = "";
  if (!form.value.kind.trim() || !form.value.title.trim()) {
    formError.value = "Kind and title are required.";
    return;
  }
  await withSubmit(
    async () => {
      const payload = toPayload(form.value);
      if (editing.value) {
        await api.updateKnowledgeNote(editing.value.id, payload);
      } else {
        await api.createKnowledgeNote(payload);
      }
      closeEditor();
      await load();
    },
    editing.value ? "Knowledge note updated" : "Knowledge note created",
  );
}

const { showSearch } = usePageHeaderActions({
  title: "Knowledge",
  titleIcon: BookOpen,
  searchInput: filterText,
  searchPlaceholder: "Search knowledge notes...",
  showFilters: false,
  showAdd: canWrite,
  onAdd: openCreate,
  addLabel: "Add note",
});

onMounted(() => {
  load();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <p class="text-sm text-[var(--text-muted)]">
      Knowledge notes (runbooks, known issues, service owners, facts) are auto-injected into agent
      prompts when their selectors match the alerts under investigation. Selectors use the same
      syntax as forwarding routes.
    </p>

    <div v-if="showSearch" class="flex flex-wrap items-end gap-2">
      <div class="flex flex-col gap-1">
        <FormLabel>Kind</FormLabel>
        <Select v-model="filterKind" class="min-w-32" @change="load">
          <option value="">All kinds</option>
          <option value="runbook">runbook</option>
          <option value="known_issue">known_issue</option>
          <option value="service_owner">service_owner</option>
          <option value="fact">fact</option>
        </Select>
      </div>
      <div class="flex flex-col gap-1">
        <FormLabel>Tag</FormLabel>
        <Input v-model="filterTag" placeholder="tag" class="min-w-32" @keyup.enter="load" />
      </div>
      <Button :loading="loading" @click="load">Filter</Button>
    </div>

    <ErrorBanner :message="error" />

    <LoadingSpinner
      v-if="loading && notes.length === 0"
      centered
      label="Loading knowledge notes..."
    />

    <Card v-if="!loading && notes.length === 0">
      <EmptyState message="No knowledge notes yet.">
        <template #footer>
          <p v-if="canWrite" class="mt-1 field-label">
            Add runbooks and known issues that Alga should attach to matching investigations.
          </p>
        </template>
      </EmptyState>
    </Card>

    <div class="space-y-3">
      <Card v-for="n in notes" :key="n.id">
        <div class="flex flex-col gap-2.5">
          <div class="flex flex-wrap items-start justify-between gap-2">
            <div class="flex flex-wrap items-center gap-1.5">
              <span class="badge" :class="kindBadgeClass(n.kind)">{{ n.kind }}</span>
              <span
                v-if="n.author_type === 'agent'"
                class="badge badge-purple inline-flex items-center"
                :title="`Authored by ${n.author_name ?? 'agent'}`"
              >
                <Bot class="mr-1 h-3 w-3" />agent
              </span>
              <span
                v-else
                class="badge badge-muted inline-flex items-center"
                :title="`Authored by ${n.author_name ?? 'user'}`"
              >
                <UserIcon class="mr-1 h-3 w-3" />user
              </span>
              <span
                v-if="n.confidence != null"
                class="rounded bg-[var(--bg-code)] px-1.5 py-0.5 text-xs font-medium text-[var(--text-muted)]"
                title="Confidence"
                >{{ n.confidence.toFixed(2) }}</span
              >
            </div>
            <div v-if="canWrite || canDelete" class="flex shrink-0 gap-2">
              <Button v-if="canWrite" size="sm" variant="outline" @click="openEdit(n)">Edit</Button>
              <Button v-if="canDelete" size="sm" variant="destructive" @click="confirmDelete(n)"
                >Delete</Button
              >
            </div>
          </div>

          <div class="min-w-0 space-y-1.5">
            <p class="text-base font-semibold leading-snug text-[var(--text-primary)]">
              {{ n.title }}
            </p>
            <p
              v-if="n.body_markdown"
              class="line-clamp-2 whitespace-pre-wrap text-sm text-[var(--text-secondary)]"
            >
              {{ n.body_markdown }}
            </p>
          </div>

          <div v-if="n.tags?.length" class="flex flex-wrap items-center gap-1.5">
            <span
              v-for="t in n.tags"
              :key="t"
              class="inline-flex items-center gap-1 rounded border border-[var(--border-secondary)] bg-[var(--bg-code)] px-1.5 py-0.5 text-xs font-medium text-[var(--text-secondary)]"
            >
              <Tag class="h-3 w-3 text-[var(--text-muted)]" />
              {{ t }}
            </span>
          </div>

          <div
            v-if="n.selectors?.length"
            class="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-2"
          >
            <div
              class="mb-1.5 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--text-muted)]"
            >
              <Filter class="h-3 w-3" />Matches when
            </div>
            <div class="flex flex-wrap gap-1.5">
              <span
                v-for="(s, i) in n.selectors"
                :key="i"
                class="inline-flex items-center rounded border border-[var(--border-secondary)] bg-[var(--bg-code)] px-2 py-0.5 font-mono text-xs text-[var(--text-secondary)]"
                >{{ summarizeCondition(s) }}</span
              >
            </div>
          </div>

          <div
            class="flex flex-wrap items-center justify-between gap-2 border-t border-[var(--border-primary)] pt-2.5"
          >
            <p class="text-xs text-[var(--text-muted)]">
              Updated {{ formatTime(n.updated_at)
              }}<span v-if="n.expires_at"> · expires {{ formatTime(n.expires_at!) }}</span>
            </p>
            <span class="break-all font-mono text-xs text-[var(--text-muted)]">{{ n.id }}</span>
          </div>
        </div>
      </Card>
    </div>
  </section>

  <Modal
    v-model:open="editorOpen"
    :title="editing ? 'Edit knowledge note' : 'New knowledge note'"
    maxWidth="3xl"
    :showHeader="false"
    :showFooter="false"
  >
    <div class="mb-3 flex items-center justify-between">
      <h3 id="knowledge-editor-title" class="text-base font-semibold">
        {{ editing ? "Edit knowledge note" : "New knowledge note" }}
      </h3>
      <Button @click="closeEditor">Cancel</Button>
    </div>

    <div class="space-y-4">
      <div class="grid gap-3 md:grid-cols-[200px_1fr]">
        <div class="flex flex-col gap-1">
          <FormLabel for="knowledge-kind">Kind</FormLabel>
          <Select id="knowledge-kind" v-model="form.kind">
            <option value="runbook">runbook</option>
            <option value="known_issue">known_issue</option>
            <option value="service_owner">service_owner</option>
            <option value="fact">fact</option>
          </Select>
        </div>
        <div class="flex flex-col gap-1">
          <FormLabel for="knowledge-title">Title</FormLabel>
          <Input id="knowledge-title" v-model="form.title" placeholder="Short, descriptive" />
        </div>
      </div>

      <div class="knowledge-editor flex flex-col gap-1">
        <FormLabel>Body (markdown)</FormLabel>
        <MarkdownEditor
          v-model="form.body_markdown"
          :enable-internal-note="false"
          :show-send-button="false"
          placeholder="Remediation steps, references, owners, caveats... (markdown supported)"
        />
      </div>

      <div class="flex flex-col gap-1">
        <FormLabel>Tags</FormLabel>
        <div
          class="flex min-h-[2.25rem] flex-wrap items-center gap-1.5 rounded-md border border-[var(--border-input)] bg-[var(--bg-input)] px-1.5 py-1 transition-colors focus-within:border-[var(--focus-ring)] focus-within:ring-1 focus-within:ring-[var(--focus-ring)]"
        >
          <span
            v-for="(t, i) in form.tags"
            :key="i"
            class="inline-flex items-center gap-1 rounded border border-[var(--border-secondary)] bg-[var(--bg-code)] px-1.5 py-0.5 text-xs font-medium text-[var(--text-secondary)]"
          >
            {{ t }}
            <button
              type="button"
              class="ml-0.5 text-[var(--text-muted)] transition-colors hover:text-[var(--text-primary)]"
              aria-label="Remove tag"
              @click="removeTag(i)"
            >
              <X class="h-3 w-3" />
            </button>
          </span>
          <Input
            v-model="tagInput"
            :placeholder="form.tags.length ? '' : 'Add a tag, then press space or enter'"
            class="min-w-[140px] flex-1 border-0 min-h-0 bg-transparent px-1 py-0.5 text-sm text-[var(--text-input)] outline-none placeholder:text-[var(--text-muted)]"
            @keydown.enter.prevent="commitTag"
            @keydown.space.prevent="commitTag"
            @keydown.delete="removeLastTag"
          />
        </div>
      </div>

      <div class="grid gap-3 md:grid-cols-2">
        <div class="flex flex-col gap-1">
          <FormLabel for="knowledge-confidence">Confidence (0–1)</FormLabel>
          <NumberInput
            id="knowledge-confidence"
            v-model="form.confidence"
            min="0"
            max="1"
            step="0.1"
            placeholder="0.9"
          />
        </div>
        <div class="flex flex-col gap-1">
          <FormLabel for="knowledge-expires">Expires at (optional)</FormLabel>
          <DateTimePicker
            id="knowledge-expires"
            v-model="form.expires_at"
            placeholder="Pick expiry date & time"
          />
        </div>
      </div>

      <div class="space-y-2">
        <p class="text-sm font-medium">Selectors</p>
        <p class="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
          All selectors must match an alert in the investigation (AND). Reuses the same fields and
          operators as forwarding routes.
        </p>
        <div
          v-for="(entry, idx) in form.selectors"
          :key="idx"
          class="grid gap-2 md:grid-cols-[1fr_1fr_1fr_1fr_auto]"
        >
          <Select v-model="entry.source">
            <option v-for="s in CONDITION_SOURCE_OPTIONS" :key="s.value" :value="s.value">
              {{ s.label }}
            </option>
          </Select>
          <Input v-model="entry.field" placeholder="field (e.g. alertname, namespace)" />
          <Select v-model="entry.operator">
            <option v-for="o in CONDITION_OPERATOR_OPTIONS" :key="o.value" :value="o.value">
              {{ o.label }}
            </option>
          </Select>
          <Input
            v-model="entry.value"
            :placeholder="
              entry.operator === 'exists' || entry.operator === 'not_exists'
                ? 'No value needed'
                : 'value'
            "
            :disabled="entry.operator === 'exists' || entry.operator === 'not_exists'"
          />
          <Button size="sm" variant="destructive" @click="removeSelector(idx)">Remove</Button>
        </div>
        <Button size="sm" @click="addSelector">Add selector</Button>
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
    title="Delete knowledge note"
    :message="`Delete '${deleteTarget?.title ?? ''}'? This cannot be undone.`"
    confirm-label="Delete"
    :destructive="true"
    @confirm="doDelete"
  />
</template>

<style scoped>
.knowledge-editor :deep(.markdown-editor-content) {
  min-height: 220px;
}
</style>
