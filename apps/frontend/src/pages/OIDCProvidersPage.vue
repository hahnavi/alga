<script setup lang="ts">
import { ref, onMounted } from "vue";
import { api, type OIDCProviderRecord } from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import Card from "@/components/ui/Card.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import Modal from "@/components/ui/Modal.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Switch from "@/components/ui/Switch.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import { useToast } from "@/lib/toast";
import { useDelete } from "@/composables/useDelete";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { HEADER_ICON_BTN_CLASS } from "@/lib/uiClasses";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { useListFilter } from "@/composables/useListFilter";
import { getErrorMessage } from "@/lib/error";
import { Plus, KeyRound, Trash2, Pencil, ExternalLink } from "@lucide/vue";

defineOptions({ name: "OIDCProvidersPage" });

const providers = ref<OIDCProviderRecord[]>([]);
const loading = ref(true);
const error = ref("");
const formOpen = ref(false);
const editing = ref<OIDCProviderRecord | null>(null);

const form = ref({
  name: "",
  issuer: "",
  client_id: "",
  client_secret: "",
  scopes: "openid email profile",
  enabled: true,
});
const { submitting: saving, formError, withSubmit } = useFormSubmit();
const { deleteTarget, showDeleteConfirm, confirmDelete, doDelete } = useDelete<OIDCProviderRecord>(
  async (item) => {
    await api.deleteOIDCProvider(item.id);
    providers.value = providers.value.filter((p) => p.id !== item.id);
  },
  "OIDC provider",
);

const { push } = useToast();

function openCreate() {
  editing.value = null;
  form.value = {
    name: "",
    issuer: "",
    client_id: "",
    client_secret: "",
    scopes: "openid email profile",
    enabled: true,
  };
  formError.value = "";
  formOpen.value = true;
}

function openEdit(p: OIDCProviderRecord) {
  editing.value = p;
  form.value = {
    name: p.name,
    issuer: p.issuer,
    client_id: p.client_id,
    client_secret: "",
    scopes: (p.scopes ?? ["openid", "email", "profile"]).join(" "),
    enabled: p.enabled,
  };
  formError.value = "";
  formOpen.value = true;
}

async function save() {
  if (!form.value.name || !form.value.issuer || !form.value.client_id) {
    formError.value = "Name, issuer, and client ID are required.";
    return;
  }
  if (!editing.value && !form.value.client_secret) {
    formError.value = "Client secret is required when creating a provider.";
    return;
  }

  await withSubmit(async () => {
    const scopes = form.value.scopes
      .split(/[\s,]+/)
      .map((s) => s.trim())
      .filter(Boolean);

    if (editing.value) {
      const data: Record<string, unknown> = {
        name: form.value.name,
        issuer: form.value.issuer,
        client_id: form.value.client_id,
        scopes,
        enabled: form.value.enabled,
      };
      if (form.value.client_secret) {
        data.client_secret = form.value.client_secret;
      }
      const updated = await api.updateOIDCProvider(editing.value.id, data);
      providers.value = providers.value.map((p) => (p.id === updated.id ? updated : p));
    } else {
      const created = await api.createOIDCProvider({
        name: form.value.name,
        issuer: form.value.issuer,
        client_id: form.value.client_id,
        client_secret: form.value.client_secret,
        scopes,
        enabled: form.value.enabled,
      });
      providers.value = [...providers.value, created];
    }
    formOpen.value = false;
  });
}

async function toggleEnabled(p: OIDCProviderRecord) {
  try {
    const updated = await api.updateOIDCProvider(p.id, { enabled: !p.enabled });
    providers.value = providers.value.map((x) => (x.id === updated.id ? updated : x));
  } catch (e) {
    push(getErrorMessage(e, "Failed to toggle provider"), "error");
  }
}

async function loadProviders() {
  loading.value = true;
  error.value = "";
  try {
    providers.value = await api.listOIDCProviders();
  } catch (e) {
    error.value = getErrorMessage(e, "Failed to load providers");
  } finally {
    loading.value = false;
  }
}

const searchInput = ref("");

