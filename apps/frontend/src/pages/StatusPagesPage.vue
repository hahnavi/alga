<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { onMounted, ref } from "vue";
import { Activity, Pencil, Trash2, ExternalLink } from "@lucide/vue";
import { api, type StatusPageRecord, type StatusPageVisibility } from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import Textarea from "@/components/ui/Textarea.vue";
import Select from "@/components/ui/Select.vue";
import Card from "@/components/ui/Card.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import Modal from "@/components/ui/Modal.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Switch from "@/components/ui/Switch.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { useToast } from "@/lib/toast";
import { useDelete } from "@/composables/useDelete";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { useListFilter } from "@/composables/useListFilter";
import { formatTime } from "@/lib/time";
import { useRouter } from "vue-router";

defineOptions({ name: "StatusPagesPage" });

const router = useRouter();
const { push } = useToast();

const pages = ref<StatusPageRecord[]>([]);
const loading = ref(false);
const error = ref("");
const searchInput = ref("");

const formOpen = ref(false);
const editing = ref<StatusPageRecord | null>(null);
const form = ref(blankForm());

const { submitting: saving, formError, withSubmit } = useFormSubmit();
const { canWrite, canDelete } = useEntityPermissions("statuspages");

const { showDeleteConfirm, confirmDelete, doDelete } = useDelete<StatusPageRecord>(async (p) => {
  await api.deleteStatusPage(p.id);
  await loadPages();
}, "Status page");

const VISIBILITY_OPTIONS: StatusPageVisibility[] = ["internal", "public"];

type PageForm = {
  name: string;
  slug: string;
  description: string;
  visibility: StatusPageVisibility;
  enabled: boolean;
};

function blankForm(): PageForm {
  return { name: "", slug: "", description: "", visibility: "internal", enabled: true };
}

