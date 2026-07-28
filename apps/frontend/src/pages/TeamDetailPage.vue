<script setup lang="ts">
import { onMounted, ref, computed, h, nextTick, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { Plus, X, Save, Trash2, ChevronDown, Search } from "@lucide/vue";
import { api, type TeamMemberRecord, type UserInfo, type EscalationPolicyRecord } from "@/lib/api";
import { usePageHeader } from "@/composables/usePageHeader";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import UserLabel from "@/components/ui/UserLabel.vue";
import Select from "@/components/ui/Select.vue";
import Modal from "@/components/ui/Modal.vue";
import Card from "@/components/ui/Card.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Avatar from "@/components/ui/Avatar.vue";
import HeaderActionsMenu from "@/components/ui/HeaderActionsMenu.vue";
import { useAsyncData } from "@/composables/useAsyncData";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { useListFilter } from "@/composables/useListFilter";
import { useDropdownLifecycle } from "@/composables/useDropdownLifecycle";
import { useEscapeKey } from "@/composables/useEscapeKey";
import { useUsersIfPermitted } from "@/composables/useUsers";

defineOptions({ name: "TeamDetailPage" });

const route = useRoute();
const router = useRouter();
const escalationPerms = useEntityPermissions("escalation");

const teamId = computed(() => route.params.id as string);
const members = ref<TeamMemberRecord[]>([]);
const allUsers = ref<UserInfo[]>([]);
const policies = ref<EscalationPolicyRecord[]>([]);
const { users: permittedUsers, loadUsers: loadPermittedUsers } =
  useUsersIfPermitted("users:manage");
const {
  data: team,
  loading,
  error,
  reload: loadTeam,
} = useAsyncData(async () => {
  const [t, m, p] = await Promise.all([
    api.getTeam(teamId.value),
    api.getTeamMembers(teamId.value),
    escalationPerms.canRead.value
      ? api.getEscalationPolicies()
      : Promise.resolve({ items: [] as EscalationPolicyRecord[] }),
  ]);
  await loadPermittedUsers();
  members.value = m;
  allUsers.value = permittedUsers.value;
  policies.value = "items" in p ? p.items : [];
  editName.value = t.name;
  editDescription.value = t.description;
  editPolicyId.value = t.escalation_policy_id ?? "";
  return t;
}, "Failed to load team");

const showEditModal = ref(false);
const editName = ref("");
const editDescription = ref("");
const editPolicyId = ref("");
const { submitting: saving, withSubmit: withSave } = useFormSubmit();

const showAddMember = ref(false);
const selectedUserId = ref("");
const selectedRole = ref<"lead" | "member">("member");
const { submitting: addingMember, withSubmit: withAddMember } = useFormSubmit();

const userDropdownOpen = ref(false);
const userDropdownQuery = ref("");
const userDropdownRoot = ref<HTMLElement | null>(null);
const userDropdownInput = ref<HTMLInputElement | null>(null);
const userDropdownList = ref<HTMLElement | null>(null);
const userDropdownActiveIndex = ref(-1);

useDropdownLifecycle(userDropdownOpen, userDropdownRoot);
useEscapeKey(
  () => {
    userDropdownOpen.value = false;
  },
  () => userDropdownOpen.value,
);

const showRemoveMember = ref(false);
const removingMember = ref<TeamMemberRecord | null>(null);
const { submitting: removing, withSubmit: withRemoveMember } = useFormSubmit();

const showDeleteTeam = ref(false);
const { submitting: deletingTeam, withSubmit: withDeleteTeam } = useFormSubmit();
const { withSubmit: withRoleUpdate } = useFormSubmit();

function handleEdit() {
  startEdit();
}

function handleDelete() {
  showDeleteTeam.value = true;
}

const { canWrite: canEdit } = useEntityPermissions("oncall");

const headerActions = computed(() => {
  if (!canEdit.value) return [];
  return [
    h(HeaderActionsMenu, {
      items: [
        { label: "Edit", icon: Save, onSelect: handleEdit },
        { label: "Delete", icon: Trash2, destructive: true, onSelect: handleDelete },
      ],
    }),
  ];
});

usePageHeader(() => {
  const t = team.value;
  if (!t) return null;
  return { title: t.name, options: { actions: headerActions.value } };
});

function startEdit() {
  if (!team.value) return;
  editName.value = team.value.name;
  editDescription.value = team.value.description;
  editPolicyId.value = team.value.escalation_policy_id ?? "";
  showEditModal.value = true;
}

async function saveTeam() {
  if (!editName.value.trim()) return;
  await withSave(async () => {
    const updated = await api.updateTeam(teamId.value, {
      name: editName.value.trim(),
      description: editDescription.value.trim(),
      escalation_policy_id: editPolicyId.value,
    });
    team.value = updated;
    showEditModal.value = false;
  }, "Team updated");
}

async function deleteTeam() {
  await withDeleteTeam(async () => {
    await api.deleteTeam(teamId.value);
    router.push("/teams");
  }, "Team deleted");
  showDeleteTeam.value = false;
}

async function addMember() {
  if (!selectedUserId.value) return;
  await withAddMember(async () => {
    const m = await api.addTeamMember(teamId.value, {
      user_id: selectedUserId.value,
      role: selectedRole.value,
    });
    members.value = [...members.value, m];
    resetAddMemberState();
    selectedUserId.value = "";
    selectedRole.value = "member";
  }, "Member added");
}

function confirmRemoveMember(m: TeamMemberRecord) {
  removingMember.value = m;
  showRemoveMember.value = true;
}

async function removeMember() {
  if (!removingMember.value) return;
  const target = removingMember.value;
  await withRemoveMember(async () => {
    await api.removeTeamMember(teamId.value, target.user_id);
    members.value = members.value.filter((m) => m.id !== target.id);
    showRemoveMember.value = false;
    removingMember.value = null;
  }, "Member removed");
}

async function updateMemberRole(m: TeamMemberRecord, newRole: string) {
  await withRoleUpdate(async () => {
    await api.updateTeamMemberRole(teamId.value, m.user_id, newRole);
    members.value = members.value.map((mem) =>
      mem.id === m.id ? { ...mem, role: newRole as "lead" | "member" } : mem,
    );
  }, "Member role updated");
}

function memberDisplayName(m: TeamMemberRecord): string {
  return m.user_name || m.user_email || m.user_id;
}

function memberInitials(m: TeamMemberRecord): string {
  const name = memberDisplayName(m);
  return name
    .split(/[\s.@]/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0].toUpperCase())
    .join("");
}

