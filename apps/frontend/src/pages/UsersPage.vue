<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { onMounted, ref, computed, watch } from "vue";
import { Users, UserCircle } from "@lucide/vue";
import { api, type UserInfo } from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Input from "@/components/ui/Input.vue";
import UserLabel from "@/components/ui/UserLabel.vue";
import UserAvatar from "@/components/ui/UserAvatar.vue";
import Select from "@/components/ui/Select.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import SkeletonRows from "@/components/ui/SkeletonRows.vue";
import Modal from "@/components/ui/Modal.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import PhoneInput from "@/components/ui/PhoneInput.vue";
import { useToast } from "@/lib/toast";
import { useAuthStore } from "@/stores/auth";
import { useDelete } from "@/composables/useDelete";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { useListFilter } from "@/composables/useListFilter";
import { useInfiniteScroll } from "@/composables/useInfiniteScroll";
import { validatePassword } from "@/lib/validators";

defineOptions({ name: "UsersPage" });

const auth = useAuthStore();
const LIST_PAGE_SIZE = 20;

const users = ref<UserInfo[]>([]);
const userSearchQuery = ref("");
const userVisibleCount = ref(LIST_PAGE_SIZE);
const showCreateDialog = ref(false);
const showEditDialog = ref(false);
const newEmail = ref("");
const newPassword = ref("");
const newRole = ref("viewer");
const newFullName = ref("");
const editUserId = ref("");
const editRole = ref("viewer");
const editPassword = ref("");
const editEmail = ref("");
const editFullName = ref("");
const editPhone = ref("");
const editPhoneCountry = ref("");
const loading = ref(false);
const { submitting, withSubmit: withCreateUser } = useFormSubmit();
const { submitting: editSubmitting, withSubmit: withSaveEdit } = useFormSubmit();
const error = ref("");
const { push } = useToast();

const createTouched = ref({ email: false, password: false });
const editPasswordTouched = ref(false);

const {
  deleteTarget: userToDelete,
  showDeleteConfirm,
  confirmDelete,
  doDelete,
} = useDelete<{ id: string; email: string }>(async (user) => {
  await api.deleteUser(user.id);
  users.value = await api.getUsers();
}, "User");

const lazySentinelRef = ref<HTMLElement | null>(null);

// --- Helpers ---

const emailError = computed(() => {
  if (!createTouched.value.email) return "";
  if (!newEmail.value.trim()) return "Email is required";
  return "";
});

const passwordError = computed(() => {
  if (!createTouched.value.password) return "";
  if (!newPassword.value) return "Password is required";
  return validatePassword(newPassword.value).error;
});

const editPasswordError = computed(() => {
  if (!editPasswordTouched.value) return "";
  const pw = editPassword.value.trim();
  if (!pw) return "";
  return validatePassword(pw).error;
});

const createFormValid = computed(() => {
  return (
    Boolean(newEmail.value.trim()) &&
    Boolean(newPassword.value) &&
    validatePassword(newPassword.value).valid
  );
});

const filteredUsers = useListFilter(users, ["full_name", "email"], userSearchQuery);

const visibleUsers = computed(() => filteredUsers.value.slice(0, userVisibleCount.value));
const hasMoreUsers = computed(() => userVisibleCount.value < filteredUsers.value.length);

// --- User actions ---

function resetVisibleUsers() {
  userVisibleCount.value = LIST_PAGE_SIZE;
}

function loadMoreUsers() {
  if (!hasMoreUsers.value) return;
  userVisibleCount.value = Math.min(
    userVisibleCount.value + LIST_PAGE_SIZE,
    filteredUsers.value.length,
  );
}

useInfiniteScroll(lazySentinelRef, () => hasMoreUsers.value, loadMoreUsers);

async function loadUsers() {
  loading.value = true;
  error.value = "";
  try {
    users.value = await api.getUsers();
    resetVisibleUsers();
  } catch (err) {
    error.value = getErrorMessage(err, "Failed to load users");
    push(getErrorMessage(err, "Failed to load users"), "error");
  } finally {
    loading.value = false;
  }
}

