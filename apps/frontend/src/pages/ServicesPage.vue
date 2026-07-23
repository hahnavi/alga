<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { Layers, Plus, X } from "@lucide/vue";
import { api, type ServiceRecord } from "@/lib/api";
import { useSearchDebounce } from "@/composables/useSearchDebounce";
import { useListPage } from "@/composables/useListPage";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { CONDITION_SOURCE_OPTIONS, CONDITION_OPERATOR_OPTIONS } from "@/lib/routeConditions";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import NumberInput from "@/components/ui/NumberInput.vue";
import Textarea from "@/components/ui/Textarea.vue";
import Select from "@/components/ui/Select.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import ServiceStatusBadge from "@/components/ui/ServiceStatusBadge.vue";
import InteractiveCard from "@/components/ui/InteractiveCard.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Modal from "@/components/ui/Modal.vue";
import { useToast } from "@/lib/toast";

defineOptions({ name: "ServicesPage" });

const router = useRouter();
const { push } = useToast();
const { canWrite } = useEntityPermissions("services");

const searchInput = ref("");
const navigatingId = ref<string | null>(null);

const showCreateDialog = ref(false);
const createSubmitting = ref(false);
const createError = ref("");
const createName = ref("");
const createDisplayName = ref("");
const createDescription = ref("");
const createSlaResponse = ref("");
const createSlaResolve = ref("");
const createLabelMatchers = ref<
  Array<{ source: string; field: string; operator: string; value: string }>
>([]);

const {
  items: services,
  total,
  loading,
  error,
  reload: loadServices,
} = useListPage<ServiceRecord>({
  fetch: () => {
    const params: { status?: string; q?: string; limit?: number; skip?: number } = {};
    if (searchInput.value.trim()) params.q = searchInput.value.trim();
    params.limit = 100;
    return api.getServices(params);
  },
  entityName: "services",
});

function resetCreateForm() {
  createName.value = "";
  createDisplayName.value = "";
  createDescription.value = "";
  createSlaResponse.value = "";
  createSlaResolve.value = "";
  createLabelMatchers.value = [];
  createError.value = "";
  createSubmitting.value = false;
}

function closeCreateDialog() {
  if (createSubmitting.value) return;
  showCreateDialog.value = false;
}

watch(
  () => showCreateDialog.value,
  (isOpen) => {
    if (isOpen) {
      resetCreateForm();
    }
  },
);

async function submitCreate() {
  if (createSubmitting.value) return;
  createError.value = "";
  const name = createName.value.trim();
  if (!name) {
    createError.value = "Name is required.";
    return;
  }

  createSubmitting.value = true;
  try {
    const input: Parameters<typeof api.createService>[0] = {
      name,
      display_name: createDisplayName.value.trim(),
    };
    const desc = createDescription.value.trim();
    if (desc) input.description = desc;
    const resp = createSlaResponse.value.trim();
    if (resp) input.sla_response_minutes = Number(resp);
    const resl = createSlaResolve.value.trim();
    if (resl) input.sla_resolve_minutes = Number(resl);
    if (createLabelMatchers.value.length > 0) {
      input.label_matchers = createLabelMatchers.value.map((m) => ({
        source: m.source,
        field: m.field,
        operator: m.operator,
        value: m.value,
      }));
    }

    await api.createService(input);
    push("Service created", "success");
    showCreateDialog.value = false;
    await loadServices();
  } catch (err) {
    const msg = getErrorMessage(err, "Failed to create service");
    createError.value = msg;
    push(msg, "error");
  } finally {
    createSubmitting.value = false;
  }
}

function goToService(serviceId: string) {
  navigatingId.value = serviceId;
  router.push(`/services/${serviceId}`);
}

const { scheduleSearchReload } = useSearchDebounce(() => loadServices(), 400);

function addLabelMatcher() {
  createLabelMatchers.value.push({ source: "labels", field: "", operator: "exact", value: "" });
}

function removeLabelMatcher(index: number) {
  createLabelMatchers.value.splice(index, 1);
}

usePageHeaderActions({
  title: "Services",
  titleIcon: Layers,
  searchInput,
  searchPlaceholder: "Search services...",
  onSearchInput: scheduleSearchReload,
  showAdd: canWrite,
  onAdd: () => {
    showCreateDialog.value = true;
  },
  addLabel: "Add service",
});