const AVATAR_COLORS = [
  "bg-[var(--accent)]",
  "bg-[var(--status-warning)]",
  "bg-[var(--status-error)]",
  "bg-[var(--status-success)]",
  "bg-purple-500",
  "bg-cyan-500",
];

function memberAvatarColor(m: TeamMemberRecord): string {
  let hash = 0;
  const s = m.user_id;
  for (let i = 0; i < s.length; i++) {
    hash = s.charCodeAt(i) + ((hash << 5) - hash);
  }
  return AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length];
}

const availableUsers = computed(() => {
  const memberIds = new Set(members.value.map((m) => m.user_id));
  return allUsers.value.filter((u) => !memberIds.has(u.id));
});

const filteredAvailableUsers = useListFilter(
  availableUsers,
  ["full_name", "email"],
  userDropdownQuery,
);

const selectedUser = computed(
  () => availableUsers.value.find((u) => u.id === selectedUserId.value) ?? null,
);

watch(userDropdownOpen, (open) => {
  if (open) {
    userDropdownQuery.value = "";
    userDropdownActiveIndex.value = -1;
    void nextTick(() => userDropdownInput.value?.focus());
  }
});

function toggleUserDropdown() {
  userDropdownOpen.value = !userDropdownOpen.value;
}

function pickUser(u: UserInfo) {
  selectedUserId.value = u.id;
  userDropdownOpen.value = false;
}

function clearSelectedUser() {
  selectedUserId.value = "";
}

function onUserDropdownKeydown(e: KeyboardEvent) {
  if (!userDropdownOpen.value) {
    if (e.key === "Enter" || e.key === " " || e.key === "ArrowDown") {
      e.preventDefault();
      userDropdownOpen.value = true;
    }
    return;
  }

  const items = filteredAvailableUsers.value;
  if (e.key === "ArrowDown") {
    e.preventDefault();
    userDropdownActiveIndex.value =
      items.length === 0 ? -1 : (userDropdownActiveIndex.value + 1) % items.length;
    scrollToActiveUser();
    return;
  }
  if (e.key === "ArrowUp") {
    e.preventDefault();
    userDropdownActiveIndex.value =
      items.length === 0 ? -1 : (userDropdownActiveIndex.value - 1 + items.length) % items.length;
    scrollToActiveUser();
    return;
  }
  if (
    e.key === "Enter" &&
    userDropdownActiveIndex.value >= 0 &&
    userDropdownActiveIndex.value < items.length
  ) {
    e.preventDefault();
    pickUser(items[userDropdownActiveIndex.value]);
    return;
  }
}

