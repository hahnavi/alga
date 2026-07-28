<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { Link2, MessageSquare, Phone, PhoneCall, Settings, Zap } from "@lucide/vue";
import { api, type IntegrationInfo } from "@/lib/api";
import { providerStatus } from "@/lib/integrations";
import Button from "@/components/ui/Button.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import Modal from "@/components/ui/Modal.vue";
import ProviderCard from "@/components/integrations/ProviderCard.vue";
import MattermostDialogBody from "@/components/integrations/MattermostDialogBody.vue";
import SlackDialogBody from "@/components/integrations/SlackDialogBody.vue";
import VoiceProviderDialogBody from "@/components/integrations/VoiceProviderDialogBody.vue";
import { useToast } from "@/lib/toast";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { queryString } from "@/lib/routing";

defineOptions({ name: "CommunicationChannelsPage" });

const { push } = useToast();
const route = useRoute();
const router = useRouter();

const integrations = ref<IntegrationInfo | null>(null);
const integrationsLoading = ref(false);
const { submitting, formError: error, withSubmit } = useFormSubmit();

const dialogProvider = ref<"mattermost" | "slack" | null>(null);
const showVoiceDialog = ref(false);

const isPopup = ref(
  typeof window !== "undefined" && window.opener !== null && window.opener !== window,
);

const { canWrite: canConfigureIntegrations } = useEntityPermissions("integrations");

usePageHeaderActions({
  title: "Communication Channels",
  titleIcon: MessageSquare,
  showAdd: false,
});

const statusForEachProvider = computed(() => {
  const info = integrations.value;
  return {
    mattermost: providerStatus("mattermost", info),
    slack: providerStatus("slack", info),
    twilio: providerStatus("twilio", info),
    telnyx: providerStatus("telnyx", info),
  } as const;
});

async function loadIntegrations() {
  integrationsLoading.value = true;
  error.value = "";
  try {
    integrations.value = await api.getIntegrations();
  } catch (err) {
    error.value = getErrorMessage(err, "Failed to load integrations");
  } finally {
    integrationsLoading.value = false;
  }
}

function openDialog(provider: "mattermost" | "slack") {
  dialogProvider.value = provider;
}

function closeDialog() {
  dialogProvider.value = null;
}

function closeAndRefresh() {
  dialogProvider.value = null;
  void loadIntegrations();
}

function openVoiceDialog() {
  showVoiceDialog.value = true;
}

function closeVoiceDialog() {
  showVoiceDialog.value = false;
}

const voiceStatus = computed(() => {
  const info = integrations.value;
  if (!info) {
    return {
      label: "Unknown",
      cls: "badge-muted" as const,
      accentClass: "bg-transparent",
    };
  }
  const active = info.voice_provider;
  const twilio = info.twilio;
  const telnyx = info.telnyx;
  const activeProvider = active === "twilio" ? twilio : telnyx;

  if (!activeProvider.enabled) {
    return {
      label: "Not configured",
      cls: "badge-muted" as const,
      accentClass: "bg-transparent",
    };
  }
  if (!activeProvider.provider_enabled) {
    return {
      label: "Paused",
      cls: "badge-yellow" as const,
      accentClass: "bg-transparent",
    };
  }
  return {
    label: "Active",
    cls: "badge-green" as const,
    accentClass: info.voice_provider === "twilio" ? "bg-red-500/40" : "bg-emerald-500/40",
  };
});

const activeVoiceLabel = computed(() => {
  if (!integrations.value) return "—";
  return integrations.value.voice_provider === "twilio" ? "Twilio" : "Telnyx";
});

const voiceFromNumber = computed(() => {
  if (!integrations.value) return "";
  const v = integrations.value.voice_provider;
  return v === "twilio"
    ? integrations.value.twilio.from_number
    : integrations.value.telnyx.from_number;
});

async function submitMattermost(payload: {
  url: string;
  secret: string;
  team: string;
  default_channel: string;
  provider_enabled: boolean;
}) {
  await withSubmit(async () => {
    await api.updateIntegrations({ mattermost: payload });
    closeAndRefresh();
  }, "Mattermost integration updated");
}

