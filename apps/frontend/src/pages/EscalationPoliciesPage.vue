<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { onMounted, ref } from "vue";
import { Shield, Trash2, ChevronDown, ChevronRight, Edit3 } from "@lucide/vue";
import { api, type EscalationPolicyRecord, type TeamRecord, type UserInfo } from "@/lib/api";
import { resolveDisplayName } from "@/lib/userDisplay";
import { useToast } from "@/lib/toast";
import Button from "@/components/ui/Button.vue";
import Modal from "@/components/ui/Modal.vue";
import Card from "@/components/ui/Card.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import EscalationPolicyForm, {
  type EscalationPolicyFormState,
} from "@/components/escalation/EscalationPolicyForm.vue";
import { useDelete } from "@/composables/useDelete";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { useListFilter } from "@/composables/useListFilter";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { useUsers } from "@/composables/useUsers";

defineOptions({ name: "EscalationPoliciesPage" });

const { push } = useToast();

const policies = ref<EscalationPolicyRecord[]>([]);
const loading = ref(false);
const error = ref("");
const expandedIds = ref<Set<string>>(new Set());

function blankFormState(): EscalationPolicyFormState {
  return {
    name: "",
    description: "",
    repeat: 0,
    levels: [
      {
        delay_minutes: 5,
        notify_channels: [],
        targets: [{ target_type: "user" }],
      },
    ],
  };
}

function toApiPayload(state: EscalationPolicyFormState) {
  return {
    name: state.name.trim(),
    description: state.description.trim() || undefined,
    repeat_count: state.repeat,
    levels: state.levels.map((l) => ({
      delay_minutes: l.delay_minutes,
      notify_channels: l.notify_channels,
      targets: l.targets,
    })),
  };
}

const showCreateModal = ref(false);
const { submitting: creating, withSubmit: withCreate } = useFormSubmit();
const createForm = ref<EscalationPolicyFormState>(blankFormState());

const showEditModal = ref(false);
const { submitting: editing, withSubmit: withEdit } = useFormSubmit();
const editId = ref("");
const editForm = ref<EscalationPolicyFormState>(blankFormState());

const {
  showDeleteConfirm: showPolicyDeleteConfirm,
  confirmDelete: confirmDeletePolicy,
  doDelete: doDeletePolicy,
} = useDelete<string>(async (policyId) => {
  await api.deleteEscalationPolicy(policyId);
  await load();
}, "Policy");

const allUsers = ref<UserInfo[]>([]);
const allTeams = ref<TeamRecord[]>([]);

const { canWrite } = useEntityPermissions("escalation");
const oncallPerms = useEntityPermissions("oncall");
const { users: permittedUsers, loadUsers: loadPermittedUsers } = useUsers();

function toggleExpand(id: string) {
  if (expandedIds.value.has(id)) {
    expandedIds.value.delete(id);
  } else {
    expandedIds.value.add(id);
  }
}

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [pData, tData] = await Promise.all([
      api.getEscalationPolicies(),
      oncallPerms.canRead.value ? api.getTeams() : Promise.resolve({ items: [] as TeamRecord[] }),
    ]);
    policies.value = pData.items || [];
    allTeams.value = "items" in tData ? tData.items : [];
    await loadPermittedUsers();
    allUsers.value = permittedUsers.value;
  } catch (err) {
    const msg = getErrorMessage(err, "Failed to load policies");
    error.value = msg;
    push(msg, "error");
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  createForm.value = blankFormState();
  showCreateModal.value = true;
}

async function handleCreate() {
  if (!createForm.value.name.trim()) return;
  await withCreate(async () => {
    await api.createEscalationPolicy(toApiPayload(createForm.value));
    showCreateModal.value = false;
    await load();
  }, "Policy created");
}

function openEdit(policy: EscalationPolicyRecord) {
  editId.value = policy.id;
  editForm.value = {
    name: policy.name,
    description: policy.description,
    repeat: policy.repeat_count,
    levels: (policy.levels ?? []).map((l) => ({
      delay_minutes: l.delay_minutes,
      notify_channels: l.notify_channels ?? [],
      targets: (l.targets ?? []).map((t) => ({
        target_type: t.target_type,
        target_user_id: t.target_user_id,
        target_team_id: t.target_team_id,
      })),
    })),
  };
  showEditModal.value = true;
}

async function handleEdit() {
  if (!editForm.value.name.trim()) return;
  await withEdit(async () => {
    await api.updateEscalationPolicy(editId.value, toApiPayload(editForm.value));
    showEditModal.value = false;
    await load();
  }, "Policy updated");
}

function userDisplayName(userId?: string): string {
  return resolveDisplayName({ userId, users: allUsers.value, fallback: userId ?? "unknown" });
}