usePageHeaderActions({
  title: "SSO Providers",
  titleIcon: KeyRound,
  searchInput,
  onAdd: openCreate,
  addLabel: "Add provider",
});

const filteredProviders = useListFilter(providers, ["name", "issuer"], searchInput);

onMounted(() => {
  loadProviders();
});
</script>

<template>
  <div class="p-6 max-w-4xl mx-auto space-y-6">
    <ErrorBanner v-if="error" :message="error" @retry="loadProviders" />

    <LoadingSpinner v-if="loading" />

    <EmptyState
      v-else-if="providers.length === 0"
      title="No SSO providers configured"
      message="Add an OIDC provider to enable single sign-on for your team."
    >
      <template #actions>
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
          </div>
          <div class="mt-1 flex items-center gap-1.5 text-sm text-[var(--text-muted)]">
            <span class="truncate">{{ p.issuer }}</span>
            <a
              :href="p.issuer + '/.well-known/openid-configuration'"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex shrink-0"
            >
              <ExternalLink class="h-3.5 w-3.5" />
            </a>
          </div>
          <div class="mt-2 flex flex-wrap gap-1.5">
            <span v-for="scope in p.scopes" :key="scope" class="badge-muted font-mono">
              {{ scope }}
            </span>
          </div>
          <div class="mt-1 text-xs text-[var(--text-muted)]">
            Client ID: <code class="font-mono">{{ p.client_id }}</code>
            <span v-if="p.client_secret_configured" class="ml-2 text-green-600">secret set</span>
            <span v-else class="ml-2 text-red-500">secret missing</span>
          </div>
        </div>

        <div class="flex items-center gap-3 shrink-0">
          <Switch :model-value="p.enabled" @update:model-value="toggleEnabled(p)" />
          <Button variant="outline" :class="HEADER_ICON_BTN_CLASS" @click="openEdit(p)">
            <Pencil class="h-4 w-4" />
          </Button>
          <Button variant="destructive" :class="HEADER_ICON_BTN_CLASS" @click="confirmDelete(p)">
            <Trash2 class="h-4 w-4" />
          </Button>
        </div>
      </Card>
    </div>

    <Modal
      :open="formOpen"
      @update:open="formOpen = $event"
      :title="editing ? 'Edit SSO Provider' : 'Add SSO Provider'"
    >
      <div class="space-y-4">
        <ErrorBanner v-if="formError" :message="formError" />

        <div>
          <label class="block text-sm font-medium mb-1.5">Display Name</label>
          <Input v-model="form.name" placeholder="My Company SSO" />
        </div>

        <div>
          <label class="block text-sm font-medium mb-1.5">Issuer URL</label>
          <Input v-model="form.issuer" placeholder="https://accounts.google.com" />
          <p class="text-xs text-[var(--text-muted)] mt-1">
            The OIDC issuer URL. Discovery happens automatically via
            /.well-known/openid-configuration
          </p>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="block text-sm font-medium mb-1.5">Client ID</label>
            <Input v-model="form.client_id" placeholder="your-client-id" />
          </div>
          <div>
            <label class="block text-sm font-medium mb-1.5">
              Client Secret{{ editing ? " (leave blank to keep)" : "" }}
            </label>
            <Input v-model="form.client_secret" type="password" placeholder="your-client-secret" />
          </div>
        </div>

        <div>
          <label class="block text-sm font-medium mb-1.5">Scopes</label>
          <Input v-model="form.scopes" placeholder="openid email profile" />
          <p class="text-xs text-[var(--text-muted)] mt-1">Space or comma separated</p>
        </div>

        <div class="flex items-center gap-3">
          <Switch :model-value="form.enabled" @update:model-value="form.enabled = $event" />
          <span class="text-sm">Enabled</span>
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
      title="Delete SSO Provider"
      :message="`Are you sure you want to delete '${deleteTarget?.name}'? Users linked via this provider will need to use password or another SSO method.`"
      confirm-label="Delete"
      destructive
      @confirm="doDelete"
      @cancel="showDeleteConfirm = false"
    />
  </div>
</template>
