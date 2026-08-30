<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { computed, onMounted, ref, watch } from "vue";
import { Plus, KeyRound, Clock, Search, Shield } from "@lucide/vue";
import { api, type PATRow } from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import Switch from "@/components/ui/Switch.vue";
import Modal from "@/components/ui/Modal.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import SecretDisplay from "@/components/ui/SecretDisplay.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import Card from "@/components/ui/Card.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import DateTimePicker from "@/components/ui/DateTimePicker.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Select from "@/components/ui/Select.vue";
import SettingsPageShell from "@/components/ui/settings/SettingsPageShell.vue";
import { useToast } from "@/lib/toast";
import { useAuthStore } from "@/stores/auth";
import { useDelete } from "@/composables/useDelete";
import { useClipboard } from "@/composables/useClipboard";
import { useEscapeKey } from "@/composables/useEscapeKey";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { useListFilter } from "@/composables/useListFilter";
import { formatExpires, localDatetimeToRFC3339 } from "@/lib/time";

defineOptions({ name: "PersonalAccessTokensPage" });

const { push } = useToast();
const auth = useAuthStore();
const { copyToClipboard } = useClipboard();

const tokens = ref<PATRow[]>([]);
const loading = ref(false);
const error = ref("");
const showCreateDialog = ref(false);
const newName = ref("");
const newPermissions = ref<string[]>([]);
const newExpiresLocal = ref("");
const permSearch = ref("");
const createdSecret = ref<string | null>(null);
const { submitting: creating, formError, withSubmit } = useFormSubmit();

type PermissionDef = {
  key: string;
  label: string;
  description: string;
};

type PermissionGroup = {
  key: string;
  label: string;
  description: string;
  permissions: PermissionDef[];
};

const PERMISSION_GROUPS: PermissionGroup[] = [
  {
    key: "alerts",
    label: "Alerts",
    description: "View and manage incoming alerts.",
    permissions: [
      { key: "alerts:read", label: "Read", description: "View alerts and their details." },
      { key: "alerts:write", label: "Write", description: "Create, ack, and update alerts." },
      { key: "alerts:delete", label: "Delete", description: "Permanently delete alerts." },
    ],
  },
  {
    key: "knowledge",
    label: "Knowledge",
    description: "Runbooks, postmortems, and knowledge base articles.",
    permissions: [
      { key: "knowledge:read", label: "Read", description: "View knowledge base articles." },
      { key: "knowledge:write", label: "Write", description: "Create and update articles." },
      { key: "knowledge:delete", label: "Delete", description: "Permanently delete articles." },
    ],
  },
  {
    key: "routes",
    label: "Routes",
    description: "Routing rules that map alerts to teams and services.",
    permissions: [
      { key: "routes:read", label: "Read", description: "View routing rules." },
      { key: "routes:write", label: "Write", description: "Create, update, and delete routes." },
    ],
  },
  {
    key: "integrations",
    label: "Integrations",
    description: "Inbound and outbound integrations (Slack, webhooks, etc.).",
    permissions: [
      { key: "integrations:read", label: "Read", description: "View integrations and config." },
      {
        key: "integrations:write",
        label: "Write",
        description: "Create, update, and delete integrations.",
      },
      { key: "integrations:test", label: "Test", description: "Send test notifications." },
    ],
  },
  {
    key: "dashboard",
    label: "Dashboard",
    description: "Read-only access to the dashboard view.",
    permissions: [
      { key: "dashboard:read", label: "Read", description: "View dashboard data and metrics." },
    ],
  },
  {
    key: "notifications",
    label: "Notifications",
    description: "Notification rules and delivery preferences.",
    permissions: [
      { key: "notifications:read", label: "Read", description: "View notification rules." },
      {
        key: "notifications:write",
        label: "Write",
        description: "Create, update, and delete notification rules.",
      },
    ],
  },
  {
    key: "memories",
    label: "Memories",
    description: "Stored agent memories and context.",
    permissions: [
      { key: "memories:read", label: "Read", description: "View stored memories." },
      { key: "memories:write", label: "Write", description: "Create and update memories." },
      { key: "memories:delete", label: "Delete", description: "Permanently delete memories." },
    ],
  },
  {
    key: "incidents",
    label: "Incidents",
    description: "Incident lifecycle, commands, and post-incident work.",
    permissions: [
      { key: "incidents:read", label: "Read", description: "View incidents and timelines." },
      {
        key: "incidents:write",
        label: "Write",
        description: "Create and update incidents.",
      },
      {
        key: "incidents:command",
        label: "Command",
        description: "Run command actions against incidents.",
      },
      {
        key: "incidents:delete",
        label: "Delete",
        description: "Permanently delete incidents.",
      },
    ],
  },
  {
    key: "services",
    label: "Services",
    description: "Service catalog and ownership.",
    permissions: [
      { key: "services:read", label: "Read", description: "View services and their details." },
      {
        key: "services:write",
        label: "Write",
        description: "Create, update, and delete services.",
      },
    ],
  },
  {
    key: "oncall",
    label: "On-Call",
    description: "On-call schedules, shifts, and overrides.",
    permissions: [
      { key: "oncall:read", label: "Read", description: "View on-call schedules." },
      {
        key: "oncall:write",
        label: "Write",
        description: "Update schedules, shifts, and overrides.",
      },
    ],
  },
  {
    key: "escalation",
    label: "Escalation",
    description: "Escalation policies and targets.",
    permissions: [
      { key: "escalation:read", label: "Read", description: "View escalation policies." },
      {
        key: "escalation:write",
        label: "Write",
        description: "Create, update, and delete escalation policies.",
      },
    ],
  },
  {
    key: "postmortems",
    label: "Post-Mortems",
    description: "Post-incident reports and action items.",
    permissions: [
      { key: "postmortems:read", label: "Read", description: "View post-mortems." },
      { key: "postmortems:write", label: "Write", description: "Create and update post-mortems." },
      {
        key: "postmortems:delete",
        label: "Delete",
        description: "Permanently delete post-mortems.",
      },
    ],
  },
];

