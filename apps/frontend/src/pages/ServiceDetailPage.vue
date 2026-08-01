<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { computed, h, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ArrowRight, ArrowLeft, Clock, AlertTriangle, Plus, Trash2 } from "@lucide/vue";
import {
  api,
  type ServiceRecord,
  type ServiceDependencyRecord,
  type IncidentRecord,
} from "@/lib/api";
import { formatTime } from "@/lib/time";
import { serviceStatusBadgeClass } from "@/lib/alertLabels";
import SeverityBadge from "@/components/ui/SeverityBadge.vue";
import ServiceActionsMenu from "@/components/ui/ServiceActionsMenu.vue";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import NumberInput from "@/components/ui/NumberInput.vue";
import Textarea from "@/components/ui/Textarea.vue";
import Select from "@/components/ui/Select.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Modal from "@/components/ui/Modal.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import { useToast } from "@/lib/toast";
import { useAsyncData } from "@/composables/useAsyncData";
import { useEntityPageHeader } from "@/composables/useEntityPageHeader";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { useDelete } from "@/composables/useDelete";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { CONDITION_SOURCE_OPTIONS, CONDITION_OPERATOR_OPTIONS } from "@/lib/routeConditions";

defineOptions({ name: "ServiceDetailPage" });

const route = useRoute();
const router = useRouter();
const { push } = useToast();
const { canWrite: canEdit, canDelete: canRemove } = useEntityPermissions("services");

const serviceId = computed(() => (route.params.service_id as string) ?? "");
const dependencies = ref<ServiceDependencyRecord[]>([]);
const dependents = ref<ServiceDependencyRecord[]>([]);
const incidents = ref<IncidentRecord[]>([]);
const {
  data: service,
  loading,
  error,
  reload: loadService,
} = useAsyncData(async () => {
  const svc = await api.getService(serviceId.value);
  dependencies.value = svc.dependencies ?? [];
  dependents.value = svc.dependents ?? [];
  await Promise.all([loadDependencies(), loadIncidents()]);
  return svc;
}, "Failed to load service");

const editing = ref(false);
const editDisplayName = ref("");
const editDescription = ref("");
const editSlaResponse = ref("");
const editSlaResolve = ref("");
const editLabelMatchers = ref<
  Array<{ source: string; field: string; operator: string; value: string }>
>([]);
const { submitting: saving, formError, withSubmit } = useFormSubmit();

const {
  showDeleteConfirm,
  deleting,
  confirmDelete: confirmDeleteService,
  doDelete: doDeleteService,
} = useDelete<ServiceRecord>(async (item) => {
  await api.deleteService(item.id);
  router.push("/services");
}, "Service");

const showAddDepModal = ref(false);
const addDepServiceId = ref("");
const addDepType = ref("depends_on");
const addingDep = ref(false);
const allServices = ref<ServiceRecord[]>([]);

const showRemoveDepDialog = ref(false);
const removeDepId = ref("");
const removingDep = ref(false);

function startEdit() {
  if (!service.value) return;
  editDisplayName.value = service.value.display_name;
  editDescription.value = service.value.description;
  editSlaResponse.value = String(service.value.sla_response_minutes ?? "");
  editSlaResolve.value = String(service.value.sla_resolve_minutes ?? "");
  editLabelMatchers.value = (service.value.label_matchers || []).map((m) => ({
    source: String(m.source ?? "labels"),
    field: String(m.field ?? ""),
    operator: String(m.operator ?? "exact"),
    value: String(m.value ?? ""),
  }));
  editing.value = true;
}

const headerRefs = useEntityPageHeader({
  source: service,
  buildTitle: (svc) => svc.display_name || svc.name,
  buildBadges: (svc) => [
    { text: svc.status, cssClass: `badge ${serviceStatusBadgeClass(svc.status)}` },
  ],
  buildActions: () => {
    if (!canEdit.value && !canRemove.value) return undefined;
    return [
      h(ServiceActionsMenu, {
        onEdit: () => startEdit(),
        onDelete: () => {
          if (service.value) confirmDeleteService(service.value);
        },
      }),
    ];
  },
  documentTitle: (svc) => (svc ? svc.display_name || svc.name : "Service"),
});
void headerRefs;

async function saveService() {
  if (!service.value) return;
  await withSubmit(async () => {
    const body: Parameters<typeof api.patchService>[1] = {};
    body.display_name = editDisplayName.value.trim();
    body.description = editDescription.value.trim() || undefined;
    const resp = editSlaResponse.value.trim();
    body.sla_response_minutes = resp ? Number(resp) : undefined;
    const resl = editSlaResolve.value.trim();
    body.sla_resolve_minutes = resl ? Number(resl) : undefined;
    body.label_matchers = editLabelMatchers.value.map((m) => ({
      source: m.source,
      field: m.field,
      operator: m.operator,
      value: m.value,
    }));

    const updated = await api.patchService(service.value!.id, body);
    service.value = updated;
    editing.value = false;
  }, "Service updated");
}

