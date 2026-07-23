<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { CheckCircle, ExternalLink, Link2, Lock, RefreshCw } from "@lucide/vue";
import { getErrorMessage } from "@/lib/error";
import type { IntegrationInfo } from "@/lib/api";
import { api } from "@/lib/api";
import { useToast } from "@/lib/toast";
import { useOAuthPopup } from "@/composables/useOAuthPopup";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import Switch from "@/components/ui/Switch.vue";
import FormLabel from "@/components/ui/FormLabel.vue";

defineOptions({ name: "SlackDialogBody" });

type SlackConnectionMode = "oauth" | "token";

export type SlackSubmitPayload = {
  bot_token: string;
  signing_secret: string;
  default_channel: string;
  provider_enabled: boolean;
  client_id: string;
  client_secret: string;
};

const props = defineProps<{
  integrations: IntegrationInfo;
  submitting: boolean;
  /** Optional flag the parent flips while executing a `disconnect` request. */
  disconnecting?: boolean;
}>();

const emit = defineEmits<{
  submit: [payload: SlackSubmitPayload];
  saveAndOAuth: [payload: SlackSubmitPayload];
  disconnect: [];
  oauthClosed: [];
  close: [];
}>();

const { push } = useToast();
const oauth = useOAuthPopup();

const slack = computed(() => props.integrations.slack);

const token = ref("");
const signingSecret = ref("");
const defaultChannel = ref("");
const providerEnabled = ref(true);
const clientId = ref("");
const clientSecret = ref("");
const reconnecting = ref(false);

const channelSuggestions = ref<string[]>([]);
const channelsLoading = ref(false);
const testing = ref(false);
const wantsOAuth = ref<SlackConnectionMode>("oauth");

watch(
  () => props.integrations.slack,
  (next) => {
    token.value = "";
    signingSecret.value = "";
    defaultChannel.value = next.default_channel ?? "";
    providerEnabled.value = next.provider_enabled !== false;
    clientId.value = next.client_id_configured ? "(configured)" : "";
    clientSecret.value = "";
  },
  { immediate: true },
);

async function loadChannelSuggestions() {
  if (channelsLoading.value) return;
  channelsLoading.value = true;
  try {
    const data = await api.getDestinations("slack");
    channelSuggestions.value = Array.isArray(data) ? data.map((c) => c.name || c.id || "") : [];
  } catch {
    // intentional — channels are optional UX
  } finally {
    channelsLoading.value = false;
  }
}

watch(
  () => props.integrations,
  () => {
    if (channelSuggestions.value.length === 0) void loadChannelSuggestions();
  },
  { immediate: true },
);

const hasOAuthCredentials = computed(() => {
  const id = clientId.value.trim();
  const secret = clientSecret.value.trim();
  return (id && id !== "(configured)") || secret;
});

const canTestSlack = computed(() => {
  return token.value.trim() !== "" || slack.value.token_configured === true;
});

function buildPayload(): SlackSubmitPayload {
  return {
    bot_token: token.value,
    signing_secret: signingSecret.value,
    default_channel: defaultChannel.value.trim(),
    provider_enabled: providerEnabled.value,
    client_id: clientId.value && clientId.value !== "(configured)" ? clientId.value : "",
    client_secret: clientSecret.value,
  };
}

function validateBeforeSubmit(): boolean {
  if (!providerEnabled.value) return true;
  const hasToken = token.value.trim() || slack.value.token_configured;
  if (hasToken && !defaultChannel.value.trim()) {
    push("Default alert channel is required", "error");
    return false;
  }
  return true;
}

function submit() {
  if (!validateBeforeSubmit()) return;
  emit("submit", buildPayload());
}

async function reconnect() {
  reconnecting.value = true;
  try {
    const { url } = await api.initiateSlackOAuth();
    oauth.open(
      url,
      () => {
        reconnecting.value = false;
        emit("oauthClosed");
      },
      "slack-oauth",
    );
  } catch (err) {
    reconnecting.value = false;
    push(getErrorMessage(err, "Failed to initiate Slack OAuth"), "error");
  }
}

function addToSlack() {
  if (!hasOAuthCredentials.value) {
    push("Enter Client ID and Client Secret first", "error");
    return;
  }
  if (!validateBeforeSubmit()) return;
  emit("saveAndOAuth", buildPayload());
}

async function test() {
  testing.value = true;
  try {
    await api.testIntegration("slack", { slack: { bot_token: token.value } });
    push("Slack connection successful", "success");
  } catch (err) {
    push(getErrorMessage(err, "Failed to test Slack"), "error");
  } finally {
    testing.value = false;
  }
}

function confirmDisconnect() {
  emit("disconnect");
}
</script>