async function submitSlack(payload: {
  bot_token: string;
  signing_secret: string;
  default_channel: string;
  provider_enabled: boolean;
  client_id: string;
  client_secret: string;
}) {
  await withSubmit(async () => {
    await api.updateIntegrations({ slack: payload });
    closeAndRefresh();
  }, "Slack integration updated");
}

async function saveSlackAndStartOAuth(payload: {
  bot_token: string;
  signing_secret: string;
  default_channel: string;
  provider_enabled: boolean;
  client_id: string;
  client_secret: string;
}) {
  await withSubmit(async () => {
    await api.updateIntegrations({
      slack: {
        ...payload,
        bot_token: "",
      },
    });
    await loadIntegrations();
    const { url } = await api.initiateSlackOAuth();
    window.open(url, "slack-oauth", "width=600,height=700");
  }, "Slack OAuth flow started");
}

async function disconnectSlack() {
  if (!integrations.value) return;
  await withSubmit(async () => {
    await api.disconnectSlack();
    push("Slack workspace disconnected", "success");
    closeAndRefresh();
  }, "Slack workspace disconnected");
}

async function submitTwilio(
  provider: "twilio" | "telnyx",
  providerEnabled: boolean,
  payload: {
    account_sid: string;
    auth_token: string;
    from_number: string;
  },
) {
  await withSubmit(async () => {
    await api.updateIntegrations({
      voice_provider: provider,
      twilio: { ...payload, provider_enabled: providerEnabled },
    });
    closeVoiceDialog();
    void loadIntegrations();
  }, "Twilio integration updated");
}

async function submitTelnyx(
  provider: "twilio" | "telnyx",
  providerEnabled: boolean,
  payload: {
    api_key: string;
    connection_id: string;
    from_number: string;
    public_key: string;
    tts_voice: string;
    tts_language: string;
    tts_api_key_ref: string;
  },
) {
  await withSubmit(async () => {
    await api.updateIntegrations({
      voice_provider: provider,
      telnyx: { ...payload, provider_enabled: providerEnabled },
    });
    closeVoiceDialog();
    void loadIntegrations();
  }, "Telnyx integration updated");
}

