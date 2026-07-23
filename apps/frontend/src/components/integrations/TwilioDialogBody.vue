<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { Lock } from "@lucide/vue";
import type { IntegrationInfo } from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import IntegrationField from "@/components/ui/IntegrationField.vue";

defineOptions({ name: "TwilioDialogBody" });

const props = defineProps<{
  integrations: IntegrationInfo;
  voiceProvider: "twilio" | "telnyx";
  /** When true, the entire form is locked (managed by environment variables). */
  voiceProviderLocked: boolean;
  submitting: boolean;
  canManage: boolean;
  /** When false, the provider radio buttons are hidden (parent handles them). */
  standalone?: boolean;
  /** When true, also render the internal Save / Cancel buttons. */
  withActions?: boolean;
}>();

const emit = defineEmits<{
  submit: [
    payload: {
      account_sid: string;
      auth_token: string;
      from_number: string;
    },
  ];
  "update:voiceProvider": [value: "twilio" | "telnyx"];
  close: [];
}>();

const twilio = computed(() => props.integrations.twilio);

const accountSid = ref("");
const authToken = ref("");
const fromNumber = ref("");

watch(
  () => props.integrations.twilio,
  (next) => {
    fromNumber.value = next.from_number ?? "";
    accountSid.value = "";
    authToken.value = "";
  },
  { immediate: true },
);

function maskedConfigured(value: boolean): string {
  return value ? "•••••••• (leave blank to keep)" : "";
}

function buildPayload() {
  return {
    account_sid: accountSid.value,
    auth_token: authToken.value,
    from_number: fromNumber.value.trim(),
  };
}

function submit() {
  emit("submit", buildPayload());
}

defineExpose({ submit });
</script>

<template>
  <form class="space-y-4" @submit.prevent="emit('submit', buildPayload())">
    <fieldset v-if="props.standalone !== false" class="space-y-2">
      <legend class="text-sm font-medium text-[var(--text-secondary)]">
        Active voice provider
      </legend>
      <div class="flex flex-wrap items-center gap-4">
        <label class="flex cursor-pointer items-center gap-2 text-sm">
          <input
            type="radio"
            value="twilio"
            :checked="voiceProvider === 'twilio'"
            :disabled="voiceProviderLocked || !canManage"
            class="h-4 w-4 accent-[var(--accent-primary)]"
            @change="emit('update:voiceProvider', 'twilio')"
          />
          <span class="text-[var(--text-primary)]">Twilio</span>
          <Lock v-if="voiceProviderLocked" class="h-3 w-3 text-[var(--text-muted)]" />
        </label>
        <label class="flex cursor-pointer items-center gap-2 text-sm">
          <input
            type="radio"
            value="telnyx"
            :checked="voiceProvider === 'telnyx'"
            :disabled="voiceProviderLocked || !canManage"
            class="h-4 w-4 accent-[var(--accent-primary)]"
            @change="emit('update:voiceProvider', 'telnyx')"
          />
          <span class="text-[var(--text-primary)]">Telnyx</span>
          <Lock v-if="voiceProviderLocked" class="h-3 w-3 text-[var(--text-muted)]" />
        </label>
      </div>
      <p class="text-xs text-[var(--text-muted)]">
        <template v-if="voiceProviderLocked">
          Managed by the <code>VOICE_PROVIDER</code> environment variable.
        </template>
        <template v-else>
          Only one voice provider can be active at a time. Saving switches the active provider
          immediately.
        </template>
      </p>
    </fieldset>

    <div
      v-if="twilio.locked"
      class="flex items-center gap-2 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2.5 text-xs text-[var(--text-muted)]"
    >
      <Lock class="h-3.5 w-3.5" />
      <span>Twilio is configured via environment variables.</span>
    </div>

    <IntegrationField
      id="integration-twilio-account-sid"
      v-model="accountSid"
      label="Account SID"
      :disabled="twilio.locked"
      :locked="twilio.locked"
      :placeholder="twilio.account_sid_configured ? maskedConfigured(true) : 'Enter Account SID'"
    />

    <IntegrationField
      id="integration-twilio-auth-token"
      v-model="authToken"
      type="password"
      label="Auth Token"
      :disabled="twilio.locked"
      :locked="twilio.locked"
      :placeholder="twilio.auth_token_configured ? maskedConfigured(true) : 'Enter Auth Token'"
    />

    <IntegrationField
      id="integration-twilio-from-number"
      v-model="fromNumber"
      label="From Number"
      :disabled="twilio.locked"
      :locked="twilio.locked"
      placeholder="+15551234567"
    />

    <div
      v-if="props.withActions !== false"
      class="flex justify-end gap-2 border-t border-[var(--border-primary)] pt-4"
    >
      <Button type="button" variant="outline" @click="emit('close')">Cancel</Button>
      <Button type="submit" :loading="submitting">Save</Button>
    </div>
  </form>
</template>
