<script setup lang="ts">
import { Lock } from "@lucide/vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Input from "@/components/ui/Input.vue";

defineOptions({ name: "IntegrationField" });

/**
 * <FormLabel + <Input> + optional <Lock>> badge for the integration
 * dialogs (Slack, Mattermost, Twilio, Telnyx, VoiceProvider). Cuts the
 * 5+ duplicated lock-icon blocks per dialog and keeps the "this field
 * is managed by env vars" affordance in one place.
 *
 * Pass `hint` for an inline "(optional)" / "(leave blank to keep)"
 * annotation that matches the existing FormLabel API. `required` and
 * `disabled` forward to FormLabel; `type` forwards to Input (e.g.
 * "password" for secrets).
 */
const props = withDefaults(
  defineProps<{
    id: string;
    label: string;
    locked?: boolean;
    type?: string;
    required?: boolean;
    disabled?: boolean;
    hint?: string;
    placeholder?: string;
  }>(),
  { locked: false, type: "text", required: false, disabled: false },
);

const model = defineModel<string>({ required: true });
</script>

<template>
  <div>
    <FormLabel
      :for="props.id"
      :required="props.required"
      :disabled="props.disabled"
      :hint="props.hint"
    >
      {{ props.label }}
      <Lock v-if="props.locked" class="ml-1 inline h-3 w-3 align-middle text-[var(--text-muted)]" />
    </FormLabel>
    <Input
      :id="props.id"
      v-model="model"
      :type="props.type"
      :disabled="props.disabled"
      :placeholder="props.placeholder"
    />
  </div>
</template>
