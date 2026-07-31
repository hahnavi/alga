<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import {
  api,
  type SharedSecretRecord,
  type CredentialProviderRecord,
  type AgentTokenRow,
} from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import Checkbox from "@/components/ui/Checkbox.vue";
import Select from "@/components/ui/Select.vue";
import Card from "@/components/ui/Card.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import Modal from "@/components/ui/Modal.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import { useDelete } from "@/composables/useDelete";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { useListFilter } from "@/composables/useListFilter";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { HEADER_ICON_BTN_CLASS } from "@/lib/uiClasses";
import { getErrorMessage } from "@/lib/error";
import { Plus, KeyRound, Trash2, Pencil, Server, Lock } from "@lucide/vue";

defineOptions({ name: "SharedSecretsPage" });

const router = useRouter();
const { canWrite: canManage } = useEntityPermissions("credentials", {
  actions: { read: "credentials:read", write: "credentials:manage" },
});

const secrets = ref<SharedSecretRecord[]>([]);
const providers = ref<CredentialProviderRecord[]>([]);
const agents = ref<AgentTokenRow[]>([]);
const loading = ref(true);
const error = ref("");
const formOpen = ref(false);
const editing = ref<SharedSecretRecord | null>(null);

const enabledProviders = computed(() => providers.value.filter((p) => p.enabled));

const form = ref({
  provider_id: "",
  name: "",
  description: "",
  value: "",
  remote_ref: "",
  rotate: false,
  allowedAgentIds: [] as string[],
});
const { submitting: saving, formError, withSubmit } = useFormSubmit();

function toggleAllowedAgent(id: string) {
  const ids = form.value.allowedAgentIds;
  form.value.allowedAgentIds = ids.includes(id) ? ids.filter((x) => x !== id) : [...ids, id];
}

const selectedProvider = computed(
  () => providers.value.find((p) => p.id === form.value.provider_id) ?? null,
);
const isInternal = computed(() => selectedProvider.value?.type === "internal");

const providerDisplayLabel = (
  p: { name: string; provider_type_name?: string; type?: string } | null | undefined,
): string => {
  if (!p) return "unknown";
  const tn = p.provider_type_name ?? p.type ?? "";
  return tn && tn !== p.name ? `${p.name} (${tn})` : p.name;
};
const providerLabel = (id: string) =>
  providerDisplayLabel(providers.value.find((x) => x.id === id));

const { deleteTarget, showDeleteConfirm, confirmDelete, doDelete } = useDelete<SharedSecretRecord>(
  async (item) => {
    await api.deleteSharedSecret(item.id);
    secrets.value = secrets.value.filter((s) => s.id !== item.id);
  },
  "shared secret",
);

function resetForm() {
  form.value = {
    provider_id: enabledProviders.value[0]?.id ?? "",
    name: "",
    description: "",
    value: "",
    remote_ref: "",
    rotate: false,
    allowedAgentIds: [],
  };
}

function openCreate() {
  editing.value = null;
  resetForm();
  formError.value = "";
  formOpen.value = true;
}

function openEdit(s: SharedSecretRecord) {
  editing.value = s;
  form.value = {
    provider_id: s.provider_id,
    name: s.name,
    description: s.description,
    value: "",
    remote_ref: s.remote_ref,
    rotate: false,
    allowedAgentIds: [...(s.allowed_agent_ids ?? [])],
  };
  formError.value = "";
  formOpen.value = true;
}

function buildAllowed(): string[] {
  return form.value.allowedAgentIds;
}

async function save() {
  if (!form.value.provider_id) {
    formError.value = "Select a credential provider.";
    return;
  }
  if (!form.value.name) {
    formError.value = "Name is required.";
    return;
  }
  if (form.value.allowedAgentIds.length === 0) {
    formError.value = "Select at least one allowed agent. Secrets are restricted by default.";
    return;
  }
  if (isInternal.value && !editing.value && !form.value.value) {
    formError.value = "Value is required for internal secrets.";
    return;
  }
  if (!isInternal.value && !form.value.remote_ref) {
    formError.value = "Remote reference (path) is required for external providers.";
    return;
  }

  const allowed = buildAllowed();
  await withSubmit(async () => {
    if (editing.value) {
      const data: Record<string, unknown> = {
        name: form.value.name,
        description: form.value.description,
        remote_ref: form.value.remote_ref,
        allowed_agent_ids: allowed,
      };
      if (isInternal.value && form.value.rotate && form.value.value) {
        data.value = form.value.value;
      }
      const updated = await api.updateSharedSecret(editing.value.id, data);
      secrets.value = secrets.value.map((s) => (s.id === updated.id ? updated : s));
    } else {
      const created = await api.createSharedSecret({
        provider_id: form.value.provider_id,
        name: form.value.name,
        description: form.value.description,
        remote_ref: isInternal.value ? undefined : form.value.remote_ref,
        value: isInternal.value ? form.value.value : undefined,
        allowed_agent_ids: allowed,
      });
      secrets.value = [...secrets.value, created];
    }
    formOpen.value = false;
  });
}