onMounted(() => {
  loadServices();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <p class="text-xs text-[var(--text-muted)]">{{ total }} service{{ total !== 1 ? "s" : "" }}</p>
    <ErrorBanner :message="error" />
    <LoadingSpinner v-if="loading && services.length === 0" centered />
    <EmptyState v-else-if="services.length === 0" message="No services found.">
      <template #icon>
        <Layers class="mb-2 h-6 w-6 opacity-40" />
      </template>
    </EmptyState>
    <div v-else class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      <InteractiveCard
        v-for="svc in services"
        :key="svc.id"
        :loading="navigatingId === svc.id"
        @navigate="goToService(svc.id)"
      >
        <div class="flex items-start justify-between gap-2">
          <div class="min-w-0 flex-1">
            <h3 class="truncate text-sm font-medium text-[var(--text-primary)]">
              {{ svc.display_name || svc.name }}
            </h3>
            <p class="mt-0.5 text-xs text-[var(--text-muted)]">{{ svc.name }}</p>
            <p
              v-if="svc.description"
              class="mt-1 line-clamp-2 text-xs text-[var(--text-secondary)]"
            >
              {{ svc.description }}
            </p>
          </div>
          <ServiceStatusBadge :status="svc.status" />
        </div>
        <div class="mt-2 flex items-center gap-3 text-xs text-[var(--text-muted)]">
          <span v-if="svc.active_incident_count" class="text-[var(--text-badge-firing)]">
            {{ svc.active_incident_count }} active incident{{
              svc.active_incident_count !== 1 ? "s" : ""
            }}
          </span>
          <span
            >SLA: {{ svc.sla_response_minutes }}m respond / {{ svc.sla_resolve_minutes }}m
            resolve</span
          >
        </div>
      </InteractiveCard>
    </div>
    <Modal
      :open="showCreateDialog"
      title="Create service"
      max-width="xl"
      :prevent-close="createSubmitting"
      @update:open="!$event && closeCreateDialog()"
      @close="closeCreateDialog"
    >
      <form class="space-y-4" @submit.prevent="submitCreate">
        <ErrorBanner :message="createError" />

        <div>
          <FormLabel required for="create-service-name">Name</FormLabel>
          <Input
            id="create-service-name"
            v-model="createName"
            required
            autocomplete="off"
            placeholder="e.g. payment-api"
            :disabled="createSubmitting"
            aria-required="true"
          />
        </div>

        <div>
          <FormLabel for="create-service-display-name">Display name</FormLabel>
          <Input
            id="create-service-display-name"
            v-model="createDisplayName"
            autocomplete="off"
            placeholder="e.g. Payment API"
            :disabled="createSubmitting"
          />
        </div>

        <div>
          <FormLabel for="create-service-desc">Description</FormLabel>
          <Textarea
            id="create-service-desc"
            v-model="createDescription"
            rows="3"
            placeholder="Optional description of the service"
            class="min-h-[4.5rem] w-full resize-y"
            :disabled="createSubmitting"
          />
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <FormLabel for="create-service-sla-response">SLA response (min)</FormLabel>
            <NumberInput
              id="create-service-sla-response"
              v-model="createSlaResponse"
              placeholder="e.g. 15"
              :disabled="createSubmitting"
            />
          </div>
          <div>
            <FormLabel for="create-service-sla-resolve">SLA resolve (min)</FormLabel>
            <NumberInput
              id="create-service-sla-resolve"
              v-model="createSlaResolve"
              placeholder="e.g. 60"
              :disabled="createSubmitting"
            />
          </div>
        </div>

        <div>
          <div class="flex items-center justify-between">
            <FormLabel>Label Matchers</FormLabel>
            <Button
              type="button"
              variant="outline"
              size="sm"
              :disabled="createSubmitting"
              @click="addLabelMatcher"
            >
              <Plus class="h-3.5 w-3.5" />
              Add
            </Button>
          </div>
          <div
            v-if="createLabelMatchers.length === 0"
            class="mt-2 text-xs text-[var(--text-muted)]"
          >
            No label matchers configured.
          </div>
          <div v-else class="mt-2 space-y-2">
            <div
              v-for="(matcher, i) in createLabelMatchers"
              :key="i"
              class="flex flex-col gap-2 rounded-md border border-[var(--border-primary)] p-2"
            >
              <div class="flex items-center gap-2">
                <Select
                  :model-value="matcher.source"
                  @update:model-value="matcher.source = $event"
                  class="w-28"
                >
                  <option
                    v-for="src in CONDITION_SOURCE_OPTIONS"
                    :key="src.value"
                    :value="src.value"
                  >
                    {{ src.label }}
                  </option>
                </Select>
                <Input
                  v-model="matcher.field"
                  placeholder="Field name"
                  class="flex-1"
                  :disabled="matcher.operator === 'exists' || matcher.operator === 'not_exists'"
                />
                <Select
                  :model-value="matcher.operator"
                  @update:model-value="matcher.operator = $event"
                  class="w-24"
                >
                  <option
                    v-for="op in CONDITION_OPERATOR_OPTIONS"
                    :key="op.value"
                    :value="op.value"
                  >
                    {{ op.label }}
                  </option>
                </Select>
                <Input
                  v-model="matcher.value"
                  placeholder="Value"
                  class="flex-1"
                  :disabled="matcher.operator === 'exists' || matcher.operator === 'not_exists'"
                />
                <button
                  type="button"
                  class="cursor-pointer rounded p-1 text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-error)]"
                  title="Remove matcher"
                  @click="removeLabelMatcher(i)"
                >
                  <X class="h-4 w-4" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </form>

      <template #footer>
        <Button variant="outline" :disabled="createSubmitting" @click="closeCreateDialog">
          Cancel
        </Button>
        <Button :disabled="createSubmitting" @click="submitCreate">
          {{ createSubmitting ? "Creating…" : "Create service" }}
        </Button>
      </template>
    </Modal>
  </section>
</template>
