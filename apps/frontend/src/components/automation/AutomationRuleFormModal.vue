<script setup lang="ts">
import { computed, ref, watch } from "vue";
import {
  api,
  type AutomationAction,
  type AutomationCondition,
  type AutomationRuleRecord,
} from "@/lib/api";
import { useFormSubmit } from "@/composables/useFormSubmit";
import Modal from "@/components/ui/Modal.vue";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import Select from "@/components/ui/Select.vue";
import Textarea from "@/components/ui/Textarea.vue";
import Switch from "@/components/ui/Switch.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";

defineOptions({ name: "AutomationRuleFormModal" });

const props = defineProps<{ open: boolean; rule: AutomationRuleRecord | null }>();
const emit = defineEmits<{ close: []; saved: [] }>();
const { submitting, formError, withSubmit } = useFormSubmit();
type FormAction = AutomationAction & { clear_bearer_token: boolean };

const name = ref("");
const description = ref("");
const triggerType = ref("alert.created");
const enabled = ref(false);
const conditions = ref<AutomationCondition[]>([]);
const actions = ref<FormAction[]>([
  { type: "webhook", url: "", bearer_token: "", clear_bearer_token: false },
]);
const isIncidentTrigger = computed(() => triggerType.value.startsWith("incident."));

function reset() {
  const rule = props.rule;
  name.value = rule?.name ?? "";
  description.value = rule?.description ?? "";
  triggerType.value = rule?.trigger_type ?? "alert.created";
  enabled.value = rule?.enabled ?? false;
  conditions.value = rule?.conditions.map((c) => ({ ...c })) ?? [];
  actions.value = rule?.actions.map((a) => ({
    ...a,
    bearer_token: "",
    clear_bearer_token: false,
  })) ?? [{ type: "webhook", url: "", bearer_token: "", clear_bearer_token: false }];
  formError.value = "";
}

watch(
  () => props.open,
  (open) => {
    if (open) reset();
  },
);

function addCondition() {
  if (conditions.value.length < 10)
    conditions.value.push({ field: "severity", operator: "equals", value: "" });
}

function addAction() {
  if (actions.value.length < 5)
    actions.value.push({ type: "webhook", url: "", bearer_token: "", clear_bearer_token: false });
}

function moveAction(index: number, offset: number) {
  const target = index + offset;
  if (target < 0 || target >= actions.value.length) return;
  [actions.value[index], actions.value[target]] = [actions.value[target], actions.value[index]];
}

async function save() {
  if (!name.value.trim()) {
    formError.value = "Name is required.";
    return;
  }
  if (!actions.value.length) {
    formError.value = "Add at least one action.";
    return;
  }
  await withSubmit(
    async () => {
      const payload = {
        name: name.value.trim(),
        description: description.value.trim(),
        trigger_type: triggerType.value,
        conditions: conditions.value.map((c) => ({
          field: c.field.trim(),
          operator: c.operator,
          value: c.value?.trim() ?? "",
        })),
        actions: actions.value.map((a) => ({
          type: a.type,
          ...(a.type === "webhook"
            ? {
                url: a.url?.trim() ?? "",
                bearer_token: a.bearer_token?.trim() || undefined,
                clear_bearer_token: a.clear_bearer_token || undefined,
              }
            : { message: a.message?.trim() ?? "" }),
        })),
        enabled: props.rule ? enabled.value : false,
      };
      if (props.rule)
        await api.updateAutomationRule(props.rule.id, { ...payload, version: props.rule.version });
      else await api.createAutomationRule(payload);
      emit("saved");
    },
    props.rule ? "Automation rule updated" : "Automation rule created",
  );
}
</script>

