<script setup lang="ts">
import { nextTick, ref, watch } from "vue";
import { Plus, Trash2 } from "@lucide/vue";
import { api, type AlertRecord } from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import Select from "@/components/ui/Select.vue";
import Textarea from "@/components/ui/Textarea.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Modal from "@/components/ui/Modal.vue";
import { useFormSubmit } from "@/composables/useFormSubmit";

const props = defineProps<{
  open: boolean;
}>();

const emit = defineEmits<{
  "update:open": [value: boolean];
  created: [alert: AlertRecord];
}>();

const MAX_ALERTNAME_LEN = 256;
const MAX_SEVERITY_LEN = 64;
const MAX_MESSAGE_LEN = 4096;
const MAX_DESCRIPTION_LEN = 4096;
const MAX_LABEL_ROWS = 20;
const MAX_LABEL_KEY_LEN = 256;
const MAX_LABEL_VALUE_LEN = 1024;
const LABEL_KEY_RE = /^[A-Za-z_][A-Za-z0-9_]*$/;
const RESERVED_LABEL_KEYS = new Set(["alertname", "severity"]);

type LabelRow = { key: string; value: string };

const alertname = ref("");
const severity = ref("");
const message = ref("");
const description = ref("");
const labelRows = ref<LabelRow[]>([]);
const { submitting, formError: errorMessage, withSubmit } = useFormSubmit();

const dialogRoot = ref<HTMLElement | null>(null);

function resetForm() {
  alertname.value = "";
  severity.value = "";
  message.value = "";
  description.value = "";
  labelRows.value = [];
  errorMessage.value = "";
  submitting.value = false;
}

function close() {
  if (submitting.value) return;
  emit("update:open", false);
}

function addLabelRow() {
  if (labelRows.value.length >= MAX_LABEL_ROWS) return;
  labelRows.value.push({ key: "", value: "" });
  void nextTick(() => {
    const inputs = dialogRoot.value?.querySelectorAll<HTMLInputElement>("input[data-label-key]");
    inputs?.[inputs.length - 1]?.focus();
  });
}

function removeLabelRow(index: number) {
  labelRows.value.splice(index, 1);
}

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      resetForm();
    }
  },
);

function validate(): { ok: false; error: string } | { ok: true; labels: Record<string, string> } {
  const name = alertname.value.trim();
  if (!name) return { ok: false, error: "Alert name is required." };
  if (name.length > MAX_ALERTNAME_LEN) {
    return { ok: false, error: `Alert name must be ${MAX_ALERTNAME_LEN} characters or fewer.` };
  }

  const sev = severity.value.trim();
  if (sev.length > MAX_SEVERITY_LEN) {
    return { ok: false, error: `Severity must be ${MAX_SEVERITY_LEN} characters or fewer.` };
  }

  if (message.value.length > MAX_MESSAGE_LEN) {
    return { ok: false, error: `Message must be ${MAX_MESSAGE_LEN} characters or fewer.` };
  }
  if (description.value.length > MAX_DESCRIPTION_LEN) {
    return { ok: false, error: `Description must be ${MAX_DESCRIPTION_LEN} characters or fewer.` };
  }

  const labels: Record<string, string> = {};
  const seen = new Set<string>();
  for (const row of labelRows.value) {
    const key = row.key.trim();
    if (!key && !row.value) continue;
    if (!key) return { ok: false, error: "Each label must have a key." };
    if (key.length > MAX_LABEL_KEY_LEN) {
      return { ok: false, error: `Label key "${key}" is too long.` };
    }
    if (!LABEL_KEY_RE.test(key)) {
      return {
        ok: false,
        error: `Label key "${key}" must start with a letter or underscore and contain only letters, numbers, and underscores.`,
      };
    }
    if (RESERVED_LABEL_KEYS.has(key)) {
      return {
        ok: false,
        error: `Label "${key}" is reserved — use the Alert name or Severity field instead.`,
      };
    }
    if (seen.has(key)) {
      return { ok: false, error: `Duplicate label key "${key}".` };
    }
    if (row.value.length > MAX_LABEL_VALUE_LEN) {
      return { ok: false, error: `Value for label "${key}" is too long.` };
    }
    seen.add(key);
    labels[key] = row.value;
  }

  return { ok: true, labels };
}