function teamName(teamId?: string): string {
  if (!teamId) return "unknown";
  const t = allTeams.value.find((t) => t.id === teamId);
  return t ? t.name : teamId;
}
const searchInput = ref("");
const filteredPolicies = useListFilter(policies, ["name", "description"], searchInput);
usePageHeaderActions({
  title: "Escalation Policies",
  titleIcon: Shield,
  searchInput,
  searchPlaceholder: "Search policies...",
  showFilters: false,
  showAdd: canWrite,
  onAdd: openCreate,
  addLabel: "New Policy",
});

onMounted(() => {
  load();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <ErrorBanner :message="error" />
    <LoadingSpinner v-if="loading && policies.length === 0" centered />
    <EmptyState
      v-else-if="filteredPolicies.length === 0"
      :message="
        searchInput ? 'No escalation policies match your search.' : 'No escalation policies found.'
      "
    >
      <template #icon>
        <Shield class="mb-2 h-6 w-6 opacity-40" />
      </template>
    </EmptyState>
    <div v-else class="space-y-3">
      <Card v-for="policy in filteredPolicies" :key="policy.id">
        <div
          class="flex cursor-pointer items-center justify-between"
          @click="toggleExpand(policy.id)"
        >
          <div class="flex items-center gap-2">
            <component
              :is="expandedIds.has(policy.id) ? ChevronDown : ChevronRight"
              class="h-4 w-4 text-[var(--text-muted)]"
            />
            <div>
              <h3 class="text-sm font-medium text-[var(--text-primary)]">{{ policy.name }}</h3>
              <p v-if="policy.description" class="text-xs text-[var(--text-muted)]">
                {{ policy.description }}
              </p>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <span class="text-xs text-[var(--text-muted)]">
              {{ policy.levels?.length ?? 0 }} level{{
                (policy.levels?.length ?? 0) !== 1 ? "s" : ""
              }}
              <template v-if="policy.repeat_count > 0">
                · repeats {{ policy.repeat_count }}x</template
              >
            </span>
            <Button v-if="canWrite" variant="outline" size="sm" @click.stop="openEdit(policy)">
              <Edit3 class="h-3.5 w-3.5" />
            </Button>
            <Button
              v-if="canWrite"
              variant="outline"
              size="sm"
              @click.stop="confirmDeletePolicy(policy.id)"
            >
              <Trash2 class="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>

        <div
          v-if="expandedIds.has(policy.id) && policy.levels?.length"
          class="mt-3 space-y-2 border-t border-[var(--border-primary)] pt-3"
        >
          <div
            v-for="level in policy.levels"
            :key="level.level_number"
            class="rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)]/40 p-3"
          >
            <div class="flex items-center justify-between">
              <span class="text-xs font-medium text-[var(--text-primary)]">
                Level {{ level.level_number }}
              </span>
              <span class="text-xs text-[var(--text-muted)]">
                {{ level.delay_minutes }}min delay
              </span>
            </div>
            <div v-if="level.targets?.length" class="mt-2 space-y-1">
              <div
                v-for="(target, targetIdx) in level.targets"
                :key="`${level.level_number}-${targetIdx}`"
                class="text-xs text-[var(--text-muted)]"
              >
                <template v-if="target.target_type === 'user'">
                  User: {{ userDisplayName(target.target_user_id) }}
                </template>
                <template v-else-if="target.target_type === 'team'">
                  Team: {{ teamName(target.target_team_id) }}
                </template>
              </div>
            </div>
            <div v-if="level.notify_channels?.length" class="mt-1 flex flex-wrap gap-1">
              <span
                v-for="ch in level.notify_channels"
                :key="ch"
                class="rounded-full border border-[var(--text-badge-info)] px-2 py-0.5 text-xs text-[var(--text-badge-info)]"
              >
                {{ ch }}
              </span>
            </div>
          </div>
        </div>
      </Card>
    </div>

    <Modal
      :open="showCreateModal"
      title="New Escalation Policy"
      :loading="creating"
      confirm-label="Create"
      @update:open="showCreateModal = $event"
      @confirm="handleCreate"
    >
      <EscalationPolicyForm
        v-model="createForm"
        :disabled="creating"
        :users="allUsers"
        :teams="allTeams"
        id-prefix="pol"
      />
    </Modal>

    <Modal
      :open="showEditModal"
      title="Edit Escalation Policy"
      :loading="editing"
      confirm-label="Save"
      @update:open="showEditModal = $event"
      @confirm="handleEdit"
    >
      <EscalationPolicyForm
        v-model="editForm"
        :disabled="editing"
        :users="allUsers"
        :teams="allTeams"
        id-prefix="edit-pol"
      />
    </Modal>

    <ConfirmDialog
      :open="showPolicyDeleteConfirm"
      title="Delete Escalation Policy"
      message="Are you sure you want to delete this policy?"
      confirm-label="Delete"
      :destructive="true"
      @update:open="showPolicyDeleteConfirm = $event"
      @confirm="doDeletePolicy"
    />
  </section>
</template>
