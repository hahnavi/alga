<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { CheckCircle, RefreshCw } from "@lucide/vue";
import { getErrorMessage } from "@/lib/error";
import type { IntegrationInfo } from "@/lib/api";
import { api } from "@/lib/api";
import { useToast } from "@/lib/toast";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import Switch from "@/components/ui/Switch.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import IntegrationField from "@/components/ui/IntegrationField.vue";

defineOptions({ name: "MattermostDialogBody" });

const props = defineProps<{
  integrations: IntegrationInfo;
  submitting: boolean;
}>();

const emit = defineEmits<{
  submit: [
    payload: {
      url: string;
      secret: string;
      team: string;
      default_channel: string;
      provider_enabled: boolean;
    },
  ];
  test: [payload: { url: string; secret: string; team: string }];
  close: [];
}>();

const { push } = useToast();

const mm = computed(() => props.integrations.mattermost);

const url = ref("");
const secret = ref("");
const team = ref("");
const defaultChannel = ref("");
const providerEnabled = ref(true);

const channelSuggestions = ref<string[]>([]);
const channelsLoading = ref(false);

watch(
  () => props.integrations.mattermost,
  (next) => {
    url.value = next.base_url?.trim() ?? next.url;
    secret.value = "";
    team.value = next.team;
    defaultChannel.value = next.default_channel ?? "";
    providerEnabled.value = next.provider_enabled !== false;
  },
  { immediate: true },
);

async function loadChannelSuggestions() {
  if (channelsLoading.value) return;
  channelsLoading.value = true;
  try {
    const data = await api.getChannels();
    channelSuggestions.value = Array.isArray(data) ? data.map((c) => c.display_name || c.name) : [];
  } catch {
    // intentional — channel suggestions are optional UX
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

const testing = ref(false);

async function submit() {
  if (providerEnabled.value) {
    if (!url.value.trim()) {
      push("Server URL is required", "error");
      return;
    }
    if (!team.value.trim()) {
      push("Team is required", "error");
      return;
    }
    if (!mm.value.secret_configured && !secret.value.trim()) {
      push("Webhook Secret is required", "error");
      return;
    }
    if (!defaultChannel.value.trim()) {
      push("Default alert channel is required", "error");
      return;
    }
  }
  emit("submit", {
    url: url.value,
    secret: secret.value,
    team: team.value,
    default_channel: defaultChannel.value.trim(),
    provider_enabled: providerEnabled.value,
  });
}

async function test() {
  testing.value = true;
  try {
    await api.testIntegration("mattermost", {
      mattermost: { url: url.value, secret: secret.value, team: team.value },
    });
    push("Mattermost connection successful", "success");
  } catch (err) {
    push(getErrorMessage(err, "Failed to test Mattermost"), "error");
  } finally {
    testing.value = false;
  }
}

function clearSecretPlaceholder(): string | undefined {
  return mm.value.secret_configured ? "•••••••• (leave blank to keep)" : "Enter webhook secret";
}
</script>

<template>
  <form class="space-y-4" @submit.prevent="submit">
    <IntegrationField
      id="integration-mm-url"
      v-model="url"
      label="Server URL"
      :required="true"
      :disabled="mm.locked"
      :locked="mm.locked"
      placeholder="https://mattermost.example.com"
    />

    <IntegrationField
      id="integration-mm-team"
      v-model="team"
      label="Team"
      :required="true"
      :disabled="mm.locked"
      placeholder="engineering"
    />

    <IntegrationField
      id="integration-mm-secret"
      v-model="secret"
      type="password"
      label="Webhook Secret"
      :required="!mm.secret_configured"
      :disabled="mm.locked"
      :locked="mm.locked"
      :placeholder="clearSecretPlaceholder()"
    />

    <div>
      <FormLabel for="integration-mm-default-channel" required>Default Alert Channel</FormLabel>
      <Input
        id="integration-mm-default-channel"
        v-model="defaultChannel"
        placeholder="e.g. alerts"
        list="mm-channel-suggestions"
      />
      <datalist id="mm-channel-suggestions">
        <option v-for="ch in channelSuggestions" :key="ch" :value="ch" />
      </datalist>
    </div>

    <div
      class="flex items-center gap-3 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2.5"
    >
      <Switch
        id="integration-mm-provider-enabled"
        v-model="providerEnabled"
        :disabled="mm.locked"
      />
      <div>
        <span
          class="text-sm font-medium"
          :class="mm.locked ? 'text-[var(--text-muted)]' : 'text-[var(--text-primary)]'"
        >
          {{ providerEnabled ? "Enabled" : "Disabled" }}
        </span>
        <p class="text-xs text-[var(--text-muted)]">
          {{
            providerEnabled ? "Alerts will be forwarded to Mattermost" : "Alerts won't be forwarded"
          }}
        </p>
      </div>
    </div>

    <div class="flex justify-end gap-2 border-t border-[var(--border-primary)] pt-4">
      <Button type="button" variant="outline" @click="emit('close')">Cancel</Button>
      <Button type="button" :disabled="testing || submitting" @click="test">
        <CheckCircle v-if="!testing" class="h-3.5 w-3.5" />
        <RefreshCw v-else class="h-3.5 w-3.5 animate-spin" />
        {{ testing ? "Testing…" : "Test Connection" }}
      </Button>
      <Button type="submit" :loading="submitting">Save</Button>
    </div>
  </form>
</template>
