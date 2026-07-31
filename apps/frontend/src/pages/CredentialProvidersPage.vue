<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import { api, type CredentialProviderRecord, CREDENTIAL_PROVIDER_TYPES } from "@/lib/api";
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
import { useToast } from "@/lib/toast";
import { useDelete } from "@/composables/useDelete";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { useListFilter } from "@/composables/useListFilter";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { HEADER_ICON_BTN_CLASS } from "@/lib/uiClasses";
import { getErrorMessage } from "@/lib/error";
import { Plus, Trash2, Pencil, ArrowLeft, Server } from "@lucide/vue";

defineOptions({ name: "CredentialProvidersPage" });

const router = useRouter();
const { canWrite: canManage } = useEntityPermissions("credentials", {
  actions: { read: "credentials:read", write: "credentials:manage" },
});

const providers = ref<CredentialProviderRecord[]>([]);
const loading = ref(true);
const error = ref("");
const formOpen = ref(false);
const editing = ref<CredentialProviderRecord | null>(null);

const form = ref({
  name: "",
  type: "internal" as CredentialProviderRecord["type"],
  configJson: "",
  enabled: true,
});
const { submitting: saving, formError, withSubmit } = useFormSubmit();

const { push } = useToast();

const typeMeta = computed(() => {
  const t = form.value.type;
  return CREDENTIAL_PROVIDER_TYPES.find((x) => x.value === t);
});

const configHint = computed(() => {
  switch (form.value.type) {
    case "hashicorp_vault":
      return '{"address": "https://vault.example:8200", "token": "hvs.x", "namespace": ""}';
    case "aws_secrets_manager":
      return '{"region": "us-east-1", "access_key_id": "AKIA...", "secret_access_key": "..."}';
    case "gcp_secret_manager":
      return '{"project": "my-project", "credentials_json": "..."}';
    case "azure_key_vault":
      return '{"vault_url": "https://my-vault.vault.azure.net", "tenant_id": "...", "client_id": "...", "client_secret": "..."}';
    default:
      return "";
  }
});

const { deleteTarget, showDeleteConfirm, confirmDelete, doDelete } =
  useDelete<CredentialProviderRecord>(async (item) => {
    await api.deleteCredentialProvider(item.id);
    providers.value = providers.value.filter((p) => p.id !== item.id);
  }, "credential provider");

function openCreate() {
  editing.value = null;
  form.value = { name: "", type: "internal", configJson: "", enabled: true };
  formError.value = "";
  formOpen.value = true;
}

function openEdit(p: CredentialProviderRecord) {
  editing.value = p;
  form.value = { name: p.name, type: p.type, configJson: "", enabled: p.enabled };
  formError.value = "";
  formOpen.value = true;
}

function parseConfig(): Record<string, string> | undefined {
  if (!typeMeta.value?.external) return undefined;
  const raw = form.value.configJson.trim();
  if (raw === "") return {};
  try {
    const parsed = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
      formError.value = "Config must be a JSON object of key/value strings.";
      return undefined;
    }
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(parsed)) {
      out[k] = typeof v === "string" ? v : String(v);
    }
    return out;
  } catch {
    formError.value = "Config is not valid JSON.";
    return undefined;
  }
}

async function save() {
  if (!form.value.name) {
    formError.value = "Name is required.";
    return;
  }
  const config = parseConfig();
  if (config === undefined && formError.value) return;

  await withSubmit(async () => {
    if (editing.value) {
      const data: Record<string, unknown> = {
        name: form.value.name,
        enabled: form.value.enabled,
      };
      if (typeMeta.value?.external && form.value.configJson.trim() !== "") {
        data.config = config;
      }
      const updated = await api.updateCredentialProvider(editing.value.id, data);
      providers.value = providers.value.map((p) => (p.id === updated.id ? updated : p));
    } else {
      const created = await api.createCredentialProvider({
        name: form.value.name,
        type: form.value.type,
        enabled: form.value.enabled,
        config: typeMeta.value?.external ? config : undefined,
      });
      providers.value = [...providers.value, created];
    }
    formOpen.value = false;
  });
}

async function toggleEnabled(p: CredentialProviderRecord) {
  try {
    const updated = await api.updateCredentialProvider(p.id, { enabled: !p.enabled });
    providers.value = providers.value.map((x) => (x.id === updated.id ? updated : x));
  } catch (e) {
    push(getErrorMessage(e, "Failed to toggle provider"), "error");
  }
}

async function loadProviders() {
  loading.value = true;
  error.value = "";
  try {
    providers.value = await api.listCredentialProviders();
  } catch (e) {
    error.value = getErrorMessage(e, "Failed to load providers");
  } finally {
    loading.value = false;
  }
}

const searchInput = ref("");

usePageHeaderActions({
  title: "Credential Providers",
  titleIcon: Server,
  searchInput,
  onAdd: openCreate,
  addLabel: "Add provider",
  showAdd: canManage,
});

const filteredProviders = useListFilter(providers, ["name", "type"], searchInput);

onMounted(() => {
  loadProviders();
});
</script>