<template>
  <form class="space-y-4" @submit.prevent="submit">
    <div v-if="slack.token_configured" class="space-y-4">
      <div
        v-if="slack.workspace_name"
        class="flex items-center gap-2 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2.5"
      >
        <Link2 class="h-4 w-4 text-[var(--text-muted)]" />
        <div>
          <p class="text-sm font-medium text-[var(--text-primary)]">
            {{ slack.workspace_name }}
          </p>
          <p v-if="slack.workspace_id" class="text-xs text-[var(--text-muted)]">
            Workspace ID: {{ slack.workspace_id }}
          </p>
        </div>
      </div>

      <div class="flex gap-2">
        <Button type="button" :loading="reconnecting" @click="reconnect">
          <RefreshCw class="h-3.5 w-3.5" />
          Reconnect
        </Button>
        <Button
          type="button"
          variant="destructive"
          :loading="props.disconnecting"
          @click="confirmDisconnect"
        >
          Disconnect
        </Button>
      </div>

      <div>
        <FormLabel
          for="integration-slack-signing-secret"
          hint="(optional)"
          :disabled="slack.locked"
        >
          Signing Secret
        </FormLabel>
        <Input
          id="integration-slack-signing-secret"
          v-model="signingSecret"
          type="password"
          :disabled="slack.locked"
          placeholder="Leave blank to keep current value"
        />
      </div>

      <div>
        <FormLabel for="integration-slack-default-channel" required>
          Default Alert Channel
        </FormLabel>
        <Input
          id="integration-slack-default-channel"
          v-model="defaultChannel"
          placeholder="e.g. C0123456789 or #alerts"
          list="slack-channel-suggestions"
        />
        <datalist id="slack-channel-suggestions">
          <option v-for="ch in channelSuggestions" :key="ch" :value="ch" />
        </datalist>
      </div>
    </div>

    <div v-else class="space-y-4">
      <p class="text-sm text-[var(--text-muted)]">
        Create a Slack app at
        <a
          href="https://api.slack.com/apps"
          target="_blank"
          rel="noopener noreferrer"
          class="text-[var(--text-primary)] underline"
          >api.slack.com/apps</a
        >, then choose a connection method below.
      </p>

      <div
        class="flex gap-1 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-1"
      >
        <button
          type="button"
          class="flex-1 rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
          :class="
            wantsOAuth === 'oauth'
              ? 'bg-[var(--bg-card)] text-[var(--text-primary)] shadow-sm'
              : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'
          "
          @click="wantsOAuth = 'oauth'"
        >
          OAuth App
        </button>
        <button
          type="button"
          class="flex-1 rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
          :class="
            wantsOAuth === 'token'
              ? 'bg-[var(--bg-card)] text-[var(--text-primary)] shadow-sm'
              : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'
          "
          @click="wantsOAuth = 'token'"
        >
          Bot Token
        </button>
      </div>

      <template v-if="wantsOAuth === 'oauth'">
        <div>
          <FormLabel for="integration-slack-client-id" :disabled="slack.locked">
            Client ID
            <Lock
              v-if="slack.locked"
              class="ml-1 inline h-3 w-3 align-middle text-[var(--text-muted)]"
            />
          </FormLabel>
          <Input
            id="integration-slack-client-id"
            v-model="clientId"
            :disabled="slack.locked"
            placeholder="e.g. 1234567890.1234567890"
          />
        </div>
        <div>
          <FormLabel for="integration-slack-client-secret" :disabled="slack.locked">
            Client Secret
            <Lock
              v-if="slack.locked"
              class="ml-1 inline h-3 w-3 align-middle text-[var(--text-muted)]"
            />
          </FormLabel>
          <Input
            id="integration-slack-client-secret"
            v-model="clientSecret"
            type="password"
            :disabled="slack.locked"
            placeholder="e.g. abc123def456…"
          />
        </div>

        <Button
          type="button"
          class="w-full"
          :disabled="!hasOAuthCredentials || slack.locked"
          :title="!hasOAuthCredentials ? 'Enter Client ID and Client Secret first' : undefined"
          @click="addToSlack"
        >
          <ExternalLink class="h-3.5 w-3.5" />
          Add to Slack
        </Button>
      </template>

      <template v-else>
        <div>
          <FormLabel for="integration-slack-token" :disabled="slack.locked"> Bot Token </FormLabel>
          <Input
            id="integration-slack-token"
            v-model="token"
            type="password"
            :disabled="slack.locked"
            placeholder="xoxb-…"
          />
        </div>
      </template>

      <div>
        <FormLabel
          for="integration-slack-signing-secret-new"
          hint="(optional)"
          :disabled="slack.locked"
        >
          Signing Secret
        </FormLabel>
        <Input
          id="integration-slack-signing-secret-new"
          v-model="signingSecret"
          type="password"
          :disabled="slack.locked"
          placeholder="Optional: enables thread sync & action buttons"
        />
      </div>

      <div>
        <FormLabel for="integration-slack-default-channel-new" required>
          Default Alert Channel
        </FormLabel>
        <Input
          id="integration-slack-default-channel-new"
          v-model="defaultChannel"
          placeholder="e.g. C0123456789 or #alerts"
          list="slack-channel-suggestions"
        />
      </div>
    </div>

    <div class="flex items-center justify-between border-t border-[var(--border-primary)] pt-4">
      <div class="flex items-center gap-3">
        <Switch
          id="integration-slack-provider-enabled"
          v-model="providerEnabled"
          :disabled="slack.locked"
        />
        <label
          for="integration-slack-provider-enabled"
          class="text-sm font-medium select-none cursor-pointer"
          :class="slack.locked ? 'text-[var(--text-muted)]' : 'text-[var(--text-primary)]'"
        >
          {{ providerEnabled ? "Enabled" : "Disabled" }}
        </label>
      </div>
      <div class="flex gap-2">
        <Button type="button" variant="outline" @click="emit('close')">Cancel</Button>
        <Button v-if="canTestSlack" type="button" :disabled="testing || submitting" @click="test">
          <CheckCircle v-if="!testing" class="h-3.5 w-3.5" />
          <RefreshCw v-else class="h-3.5 w-3.5 animate-spin" />
          {{ testing ? "Testing…" : "Test Connection" }}
        </Button>
        <Button type="submit" :loading="submitting">Save</Button>
      </div>
    </div>
  </form>
</template>