function scrollToActiveUser() {
  void nextTick(() => {
    const list = userDropdownList.value;
    if (!list) return;
    list.querySelector("[data-active='true']")?.scrollIntoView({ block: "nearest" });
  });
}

function resetAddMemberState() {
  showAddMember.value = false;
  userDropdownOpen.value = false;
  userDropdownQuery.value = "";
}

onMounted(loadTeam);
watch(teamId, loadTeam);
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <ErrorBanner :message="error" />
    <LoadingSpinner v-if="loading" centered />

    <template v-if="team && !loading">
      <Card v-if="team.description || team.escalation_policy_id" class="space-y-3">
        <p v-if="team.description" class="text-sm text-[var(--text-muted)]">
          {{ team.description }}
        </p>
        <p v-if="team.escalation_policy_id" class="text-xs text-[var(--text-muted)]">
          Escalation policy linked
        </p>
      </Card>

      <div class="flex items-center justify-between">
        <h2 class="text-base font-medium text-[var(--text-primary)]">
          Members ({{ members.length }})
        </h2>
        <Button v-if="canEdit && availableUsers.length > 0" size="sm" @click="showAddMember = true">
          <Plus class="h-3.5 w-3.5" />
          Add Member
        </Button>
      </div>

      <div v-if="members.length === 0" class="text-sm text-[var(--text-muted)]">
        No members in this team.
      </div>
      <div v-else class="space-y-2">
        <div
          v-for="m in members"
          :key="m.id"
          class="flex items-center gap-3 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-card)] px-4 py-3"
        >
          <Avatar :letter="memberInitials(m)" :bg-class="memberAvatarColor(m)" />
          <span class="min-w-0 flex-1 truncate text-sm text-[var(--text-primary)]">{{
            memberDisplayName(m)
          }}</span>
          <Select
            v-if="canEdit"
            :model-value="m.role"
            class="w-[100px] shrink-0 text-xs"
            @update:model-value="updateMemberRole(m, $event)"
          >
            <option value="member">Member</option>
            <option value="lead">Lead</option>
          </Select>
          <span
            v-else
            class="shrink-0 text-xs font-medium"
            :class="
              m.role === 'lead' ? 'text-[var(--text-badge-info)]' : 'text-[var(--text-muted)]'
            "
          >
            {{ m.role }}
          </span>
          <button
            v-if="canEdit"
            type="button"
            class="flex h-7 w-7 shrink-0 cursor-pointer items-center justify-center rounded-md text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-error)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
            @click="confirmRemoveMember(m)"
          >
            <X class="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
    </template>

    <Modal
      :open="showEditModal"
      title="Edit Team"
      :loading="saving"
      confirm-label="Save"
      @update:open="showEditModal = $event"
      @confirm="saveTeam"
    >
      <div class="space-y-4">
        <div>
          <FormLabel for="edit-team-name">Name</FormLabel>
          <Input id="edit-team-name" v-model="editName" />
        </div>
        <div>
          <FormLabel for="edit-team-desc">Description</FormLabel>
          <Input id="edit-team-desc" v-model="editDescription" />
        </div>
        <div v-if="policies.length > 0">
          <FormLabel for="edit-team-policy">Escalation Policy</FormLabel>
          <Select
            id="edit-team-policy"
            :model-value="editPolicyId"
            class="w-full"
            @update:model-value="editPolicyId = $event"
          >
            <option value="">None</option>
            <option v-for="p in policies" :key="p.id" :value="p.id">{{ p.name }}</option>
          </Select>
        </div>
      </div>
    </Modal>

    <Modal
      :open="showAddMember"
      title="Add Member"
      :loading="addingMember"
      confirm-label="Add"
      @update:open="!$event && resetAddMemberState()"
      @confirm="addMember"
    >
      <div class="space-y-4" @keydown="onUserDropdownKeydown">
        <div>
          <FormLabel for="add-member-user">User</FormLabel>
          <div ref="userDropdownRoot" class="relative">
            <button
              id="add-member-user"
              type="button"
              :aria-expanded="userDropdownOpen"
              aria-haspopup="listbox"
              :disabled="availableUsers.length === 0"
              class="field flex w-full cursor-pointer items-center justify-between gap-2 text-left disabled:cursor-not-allowed disabled:opacity-60"
              @click="toggleUserDropdown"
            >
              <span class="truncate">
                <UserLabel v-if="selectedUser" :user="selectedUser" />
                <span v-else-if="availableUsers.length === 0" class="text-[var(--text-muted)]">
                  No additional users available to add
                </span>
                <span v-else class="text-[var(--text-muted)]">Select user…</span>
              </span>
              <span class="flex shrink-0 items-center gap-1">
                <button
                  v-if="selectedUser"
                  type="button"
                  class="cursor-pointer rounded p-0.5 text-[var(--text-muted)] transition-colors hover:text-[var(--text-primary)]"
                  aria-label="Clear selection"
                  tabindex="-1"
                  @click="clearSelectedUser"
                >
                  <X class="h-3.5 w-3.5" />
                </button>
                <ChevronDown
                  class="h-4 w-4 text-[var(--text-muted)] transition-transform duration-200"
                  :class="userDropdownOpen ? 'rotate-180' : ''"
                />
              </span>
            </button>
            <Transition
              enter-active-class="transition duration-150 ease-out"
              enter-from-class="opacity-0 -translate-y-1"
              enter-to-class="opacity-100 translate-y-0"
              leave-active-class="transition duration-100 ease-in"
              leave-from-class="opacity-100 translate-y-0"
              leave-to-class="opacity-0 -translate-y-1"
            >
              <div
                v-if="userDropdownOpen"
                class="absolute z-50 mt-1 w-full overflow-hidden rounded-lg border border-[var(--border-primary)] bg-[var(--bg-card)] shadow-xl"
              >
                <div class="border-b border-[var(--border-primary)] p-2">
                  <div class="relative">
                    <Search
                      class="pointer-events-none absolute left-2.5 top-1/2 z-[1] h-4 w-4 -translate-y-1/2 text-[var(--text-muted)]"
                    />
                    <input
                      ref="userDropdownInput"
                      v-model="userDropdownQuery"
                      type="search"
                      class="field w-full pl-9 text-sm"
                      placeholder="Search by name or email…"
                      autocomplete="off"
                    />
                  </div>
                </div>
                <ul
                  ref="userDropdownList"
                  role="listbox"
                  class="max-h-60 overflow-y-auto overscroll-contain py-1"
                >
                  <li
                    v-if="filteredAvailableUsers.length === 0"
                    class="px-3 py-2 text-sm text-[var(--text-muted)]"
                  >
                    {{ userDropdownQuery.trim() ? "No matching users" : "No users available" }}
                  </li>
                  <li
                    v-for="(u, i) in filteredAvailableUsers"
                    :key="u.id"
                    role="option"
                    :aria-selected="u.id === selectedUserId"
                    :data-active="i === userDropdownActiveIndex"
                    class="flex cursor-pointer items-center justify-between gap-2 px-3 py-2 text-sm transition-colors"
                    :class="[
                      u.id === selectedUserId
                        ? 'bg-[var(--accent-primary)]/10 text-[var(--text-primary)]'
                        : i === userDropdownActiveIndex
                          ? 'bg-[var(--bg-secondary)] text-[var(--text-primary)]'
                          : 'text-[var(--text-secondary)]',
                    ]"
                    @click="pickUser(u)"
                    @pointerenter="userDropdownActiveIndex = i"
                  >
                    <UserLabel :user="u" />
                    <span class="truncate text-xs text-[var(--text-muted)]">{{ u.email }}</span>
                  </li>
                </ul>
              </div>
            </Transition>
          </div>
        </div>
        <div>
          <FormLabel for="add-member-role">Role</FormLabel>
          <Select
            id="add-member-role"
            :model-value="selectedRole"
            class="w-full"
            @update:model-value="selectedRole = $event as 'lead' | 'member'"
          >
            <option value="member">Member</option>
            <option value="lead">Lead</option>
          </Select>
        </div>
      </div>
    </Modal>

    <ConfirmDialog
      :open="showRemoveMember"
      title="Remove Member"
      :message="`Remove ${removingMember ? memberDisplayName(removingMember) : 'this member'} from the team?`"
      confirm-label="Remove"
      :destructive="true"
      :loading="removing"
      @update:open="showRemoveMember = $event"
      @confirm="removeMember"
    />

    <ConfirmDialog
      :open="showDeleteTeam"
      title="Delete Team"
      :message="'Delete team ' + (team?.name ?? '') + '? This action cannot be undone.'"
      confirm-label="Delete"
      :destructive="true"
      :loading="deletingTeam"
      @update:open="showDeleteTeam = $event"
      @confirm="deleteTeam"
    />
  </section>
</template>