function openCreateDialog() {
  newEmail.value = "";
  newPassword.value = "";
  newRole.value = "viewer";
  newFullName.value = "";
  createTouched.value = { email: false, password: false };
  error.value = "";
  showCreateDialog.value = true;
}

async function createUser() {
  createTouched.value = { email: true, password: true };
  if (!createFormValid.value) return;

  await withCreateUser(async () => {
    await api.createUser(
      newEmail.value.trim(),
      newPassword.value,
      newRole.value,
      newFullName.value.trim(),
    );
    newEmail.value = "";
    newPassword.value = "";
    newRole.value = "viewer";
    newFullName.value = "";
    showCreateDialog.value = false;
    await loadUsers();
  }, "User created");
}

function openEditDialog(user: UserInfo) {
  editUserId.value = user.id;
  editRole.value = user.role;
  editPassword.value = "";
  editPasswordTouched.value = false;
  editEmail.value = user.email ?? "";
  editFullName.value = user.full_name ?? "";
  editPhone.value = user.phone ?? "";
  editPhoneCountry.value = user.phone_country ?? "";
  error.value = "";
  showEditDialog.value = true;
}

async function saveEdit() {
  editPasswordTouched.value = true;
  const trimmedPw = editPassword.value.trim();
  if (trimmedPw) {
    const pe = validatePassword(trimmedPw).error;
    if (pe) {
      push(pe, "error");
      return;
    }
  }

  await withSaveEdit(async () => {
    const payload: {
      role?: string;
      password?: string;
      email?: string;
      full_name?: string;
      phone?: string;
      phone_country?: string;
    } = {
      role: editRole.value,
    };
    if (trimmedPw) {
      payload.password = trimmedPw;
    }
    if (editEmail.value.trim()) {
      payload.email = editEmail.value.trim();
    } else {
      payload.email = "";
    }
    payload.full_name = editFullName.value.trim();
    payload.phone = editPhone.value;
    payload.phone_country = editPhoneCountry.value;
    await api.updateUser(editUserId.value, payload);
    editUserId.value = "";
    editRole.value = "viewer";
    editPassword.value = "";
    editPasswordTouched.value = false;
    editPhone.value = "";
    editPhoneCountry.value = "";
    showEditDialog.value = false;
    users.value = await api.getUsers();
  }, "User updated");
}

// --- Lifecycle ---
usePageHeaderActions({
  title: "Users",
  titleIcon: Users,
  searchInput: userSearchQuery,
  searchPlaceholder: "Search by name or email",
  showFilters: false,
  onAdd: openCreateDialog,
  addLabel: "Add user",
});

onMounted(() => {
  loadUsers();
});

