<script setup lang="ts">
import { onMounted, ref } from "vue";
import { api, type LDAPProviderInput, type LDAPProviderRecord } from "@/lib/api";
import { getErrorMessage } from "@/lib/error";
import { useToast } from "@/lib/toast";
import { useDelete } from "@/composables/useDelete";
import { useFormSubmit } from "@/composables/useFormSubmit";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Input from "@/components/ui/Input.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import Modal from "@/components/ui/Modal.vue";
import Switch from "@/components/ui/Switch.vue";
import { KeyRound, Pencil, Plus, Trash2 } from "@lucide/vue";

const providers = ref<LDAPProviderRecord[]>([]);
const loading = ref(true);
const error = ref("");
const formOpen = ref(false);
const editing = ref<LDAPProviderRecord | null>(null);
const { submitting: saving, formError, withSubmit } = useFormSubmit();
const { push } = useToast();
const blank = (): LDAPProviderInput => ({
  name: "",
  url: "",
  base_dn: "",
  bind_dn: "",
  bind_password: "",
  login_attribute: "uid",
  id_attribute: "entryUUID",
  email_attribute: "mail",
  start_tls: false,
  enabled: false,
});
const form = ref<LDAPProviderInput>(blank());

const { deleteTarget, showDeleteConfirm, confirmDelete, doDelete } = useDelete<LDAPProviderRecord>(
  async (provider) => {
    await api.deleteLDAPProvider(provider.id);
    providers.value = providers.value.filter((item) => item.id !== provider.id);
  },
  "LDAP provider",
);

function openCreate() {
  editing.value = null;
  form.value = blank();
  formError.value = "";
  formOpen.value = true;
}

function openEdit(provider: LDAPProviderRecord) {
  editing.value = provider;
  form.value = {
    name: provider.name,
    url: provider.url,
    base_dn: provider.base_dn,
    bind_dn: provider.bind_dn,
    bind_password: "",
    login_attribute: provider.login_attribute,
    id_attribute: provider.id_attribute,
    email_attribute: provider.email_attribute,
    start_tls: provider.start_tls,
    enabled: provider.enabled,
  };
  formError.value = "";
  formOpen.value = true;
}

async function loadProviders() {
  loading.value = true;
  error.value = "";
  try {
    providers.value = await api.listLDAPProviders();
  } catch (cause) {
    error.value = getErrorMessage(cause, "Failed to load LDAP providers");
  } finally {
    loading.value = false;
  }
}

async function save() {
  await withSubmit(async () => {
    const data = { ...form.value };
    if (!data.bind_password) delete data.bind_password;
    const updated = editing.value
      ? await api.updateLDAPProvider(editing.value.id, data)
      : await api.createLDAPProvider(data);
    providers.value = editing.value
      ? providers.value.map((item) => (item.id === updated.id ? updated : item))
      : [...providers.value, updated];
    formOpen.value = false;
  });
}

async function toggleEnabled(provider: LDAPProviderRecord) {
  try {
    const data: LDAPProviderInput = {
      name: provider.name,
      url: provider.url,
      base_dn: provider.base_dn,
      bind_dn: provider.bind_dn,
      login_attribute: provider.login_attribute,
      id_attribute: provider.id_attribute,
      email_attribute: provider.email_attribute,
      start_tls: provider.start_tls,
      enabled: !provider.enabled,
    };
    const updated = await api.updateLDAPProvider(provider.id, data);
    providers.value = providers.value.map((item) => (item.id === updated.id ? updated : item));
  } catch (cause) {
    push(getErrorMessage(cause, "Failed to change LDAP provider status"), "error");
  }
}

onMounted(() => {
  void loadProviders();
});
</script>