function slugify(name: string): string {
  return name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

function formFromPage(p: StatusPageRecord): PageForm {
  return {
    name: p.name,
    slug: p.slug,
    description: p.description ?? "",
    visibility: p.visibility,
    enabled: p.enabled,
  };
}

const filteredPages = useListFilter(pages, ["name", "slug"], searchInput);

async function loadPages() {
  loading.value = true;
  error.value = "";
  try {
    pages.value = await api.getStatusPages();
  } catch (e: unknown) {
    error.value = getErrorMessage(e, "Failed to load status pages");
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editing.value = null;
  form.value = blankForm();
  formOpen.value = true;
  formError.value = "";
}

function openEdit(p: StatusPageRecord) {
  editing.value = p;
  form.value = formFromPage(p);
  formOpen.value = true;
  formError.value = "";
}

async function save() {
  await withSubmit(
    async () => {
      const payload = {
        name: form.value.name.trim(),
        slug: form.value.slug.trim(),
        description: form.value.description.trim(),
        visibility: form.value.visibility,
        enabled: form.value.enabled,
      };
      if (editing.value) {
        await api.updateStatusPage(editing.value.id, payload);
        push("Status page updated", "success");
      } else {
        await api.createStatusPage(payload);
        push("Status page created", "success");
      }
      formOpen.value = false;
      editing.value = null;
      await loadPages();
    },
    editing.value ? "Status page updated" : "Status page created",
  );
}

async function toggleEnabled(p: StatusPageRecord) {
  try {
    await api.updateStatusPage(p.id, { enabled: !p.enabled });
    await loadPages();
  } catch (e: unknown) {
    push(getErrorMessage(e, "Failed to toggle"), "error");
  }
}

function viewPage(p: StatusPageRecord) {
  router.push(`/status/${p.slug}`);
}

usePageHeaderActions({
  title: "Status Pages",
  titleIcon: Activity,
  searchInput,
  searchPlaceholder: "Search status pages...",
  showAdd: canWrite,
  onAdd: openCreate,
  addLabel: "New Status Page",
});

onMounted(() => {
  loadPages();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <ErrorBanner v-if="error" :message="error" @dismiss="error = ''" />

    <p class="text-sm text-[var(--text-muted)]">
      Status pages communicate system health. Add components, set their status during incidents, and
      share the page link so stakeholders see live impact.
    </p>

    <div class="flex flex-wrap items-center gap-2">
      <span class="text-sm text-[var(--text-muted)]"
        >{{ filteredPages.length }} {{ filteredPages.length === 1 ? "page" : "pages" }}</span
      >
      <div class="flex-1" />
    </div>

    <LoadingSpinner v-if="loading && pages.length === 0" />

    <EmptyState
      v-else-if="filteredPages.length === 0"
      :message="searchInput ? 'No status pages match your search.' : 'No status pages configured.'"
    >
      <template #icon>
        <Activity class="mb-2 h-6 w-6 opacity-40" />
      </template>
    </EmptyState>

    <div v-else class="space-y-3">
      <Card v-for="p in filteredPages" :key="p.id">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div class="min-w-0 flex-1 space-y-1">
            <div class="flex flex-wrap items-center gap-2">
              <span class="font-medium">{{ p.name }}</span>
              <span class="badge badge-muted">{{ p.visibility }}</span>
              <span v-if="!p.enabled" class="badge badge-muted">Disabled</span>
            </div>
            <div
              class="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--text-muted)]"
            >
              <span class="font-mono">/status/{{ p.slug }}</span>
              <span>Updated {{ formatTime(p.updated_at) }}</span>
            </div>
            <p v-if="p.description" class="text-sm text-[var(--text-secondary)]">
              {{ p.description }}
            </p>
          </div>
          <div class="flex shrink-0 flex-wrap items-center gap-2">
            <Button variant="outline" size="sm" @click="viewPage(p)">
              <ExternalLink class="mr-1 h-3.5 w-3.5" /> View
            </Button>
            <label class="flex items-center gap-2 text-sm text-[var(--text-muted)]">
              <Switch :modelValue="p.enabled" @update:modelValue="toggleEnabled(p)" />
            </label>
            <Button v-if="canWrite" variant="outline" size="sm" @click="openEdit(p)">
              <Pencil class="mr-1 h-3.5 w-3.5" /> Edit
            </Button>
            <Button v-if="canDelete" variant="destructive" size="sm" @click="confirmDelete(p)">
              <Trash2 class="mr-1 h-3.5 w-3.5" /> Delete
            </Button>
          </div>
        </div>
      </Card>
    </div>

    <Modal
      v-model:open="formOpen"
      :title="editing ? 'Edit Status Page' : 'Create Status Page'"
      :loading="saving"
      @confirm="save"
      @close="editing = null"
    >
      <ErrorBanner v-if="formError" :message="formError" @dismiss="formError = ''" />
      <div class="space-y-4">
        <div>
          <FormLabel for="status-page-form-name">Name</FormLabel>
          <Input
            id="status-page-form-name"
            v-model="form.name"
            type="text"
            class="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-2 text-sm"
            placeholder="e.g. Customer Platform Status"
            @input="!editing && (form.slug = slugify(form.name))"
          />
        </div>
        <div>
          <FormLabel for="status-page-form-slug">Slug (URL path)</FormLabel>
          <Input
            id="status-page-form-slug"
            v-model="form.slug"
            type="text"
            class="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-2 font-mono text-sm"
            placeholder="e.g. customer-platform"
          />
        </div>
        <div>
          <FormLabel for="status-page-form-description">Description</FormLabel>
          <Textarea
            id="status-page-form-description"
            v-model="form.description"
            rows="2"
            class="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-2 text-sm"
            placeholder="Shown at the top of the status page"
          />
        </div>
        <div>
          <FormLabel for="status-page-form-visibility">Visibility</FormLabel>
          <Select
            id="status-page-form-visibility"
            v-model="form.visibility"
            class="w-full rounded-md p-2 text-sm"
          >
            <option v-for="v in VISIBILITY_OPTIONS" :key="v" :value="v">{{ v }}</option>
          </Select>
        </div>
        <label class="flex items-center gap-2 text-sm font-medium">
          <Switch v-model="form.enabled" />
          Enabled
        </label>
      </div>
    </Modal>

    <ConfirmDialog
      v-model:open="showDeleteConfirm"
      title="Delete Status Page"
      message="Are you sure you want to delete this status page and all its components? This action cannot be undone."
      confirm-label="Delete"
      :destructive="true"
      @confirm="doDelete"
    />
  </section>
</template>
