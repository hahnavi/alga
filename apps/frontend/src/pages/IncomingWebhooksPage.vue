<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { computed, onMounted, ref } from "vue";
import { KeyRound, Plus, Trash2, Webhook } from "@lucide/vue";
import { api, type WebhookTokenRow } from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import Modal from "@/components/ui/Modal.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import SecretDisplay from "@/components/ui/SecretDisplay.vue";
import DateTimePicker from "@/components/ui/DateTimePicker.vue";
import { useToast } from "@/lib/toast";
import { useDelete } from "@/composables/useDelete";
import { useClipboard } from "@/composables/useClipboard";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { useListFilter } from "@/composables/useListFilter";
import { formatExpires, localDatetimeToRFC3339 } from "@/lib/time";

defineOptions({ name: "IncomingWebhooksPage" });

const { push } = useToast();
const { copyToClipboard } = useClipboard();

const webhookTokens = ref<WebhookTokenRow[]>([]);
const webhookTokensLoading = ref(false);
const webhookSubmitting = ref(false);
const webhookError = ref("");
const webhookNewName = ref("");
const webhookNewExpiresLocal = ref("");
const webhookCreatedSecret = ref<string | null>(null);
const webhookTouched = ref({ name: false });
const showWebhookDialog = ref(false);

const webhookNameError = computed(() => {
  if (!webhookTouched.value.name) return "";
  if (!webhookNewName.value.trim()) return "Token name is required";
  return "";
});

const webhookFormValid = computed(() => webhookNewName.value.trim());

const searchInput = ref("");

usePageHeaderActions({
  title: "Incoming Webhooks",
  titleIcon: Webhook,
  searchInput,
  onAdd: openWebhookDialog,
  addLabel: "Create token",
});

const filteredWebhookTokens = useListFilter(webhookTokens, ["name"], searchInput);

async function loadWebhookTokens() {
  webhookTokensLoading.value = true;
  try {
    webhookTokens.value = await api.getWebhookTokens();
  } catch (err) {
    push(getErrorMessage(err, "Failed to load webhook tokens"), "error");
  } finally {
    webhookTokensLoading.value = false;
  }
}

function openWebhookDialog() {
  resetWebhookForm();
  showWebhookDialog.value = true;
}

function closeWebhookDialog() {
  showWebhookDialog.value = false;
  resetWebhookForm();
}

function resetWebhookForm() {
  webhookError.value = "";
  webhookCreatedSecret.value = null;
  webhookNewName.value = "";
  webhookNewExpiresLocal.value = "";
  webhookTouched.value = { name: false };
}

async function submitWebhookToken() {
  webhookTouched.value = { name: true };
  if (!webhookFormValid.value) return;

  let expiresAt: string | undefined;
  if (webhookNewExpiresLocal.value.trim()) {
    const iso = localDatetimeToRFC3339(webhookNewExpiresLocal.value);
    if (!iso) {
      push("Invalid expiration date", "error");
      return;
    }
    expiresAt = iso;
  }

  webhookSubmitting.value = true;
  webhookError.value = "";
  try {
    const res = await api.createWebhookToken(webhookNewName.value.trim(), expiresAt);
    webhookCreatedSecret.value = res.token;
    push("Webhook token created", "success");
    await loadWebhookTokens();
  } catch (err) {
    webhookError.value = getErrorMessage(err, "Failed to create webhook token");
  } finally {
    webhookSubmitting.value = false;
  }
}

async function copyWebhookSecret() {
  if (!webhookCreatedSecret.value) return;
  await copyToClipboard(webhookCreatedSecret.value, "Secret copied");
}

const {
  deleteTarget: webhookTokenToDelete,
  showDeleteConfirm: showWebhookDeleteConfirm,
  confirmDelete: confirmDeleteWebhookToken,
  doDelete: doDeleteWebhookToken,
} = useDelete<WebhookTokenRow>(async (token) => {
  await api.revokeWebhookToken(token.id);
  await loadWebhookTokens();
}, "Webhook token");