const availablePermissionGroups = computed<PermissionGroup[]>(() =>
  PERMISSION_GROUPS.map((g) => ({
    ...g,
    permissions: g.permissions.filter((p) => auth.hasPermission(p.key)),
  })).filter((g) => g.permissions.length > 0),
);

const totalAvailablePermissions = computed(() =>
  availablePermissionGroups.value.reduce((acc, g) => acc + g.permissions.length, 0),
);

function matchSearch(perm: PermissionDef, group: PermissionGroup): boolean {
  const q = permSearch.value.trim().toLowerCase();
  if (!q) return true;
  return (
    perm.key.toLowerCase().includes(q) ||
    perm.label.toLowerCase().includes(q) ||
    perm.description.toLowerCase().includes(q) ||
    group.label.toLowerCase().includes(q) ||
    group.description.toLowerCase().includes(q)
  );
}

function groupSearchText(g: PermissionGroup): string {
  const parts: string[] = [g.label, g.description];
  for (const p of g.permissions) {
    parts.push(p.key, p.label, p.description);
  }
  return parts.join(" ");
}

const searchedGroups = useListFilter(availablePermissionGroups, [groupSearchText], permSearch);

const filteredPermissionGroups = computed<PermissionGroup[]>(() => {
  if (!permSearch.value.trim()) return searchedGroups.value;
  return searchedGroups.value
    .map((g) => ({
      ...g,
      permissions: g.permissions.filter((p) => matchSearch(p, g)),
    }))
    .filter((g) => g.permissions.length > 0);
});

function togglePermission(key: string, on: boolean) {
  if (on) {
    if (!newPermissions.value.includes(key)) newPermissions.value.push(key);
    return;
  }
  newPermissions.value = newPermissions.value.filter((p) => p !== key);
}

function toggleGroup(group: PermissionGroup, on: boolean) {
  const next = new Set(newPermissions.value);
  for (const perm of group.permissions) {
    if (on) next.add(perm.key);
    else next.delete(perm.key);
  }
  newPermissions.value = Array.from(next);
}

