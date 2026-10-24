<script setup lang="ts">
import { onMounted, ref } from "vue";
import { api, type SAMLProviderRecord } from "@/lib/api";
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
import Textarea from "@/components/ui/Textarea.vue";
import { useClipboard } from "@/composables/useClipboard";
import { useDelete } from "@/composables/useDelete";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { getErrorMessage } from "@/lib/error";
import { useToast } from "@/lib/toast";
import { KeyRound, Pencil, Plus, Trash2 } from "@lucide/vue";

defineOptions({ name: "SAMLProvidersSection" });

const providers = ref<SAMLProviderRecord[]>([]);
const loading = ref(true);
const error = ref("");
const formOpen = ref(false);
const editing = ref<SAMLProviderRecord | null>(null);
const form = ref({ name: "", email_attribute: "email", metadata_xml: "", enabled: false });
const { submitting: saving, formError, withSubmit } = useFormSubmit();
const { push } = useToast();
const { copyToClipboard } = useClipboard();

const { deleteTarget, showDeleteConfirm, confirmDelete, doDelete } = useDelete<SAMLProviderRecord>(
  async (provider) => {
    await api.deleteSAMLProvider(provider.id);
    providers.value = providers.value.filter((item) => item.id !== provider.id);
  },
  "SAML provider",
);

function openCreate() {
  editing.value = null;
  form.value = { name: "", email_attribute: "email", metadata_xml: "", enabled: false };
  formError.value = "";
  formOpen.value = true;
}

function openEdit(provider: SAMLProviderRecord) {
  editing.value = provider;
  form.value = {
    name: provider.name,
    email_attribute: provider.email_attribute,
    metadata_xml: "",
    enabled: provider.enabled,
  };
  formError.value = "";
  formOpen.value = true;
}

async function save() {
  if (!form.value.name.trim()) {
    formError.value = "Provider name is required.";
    return;
  }
  await withSubmit(async () => {
    const data = {
      name: form.value.name.trim(),
      email_attribute: form.value.email_attribute.trim(),
      enabled: form.value.enabled,
      ...(form.value.metadata_xml.trim() ? { metadata_xml: form.value.metadata_xml.trim() } : {}),
    };
    const updated = editing.value
      ? await api.updateSAMLProvider(editing.value.id, data)
      : await api.createSAMLProvider(data);
    providers.value = editing.value
      ? providers.value.map((provider) => (provider.id === updated.id ? updated : provider))
      : [...providers.value, updated];
    formOpen.value = false;
  });
}

async function toggleEnabled(provider: SAMLProviderRecord) {
  try {
    const updated = await api.updateSAMLProvider(provider.id, { enabled: !provider.enabled });
    providers.value = providers.value.map((item) => (item.id === updated.id ? updated : item));
  } catch (cause) {
    push(getErrorMessage(cause, "Failed to change provider status"), "error");
  }
}

