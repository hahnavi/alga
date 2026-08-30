<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { BellRing, Send } from "@lucide/vue";
import { api, type NotificationPreferenceRule, type NotificationPreferences } from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import PreferenceRuleEditor from "@/components/notification/PreferenceRuleEditor.vue";
import SettingsPageShell from "@/components/ui/settings/SettingsPageShell.vue";
import { useAsyncData } from "@/composables/useAsyncData";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";

defineOptions({ name: "NotificationPreferencesPage" });

// `useAsyncData` exposes `Ref<T | null>`; mirror the loaded payload into
// dedicated refs so add/update/remove can mutate rules in place and the
// default channel stays independently editable.
const rules = ref<NotificationPreferenceRule[]>([]);
const defaultChannel = ref("");
const DEFAULT_CHANNELS = ["in_app", "email", "mattermost", "slack", "voice"] as const;
const {
  data: rulesData,
  loading,
  error,
  reload: load,
} = useAsyncData<NotificationPreferences>(
  async () => api.getNotificationPreferences(),
  "Failed to load preferences",
);
watch(
  rulesData,
  (next) => {
    rules.value = next?.rules ?? [];
    defaultChannel.value = next?.default_channel ?? "";
  },
  { immediate: true },
);
const { submitting: saving, withSubmit: withSave } = useFormSubmit();
const { submitting: sending, withSubmit: withTest } = useFormSubmit();
const preferencesSearchInput = ref("");

function addRule() {
  rules.value.push({
    notification_type: "*",
    channels: ["in_app"],
    enabled: true,
  });
}

function updateRule(index: number, data: Partial<NotificationPreferenceRule>) {
  rules.value[index] = { ...rules.value[index], ...data };
}

function removeRule(index: number) {
  rules.value.splice(index, 1);
}

async function handleSave() {
  await withSave(async () => {
    const prefs: NotificationPreferences = { rules: rules.value };
    if (defaultChannel.value) prefs.default_channel = defaultChannel.value;
    await api.updateNotificationPreferences(prefs);
  }, "Preferences saved");
}

async function handleTest() {
  await withTest(async () => {
    await api.sendTestNotification();
  }, "Test notification sent");
}

usePageHeaderActions({
  title: "Notification Preferences",
  titleIcon: BellRing,
  searchInput: preferencesSearchInput,
  searchPlaceholder: "Search preferences...",
  onAdd: addRule,
  addLabel: "Add Rule",
});

onMounted(() => {
  load();
});
</script>

<template>
  <SettingsPageShell
    description="Rules that route notifications to channels. The default channel applies when no rule matches."
  >
    <div class="flex justify-end gap-2">
      <Button variant="outline" :loading="sending" @click="handleTest">
        <Send class="h-3.5 w-3.5" />
        Test
      </Button>
      <Button :loading="saving" @click="handleSave">Save</Button>
    </div>

    <ErrorBanner :message="error" />
    <LoadingSpinner v-if="loading" centered />

    <template v-else>
      <Card>
        <div class="flex flex-wrap items-center gap-3 px-3 py-2">
          <label for="default-channel" class="text-xs font-medium text-[var(--text-secondary)]">
            Default channel
          </label>
          <Select id="default-channel" v-model="defaultChannel" class="text-xs">
            <option value="">none</option>
            <option v-for="ch in DEFAULT_CHANNELS" :key="ch" :value="ch">
              {{ ch.replace(/_/g, " ") }}
            </option>
          </Select>
          <span class="text-xs text-[var(--text-muted)]">
            Used when no rule matches a notification.
          </span>
        </div>
      </Card>

      <Card v-if="rules.length === 0">
        <EmptyState message="No notification rules configured." />
      </Card>

      <Card v-else>
        <div class="overflow-x-auto">
          <table class="w-full text-left">
            <thead>
              <tr
                class="border-b border-[var(--border-primary)] text-xs font-medium text-[var(--text-muted)]"
              >
                <th class="px-3 py-2">Notification Type</th>
                <th class="px-3 py-2">Channels</th>
                <th class="px-3 py-2">Enabled</th>
                <th class="px-3 py-2"></th>
              </tr>
            </thead>
            <tbody>
              <PreferenceRuleEditor
                v-for="(rule, i) in rules"
                :key="i"
                :rule="rule"
                :index="i"
                @update="updateRule(i, $event)"
                @remove="removeRule(i)"
              />
            </tbody>
          </table>
        </div>
      </Card>
    </template>
  </SettingsPageShell>
</template>