<template>
  <Modal
    :open="open"
    :title="rule ? 'Edit automation rule' : 'Create automation rule'"
    max-width="3xl"
    :loading="submitting"
    :confirm-label="rule ? 'Save rule' : 'Create disabled rule'"
    @update:open="if (!$event) emit('close');"
    @confirm="save"
  >
    <div class="max-h-[70vh] space-y-5 overflow-y-auto pr-1">
      <div class="grid gap-3 sm:grid-cols-2">
        <div>
          <FormLabel for="automation-name">Name</FormLabel>
          <Input
            id="automation-name"
            v-model="name"
            maxlength="120"
            required
            placeholder="Notify the response team"
          />
        </div>
        <div>
          <FormLabel for="automation-trigger">When</FormLabel>
          <Select id="automation-trigger" v-model="triggerType">
            <option value="alert.created">Alert created</option>
            <option value="alert.acknowledged">Alert acknowledged</option>
            <option value="alert.resolved">Alert resolved</option>
            <option value="alert.reopened">Alert reopened</option>
            <option value="incident.created">Incident created</option>
            <option value="incident.status_changed">Incident status changed</option>
          </Select>
        </div>
      </div>

      <div>
        <FormLabel for="automation-description">Description</FormLabel>
        <Textarea
          id="automation-description"
          v-model="description"
          rows="2"
          placeholder="What this rule handles"
        />
      </div>

      <div v-if="rule" class="flex items-center gap-3">
        <Switch id="automation-enabled" v-model="enabled" />
        <FormLabel for="automation-enabled">Enabled</FormLabel>
      </div>
      <p v-else class="text-sm text-[var(--text-muted)]">
        New rules start disabled. Review the rule, then enable it.
      </p>

      <div class="space-y-3">
        <div class="flex items-center justify-between">
          <div>
            <h3 class="font-medium">If all conditions match</h3>
            <p class="text-xs text-[var(--text-muted)]">
              Leave empty to match every event of this type. Use label.alertname or another label
              key for alerts.
            </p>
          </div>
          <Button
            size="sm"
            variant="outline"
            :disabled="conditions.length >= 10"
            @click="addCondition"
            >Add condition</Button
          >
        </div>
        <div
          v-for="(condition, index) in conditions"
          :key="index"
          class="grid gap-2 rounded-lg border border-[var(--border-primary)] p-3 sm:grid-cols-[1.2fr_1fr_1.2fr_auto]"
        >
          <div>
            <FormLabel :for="`automation-field-${index}`">Field</FormLabel
            ><Input
              :id="`automation-field-${index}`"
              v-model="condition.field"
              placeholder="severity or label.alertname"
            />
          </div>
          <div>
            <FormLabel :for="`automation-operator-${index}`">Operator</FormLabel
            ><Select :id="`automation-operator-${index}`" v-model="condition.operator"
              ><option value="equals">Equals</option>
              <option value="not_equals">Does not equal</option>
              <option value="contains">Contains</option>
              <option value="exists">Exists</option></Select
            >
          </div>
          <div>
            <FormLabel :for="`automation-value-${index}`">Value</FormLabel
            ><Input
              :id="`automation-value-${index}`"
              v-model="condition.value"
              :disabled="condition.operator === 'exists'"
            />
          </div>
          <Button
            size="sm"
            variant="destructive"
            class="self-end"
            @click="conditions.splice(index, 1)"
            >Remove</Button
          >
        </div>
      </div>

      <div class="space-y-3">
        <div class="flex items-center justify-between">
          <h3 class="font-medium">Then run these actions in order</h3>
          <Button size="sm" variant="outline" :disabled="actions.length >= 5" @click="addAction"
            >Add action</Button
          >
        </div>
        <div
          v-for="(action, index) in actions"
          :key="index"
          class="space-y-3 rounded-lg border border-[var(--border-primary)] p-3"
        >
          <div class="flex flex-wrap items-end gap-2">
            <div class="min-w-48 flex-1">
              <FormLabel :for="`automation-action-type-${index}`">Action {{ index + 1 }}</FormLabel
              ><Select :id="`automation-action-type-${index}`" v-model="action.type"
                ><option value="webhook">Send HTTPS request</option>
                <option v-if="isIncidentTrigger" value="incident_note">
                  Add incident note
                </option></Select
              >
            </div>
            <Button
              size="sm"
              variant="outline"
              :disabled="index === 0"
              @click="moveAction(index, -1)"
              >Up</Button
            >
            <Button
              size="sm"
              variant="outline"
              :disabled="index === actions.length - 1"
              @click="moveAction(index, 1)"
              >Down</Button
            >
            <Button
              size="sm"
              variant="destructive"
              :disabled="actions.length === 1"
              @click="actions.splice(index, 1)"
              >Remove</Button
            >
          </div>
          <template v-if="action.type === 'webhook'">
            <div>
              <FormLabel :for="`automation-url-${index}`">HTTPS URL</FormLabel
              ><Input
                :id="`automation-url-${index}`"
                v-model="action.url"
                type="url"
                placeholder="https://n8n.example.com/webhook/alga"
              />
            </div>
            <div>
              <FormLabel :for="`automation-token-${index}`"
                >Bearer token
                {{
                  action.has_bearer_token ? "(stored; leave blank to keep)" : "(optional)"
                }}</FormLabel
              ><Input
                :id="`automation-token-${index}`"
                v-model="action.bearer_token"
                type="password"
                autocomplete="new-password"
                :disabled="action.clear_bearer_token === true"
              />
            </div>
            <div v-if="action.has_bearer_token" class="flex items-center gap-3">
              <Switch :id="`automation-clear-token-${index}`" v-model="action.clear_bearer_token" />
              <FormLabel :for="`automation-clear-token-${index}`"
                >Remove stored bearer token</FormLabel
              >
            </div>
          </template>
          <div v-else>
            <FormLabel :for="`automation-message-${index}`">Incident note</FormLabel
            ><Textarea :id="`automation-message-${index}`" v-model="action.message" rows="2" />
          </div>
        </div>
        <p class="text-xs text-[var(--text-muted)]">
          Outbound hosts must be approved in server configuration. Receivers should deduplicate
          using the Idempotency-Key header.
        </p>
      </div>
      <ErrorBanner :message="formError" />
    </div>
  </Modal>
</template>