<template>
  <div class="p-6 max-w-4xl mx-auto space-y-6">
    <ErrorBanner v-if="error" :message="error" @retry="loadProviders" />

    <div class="flex items-center justify-end">
      <Button variant="outline" @click="router.push('/credentials')">
        <ArrowLeft class="h-4 w-4" />
        Back to Secrets
      </Button>
    </div>

    <LoadingSpinner v-if="loading" />

    <EmptyState
      v-else-if="providers.length === 0"
      title="No credential providers configured"
      message="Add a provider to store shared secrets in Alga or proxy them from an external vault."
    >
      <template v-if="canManage" #actions>
        <Button @click="openCreate">
          <Plus class="h-4 w-4" />
          Add Provider
        </Button>
      </template>
    </EmptyState>

    <div v-else class="space-y-4">
      <Card v-for="p in filteredProviders" :key="p.id" class="p-5 flex items-start gap-4">
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-3">
            <h3 class="font-semibold text-lg truncate">{{ p.name }}</h3>
            <span
              class="text-xs px-2 py-0.5 rounded-full font-medium"
              :class="
                p.enabled
                  ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                  : 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400'
              "
            >
              {{ p.enabled ? "Enabled" : "Disabled" }}
            </span>
            <span class="badge-muted">
              {{ p.provider_type_name ?? p.type }}
            </span>
            <span v-if="p.system" class="badge-blue"> system </span>
          </div>
          <div class="mt-1 text-xs text-[var(--text-muted)]">
            <span v-if="p.type === 'internal'">Secrets stored encrypted in Alga.</span>
            <span v-else>
              External provider
              <span v-if="p.config_configured" class="ml-1 text-green-600">config set</span>
              <span v-else class="ml-1 text-amber-500">config missing</span>
            </span>
          </div>
        </div>

        <div class="flex items-center gap-3 shrink-0">
          <Switch
            :model-value="p.enabled"
            :disabled="!canManage || p.system"
            @update:model-value="toggleEnabled(p)"
          />
          <Button
            v-if="canManage && !p.system"
            variant="outline"
            :class="HEADER_ICON_BTN_CLASS"
            @click="openEdit(p)"
          >
            <Pencil class="h-4 w-4" />
          </Button>
          <Button
            v-if="canManage && !p.system"
            variant="destructive"
            :class="HEADER_ICON_BTN_CLASS"
            @click="confirmDelete(p)"
          >
            <Trash2 class="h-4 w-4" />
          </Button>
        </div>
      </Card>
    </div>

    <Modal
      :open="formOpen"
      @update:open="formOpen = $event"
      :title="editing ? 'Edit Credential Provider' : 'Add Credential Provider'"
    >
      <div class="space-y-4">
        <ErrorBanner v-if="formError" :message="formError" />

        <div>
          <label class="block text-sm font-medium mb-1.5">Name</label>
          <Input v-model="form.name" placeholder="prod-vault" />
        </div>

        <div>
          <label class="block text-sm font-medium mb-1.5">Provider Type</label>
          <Select v-model="form.type" :disabled="!!editing">
            <option v-for="t in CREDENTIAL_PROVIDER_TYPES" :key="t.value" :value="t.value">
              {{ t.label }}
            </option>
          </Select>
          <p class="text-xs text-[var(--text-muted)] mt-1">
            <span v-if="form.type === 'internal'">
              Secrets are encrypted and stored inside Alga. Ready to use immediately.
            </span>
            <span v-else>
              Secrets are resolved through the external provider at read time. External fetch is a
              placeholder until a backend SDK is wired in.
            </span>
          </p>
        </div>

        <div v-if="typeMeta?.external">
          <label class="block text-sm font-medium mb-1.5">
            Connection Config{{ editing ? " (leave blank to keep)" : "" }}
          </label>
          <Textarea
            v-model="form.configJson"
            :placeholder="configHint"
            class="font-mono text-xs"
            rows="4"
          />
          <p class="text-xs text-[var(--text-muted)] mt-1">
            JSON object of key/value pairs. Example:
          </p>
          <pre class="text-xs text-[var(--text-muted)] mt-1 bg-[var(--bg-secondary)] p-2 rounded">{{
            configHint
          }}</pre>
        </div>

        <div class="flex items-center gap-3">
          <Switch
            :model-value="form.enabled"
            :disabled="!!editing && editing.system"
            @update:model-value="form.enabled = $event"
          />
          <span class="text-sm">Enabled</span>
          <span v-if="editing?.system" class="text-xs text-[var(--text-muted)]">
            System providers cannot be disabled or deleted.
          </span>
        </div>
      </div>

      <template #footer>
        <Button variant="outline" :disabled="saving" @click="formOpen = false">Cancel</Button>
        <Button variant="primary" :loading="saving" @click="save">
          {{ editing ? "Save Changes" : "Create Provider" }}
        </Button>
      </template>
    </Modal>

    <ConfirmDialog
      :open="showDeleteConfirm"
      title="Delete Credential Provider"
      :message="`Are you sure you want to delete '${deleteTarget?.name}'? Shared secrets referencing this provider must be moved or removed first.`"
      confirm-label="Delete"
      destructive
      @confirm="doDelete"
      @cancel="showDeleteConfirm = false"
    />
  </div>
</template>