onMounted(async () => {
  await loadIntegrations();

  const oauthStatus = queryString(route.query, "slack_oauth");
  if (oauthStatus === "success") {
    if (isPopup.value) {
      const div = document.createElement("div");
      div.style.cssText =
        "display:flex;align-items:center;justify-content:center;height:100vh;font-family:system-ui";
      const p = document.createElement("p");
      p.style.fontSize = "1.25rem";
      p.textContent = "Connected! You can close this window.";
      div.appendChild(p);
      document.body.replaceChildren(div);
      setTimeout(() => window.close(), 1500);
    } else {
      push("Slack workspace connected!", "success");
    }
    void router.replace("/communication-channels");
  } else if (oauthStatus === "error") {
    const SLACK_ERRORS: Record<string, string> = {
      access_denied: "Slack access was denied.",
      invalid_state: "Authentication session expired. Please try again.",
      oauth_error: "Slack OAuth error occurred.",
    };
    const message = SLACK_ERRORS[queryString(route.query, "message")] ?? "Unknown error";
    if (isPopup.value) {
      const div = document.createElement("div");
      div.style.cssText =
        "display:flex;align-items:center;justify-content:center;height:100vh;font-family:system-ui;color:#dc2626";
      const p = document.createElement("p");
      p.textContent = `Error: ${message}`;
      div.appendChild(p);
      document.body.replaceChildren(div);
      setTimeout(() => window.close(), 3000);
    } else {
      push(`Slack OAuth error: ${message}`, "error");
    }
    void router.replace("/communication-channels");
  }
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <ErrorBanner :message="error" />

    <p class="text-sm text-[var(--text-muted)]">
      Configure Mattermost, Slack, and voice-call delivery channels.
    </p>

    <LoadingSpinner v-if="integrationsLoading" centered />
    <div v-else-if="integrations" class="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
      <ProviderCard
        provider-id="slack"
        :status="statusForEachProvider.slack"
        :allow-configure="canConfigureIntegrations"
        @configure="openDialog('slack')"
      >
        <template #details>
          <div
            v-if="integrations.slack.enabled && integrations.slack.workspace_name"
            class="flex items-center gap-1.5 text-xs text-[var(--text-muted)]"
          >
            <Link2 class="h-3 w-3 shrink-0" />
            <span class="truncate">Connected to {{ integrations.slack.workspace_name }}</span>
          </div>
          <div
            v-if="integrations.slack.enabled && integrations.slack.default_channel"
            class="flex items-center gap-1.5 text-xs text-[var(--text-muted)]"
          >
            <Zap class="h-3 w-3 shrink-0" />
            <span class="truncate">Default: {{ integrations.slack.default_channel }}</span>
          </div>
        </template>
      </ProviderCard>

      <div
        class="group relative overflow-hidden rounded-lg border border-[var(--border-primary)] bg-[var(--bg-card)] transition-all duration-200 hover:border-[var(--border-secondary)]"
      >
        <div
          class="pointer-events-none absolute inset-x-0 top-0 h-px"
          :class="voiceStatus.cls === 'badge-green' ? voiceStatus.accentClass : 'bg-transparent'"
        />
        <div class="p-4">
          <div class="mb-3 flex items-start justify-between gap-3">
            <div class="flex min-w-0 items-center gap-3">
              <div
                class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--bg-badge-muted)]"
              >
                <Phone class="h-5 w-5 text-[var(--text-badge-muted)]" />
              </div>
              <div class="min-w-0">
                <h4 class="truncate font-medium text-[var(--text-primary)]">Voice & SMS</h4>
                <span class="mt-0.5 inline-block" :class="['badge', voiceStatus.cls]">
                  {{ voiceStatus.label }}
                </span>
              </div>
            </div>
            <Button
              size="sm"
              :disabled="!canConfigureIntegrations"
              :title="canConfigureIntegrations ? 'Configure voice provider' : 'Configure'"
              :aria-label="`Configure Voice & SMS (${voiceStatus.label})`"
              @click="openVoiceDialog"
            >
              <Settings class="h-4 w-4" />
            </Button>
          </div>
          <div class="space-y-1.5">
            <div class="flex items-center gap-1.5 text-xs text-[var(--text-muted)]">
              <PhoneCall class="h-3 w-3 shrink-0" />
              <span class="truncate">Active: {{ activeVoiceLabel }}</span>
            </div>
            <div
              v-if="voiceFromNumber"
              class="flex items-center gap-1.5 text-xs text-[var(--text-muted)]"
            >
              <Phone class="h-3 w-3 shrink-0" />
              <span class="truncate">From: {{ voiceFromNumber }}</span>
            </div>
          </div>
        </div>
      </div>

      <ProviderCard
        provider-id="mattermost"
        :status="statusForEachProvider.mattermost"
        :allow-configure="false"
      />
    </div>

    <Modal
      :open="dialogProvider !== null"
      :show-footer="false"
      :title="dialogProvider === 'mattermost' ? 'Configure Mattermost' : 'Configure Slack'"
      max-width="xl"
      :prevent-close="submitting"
      @update:open="
        (v: boolean) => {
          if (!v) closeDialog();
        }
      "
    >
      <MattermostDialogBody
        v-if="dialogProvider === 'mattermost' && integrations"
        :integrations="integrations"
        :submitting="submitting"
        @submit="submitMattermost"
        @close="closeDialog"
      />
      <SlackDialogBody
        v-else-if="dialogProvider === 'slack' && integrations"
        :integrations="integrations"
        :submitting="submitting"
        @submit="submitSlack"
        @save-and-o-auth="saveSlackAndStartOAuth"
        @disconnect="disconnectSlack"
        @oauth-closed="loadIntegrations"
        @close="closeDialog"
      />
    </Modal>

    <Modal
      v-model:open="showVoiceDialog"
      :show-footer="false"
      title="Configure Voice & SMS"
      max-width="xl"
      :prevent-close="submitting"
      @close="closeVoiceDialog"
    >
      <VoiceProviderDialogBody
        v-if="integrations"
        :integrations="integrations"
        :voice-provider-locked="integrations.voice_provider_locked"
        :can-manage="canConfigureIntegrations"
        :submitting="submitting"
        @submit-twilio="submitTwilio"
        @submit-telnyx="submitTelnyx"
        @close="closeVoiceDialog"
      />
    </Modal>
  </section>
</template>