function cancelEdit() {
  editing.value = false;
}

async function loadDependencies() {
  try {
    dependencies.value = await api.getServiceDependencies(serviceId.value);
  } catch {
    // keep any data from getService response
  }
}

async function loadIncidents() {
  try {
    incidents.value = await api.getServiceIncidents(serviceId.value, { limit: 10 });
  } catch {
    incidents.value = [];
  }
}

function goToService(id: string) {
  router.push(`/services/${id}`);
}

function goToIncident(incidentNumber: number) {
  router.push(`/incidents/${incidentNumber}`);
}

async function addDependency() {
  if (!addDepServiceId.value.trim()) return;
  addingDep.value = true;
  try {
    await api.addServiceDependency(serviceId.value, {
      dependent_on_service_id: addDepServiceId.value.trim(),
      dependency_type: addDepType.value,
    });
    showAddDepModal.value = false;
    addDepServiceId.value = "";
    addDepType.value = "depends_on";
    push("Dependency added", "success");
    await loadDependencies();
    const updated = await api.getService(serviceId.value);
    service.value = updated;
  } catch (err) {
    push(getErrorMessage(err, "Failed to add dependency"), "error");
  } finally {
    addingDep.value = false;
  }
}

function confirmRemoveDependency(depId: string) {
  removeDepId.value = depId;
  showRemoveDepDialog.value = true;
}

async function removeDependency() {
  removingDep.value = true;
  try {
    await api.removeServiceDependency(serviceId.value, removeDepId.value);
    showRemoveDepDialog.value = false;
    push("Dependency removed", "success");
    await loadDependencies();
    const updated = await api.getService(serviceId.value);
    service.value = updated;
  } catch (err) {
    push(getErrorMessage(err, "Failed to remove dependency"), "error");
  } finally {
    removingDep.value = false;
  }
}

const deleteMessage = computed(
  () =>
    `Delete "${service.value?.display_name || service.value?.name}"? This action cannot be undone.`,
);

const availableServices = computed(() =>
  allServices.value.filter((s) => s.id !== service.value?.id),
);

function addLabelMatcher() {
  editLabelMatchers.value.push({ source: "labels", field: "", operator: "exact", value: "" });
}

function removeLabelMatcher(index: number) {
  editLabelMatchers.value.splice(index, 1);
}

async function openAddDepModal() {
  addDepServiceId.value = "";
  addDepType.value = "depends_on";
  try {
    const data = await api.getServices({ limit: 200 });
    allServices.value = data.items || [];
  } catch {
    allServices.value = [];
  }
  showAddDepModal.value = true;
}