async function loadAll() {
  loading.value = true;
  error.value = "";
  try {
    const [secretsResp, providersResp] = await Promise.all([
      api.listSharedSecrets(),
      api.listCredentialProviders(),
    ]);
    secrets.value = secretsResp;
    providers.value = providersResp;
    if (canManage.value) {
      try {
        agents.value = await api.getAgentTokens();
      } catch {
        agents.value = [];
      }
    }
  } catch (e) {
    error.value = getErrorMessage(e, "Failed to load shared secrets");
  } finally {
    loading.value = false;
  }
}

const searchInput = ref("");

usePageHeaderActions({
  title: "Shared Secrets",
  titleIcon: KeyRound,
  searchInput,
  showAdd: canManage,
  onAdd: openCreate,
  addLabel: "Add secret",
});

const filteredSecrets = useListFilter(secrets, ["name", "secret_id", "description"], searchInput);

onMounted(() => {
  loadAll();
});
</script>

<template>
  <div class="p-6 max-w-5xl mx-auto space-y-6">
    <ErrorBanner v-if="error" :message="error" />

    <div class="flex items-center justify-end">
      <Button variant="outline" @click="router.push('/settings/credential-providers')">
        <Server class="h-4 w-4" />
        Manage providers
      </Button>
    </div>

    <LoadingSpinner v-if="loading" />

    <EmptyState
      v-else-if="secrets.length === 0"
      title="No shared secrets yet"
      message="Create a secret that agents can fetch by ID via the agent tool API."
    >
      <template v-if="canManage" #actions>
        <Button @click="openCreate">
          <Plus class="h-4 w-4" />
          Add Secret
        </Button>
      </template>
    </EmptyState>

    <div v-else class="space-y-4">
      <Card v-for="s in filteredSecrets" :key="s.id" class="p-5 flex items-start gap-4">
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-3 flex-wrap">
            <h3 class="font-semibold text-lg truncate">{{ s.name }}</h3>
            <span class="badge-muted font-mono">
              {{ s.secret_id }}
            </span>
            <span class="badge-muted">
              {{ s.provider ? providerDisplayLabel(s.provider) : providerLabel(s.provider_id) }}
            </span>
            <span v-if="s.provider && !s.provider.enabled" class="badge-yellow">
              provider disabled
            </span>
            <span
              v-if="(s.allowed_agent_ids?.length ?? 0) > 0"
              class="badge-blue inline-flex items-center gap-1"
            >
              <Lock class="h-3 w-3" />
              {{ s.allowed_agent_ids!.length }} agent(s)
            </span>
            <span v-else class="badge-yellow inline-flex items-center gap-1">
              <Lock class="h-3 w-3" />
              no access
            </span>
          </div>
          <p v-if="s.description" class="mt-1 text-sm text-[var(--text-muted)]">
            {{ s.description }}
          </p>
          <div class="mt-2 text-xs text-[var(--text-muted)]">
            <template v-if="s.provider?.type === 'internal'">
              <span v-if="s.value_configured" class="text-green-600">value stored</span>
              <span v-else class="text-red-500">no value</span>
            </template>
            <template v-else>
              remote ref: <code class="font-mono">{{ s.remote_ref || "—" }}</code>
            </template>
          </div>
          <div
            class="mt-2 text-xs text-[var(--text-muted)] font-mono bg-[var(--bg-secondary)] inline-block px-2 py-1 rounded"
          >
            GET /api/v1/agent/secrets/{{ s.secret_id }}
          </div>
        </div>

        <div class="flex items-center gap-3 shrink-0">
          <Button
            v-if="canManage"
            variant="outline"
            :class="HEADER_ICON_BTN_CLASS"
            @click="openEdit(s)"
          >
            <Pencil class="h-4 w-4" />
          </Button>
          <Button
            v-if="canManage"
            variant="destructive"
            :class="HEADER_ICON_BTN_CLASS"
            @click="confirmDelete(s)"
          >
            <Trash2 class="h-4 w-4" />
          </Button>
        </div>
      </Card>
    </div>

    <Modal
      :open="formOpen"
      @update:open="formOpen = $event"
      :title="editing ? 'Edit Shared Secret' : 'Add Shared Secret'"
    >
      <div class="space-y-4">
        <ErrorBanner v-if="formError" :message="formError" />

        <div v-if="enabledProviders.length === 0" class="text-sm text-[var(--text-muted)]">
          No enabled credential providers. Add and enable a provider first.
          <button
            type="button"
            class="px-1 text-primary hover:underline"
            @click="router.push('/settings/credential-providers')"
          >
            Manage providers
          </button>
        </div>

        <div>
          <label class="block text-sm font-medium mb-1.5">Credential Provider</label>
          <Select v-model="form.provider_id" :disabled="!!editing || enabledProviders.length === 0">
            <option value="" disabled>Select a provider…</option>
            <option v-for="p in enabledProviders" :key="p.id" :value="p.id">
              {{ providerDisplayLabel(p) }}
            </option>
          </Select>
          <p class="text-xs text-[var(--text-muted)] mt-1">
            Choose where this secret is stored or resolved from. Internal stores it encrypted in
            Alga; external providers proxy the read.
          </p>
        </div>

        <div>
          <label class="block text-sm font-medium mb-1.5">Name</label>
          <Input v-model="form.name" placeholder="DB Password" />
          <p class="text-xs text-[var(--text-muted)] mt-1">
            A unique secret ID for agents to fetch by is generated automatically.
          </p>
        </div>

        <div>
          <label class="block text-sm font-medium mb-1.5">Description</label>
          <Input v-model="form.description" placeholder="Production database root password" />
        </div>

        <div v-if="isInternal">
          <label class="block text-sm font-medium mb-1.5">
            Secret Value
            <span v-if="editing" class="font-normal text-[var(--text-muted)]">
              (check to rotate and enter a new value)
            </span>
          </label>
          <div v-if="editing" class="flex items-center gap-2 mb-2">
            <Checkbox id="rotate" v-model="form.rotate" />
            <label for="rotate" class="text-sm">Rotate value</label>
          </div>
          <Input
            v-if="!editing || form.rotate"
            v-model="form.value"
            type="password"
            placeholder="the secret value"
          />
          <p v-else class="text-xs text-green-600">A value is stored (hidden).</p>
        </div>

        <div v-else>
          <label class="block text-sm font-medium mb-1.5">Remote Reference (Path)</label>
          <Input v-model="form.remote_ref" placeholder="secret/data/db" />
          <p class="text-xs text-[var(--text-muted)] mt-1">
            The backend-specific path/key the provider resolves at read time.
          </p>
        </div>

        <div class="border-t pt-4">
          <div class="flex items-center gap-2 mb-1">
            <Lock class="h-4 w-4 text-[var(--text-muted)]" />
            <label class="text-sm font-medium">Allowed agents</label>
          </div>
          <p class="text-xs text-[var(--text-muted)] mb-2">
            Only the agents selected here can fetch this secret. Secrets are restricted by default.
          </p>
          <div class="max-h-40 overflow-y-auto space-y-1 border rounded p-2">
            <div v-if="agents.length === 0" class="text-xs text-[var(--text-muted)]">
              No agents available.
            </div>
            <label
              v-for="a in agents"
              :key="a.id"
              class="flex items-center gap-2 text-sm cursor-pointer"
            >
              <Checkbox
                :model-value="form.allowedAgentIds.includes(a.id)"
                @update:model-value="toggleAllowedAgent(a.id)"
              />
              <span>{{ a.name }}</span>
              <span class="text-xs text-[var(--text-muted)]">({{ a.agent_type ?? "agent" }})</span>
            </label>
          </div>
        </div>
      </div>

      <template #footer>
        <Button variant="outline" :disabled="saving" @click="formOpen = false">Cancel</Button>
        <Button variant="primary" :loading="saving" @click="save">
          {{ editing ? "Save Changes" : "Create Secret" }}
        </Button>
      </template>
    </Modal>

    <ConfirmDialog
      :open="showDeleteConfirm"
      title="Delete Shared Secret"
      :message="`Are you sure you want to delete '${deleteTarget?.name}'? Agents will no longer be able to fetch it.`"
      confirm-label="Delete"
      destructive
      @confirm="doDelete"
      @cancel="showDeleteConfirm = false"
    />
  </div>
</template>