function isGroupAllSelected(group: PermissionGroup): boolean {
  return group.permissions.every((p) => newPermissions.value.includes(p.key));
}

function isGroupSomeSelected(group: PermissionGroup): boolean {
  return group.permissions.some((p) => newPermissions.value.includes(p.key));
}

function isGroupFullySelectedInFilter(group: PermissionGroup): boolean {
  const perms = filteredPermissionGroupPermissions(group);
  if (perms.length === 0) return false;
  return perms.every((p) => newPermissions.value.includes(p.key));
}

function filteredPermissionGroupPermissions(group: PermissionGroup): PermissionDef[] {
  return group.permissions.filter((p) => matchSearch(p, group));
}

function selectAllPermissions() {
  const next = new Set(newPermissions.value);
  for (const g of availablePermissionGroups.value) {
    for (const p of g.permissions) next.add(p.key);
  }
  newPermissions.value = Array.from(next);
}

function clearAllPermissions() {
  newPermissions.value = [];
}

type ExpiryMode = "30" | "90" | "365" | "0" | "custom";

const EXPIRY_OPTIONS: { value: ExpiryMode; label: string; days: number | null }[] = [
  { value: "30", label: "30 days", days: 30 },
  { value: "90", label: "90 days", days: 90 },
  { value: "365", label: "1 year", days: 365 },
  { value: "0", label: "No expiry", days: 0 },
  { value: "custom", label: "Custom date…", days: null },
];

const expiryMode = ref<ExpiryMode>("90");

