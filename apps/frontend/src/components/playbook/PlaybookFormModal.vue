<script setup lang="ts">
import { ref, watch } from "vue";
import { api, type PlaybookRecord } from "@/lib/api";
import Modal from "@/components/ui/Modal.vue";
import Input from "@/components/ui/Input.vue";
import Select from "@/components/ui/Select.vue";
import Textarea from "@/components/ui/Textarea.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Button from "@/components/ui/Button.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import { useFormSubmit } from "@/composables/useFormSubmit";

type SelectorEntry = { key: string; op: string; value: string };

const props = defineProps<{
  playbook: PlaybookRecord | null;
  show: boolean;
}>();

const emit = defineEmits<{
  close: [];
  saved: [];
}>();

const { submitting, formError, withSubmit } = useFormSubmit();

const title = ref("");
const kind = ref("procedure");
const summary = ref("");
const serviceId = ref("");
const tagsCsv = ref("");
const selectors = ref<SelectorEntry[]>([blankSelector()]);

function blankSelector(): SelectorEntry {
  return { key: "", op: "exact", value: "" };
}

function resetForm() {
  if (props.playbook) {
    title.value = props.playbook.title;
    kind.value = props.playbook.kind;
    summary.value = props.playbook.summary ?? "";
    serviceId.value = props.playbook.service_id ?? "";
    tagsCsv.value = (props.playbook.tags ?? []).join(", ");
    selectors.value = props.playbook.label_selectors?.length
      ? props.playbook.label_selectors.map((s) => ({
          key: s.key,
          op: s.op,
          value: s.value,
        }))
      : [blankSelector()];
  } else {
    title.value = "";
    kind.value = "procedure";
    summary.value = "";
    serviceId.value = "";
    tagsCsv.value = "";
    selectors.value = [blankSelector()];
  }
  formError.value = "";
}

watch(
  () => props.show,
  () => {
    if (props.show) resetForm();
  },
);

function addSelector() {
  selectors.value.push(blankSelector());
}

function removeSelector(i: number) {
  selectors.value.splice(i, 1);
  if (selectors.value.length === 0) addSelector();
}

async function handleSubmit() {
  if (!title.value.trim()) {
    formError.value = "Title is required.";
    return;
  }
  await withSubmit(
    async () => {
      const sel = selectors.value
        .filter((s) => s.key.trim())
        .map((s) => ({ key: s.key.trim(), op: s.op, value: s.value.trim() }));
      const tags = tagsCsv.value
        .split(",")
        .map((t) => t.trim())
        .filter(Boolean);
      const payload = {
        title: title.value.trim(),
        kind: kind.value,
        summary: summary.value.trim() || undefined,
        service_id: serviceId.value.trim() || undefined,
        label_selectors: sel.length ? sel : undefined,
        tags: tags.length ? tags : undefined,
      };
      if (props.playbook) {
        await api.updatePlaybook(props.playbook.id, payload);
      } else {
        await api.createPlaybook(payload);
      }
      emit("saved");
    },
    props.playbook ? "Playbook updated" : "Playbook created",
  );
}
</script>

<template>
  <Modal
    :open="show"
    :title="playbook ? 'Edit Playbook' : 'Create Playbook'"
    :loading="submitting"
    :confirm-label="playbook ? 'Save' : 'Create'"
    @update:open="if (!$event) emit('close');"
    @confirm="handleSubmit"
  >
    <div class="space-y-4">
      <div>
        <FormLabel for="pb-title">Title</FormLabel>
        <Input id="pb-title" v-model="title" placeholder="Playbook title" required />
      </div>
      <div>
        <FormLabel for="pb-kind">Kind</FormLabel>
        <Select id="pb-kind" v-model="kind">
          <option value="procedure">Procedure</option>
          <option value="mitigation">Mitigation</option>
        </Select>
      </div>
      <div>
        <FormLabel for="pb-summary">Summary</FormLabel>
        <Textarea
          id="pb-summary"
          v-model="summary"
          rows="3"
          placeholder="Brief description of this playbook..."
        />
      </div>
      <div>
        <FormLabel for="pb-service">Service ID (optional)</FormLabel>
        <Input id="pb-service" v-model="serviceId" placeholder="UUID of the service" />
      </div>
      <div>
        <FormLabel for="pb-tags">Tags (comma-separated)</FormLabel>
        <Input id="pb-tags" v-model="tagsCsv" placeholder="postgres, database, runbook" />
      </div>
      <div class="space-y-2">
        <p class="text-sm font-medium text-[var(--text-primary)]">Label Selectors</p>
        <div
          v-for="(entry, idx) in selectors"
          :key="idx"
          class="grid gap-2 md:grid-cols-[1fr_1fr_1fr_auto]"
        >
          <Input v-model="entry.key" placeholder="key (e.g. alertname)" />
          <Select v-model="entry.op">
            <option value="exact">exact</option>
            <option value="contains">contains</option>
            <option value="prefix">prefix</option>
            <option value="suffix">suffix</option>
            <option value="regex">regex</option>
            <option value="exists">exists</option>
            <option value="not_exists">not_exists</option>
          </Select>
          <Input
            v-model="entry.value"
            placeholder="value"
            :disabled="entry.op === 'exists' || entry.op === 'not_exists'"
          />
          <Button size="sm" variant="destructive" @click="removeSelector(idx)">Remove</Button>
        </div>
        <Button size="sm" @click="addSelector">Add selector</Button>
      </div>
      <ErrorBanner :message="formError" />
    </div>
  </Modal>
</template>