onMounted(() => {
  loadService();
});
watch(serviceId, (id) => {
  if (id) loadService();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <LoadingSpinner v-if="loading && !service" centered />
    <ErrorBanner v-else-if="error && !service" :message="error" />
    <template v-else-if="service">
      <div class="grid gap-6 lg:grid-cols-[1fr_320px]">
        <div class="space-y-4">
          <Card>
            <div class="space-y-3">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0 flex-1">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="font-mono text-sm text-[var(--text-muted)]">{{
                      service.name
                    }}</span>
                  </div>
                  <h2 class="mt-2 text-lg font-semibold text-[var(--text-primary)]">
                    {{ service.display_name }}
                  </h2>
                </div>
                <div
                  v-if="service.active_incident_count"
                  class="flex items-center gap-1.5 rounded-md bg-[var(--color-error-subtle)] px-2.5 py-1 text-xs font-medium text-[var(--color-error)]"
                >
                  <AlertTriangle class="h-3 w-3" />
                  {{ service.active_incident_count }} active incident{{
                    service.active_incident_count !== 1 ? "s" : ""
                  }}
                </div>
              </div>
              <p
                v-if="service.description"
                class="text-sm text-[var(--text-secondary)] whitespace-pre-wrap"
              >
                {{ service.description }}
              </p>
              <div class="flex flex-wrap gap-x-4 gap-y-1 text-xs text-[var(--text-muted)]">
                <span class="flex items-center gap-1">
                  <Clock class="h-3 w-3" />
                  Created {{ formatTime(service.created_at) }}
                </span>
              </div>
            </div>
          </Card>

          <Card>
            <div class="flex items-center justify-between">
              <h3 class="text-sm font-medium text-[var(--text-primary)]">Dependencies</h3>
              <Button v-if="canEdit" variant="outline" size="sm" @click="openAddDepModal">
                <Plus class="h-3.5 w-3.5" />
                Add
              </Button>
            </div>
            <div v-if="dependencies.length === 0" class="mt-3 text-xs text-[var(--text-muted)]">
              No dependencies configured.
            </div>
            <div v-else class="mt-3 space-y-2">
              <div
                v-for="dep in dependencies"
                :key="dep.id"
                class="flex items-center gap-3 rounded-md border border-[var(--border-primary)] px-3 py-2 transition-colors hover:border-[var(--border-secondary)] hover:bg-[var(--bg-secondary)]"
              >
                <ArrowRight class="h-3 w-3 shrink-0 text-[var(--text-muted)]" />
                <div
                  class="min-w-0 flex-1 cursor-pointer"
                  @click="goToService(dep.dependent_on_service_id)"
                >
                  <span class="text-sm font-medium text-[var(--text-primary)]">{{
                    dep.dependent_on_service_name || dep.dependent_on_service_id
                  }}</span>
                </div>
                <span class="text-xs text-[var(--text-muted)]">{{ dep.dependency_type }}</span>
                <button
                  v-if="canEdit"
                  type="button"
                  class="cursor-pointer rounded p-1 text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-error)]"
                  title="Remove dependency"
                  @click.stop="confirmRemoveDependency(dep.id)"
                >
                  <Trash2 class="h-3 w-3" />
                </button>
              </div>
            </div>
          </Card>

          <Card>
            <h3 class="mb-3 text-sm font-medium text-[var(--text-primary)]">Depended On By</h3>
            <div v-if="dependents.length === 0" class="text-xs text-[var(--text-muted)]">
              No services depend on this service.
            </div>
            <div v-else class="space-y-2">
              <div
                v-for="dep in dependents"
                :key="dep.id"
                class="flex cursor-pointer items-center gap-3 rounded-md border border-[var(--border-primary)] px-3 py-2 transition-colors hover:border-[var(--border-secondary)] hover:bg-[var(--bg-secondary)]"
                @click="goToService(dep.service_id)"
              >
                <ArrowLeft class="h-3 w-3 shrink-0 text-[var(--text-muted)]" />
                <div class="min-w-0 flex-1">
                  <span class="text-sm font-medium text-[var(--text-primary)]">{{
                    dep.dependent_on_service_name || dep.service_id
                  }}</span>
                </div>
                <span class="text-xs text-[var(--text-muted)]">{{ dep.dependency_type }}</span>
              </div>
            </div>
          </Card>

          <Card>
            <h3 class="mb-3 text-sm font-medium text-[var(--text-primary)]">
              Active Incidents
              <span
                v-if="incidents.length > 0"
                class="ml-1 text-xs font-normal text-[var(--text-muted)]"
                >({{ incidents.length }})</span
              >
            </h3>
            <div v-if="incidents.length === 0" class="text-xs text-[var(--text-muted)]">
              No active incidents.
            </div>
            <div v-else class="space-y-2">
              <div
                v-for="inc in incidents"
                :key="inc.id"
                class="flex cursor-pointer items-center gap-3 rounded-md border border-[var(--border-primary)] px-3 py-2 transition-colors hover:border-[var(--border-secondary)] hover:bg-[var(--bg-secondary)]"
                @click="goToIncident(inc.incident_number)"
              >
                <SeverityBadge :severity="inc.severity" />
                <div class="min-w-0 flex-1">
                  <span class="text-sm text-[var(--text-primary)]">{{ inc.title }}</span>
                </div>
                <span class="shrink-0 text-xs text-[var(--text-muted)]">{{
                  formatTime(inc.created_at)
                }}</span>
              </div>
            </div>
          </Card>
        </div>

        <div class="space-y-4">
          <Card>
            <h3 class="mb-3 text-sm font-medium text-[var(--text-primary)]">SLA Targets</h3>
            <div class="space-y-2 text-sm">
              <div class="flex items-center justify-between">
                <span class="text-[var(--text-muted)]">Response time</span>
                <span class="font-medium text-[var(--text-primary)]"
                  >{{ service.sla_response_minutes }} minutes</span
                >
              </div>
              <div class="flex items-center justify-between">
                <span class="text-[var(--text-muted)]">Resolution time</span>
                <span class="font-medium text-[var(--text-primary)]"
                  >{{ service.sla_resolve_minutes }} minutes</span
                >
              </div>
            </div>
          </Card>

          <Card>
            <h3 class="mb-3 text-sm font-medium text-[var(--text-primary)]">Label Matchers</h3>
            <div v-if="!service.label_matchers?.length" class="text-xs text-[var(--text-muted)]">
              No label matchers configured.
            </div>
            <div v-else class="space-y-1">
              <div
                v-for="(matcher, i) in service.label_matchers"
                :key="i"
                class="rounded-md border border-[var(--border-primary)] px-3 py-2 text-xs"
              >
                <span class="font-medium text-[var(--text-primary)]">
                  {{ matcher.source || "labels" }}
                </span>
                <span class="text-[var(--text-muted)]">.</span>
                <span class="font-medium text-[var(--text-primary)]">
                  {{ matcher.field || "*" }}
                </span>
                <span class="mx-1 text-[var(--text-muted)]">→</span>
                <span
                  class="rounded bg-[var(--bg-secondary)] px-1.5 py-0.5 font-medium text-[var(--text-primary)]"
                >
                  {{ matcher.operator || "exact" }}
                </span>
                <span v-if="matcher.value" class="ml-1 text-[var(--text-secondary)]">
                  "{{ matcher.value }}"
                </span>
              </div>
            </div>
          </Card>
        </div>
      </div>
    </template>

    <Modal
      :open="editing && !!service"
      title="Edit service"
      max-width="xl"
      :prevent-close="saving"
      @update:open="!$event && cancelEdit()"
      @close="cancelEdit"
    >
      <form v-if="service" class="space-y-4" @submit.prevent="saveService">
        <ErrorBanner v-if="formError" :message="formError" />
        <div>
          <FormLabel for="edit-service-name">Name</FormLabel>
          <Input id="edit-service-name" :model-value="service.name" disabled autocomplete="off" />
          <p class="mt-1 text-xs text-[var(--text-muted)]">Name cannot be changed.</p>
        </div>

        <div>
          <FormLabel for="edit-service-display-name">Display name</FormLabel>
          <Input
            id="edit-service-display-name"
            v-model="editDisplayName"
            autocomplete="off"
            placeholder="e.g. Payment API"
            :disabled="saving"
          />
        </div>

        <div>
          <FormLabel for="edit-service-desc">Description</FormLabel>
          <Textarea
            id="edit-service-desc"
            v-model="editDescription"
            rows="3"
            placeholder="Optional description of the service"
            class="min-h-[4.5rem] w-full resize-y"
            :disabled="saving"
          />
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <FormLabel for="edit-service-sla-response">SLA response (min)</FormLabel>
            <NumberInput
              id="edit-service-sla-response"
              v-model="editSlaResponse"
              placeholder="e.g. 15"
              :disabled="saving"
            />
          </div>
          <div>
            <FormLabel for="edit-service-sla-resolve">SLA resolve (min)</FormLabel>
            <NumberInput
              id="edit-service-sla-resolve"
              v-model="editSlaResolve"
              placeholder="e.g. 60"
              :disabled="saving"
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
              :disabled="saving"
              @click="addLabelMatcher"
            >
              <Plus class="h-3.5 w-3.5" />
              Add
            </Button>
          </div>
          <div v-if="editLabelMatchers.length === 0" class="mt-2 text-xs text-[var(--text-muted)]">
            No label matchers configured.
          </div>
          <div v-else class="mt-2 space-y-2">
            <div
              v-for="(matcher, i) in editLabelMatchers"
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
        <Button variant="outline" :disabled="saving" @click="cancelEdit">Cancel</Button>
        <Button variant="primary" :loading="saving" @click="saveService">Save</Button>
      </template>
    </Modal>

    <ConfirmDialog
      :open="showDeleteConfirm"
      title="Delete service"
      :message="deleteMessage"
      confirm-label="Delete"
      :destructive="true"
      :loading="deleting"
      @update:open="showDeleteConfirm = $event"
      @confirm="doDeleteService"
    />

    <Modal
      :open="showAddDepModal"
      title="Add Dependency"
      :loading="addingDep"
      confirm-label="Add"
      @update:open="showAddDepModal = $event"
      @confirm="addDependency"
    >
      <div class="space-y-4">
        <div>
          <FormLabel for="add-dep-service-id" required>Service</FormLabel>
          <Select
            id="add-dep-service-id"
            :model-value="addDepServiceId"
            @update:model-value="addDepServiceId = $event"
          >
            <option value="">Select a service</option>
            <option v-for="svc in availableServices" :key="svc.id" :value="svc.id">
              {{ svc.display_name || svc.name }}
            </option>
          </Select>
        </div>
        <div>
          <FormLabel for="add-dep-type">Dependency Type</FormLabel>
          <Select
            id="add-dep-type"
            :model-value="addDepType"
            @update:model-value="addDepType = $event"
          >
            <option value="depends_on">Depends On</option>
          </Select>
        </div>
      </div>
    </Modal>

    <ConfirmDialog
      :open="showRemoveDepDialog"
      title="Remove Dependency"
      message="Are you sure you want to remove this dependency?"
      confirm-label="Remove"
      :destructive="true"
      :loading="removingDep"
      @update:open="showRemoveDepDialog = $event"
      @confirm="removeDependency"
    />
  </section>
</template>
