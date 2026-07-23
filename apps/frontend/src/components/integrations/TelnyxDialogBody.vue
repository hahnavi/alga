<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { Lock } from "@lucide/vue";
import type { IntegrationInfo } from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import FormLabel from "@/components/ui/FormLabel.vue";

defineOptions({ name: "TelnyxDialogBody" });

const props = defineProps<{
  integrations: IntegrationInfo;
  voiceProvider: "twilio" | "telnyx";
  /** When true, the entire form is locked (managed by environment variables). */
  voiceProviderLocked: boolean;
  submitting: boolean;
  /** Whether the user is allowed to switch voice providers (gates radios). */
  canManage: boolean;
  /** When false, the provider radio buttons are hidden (parent handles them). */
  standalone?: boolean;
  /** When true, also render the internal Save / Cancel buttons. */
  withActions?: boolean;
}>();

const emit = defineEmits<{
  submit: [
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
  "update:voiceProvider": [value: "twilio" | "telnyx"];
  close: [];
}>();

const telnyx = computed(() => props.integrations.telnyx);

const apiKey = ref("");
const connectionId = ref("");
const fromNumber = ref("");
const publicKey = ref("");
const ttsVoice = ref("");
const ttsLanguage = ref("");
const ttsApiKeyRef = ref("");

watch(
  () => props.integrations.telnyx,
  (next) => {
    connectionId.value = next.connection_id ?? "";
    fromNumber.value = next.from_number ?? "";
    ttsVoice.value = next.tts_voice ?? "";
    ttsLanguage.value = next.tts_language ?? "";
    ttsApiKeyRef.value = next.tts_api_key_ref ?? "";
    apiKey.value = "";
    publicKey.value = "";
  },
  { immediate: true },
);

function buildPayload() {
  return {
    api_key: apiKey.value,
    connection_id: connectionId.value,
    from_number: fromNumber.value.trim(),
    public_key: publicKey.value,
    tts_voice: ttsVoice.value.trim(),
    tts_language: ttsLanguage.value.trim(),
    tts_api_key_ref: ttsApiKeyRef.value.trim(),
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
      v-if="telnyx.locked"
      class="flex items-center gap-2 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2.5 text-xs text-[var(--text-muted)]"
    >
      <Lock class="h-3.5 w-3.5" />
      <span>Telnyx is configured via environment variables.</span>
    </div>

    <div>
      <FormLabel for="integration-telnyx-api-key" :disabled="telnyx.locked">
        API Key
        <Lock
          v-if="telnyx.locked"
          class="ml-1 inline h-3 w-3 align-middle text-[var(--text-muted)]"
        />
      </FormLabel>
      <Input
        id="integration-telnyx-api-key"
        v-model="apiKey"
        type="password"
        :disabled="telnyx.locked"
        :placeholder="
          telnyx.api_key_configured ? '•••••••• (leave blank to keep)' : 'Enter API Key'
        "
      />
    </div>

    <div>
      <FormLabel for="integration-telnyx-connection-id" :disabled="telnyx.locked">
        Application ID
        <Lock
          v-if="telnyx.locked"
          class="ml-1 inline h-3 w-3 align-middle text-[var(--text-muted)]"
        />
      </FormLabel>
      <Input
        id="integration-telnyx-connection-id"
        v-model="connectionId"
        :disabled="telnyx.locked"
        placeholder="Call Control Application ID"
      />
    </div>

    <div>
      <FormLabel for="integration-telnyx-from-number" :disabled="telnyx.locked">
        From Number
        <Lock
          v-if="telnyx.locked"
          class="ml-1 inline h-3 w-3 align-middle text-[var(--text-muted)]"
        />
      </FormLabel>
      <Input
        id="integration-telnyx-from-number"
        v-model="fromNumber"
        :disabled="telnyx.locked"
        placeholder="+15551234567"
      />
    </div>

    <div>
      <FormLabel for="integration-telnyx-public-key" :disabled="telnyx.locked">
        Public Key
        <Lock
          v-if="telnyx.locked"
          class="ml-1 inline h-3 w-3 align-middle text-[var(--text-muted)]"
        />
      </FormLabel>
      <Input
        id="integration-telnyx-public-key"
        v-model="publicKey"
        type="password"
        :disabled="telnyx.locked"
        :placeholder="
          telnyx.public_key_configured
            ? '•••••••• (leave blank to keep)'
            : 'Ed25519 public key (base64)'
        "
      />
    </div>

    <div class="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-tertiary)] p-3">
      <p class="mb-3 text-xs text-[var(--text-muted)]">
        Text-to-speech voice for spoken prompts. The provider is encoded in the voice prefix (e.g.
        <code>Polly.Brian</code>, <code>Azure.en-CA-ClaraNeural</code>,
        <code>ElevenLabs.eleven_flash_v2_5.iP95p4xoKVk53GoZ742B</code>). For ElevenLabs BYOK,
        register your ElevenLabs API key as a Telnyx integration secret and put its identifier in
        the API Key Ref field below.
      </p>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <div>
          <FormLabel for="integration-telnyx-tts-voice" :disabled="telnyx.locked">
            TTS Voice
            <Lock
              v-if="telnyx.locked"
              class="ml-1 inline h-3 w-3 align-middle text-[var(--text-muted)]"
            />
          </FormLabel>
          <Input
            id="integration-telnyx-tts-voice"
            v-model="ttsVoice"
            :disabled="telnyx.locked"
            placeholder="Polly.Brian"
          />
        </div>
        <div>
          <FormLabel for="integration-telnyx-tts-language" :disabled="telnyx.locked">
            TTS Language
            <Lock
              v-if="telnyx.locked"
              class="ml-1 inline h-3 w-3 align-middle text-[var(--text-muted)]"
            />
          </FormLabel>
          <Input
            id="integration-telnyx-tts-language"
            v-model="ttsLanguage"
            :disabled="telnyx.locked"
            placeholder="en-US"
          />
        </div>
        <div>
          <FormLabel for="integration-telnyx-tts-api-key-ref" :disabled="telnyx.locked">
            TTS API Key Ref
            <Lock
              v-if="telnyx.locked"
              class="ml-1 inline h-3 w-3 align-middle text-[var(--text-muted)]"
            />
          </FormLabel>
          <Input
            id="integration-telnyx-tts-api-key-ref"
            v-model="ttsApiKeyRef"
            :disabled="telnyx.locked"
            placeholder="elevenlabs-prod (ElevenLabs only)"
          />
        </div>
      </div>
    </div>

    <div
      v-if="props.withActions !== false"
      class="flex justify-end gap-2 border-t border-[var(--border-primary)] pt-4"
    >
      <Button type="button" variant="outline" @click="emit('close')">Cancel</Button>
      <Button type="submit" :loading="submitting">Save</Button>
    </div>
  </form>
</template>