async function loadProviders() {
  loading.value = true;
  error.value = "";
  try {
    providers.value = await api.listSAMLProviders();
  } catch (cause) {
    error.value = getErrorMessage(cause, "Failed to load SAML providers");
  } finally {
    loading.value = false;
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
          <h2 class="text-base font-semibold text-[var(--text-primary)]">SAML providers</h2>
          <p class="text-xs text-[var(--text-muted)]">
            Sign in through an identity provider that supports SAML 2.0.
          </p>
        </div>
      </div>
      <Button @click="openCreate">
        <Plus class="h-4 w-4" />
        Add SAML provider
      </Button>
    </header>

    <ErrorBanner v-if="error" :message="error" @retry="loadProviders" />
    <LoadingSpinner v-if="loading" />
    <EmptyState
      v-else-if="providers.length === 0"
      title="No SAML providers configured"
      message="Add an identity provider to let your team sign in with SAML."
    >
      <template #actions>
        <Button @click="openCreate">
          <Plus class="h-4 w-4" />
          Add SAML provider
        </Button>
      </template>
    </EmptyState>

    <div v-else class="space-y-3">
      <Card v-for="provider in providers" :key="provider.id" class="space-y-4 p-5">
        <div class="flex flex-wrap items-start gap-4">
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-3">
              <h3 class="truncate text-base font-semibold">{{ provider.name }}</h3>
              <span v-if="provider.enabled" class="badge-green">Enabled</span>
              <span v-else-if="!provider.metadata_configured" class="badge-muted"
                >Draft · metadata required</span
              >
              <span v-else class="badge-muted">Disabled</span>
            </div>
            <p
              v-if="provider.idp_entity_id"
              class="mt-1 break-all font-mono text-xs text-[var(--text-muted)]"
            >
              {{ provider.idp_entity_id }}
            </p>
            <p class="mt-2 text-xs text-[var(--text-muted)]">
              Email attribute: <code>{{ provider.email_attribute }}</code>
            </p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <Switch
              :model-value="provider.enabled"
              :disabled="!provider.configuration_valid && !provider.enabled"
              @update:model-value="toggleEnabled(provider)"
            />
            <Button
              variant="outline"
              size="sm"
              @click="openEdit(provider)"
              aria-label="Edit SAML provider"
            >
              <Pencil class="h-4 w-4" />
            </Button>
            <Button
              variant="destructive"
              size="sm"
              @click="confirmDelete(provider)"
              aria-label="Delete SAML provider"
            >
              <Trash2 class="h-4 w-4" />
            </Button>
          </div>
        </div>

        <div
          v-if="provider.acs_url"
          class="grid gap-3 border-t border-[var(--border-primary)] pt-3 sm:grid-cols-2"
        >
          <div class="min-w-0 sm:col-span-2">
            <p class="text-xs font-medium text-[var(--text-secondary)]">SP-initiated sign-in URL</p>
            <p class="mt-1 break-all font-mono text-xs text-[var(--text-muted)]">
              {{ provider.authorize_url }}
            </p>
            <Button
              variant="outline"
              size="sm"
              class="mt-2"
              @click="copyToClipboard(provider.authorize_url, 'Sign-in URL copied')"
            >
              Copy sign-in URL
            </Button>
          </div>
          <div class="min-w-0">
            <p class="text-xs font-medium text-[var(--text-secondary)]">Alga ACS URL</p>
            <p class="mt-1 break-all font-mono text-xs text-[var(--text-muted)]">
              {{ provider.acs_url }}
            </p>
            <Button
              variant="outline"
              size="sm"
              class="mt-2"
              @click="copyToClipboard(provider.acs_url, 'ACS URL copied')"
            >
              Copy ACS URL
            </Button>
          </div>
          <div class="min-w-0">
            <p class="text-xs font-medium text-[var(--text-secondary)]">
              SAML audience / SP entity ID
            </p>
            <p class="mt-1 break-all font-mono text-xs text-[var(--text-muted)]">
              {{ provider.entity_id }}
            </p>
            <div class="mt-2 flex flex-wrap gap-2">
              <Button
                variant="outline"
                size="sm"
                @click="copyToClipboard(provider.entity_id, 'Audience copied')"
              >
                Copy audience
              </Button>
              <a
                :href="provider.metadata_url"
                target="_blank"
                rel="noopener noreferrer"
                class="inline-flex items-center rounded-md border border-[var(--border-primary)] px-3 py-1.5 text-xs font-medium hover:bg-[var(--bg-secondary)]"
              >
                Download SP metadata
              </a>
            </div>
          </div>
        </div>
        <p v-else class="rounded-md bg-[var(--bg-secondary)] p-3 text-xs text-[var(--text-muted)]">
          SAML endpoints are unavailable. Configure <code>ALGA_PUBLIC_URL</code> as Alga’s public
          HTTPS URL and verify the identity provider metadata.
        </p>
        <p
          v-if="!provider.configuration_valid && !provider.metadata_configured && provider.acs_url"
          class="rounded-md bg-[var(--bg-secondary)] p-3 text-xs text-[var(--text-muted)]"
        >
          This provider is a draft. Add the identity provider metadata before enabling sign-in.
        </p>
        <p
          v-else-if="provider.metadata_configured && !provider.configuration_valid"
          class="rounded-md bg-[var(--bg-secondary)] p-3 text-xs text-[var(--text-muted)]"
        >
          The identity provider metadata is invalid or expired. Replace it before enabling sign-in.
        </p>
      </Card>
    </div>

    <Card class="space-y-2 p-4 text-sm text-[var(--text-secondary)]">
      <h3 class="font-semibold text-[var(--text-primary)]">SAML setup</h3>
      <ol class="list-inside list-decimal space-y-1 text-xs">
        <li>
          Create a disabled SAML provider in Alga to get its ACS URL, audience, and sign-in URL.
        </li>
        <li>
          Create a SAML 2.0 application in your identity provider and download its metadata XML.
        </li>
        <li>
          Configure the Alga ACS URL and audience in your identity provider. If it supports
          SP-initiated sign-in, set its start URL to the URL shown above.
        </li>
        <li>
          Paste the identity provider metadata into Alga, map the subject to a stable identifier and
          an attribute named <code>email</code> (or the configured email attribute), then enable the
          provider.
        </li>
        <li>Assign the intended users or groups in your identity provider and test sign-in.</li>
      </ol>
      <p class="text-xs text-[var(--text-muted)]">
        Alga links sign-ins to existing accounts with the same email. New account provisioning is
        not enabled. Alga uses SP-initiated requests and requires a signed response or assertion.
      </p>
    </Card>

    <Modal
      :open="formOpen"
      @update:open="formOpen = $event"
      :title="editing ? 'Edit SAML provider' : 'Add SAML provider'"
    >
      <div class="space-y-4">
        <ErrorBanner v-if="formError" :message="formError" />
        <div>
          <FormLabel for="saml-provider-name" required>Display name</FormLabel>
          <Input
            id="saml-provider-name"
            v-model="form.name"
            placeholder="Company identity provider"
          />
        </div>
        <div>
          <FormLabel for="saml-email-attribute" required>Email attribute name</FormLabel>
          <Input id="saml-email-attribute" v-model="form.email_attribute" placeholder="email" />
          <p class="mt-1 text-xs text-[var(--text-muted)]">
            Must match the SAML attribute name configured in the identity provider.
          </p>
        </div>
        <div>
          <FormLabel for="saml-metadata">Identity provider metadata XML</FormLabel>
          <Textarea
            id="saml-metadata"
            v-model="form.metadata_xml"
            :rows="8"
            spellcheck="false"
            autocomplete="off"
            placeholder="Paste the SAML metadata XML from your identity provider after creating the provider draft."
          />
          <p class="mt-1 text-xs text-[var(--text-muted)]">
            {{
              editing
                ? "Leave blank to keep the existing metadata. Paste refreshed metadata here to rotate signing certificates."
                : "Create the provider first to get Alga’s ACS URL, audience, and application start URL. Add metadata before enabling sign-in."
            }}
          </p>
        </div>
        <label
          class="flex items-center justify-between gap-3 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2.5"
        >
          <span class="text-sm font-medium text-[var(--text-primary)]">Enable provider</span>
          <Switch v-model="form.enabled" />
        </label>
      </div>
      <template #footer>
        <Button variant="outline" :disabled="saving" @click="formOpen = false">Cancel</Button>
        <Button :loading="saving" @click="save">{{
          editing ? "Save changes" : "Create provider"
        }}</Button>
      </template>
    </Modal>

    <ConfirmDialog
      :open="showDeleteConfirm"
      title="Delete SAML provider"
      :message="`Delete '${deleteTarget?.name}'? Linked SAML identities will be removed; Alga user accounts remain.`"
      confirm-label="Delete"
      destructive
      @confirm="doDelete"
      @cancel="showDeleteConfirm = false"
    />
  </section>
</template>