<template>
  <section class="space-y-4">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex items-center gap-3">
        <span
          class="flex h-9 w-9 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)]"
        >
          <KeyRound class="h-4 w-4" />
        </span>
        <div>
          <h2 class="text-base font-semibold">LDAP directories</h2>
          <p class="text-xs text-[var(--text-muted)]">
            Let existing users sign in with directory credentials.
          </p>
        </div>
      </div>
      <Button @click="openCreate"><Plus class="h-4 w-4" /> Add LDAP provider</Button>
    </header>
    <ErrorBanner v-if="error" :message="error" @retry="loadProviders" />
    <LoadingSpinner v-if="loading" />
    <EmptyState
      v-else-if="providers.length === 0"
      title="No LDAP providers configured"
      message="Add a directory to enable LDAP sign-in for existing users."
    />
    <div v-else class="space-y-3">
      <Card v-for="provider in providers" :key="provider.id" class="flex items-start gap-4 p-5">
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <h3 class="truncate font-semibold">{{ provider.name }}</h3>
            <span :class="provider.enabled ? 'badge-green' : 'badge-muted'">{{
              provider.enabled ? "Enabled" : "Disabled"
            }}</span>
          </div>
          <p class="mt-1 truncate text-sm text-[var(--text-muted)]">
            {{ provider.url }} · {{ provider.base_dn }}
          </p>
          <p class="mt-1 text-xs text-[var(--text-muted)]">
            Login: {{ provider.login_attribute }} · ID: {{ provider.id_attribute }} · Email:
            {{ provider.email_attribute }}
          </p>
        </div>
        <Switch :model-value="provider.enabled" @update:model-value="toggleEnabled(provider)" />
        <Button variant="outline" aria-label="Edit LDAP provider" @click="openEdit(provider)"
          ><Pencil class="h-4 w-4"
        /></Button>
        <Button
          variant="destructive"
          aria-label="Delete LDAP provider"
          @click="confirmDelete(provider)"
          ><Trash2 class="h-4 w-4"
        /></Button>
      </Card>
    </div>

    <Modal
      :open="formOpen"
      :title="editing ? 'Edit LDAP provider' : 'Add LDAP provider'"
      @update:open="formOpen = $event"
    >
      <form class="space-y-4" @submit.prevent="save">
        <ErrorBanner v-if="formError" :message="formError" />
        <div>
          <FormLabel for="ldap-name">Display name</FormLabel
          ><Input id="ldap-name" v-model="form.name" required />
        </div>
        <div>
          <FormLabel for="ldap-url">Directory URL</FormLabel
          ><Input
            id="ldap-url"
            v-model="form.url"
            placeholder="ldaps://directory.example.com:636"
            required
          />
        </div>
        <p class="text-xs text-[var(--text-muted)]">
          Use LDAPS, or ldap:// with StartTLS. The directory certificate must be trusted by the Alga
          server.
        </p>
        <div class="flex items-center gap-3">
          <Switch v-model="form.start_tls" /><span class="text-sm">Use StartTLS</span>
        </div>
        <div>
          <FormLabel for="ldap-base">Search base DN</FormLabel
          ><Input
            id="ldap-base"
            v-model="form.base_dn"
            placeholder="ou=people,dc=example,dc=com"
            required
          />
        </div>
        <div>
          <FormLabel for="ldap-bind-dn">Service bind DN</FormLabel
          ><Input id="ldap-bind-dn" v-model="form.bind_dn" required />
        </div>
        <div>
          <FormLabel for="ldap-bind-password">Service bind password</FormLabel
          ><Input
            id="ldap-bind-password"
            v-model="form.bind_password"
            type="password"
            :placeholder="
              editing ? 'Leave blank to keep current secret' : 'Required before enabling'
            "
            autocomplete="new-password"
          />
        </div>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div>
            <FormLabel for="ldap-login-attr">Login attribute</FormLabel
            ><Input id="ldap-login-attr" v-model="form.login_attribute" required />
          </div>
          <div>
            <FormLabel for="ldap-id-attr">Stable ID attribute</FormLabel
            ><Input id="ldap-id-attr" v-model="form.id_attribute" required />
          </div>
          <div>
            <FormLabel for="ldap-email-attr">Email attribute</FormLabel
            ><Input id="ldap-email-attr" v-model="form.email_attribute" required />
          </div>
        </div>
        <p class="text-xs text-[var(--text-muted)]">
          For Active Directory, common attributes are sAMAccountName, objectGUID, and mail. The
          email must match an existing Alga user on first sign-in.
        </p>
        <div class="flex items-center gap-3">
          <Switch v-model="form.enabled" /><span class="text-sm">Enable sign-in</span>
        </div>
        <div class="flex justify-end gap-2">
          <Button type="button" variant="outline" @click="formOpen = false">Cancel</Button
          ><Button type="submit" :disabled="saving">{{
            saving ? "Saving…" : "Save provider"
          }}</Button>
        </div>
      </form>
    </Modal>
    <ConfirmDialog
      :open="showDeleteConfirm"
      :title="'Delete LDAP provider?'"
      :message="`Delete ${deleteTarget?.name ?? 'this provider'} and its identity links?`"
      confirm-label="Delete"
      destructive
      @confirm="doDelete"
      @cancel="showDeleteConfirm = false"
    />
  </section>
</template>
