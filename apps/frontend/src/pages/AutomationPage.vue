<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Workflow } from "@lucide/vue";
import {
  api,
  type AutomationRuleInput,
  type AutomationRuleRecord,
  type AutomationRunRecord,
} from "@/lib/api";
import { formatTime } from "@/lib/time";
import { getErrorMessage } from "@/lib/error";
import { useToast } from "@/lib/toast";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { useListPage } from "@/composables/useListPage";
import { useDelete } from "@/composables/useDelete";
import AutomationRuleFormModal from "@/components/automation/AutomationRuleFormModal.vue";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import Modal from "@/components/ui/Modal.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";

defineOptions({ name: "AutomationPage" });

const { push } = useToast();
const { canWrite, canDelete } = useEntityPermissions("automation");
const activeTab = ref<"rules" | "runs">("rules");
const rulesSkip = ref(0);
const runsSkip = ref(0);
const pageSize = 20;
const formOpen = ref(false);
const editingRule = ref<AutomationRuleRecord | null>(null);
const runOpen = ref(false);
const selectedRun = ref<AutomationRunRecord | null>(null);
const runLoading = ref(false);
const actionBusy = ref(false);

const {
  items: rules,
  total: rulesTotal,
  loading: rulesLoading,
  error: rulesError,
  reload: loadRules,
} = useListPage<AutomationRuleRecord>({
  fetch: () => api.listAutomationRules({ limit: pageSize, skip: rulesSkip.value }),
  entityName: "automation rules",
});
const {
  items: runs,
  total: runsTotal,
  loading: runsLoading,
  error: runsError,
  reload: loadRuns,
} = useListPage<AutomationRunRecord>({
  fetch: () => api.listAutomationRuns({ limit: pageSize, skip: runsSkip.value }),
  entityName: "automation runs",
});

const showAdd = computed(() => canWrite.value && activeTab.value === "rules");
usePageHeaderActions({
  title: "Automation",
  titleIcon: Workflow,
  showAdd,
  addLabel: "Create automation rule",
  onAdd: () => {
    editingRule.value = null;
    formOpen.value = true;
  },
});

const { deleteTarget, showDeleteConfirm, deleting, confirmDelete, doDelete } =
  useDelete<AutomationRuleRecord>(async (rule) => {
    await api.archiveAutomationRule(rule.id);
    await loadRules();
  }, "Automation rule");

function editableInput(rule: AutomationRuleRecord): AutomationRuleInput {
  return {
    name: rule.name,
    description: rule.description,
    trigger_type: rule.trigger_type,
    conditions: rule.conditions.map((c) => ({ ...c })),
    actions: rule.actions.map((a) => ({ ...a, bearer_token: undefined })),
    enabled: rule.enabled,
  };
}

async function toggleRule(rule: AutomationRuleRecord) {
  actionBusy.value = true;
  try {
    await api.updateAutomationRule(rule.id, {
      ...editableInput(rule),
      enabled: !rule.enabled,
      version: rule.version,
    });
    await loadRules();
    push(rule.enabled ? "Automation rule disabled" : "Automation rule enabled", "success");
  } catch (err) {
    push(getErrorMessage(err, "Failed to update automation rule"), "error");
  } finally {
    actionBusy.value = false;
  }
}

async function viewRun(id: string) {
  runOpen.value = true;
  runLoading.value = true;
  selectedRun.value = null;
  try {
    selectedRun.value = await api.getAutomationRun(id);
  } catch (err) {
    push(getErrorMessage(err, "Failed to load automation run"), "error");
    runOpen.value = false;
  } finally {
    runLoading.value = false;
  }
}

async function retryRun() {
  if (!selectedRun.value) return;
  actionBusy.value = true;
  try {
    await api.retryAutomationRun(selectedRun.value.id);
    selectedRun.value = await api.getAutomationRun(selectedRun.value.id);
    await loadRuns();
    push("Automation run queued for retry", "success");
  } catch (err) {
    push(getErrorMessage(err, "Failed to retry automation run"), "error");
  } finally {
    actionBusy.value = false;
  }
}

