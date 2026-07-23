<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { Lock } from "@lucide/vue";
import type { IntegrationInfo } from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import Switch from "@/components/ui/Switch.vue";
import TwilioDialogBody from "./TwilioDialogBody.vue";
import TelnyxDialogBody from "./TelnyxDialogBody.vue";

defineOptions({ name: "VoiceProviderDialogBody" });

const props = defineProps<{
  integrations: IntegrationInfo;
  voiceProviderLocked: boolean;
  canManage: boolean;
  submitting: boolean;
}>();

const emit = defineEmits<{
  submitTwilio: [
    provider: "twilio" | "telnyx",
    providerEnabled: boolean,
    payload: {
      account_sid: string;
      auth_token: string;
      from_number: string;
    },
  ];
  submitTelnyx: [
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
  ];
  close: [];
}>();

const activeProvider = computed(() => props.integrations.voice_provider ?? "twilio");

const selectedProvider = ref<"twilio" | "telnyx">(activeProvider.value);

const voiceEnabled = ref(
  (activeProvider.value === "twilio" ? props.integrations.twilio : props.integrations.telnyx)
    .provider_enabled !== false,
);

watch(
  () => props.integrations.voice_provider,
  (next) => {
    if (next && next !== selectedProvider.value) {
      selectedProvider.value = next;
    }
  },
);

watch(activeProvider, (next) => {
  const state = next === "twilio" ? props.integrations.twilio : props.integrations.telnyx;
  voiceEnabled.value = state.provider_enabled !== false;
});

const locked = computed(() =>
  selectedProvider.value === "twilio"
    ? props.integrations.twilio.locked
    : props.integrations.telnyx.locked,
);

const toggleable = computed(() => props.canManage && !props.voiceProviderLocked && !locked.value);

const providerDisabled = computed(() => props.voiceProviderLocked || !props.canManage);

function handleSaveTwilio(payload: {
  account_sid: string;
  auth_token: string;
  from_number: string;
}) {
  emit("submitTwilio", selectedProvider.value, voiceEnabled.value, payload);
}

function handleSaveTelnyx(payload: {
  api_key: string;
  connection_id: string;
  from_number: string;
  public_key: string;
  tts_voice: string;
  tts_language: string;
  tts_api_key_ref: string;
}) {
  emit("submitTelnyx", selectedProvider.value, voiceEnabled.value, payload);
}

const twilioRef = ref<InstanceType<typeof TwilioDialogBody> | null>(null);
const telnyxRef = ref<InstanceType<typeof TelnyxDialogBody> | null>(null);

function triggerSubmit() {
  if (selectedProvider.value === "twilio") {
    twilioRef.value?.submit();
  } else {
    telnyxRef.value?.submit();
  }
}
</script>

<template>
  <div class="space-y-4">
    <fieldset class="space-y-2">
      <legend class="text-sm font-medium text-[var(--text-secondary)]">
        Active voice provider
      </legend>
      <div class="flex flex-wrap items-center gap-4" role="radiogroup" aria-label="Voice provider">
        <label class="flex cursor-pointer items-center gap-2 text-sm">
          <input
            v-model="selectedProvider"
            type="radio"
            value="twilio"
            :disabled="providerDisabled"
            class="h-4 w-4 accent-[var(--accent-primary)]"
          />
          <span class="text-[var(--text-primary)]">Twilio</span>
          <Lock v-if="voiceProviderLocked" class="h-3 w-3 text-[var(--text-muted)]" />
        </label>
        <label class="flex cursor-pointer items-center gap-2 text-sm">
          <input
            v-model="selectedProvider"
            type="radio"
            value="telnyx"
            :disabled="providerDisabled"
            class="h-4 w-4 accent-[var(--accent-primary)]"
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

    <TwilioDialogBody
      v-if="selectedProvider === 'twilio'"
      ref="twilioRef"
      :integrations="integrations"
      :voice-provider="selectedProvider"
      :voice-provider-locked="voiceProviderLocked"
      :can-manage="canManage"
      :submitting="submitting"
      :standalone="false"
      :with-actions="false"
      @submit="handleSaveTwilio"
      @close="emit('close')"
    />

    <TelnyxDialogBody
      v-else
      ref="telnyxRef"
      :integrations="integrations"
      :voice-provider="selectedProvider"
      :voice-provider-locked="voiceProviderLocked"
      :can-manage="canManage"
      :submitting="submitting"
      :standalone="false"
      :with-actions="false"
      @submit="handleSaveTelnyx"
      @close="emit('close')"
    />

    <div class="flex items-center justify-between border-t border-[var(--border-primary)] pt-4">
      <div class="flex items-center gap-3">
        <Switch id="integration-voice-enabled" v-model="voiceEnabled" :disabled="!toggleable" />
        <label
          for="integration-voice-enabled"
          class="text-sm font-medium select-none cursor-pointer"
          :class="!toggleable ? 'text-[var(--text-muted)]' : 'text-[var(--text-primary)]'"
        >
          {{ voiceEnabled ? "Enabled" : "Disabled" }}
        </label>
      </div>
      <div class="flex gap-2">
        <Button type="button" variant="outline" @click="emit('close')">Cancel</Button>
        <Button type="button" :loading="submitting" @click="triggerSubmit"> Save </Button>
      </div>
    </div>
  </div>
</template>