onMounted(() => {
  void loadWebhookTokens();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <p class="text-sm text-[var(--text-muted)]">
      Authenticate incoming Grafana alert webhooks at
      <code class="rounded bg-[var(--bg-code)] px-1.5 py-0.5 text-xs">/webhooks/alerts</code>.
    </p>
    <LoadingSpinner v-if="webhookTokensLoading" centered />
    <div v-else-if="webhookTokens.length" class="space-y-2">
      <div
        v-for="t in filteredWebhookTokens"
        :key="t.id"
        class="group rounded-lg border border-[var(--border-primary)] bg-[var(--bg-card)] px-4 py-3 transition-colors hover:border-[var(--border-secondary)]"
      >
        <div class="flex items-center gap-3">
          <div
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-[var(--bg-badge-muted)]"
          >
            <KeyRound class="h-4 w-4 text-[var(--text-badge-muted)]" />
          </div>
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <span class="text-sm font-medium text-[var(--text-primary)]">{{ t.name }}</span>
              <span v-if="t.expired" class="badge-red">Expired</span>
            </div>
            <div class="text-xs text-[var(--text-muted)]">
              <span>Expires: {{ formatExpires(t) }}</span>
            </div>
          </div>
          <Button
            size="sm"
            variant="destructive"
            class="shrink-0 md:opacity-0 md:transition-opacity md:duration-150 md:group-hover:opacity-100"
            @click="confirmDeleteWebhookToken(t)"
          >
            <Trash2 class="h-3.5 w-3.5" />
            Revoke
          </Button>
        </div>
      </div>
    </div>
    <p v-else class="text-sm text-[var(--text-muted)]">No tokens yet.</p>

    <Modal
      v-model:open="showWebhookDialog"
      :show-footer="false"
      title="Create Webhook Token"
      max-width="lg"
      @close="closeWebhookDialog"
    >
      <ErrorBanner v-if="webhookError" :message="webhookError" class="mb-3" />
      <template v-if="!webhookCreatedSecret">
        <div class="space-y-4">
          <div>
            <label for="webhook-token-name-input" class="mb-1.5 block text-[var(--text-secondary)]">
              Name
              <span class="text-[var(--text-error)]" aria-hidden="true">*</span>
            </label>
            <Input
              id="webhook-token-name-input"
              v-model="webhookNewName"
              placeholder="e.g. grafana-prod"
              :error="webhookNameError"
              @blur="webhookTouched.name = true"
            />
          </div>
          <div>
            <label class="mb-1.5 block text-[var(--text-secondary)]"> Expiration (optional) </label>
            <DateTimePicker
              id="webhook-expiry"
              v-model="webhookNewExpiresLocal"
              placeholder="Pick expiry date & time"
            />
          </div>
        </div>
        <div class="mt-5 flex justify-end gap-2">
          <Button variant="outline" @click="closeWebhookDialog">Cancel</Button>
          <Button
            :loading="webhookSubmitting"
            :disabled="!webhookFormValid"
            @click="submitWebhookToken"
          >
            <Plus class="h-3.5 w-3.5" />
            Create
          </Button>
        </div>
      </template>
      <template v-else>
        <p class="mb-3 text-sm text-[var(--text-secondary)]">
          Save this token now. It won't be shown again. Use in Grafana webhook requests with
          <code class="rounded bg-[var(--bg-code)] px-1.5 py-0.5 text-xs">?token=</code>
          or
          <code class="rounded bg-[var(--bg-code)] px-1.5 py-0.5 text-xs"
            >Authorization: Bearer</code
          >.
        </p>
        <SecretDisplay :secret="webhookCreatedSecret" @copy="copyWebhookSecret" />
        <div class="mt-5 flex justify-end">
          <Button id="webhook-token-done" @click="closeWebhookDialog">Done</Button>
        </div>
      </template>
    </Modal>

    <ConfirmDialog
      v-model:open="showWebhookDeleteConfirm"
      title="Delete webhook token"
      :message="`Are you sure you want to delete '${webhookTokenToDelete?.name}'? This action cannot be undone.`"
      confirm-label="Delete"
      :destructive="true"
      @confirm="doDeleteWebhookToken"
    />
  </section>
</template>