function pageRules(delta: number) {
  rulesSkip.value = Math.max(0, rulesSkip.value + delta * pageSize);
  loadRules();
}
function pageRuns(delta: number) {
  runsSkip.value = Math.max(0, runsSkip.value + delta * pageSize);
  loadRuns();
}

onMounted(() => {
  loadRules();
  loadRuns();
});
</script>

<template>
  <section class="space-y-5 px-4 py-4 md:px-6 md:py-6">
    <div class="flex gap-2" role="tablist" aria-label="Automation views">
      <Button
        :variant="activeTab === 'rules' ? 'default' : 'outline'"
        role="tab"
        :aria-selected="activeTab === 'rules'"
        @click="activeTab = 'rules'"
        >Rules</Button
      >
      <Button
        :variant="activeTab === 'runs' ? 'default' : 'outline'"
        role="tab"
        :aria-selected="activeTab === 'runs'"
        @click="activeTab = 'runs'"
        >Runs</Button
      >
    </div>

    <template v-if="activeTab === 'rules'">
      <ErrorBanner :message="rulesError" />
      <LoadingSpinner v-if="rulesLoading" centered label="Loading automation rules..." />
      <EmptyState
        v-else-if="rules.length === 0"
        message="No automation rules yet. Create a rule to respond to alert or incident events."
      />
      <div v-else class="space-y-3">
        <Card v-for="rule in rules" :key="rule.id" class="p-4">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <h2 class="font-medium">{{ rule.name }}</h2>
                <span
                  class="rounded-full px-2 py-0.5 text-xs"
                  :class="
                    rule.enabled
                      ? 'bg-green-500/10 text-green-700 dark:text-green-300'
                      : 'bg-[var(--hover-neutral)] text-[var(--text-muted)]'
                  "
                  >{{ rule.enabled ? "Enabled" : "Disabled" }}</span
                >
              </div>
              <p class="mt-1 text-sm text-[var(--text-secondary)]">
                {{ rule.description || rule.trigger_type }}
              </p>
              <p class="mt-1 text-xs text-[var(--text-muted)]">
                When {{ rule.trigger_type }} · {{ rule.conditions.length }} conditions ·
                {{ rule.actions.length }} actions · Version {{ rule.version }}
              </p>
            </div>
            <div class="flex flex-wrap gap-2">
              <Button
                v-if="canWrite"
                size="sm"
                variant="outline"
                :disabled="actionBusy"
                @click="toggleRule(rule)"
                >{{ rule.enabled ? "Disable" : "Enable" }}</Button
              >
              <Button
                v-if="canWrite"
                size="sm"
                variant="outline"
                @click="
                  editingRule = rule;
                  formOpen = true;
                "
                >Edit</Button
              >
              <Button v-if="canDelete" size="sm" variant="destructive" @click="confirmDelete(rule)"
                >Archive</Button
              >
            </div>
          </div>
        </Card>
      </div>
      <div v-if="rulesTotal > pageSize" class="flex items-center justify-between text-sm">
        <span
          >{{ rulesSkip + 1 }}–{{ Math.min(rulesSkip + rules.length, rulesTotal) }} of
          {{ rulesTotal }}</span
        >
        <div class="flex gap-2">
          <Button size="sm" variant="outline" :disabled="rulesSkip === 0" @click="pageRules(-1)"
            >Previous</Button
          ><Button
            size="sm"
            variant="outline"
            :disabled="rulesSkip + pageSize >= rulesTotal"
            @click="pageRules(1)"
            >Next</Button
          >
        </div>
      </div>
    </template>

    <template v-else>
      <ErrorBanner :message="runsError" />
      <LoadingSpinner v-if="runsLoading" centered label="Loading automation runs..." />
      <EmptyState v-else-if="runs.length === 0" message="No automation runs yet." />
      <div v-else class="space-y-3">
        <Card v-for="run in runs" :key="run.id" class="p-4">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h2 class="font-medium">{{ run.rule_name }}</h2>
              <p class="text-sm text-[var(--text-secondary)]">
                {{ run.event_payload.event_type }} #{{ run.event_payload.entity_number }} ·
                {{ formatTime(run.created_at) }}
              </p>
              <p v-if="run.last_error" class="text-xs text-[var(--text-error)]">
                {{ run.last_error }}
              </p>
            </div>
            <div class="flex items-center gap-3">
              <span class="text-sm font-medium capitalize">{{ run.status }}</span
              ><Button size="sm" variant="outline" @click="viewRun(run.id)">Details</Button>
            </div>
          </div>
        </Card>
      </div>
      <div v-if="runsTotal > pageSize" class="flex items-center justify-between text-sm">
        <span
          >{{ runsSkip + 1 }}–{{ Math.min(runsSkip + runs.length, runsTotal) }} of
          {{ runsTotal }}</span
        >
        <div class="flex gap-2">
          <Button size="sm" variant="outline" :disabled="runsSkip === 0" @click="pageRuns(-1)"
            >Previous</Button
          ><Button
            size="sm"
            variant="outline"
            :disabled="runsSkip + pageSize >= runsTotal"
            @click="pageRuns(1)"
            >Next</Button
          >
        </div>
      </div>
    </template>

    <AutomationRuleFormModal
      :open="formOpen"
      :rule="editingRule"
      @close="formOpen = false"
      @saved="
        formOpen = false;
        loadRules();
      "
    />
    <ConfirmDialog
      :open="showDeleteConfirm"
      title="Archive automation rule"
      :message="`Archive ${deleteTarget?.name ?? 'this rule'}? Its existing runs remain visible.`"
      confirm-label="Archive"
      destructive
      :loading="deleting"
      @confirm="doDelete"
      @cancel="showDeleteConfirm = false"
      @update:open="showDeleteConfirm = $event"
    />
    <Modal
      :open="runOpen"
      title="Automation run"
      max-width="xl"
      :show-footer="false"
      @update:open="runOpen = $event"
    >
      <LoadingSpinner v-if="runLoading" centered label="Loading run..." />
      <div v-else-if="selectedRun" class="space-y-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="font-medium">{{ selectedRun.rule_name }}</p>
            <p class="text-sm text-[var(--text-muted)]">
              Version {{ selectedRun.rule_version }} ·
              {{ selectedRun.event_payload.event_type }} #{{
                selectedRun.event_payload.entity_number
              }}
            </p>
          </div>
          <span class="text-sm font-medium capitalize">{{ selectedRun.status }}</span>
        </div>
        <p v-if="selectedRun.last_error" class="text-sm text-[var(--text-error)]">
          Last error: {{ selectedRun.last_error }}
        </p>
        <div class="space-y-2">
          <h3 class="font-medium">Action attempts</h3>
          <p v-if="!selectedRun.steps?.length" class="text-sm text-[var(--text-muted)]">
            No actions attempted yet.
          </p>
          <div
            v-for="step in selectedRun.steps"
            :key="step.id"
            class="flex flex-wrap justify-between gap-2 rounded-lg border border-[var(--border-primary)] p-3 text-sm"
          >
            <span>Action {{ step.action_index + 1 }} · Attempt {{ step.attempt }}</span
            ><span class="capitalize"
              >{{ step.status }}{{ step.http_status ? ` · HTTP ${step.http_status}` : ""
              }}{{ step.error_code ? ` · ${step.error_code}` : "" }}</span
            >
          </div>
        </div>
        <Button
          v-if="canWrite && selectedRun.status === 'failed'"
          :disabled="actionBusy"
          @click="retryRun"
          >Retry failed action</Button
        >
      </div>
    </Modal>
  </section>
</template>