function setExpiryFromNow(days: number) {
  const d = new Date();
  d.setDate(d.getDate() + days);
  const pad = (n: number) => String(n).padStart(2, "0");
  newExpiresLocal.value = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function daysFromNow(dateStr: string): number | null {
  const target = new Date(dateStr).getTime();
  if (!Number.isFinite(target)) return null;
  return Math.round((target - Date.now()) / 86400000);
}

function detectExpiryMode(dateStr: string): ExpiryMode {
  if (dateStr.trim() === "") return "0";
  const days = daysFromNow(dateStr);
  if (days === null) return "custom";
  if (days >= 29 && days <= 31) return "30";
  if (days >= 89 && days <= 91) return "90";
  if (days >= 364 && days <= 366) return "365";
  return "custom";
}

watch(expiryMode, (mode) => {
  if (mode === "custom") return;
  const opt = EXPIRY_OPTIONS.find((o) => o.value === mode);
  if (!opt || opt.days === null) return;
  if (opt.days === 0) {
    newExpiresLocal.value = "";
    return;
  }
  setExpiryFromNow(opt.days);
});

watch(newExpiresLocal, (val) => {
  const detected = detectExpiryMode(val);
  if (detected !== expiryMode.value) expiryMode.value = detected;
});

const {
  deleteTarget: tokenToDelete,
  showDeleteConfirm: showRevokeConfirm,
  confirmDelete: confirmRevoke,
  doDelete: doRevoke,
} = useDelete<PATRow>(async (token) => {
  await api.revokePAT(token.id);
  tokens.value = await api.getPATs();
}, "Token");

function tokenStatus(t: PATRow): "active" | "expired" | "revoked" {
  if (t.revoked) return "revoked";
  if (t.expires_at && new Date(t.expires_at) < new Date()) return "expired";
  return "active";
}

function statusBadgeClass(status: "active" | "expired" | "revoked") {
  if (status === "active") return "badge-green";
  if (status === "expired") return "badge-yellow";
  return "badge-red";
}

async function loadTokens() {
  loading.value = true;
  error.value = "";
  try {
    tokens.value = await api.getPATs();
  } catch (err) {
    error.value = getErrorMessage(err, "Failed to load tokens");
  } finally {
    loading.value = false;
  }
}

function openCreateDialog() {
  newName.value = "";
  newPermissions.value = [];
  permSearch.value = "";
  createdSecret.value = null;
  error.value = "";
  formError.value = "";
  expiryMode.value = "90";
  setExpiryFromNow(90);
  showCreateDialog.value = true;
}

function closeCreateDialog() {
  showCreateDialog.value = false;
  createdSecret.value = null;
}

async function createToken() {
  const name = newName.value.trim();
  if (!name) {
    push("Token name is required", "error");
    return;
  }
  if (newPermissions.value.length === 0) {
    push("Select at least one permission", "error");
    return;
  }
  let expiresAt: string | undefined;
  if (newExpiresLocal.value.trim()) {
    const iso = localDatetimeToRFC3339(newExpiresLocal.value);
    if (!iso) {
      push("Invalid expiration date", "error");
      return;
    }
    expiresAt = iso;
  }
  await withSubmit(async () => {
    const res = await api.createPAT(name, newPermissions.value, expiresAt);
    createdSecret.value = res.token;
    tokens.value = await api.getPATs();
  }, "Personal access token created");
}

async function copySecret() {
  if (!createdSecret.value) return;
  await copyToClipboard(createdSecret.value, "Secret copied");
}

useEscapeKey(
  () => closeCreateDialog(),
  () => !showRevokeConfirm.value && showCreateDialog.value,
);

usePageHeaderActions({
  title: "Personal Access Tokens",
  onAdd: openCreateDialog,
  addLabel: "Create token",
});

onMounted(() => {
  loadTokens();
});
</script>

<template>
  <SettingsPageShell
    description="Manage personal access tokens for API authentication. Each token's secret is shown only once after creation. Tokens are scoped to the permissions you select."
  >
    <ErrorBanner :message="error" />

    <LoadingSpinner v-if="loading" centered />

    <template v-else-if="tokens.length">
      <div class="space-y-3">
        <Card
          v-for="t in tokens"
          :key="t.id"
          class="relative transition-colors"
          :class="{ 'opacity-50': tokenStatus(t) !== 'active' }"
        >
          <div class="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
            <div class="flex min-w-0 items-start gap-3">
              <div
                class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-[var(--bg-secondary)]"
              >
                <KeyRound class="h-4 w-4 text-[var(--text-secondary)]" />
              </div>
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="text-sm font-semibold text-[var(--text-primary)]">{{ t.name }}</span>
                  <span :class="statusBadgeClass(tokenStatus(t))">
                    {{ tokenStatus(t) }}
                  </span>
                </div>
                <div
                  class="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--text-muted)]"
                >
                  <span class="inline-flex items-center gap-1">
                    <Clock class="h-3 w-3" />
                    Created {{ formatExpires({ expires_at: t.created_at }) }}
                  </span>
                  <span class="inline-flex items-center gap-1">
                    <Clock class="h-3 w-3" />
                    {{
                      t.expires_at
                        ? `Expires ${formatExpires({ expires_at: t.expires_at })}`
                        : "No expiry"
                    }}
                  </span>
                  <span v-if="t.last_used_at" class="inline-flex items-center gap-1">
                    Last used {{ formatExpires({ expires_at: t.last_used_at }) }}
                  </span>
                  <span v-else class="inline-flex items-center gap-1">Never used</span>
                </div>
                <div v-if="t.permissions?.length" class="mt-2 flex flex-wrap gap-1.5">
                  <span
                    v-for="p in t.permissions"
                    :key="p"
                    class="rounded-md bg-[var(--bg-secondary)] px-2 py-0.5 text-[11px] text-[var(--text-secondary)]"
                  >
                    {{ p }}
                  </span>
                </div>
              </div>
            </div>
            <div class="flex shrink-0 items-center gap-2 md:mt-0">
              <Button
                size="sm"
                variant="destructive"
                :disabled="tokenStatus(t) !== 'active'"
                @click="confirmRevoke(t)"
              >
                Revoke
              </Button>
            </div>
          </div>
        </Card>
      </div>
    </template>

    <EmptyState v-else message="No personal access tokens yet.">
      <template #icon>
        <KeyRound class="mb-2 h-6 w-6 opacity-40" />
      </template>
      <template #footer>
        <p class="text-xs text-[var(--text-muted)]">Create a token to authenticate API requests.</p>
        <Button size="sm" class="mt-2" @click="openCreateDialog">
          <Plus class="h-4 w-4" />
          Create token
        </Button>
      </template>
    </EmptyState>

    <Modal
      v-model:open="showCreateDialog"
      title="Create personal access token"
      max-width="3xl"
      @close="closeCreateDialog"
    >
      <ErrorBanner :message="formError" />
      <template v-if="!createdSecret">
        <div class="max-h-[calc(100vh-220px)] space-y-5 overflow-y-auto pr-1">
          <p class="text-sm text-[var(--text-secondary)]">
            Select the permissions you'd like to grant this token. The secret is shown only once
            after creation — store it securely.
          </p>

          <Card class="!p-4">
            <FormLabel for="pat-name-input" required>Token name</FormLabel>
            <Input
              id="pat-name-input"
              v-model="newName"
              placeholder="e.g. ci-pipeline"
              class="mt-1.5"
            />
            <p class="mt-1.5 text-xs text-[var(--text-muted)]">
              Use a name that helps you recognize this token later (where it's used, who owns it).
            </p>
          </Card>

          <Card class="!p-4">
            <div class="flex items-start justify-between gap-3">
              <div>
                <FormLabel for="pat-expiry-mode">Expiration</FormLabel>
                <p class="mt-0.5 text-xs text-[var(--text-muted)]">
                  Set an expiration to limit how long this token remains valid.
                </p>
              </div>
            </div>
            <div class="mt-3">
              <Select id="pat-expiry-mode" v-model="expiryMode" class="w-full">
                <option v-for="opt in EXPIRY_OPTIONS" :key="opt.value" :value="opt.value">
                  {{ opt.label }}
                </option>
              </Select>
            </div>
            <div v-if="expiryMode === 'custom'" class="mt-3">
              <DateTimePicker
                id="pat-expiry"
                v-model="newExpiresLocal"
                placeholder="Pick expiry date & time"
              />
            </div>
          </Card>

          <Card class="!p-4">
            <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div>
                <div class="flex items-center gap-2">
                  <Shield class="h-4 w-4 text-[var(--text-muted)]" />
                  <span class="text-sm font-semibold text-[var(--text-primary)]">Permissions</span>
                </div>
                <p class="mt-1 text-xs text-[var(--text-muted)]">
                  {{ newPermissions.length }} of {{ totalAvailablePermissions }} permissions
                  selected
                </p>
              </div>
              <div class="flex flex-wrap items-center gap-3">
                <div class="relative w-full sm:w-56">
                  <Search
                    class="pointer-events-none absolute left-2.5 top-1/2 z-[1] h-3.5 w-3.5 -translate-y-1/2 text-[var(--text-muted)]"
                  />
                  <Input
                    v-model="permSearch"
                    type="search"
                    placeholder="Filter permissions"
                    class="w-full pl-8"
                  />
                </div>
                <div class="flex shrink-0 items-center gap-1">
                  <Button
                    size="sm"
                    variant="outline"
                    :disabled="newPermissions.length === totalAvailablePermissions"
                    @click="selectAllPermissions"
                  >
                    Select all
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    :disabled="newPermissions.length === 0"
                    @click="clearAllPermissions"
                  >
                    Clear
                  </Button>
                </div>
              </div>
            </div>

            <div class="mt-4 max-h-[min(60vh,520px)] space-y-2 overflow-y-auto rounded-md pr-1">
              <div
                v-if="filteredPermissionGroups.length === 0"
                class="rounded-md border border-dashed border-[var(--border-primary)] bg-[var(--bg-secondary)]/40 p-4 text-center text-xs text-[var(--text-muted)]"
              >
                No permissions match your filter.
              </div>
              <div
                v-for="group in filteredPermissionGroups"
                :key="group.key"
                class="overflow-hidden rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)]"
              >
                <div
                  class="flex flex-col gap-2 border-b border-[var(--border-primary)] bg-[var(--bg-secondary)]/50 px-3 py-2.5 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div class="min-w-0">
                    <div class="flex items-center gap-2">
                      <span class="text-sm font-semibold text-[var(--text-primary)]">
                        {{ group.label }}
                      </span>
                      <span class="text-xs text-[var(--text-muted)]">
                        {{ group.permissions.filter((p) => newPermissions.includes(p.key)).length }}
                        of {{ group.permissions.length }}
                      </span>
                    </div>
                    <p class="mt-0.5 text-xs text-[var(--text-muted)]">
                      {{ group.description }}
                    </p>
                  </div>
                  <div class="flex shrink-0 items-center gap-2">
                    <span
                      v-if="
                        isGroupSomeSelected(group) &&
                        !isGroupFullySelectedInFilter(group) &&
                        !isGroupAllSelected(group)
                      "
                      class="text-[11px] text-[var(--text-muted)]"
                    >
                      {{
                        filteredPermissionGroupPermissions(group).filter((p) =>
                          newPermissions.includes(p.key),
                        ).length
                      }}
                      of {{ filteredPermissionGroupPermissions(group).length }} in filter
                    </span>
                    <Button
                      v-if="!isGroupAllSelected(group) || isGroupSomeSelected(group)"
                      size="sm"
                      variant="outline"
                      @click="toggleGroup(group, true)"
                    >
                      Select all
                    </Button>
                    <Button v-else size="sm" variant="outline" @click="toggleGroup(group, false)">
                      Clear
                    </Button>
                  </div>
                </div>
                <ul class="divide-y divide-[var(--border-primary)]">
                  <li
                    v-for="perm in group.permissions"
                    :key="perm.key"
                    class="flex items-start justify-between gap-3 px-3 py-2.5 transition-colors hover:bg-[var(--bg-secondary)]/40"
                  >
                    <div class="min-w-0 flex-1">
                      <div class="flex items-center gap-2">
                        <span class="text-sm font-medium text-[var(--text-primary)]">
                          {{ perm.label }}
                        </span>
                        <code
                          class="rounded bg-[var(--bg-secondary)] px-1.5 py-0.5 text-[10px] text-[var(--text-muted)]"
                        >
                          {{ perm.key }}
                        </code>
                      </div>
                      <p class="mt-0.5 text-xs text-[var(--text-muted)]">
                        {{ perm.description }}
                      </p>
                    </div>
                    <div class="shrink-0 pt-0.5">
                      <Switch
                        :model-value="newPermissions.includes(perm.key)"
                        @update:model-value="(v: boolean) => togglePermission(perm.key, v)"
                      />
                    </div>
                  </li>
                </ul>
              </div>
            </div>
          </Card>

          <p class="text-xs text-[var(--text-muted)]">
            After creation you will receive a secret token. Store it securely; it will not be shown
            again.
          </p>
        </div>
      </template>
      <template v-else>
        <p class="mb-3 text-sm text-[var(--text-secondary)]">
          Save this token now. It will not be shown again. Use as
          <code class="rounded bg-[var(--bg-code)] px-1 text-xs">Authorization: Bearer ...</code>
          for API authentication.
        </p>
        <SecretDisplay :secret="createdSecret" @copy="copySecret" />
      </template>

      <template #footer>
        <template v-if="!createdSecret">
          <Button variant="outline" :disabled="creating" @click="closeCreateDialog">Cancel</Button>
          <Button variant="primary" :loading="creating" @click="createToken">
            Generate token
          </Button>
        </template>
        <template v-else>
          <Button id="pat-done-btn" variant="primary" @click="closeCreateDialog">Done</Button>
        </template>
      </template>
    </Modal>

    <ConfirmDialog
      v-model:open="showRevokeConfirm"
      title="Revoke token"
      :message="`Are you sure you want to revoke '${tokenToDelete?.name}'? This action cannot be undone.`"
      confirm-label="Revoke"
      :destructive="true"
      @confirm="doRevoke"
    />
  </SettingsPageShell>
</template>