async function submit() {
  if (submitting.value) return;
  const v = validate();
  if (!v.ok) {
    errorMessage.value = v.error;
    return;
  }

  await withSubmit(async () => {
    const payload: Parameters<typeof api.createAlert>[0] = {
      alertname: alertname.value.trim(),
    };
    const sev = severity.value.trim();
    if (sev) payload.severity = sev;
    const msg = message.value.trim();
    if (msg) payload.message = msg;
    const desc = description.value.trim();
    if (desc) payload.description = desc;
    if (Object.keys(v.labels).length > 0) payload.labels = v.labels;

    const created = await api.createAlert(payload);
    emit("created", created);
    emit("update:open", false);
  }, "Alert created");
}
</script>

<template>
  <Modal
    :open="open"
    title="Create alert"
    max-width="xl"
    :prevent-close="submitting"
    @update:open="!$event && close()"
    @close="close"
  >
    <form ref="dialogRoot" class="space-y-4" @submit.prevent="submit">
      <ErrorBanner :message="errorMessage" />

      <div>
        <FormLabel for="create-alert-name" compact required class="mb-1.5 block">
          Alert name
        </FormLabel>
        <Input
          id="create-alert-name"
          v-model="alertname"
          :maxlength="MAX_ALERTNAME_LEN"
          required
          autocomplete="off"
          placeholder="e.g. DiskSpaceLow"
          :disabled="submitting"
          aria-required="true"
        />
      </div>

      <div>
        <FormLabel for="create-alert-severity" compact class="mb-1.5 block"> Severity </FormLabel>
        <Select
          id="create-alert-severity"
          v-model="severity"
          class="w-full"
          :disabled="submitting"
          aria-label="Severity"
        >
          <option value="">No severity</option>
          <option value="critical">Critical</option>
          <option value="warning">Warning</option>
          <option value="info">Info</option>
        </Select>
      </div>

      <div>
        <FormLabel for="create-alert-message" compact class="mb-1.5 block"> Summary </FormLabel>
        <Input
          id="create-alert-message"
          v-model="message"
          :maxlength="MAX_MESSAGE_LEN"
          placeholder="Short one-line summary"
          :disabled="submitting"
        />
      </div>

      <div>
        <FormLabel for="create-alert-description" compact class="mb-1.5 block">
          Description
        </FormLabel>
        <Textarea
          id="create-alert-description"
          v-model="description"
          rows="3"
          :maxlength="MAX_DESCRIPTION_LEN"
          placeholder="Optional longer description, impact, runbook pointers…"
          class="min-h-[4.5rem] w-full resize-y"
          :disabled="submitting"
        />
      </div>

      <div>
        <div class="mb-1.5 flex items-center justify-between gap-2">
          <span class="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            Additional labels
          </span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            :disabled="submitting || labelRows.length >= MAX_LABEL_ROWS"
            @click="addLabelRow"
          >
            <Plus class="h-3.5 w-3.5" aria-hidden="true" />
            Add label
          </Button>
        </div>
        <p v-if="labelRows.length === 0" class="text-xs text-[var(--text-muted)]">
          Used by routing rules, correlation, and filters. Optional.
        </p>
        <div v-else class="space-y-2">
          <div
            v-for="(row, i) in labelRows"
            :key="i"
            class="grid grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)_auto] items-center gap-2"
          >
            <Input
              v-model="row.key"
              data-label-key
              :maxlength="MAX_LABEL_KEY_LEN"
              placeholder="key"
              :disabled="submitting"
              :aria-label="`Label key ${i + 1}`"
            />
            <Input
              v-model="row.value"
              :maxlength="MAX_LABEL_VALUE_LEN"
              placeholder="value"
              :disabled="submitting"
              :aria-label="`Label value ${i + 1}`"
            />
            <button
              type="button"
              class="inline-flex h-8 w-8 cursor-pointer items-center justify-center rounded-md text-[var(--text-muted)] transition-colors hover:bg-[var(--btn-default-hover)] hover:text-[var(--text-error)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)] disabled:cursor-not-allowed disabled:opacity-40"
              :disabled="submitting"
              :aria-label="`Remove label ${i + 1}`"
              @click="removeLabelRow(i)"
            >
              <Trash2 class="h-3.5 w-3.5" aria-hidden="true" />
            </button>
          </div>
        </div>
      </div>
    </form>

    <template #footer>
      <Button variant="outline" :disabled="submitting" @click="close">Cancel</Button>
      <Button :disabled="submitting" @click="submit">
        {{ submitting ? "Creating…" : "Create alert" }}
      </Button>
    </template>
  </Modal>
</template>