watch(
  () => filteredUsers.value.length,
  () => {
    resetVisibleUsers();
  },
);
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <p class="text-sm text-[var(--text-muted)]">
      Manage user accounts and their access levels. Admins can view and modify all settings.
    </p>

    <ErrorBanner v-if="!showCreateDialog && !showEditDialog" :message="error" />

    <div v-if="loading" class="space-y-2" aria-busy="true" aria-label="Loading users">
      <SkeletonRows :count="4" />
    </div>

    <EmptyState
      v-if="!loading && filteredUsers.length === 0"
      :message="
        userSearchQuery.trim()
          ? 'No users found. Try a different search term.'
          : 'No users found. Create your first user to get started.'
      "
    >
      <template #icon>
        <UserCircle class="mb-2 h-6 w-6 opacity-40" />
      </template>
    </EmptyState>

    <div v-if="!loading && filteredUsers.length > 0" class="space-y-3">
      <Card
        v-for="u in visibleUsers"
        :key="u.id"
        class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between"
      >
        <div class="flex min-w-0 items-start gap-3">
          <UserAvatar :user="u" />
          <div class="min-w-0">
            <div class="flex min-w-0 items-center gap-2">
              <p class="truncate text-sm font-medium text-[var(--text-primary)]">
                <UserLabel :user="u" />
              </p>
              <span
                :class="
                  u.role === 'admin'
                    ? 'badge-yellow'
                    : 'rounded bg-[var(--bg-secondary)] px-2 py-0.5 text-xs text-[var(--text-muted)]'
                "
                >{{ u.role }}</span
              >
            </div>
            <p class="truncate text-xs text-[var(--text-muted)]">{{ u.email }}</p>
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <Button size="sm" @click="openEditDialog(u)">Edit</Button>
          <Button
            v-if="u.id !== auth.user?.id"
            size="sm"
            variant="destructive"
            @click="confirmDelete(u)"
            >Delete</Button
          >
        </div>
      </Card>
      <div ref="lazySentinelRef" class="h-1 w-full" aria-hidden="true" />
      <div v-if="hasMoreUsers" class="flex justify-center">
        <Button size="sm" variant="outline" @click="loadMoreUsers">Load more</Button>
      </div>
    </div>

    <!-- ======================== CREATE USER DIALOG ======================== -->
    <Modal
      v-model:open="showCreateDialog"
      title="Add User"
      :loading="submitting"
      confirmLabel="Create"
      @confirm="createUser"
    >
      <div class="space-y-3">
        <div>
          <FormLabel for="new-email" required>Email</FormLabel>
          <Input
            id="new-email"
            v-model="newEmail"
            type="email"
            placeholder="you@example.com"
            :error="emailError"
            @blur="createTouched.email = true"
          />
        </div>
        <div>
          <FormLabel for="new-full-name">Full Name</FormLabel>
          <Input id="new-full-name" v-model="newFullName" placeholder="Jane Doe" />
        </div>
        <div>
          <FormLabel for="new-password" required>Password</FormLabel>
          <Input
            id="new-password"
            v-model="newPassword"
            type="password"
            autocomplete="new-password"
            :error="passwordError"
            @blur="createTouched.password = true"
          />
          <p class="mt-1 text-xs text-[var(--text-muted)]">
            At least 8 characters with upper and lowercase letters, a digit, and one
            non-alphanumeric character.
          </p>
        </div>
        <div>
          <FormLabel for="new-role">Role</FormLabel>
          <Select id="new-role" v-model="newRole">
            <option value="viewer">Viewer</option>
            <option value="operator">Operator</option>
            <option value="admin">Admin</option>
          </Select>
        </div>
      </div>
    </Modal>

    <!-- ======================== EDIT USER DIALOG ======================== -->
    <Modal
      v-model:open="showEditDialog"
      title="Edit User"
      :loading="editSubmitting"
      confirmLabel="Save"
      @confirm="saveEdit"
    >
      <div class="space-y-3">
        <div>
          <FormLabel for="edit-full-name">Full Name</FormLabel>
          <Input id="edit-full-name" v-model="editFullName" placeholder="Jane Doe" />
        </div>
        <div>
          <FormLabel for="edit-phone">Phone</FormLabel>
          <PhoneInput id="edit-phone" v-model="editPhone" v-model:country="editPhoneCountry" />
        </div>
        <div>
          <FormLabel for="edit-email">Email</FormLabel>
          <Input id="edit-email" v-model="editEmail" type="email" placeholder="you@example.com" />
        </div>
        <div>
          <FormLabel for="edit-role">Role</FormLabel>
          <Select id="edit-role" v-model="editRole">
            <option value="viewer">Viewer</option>
            <option value="operator">Operator</option>
            <option value="admin">Admin</option>
          </Select>
        </div>
        <div>
          <FormLabel for="edit-password">New Password (optional)</FormLabel>
          <Input
            id="edit-password"
            v-model="editPassword"
            type="password"
            autocomplete="new-password"
            :error="editPasswordError"
            @blur="editPasswordTouched = true"
          />
          <p class="mt-1 text-xs text-[var(--text-muted)]">
            Leave blank to keep current password. If set, the same complexity rules as for new users
            apply.
          </p>
        </div>
      </div>
    </Modal>

    <!-- ======================== DELETE CONFIRMATIONS ======================== -->
    <ConfirmDialog
      v-model:open="showDeleteConfirm"
      title="Delete user"
      :message="`Are you sure you want to delete '${userToDelete?.email}'? This action cannot be undone.`"
      confirm-label="Delete"
      :destructive="true"
      @confirm="doDelete"
    />
  </section>
</template>
