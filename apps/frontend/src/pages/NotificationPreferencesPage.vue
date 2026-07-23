<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { Send } from "@lucide/vue";
import { api, type NotificationPreferenceRule } from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import PreferenceRuleEditor from "@/components/notification/PreferenceRuleEditor.vue";
import { useAsyncData } from "@/composables/useAsyncData";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";

defineOptions({ name: "NotificationPreferencesPage" });

// `useAsyncData` exposes `Ref<T | null>`; mirror the loaded list into a
// dedicated `ref<T[]>([])` so add/update/remove can mutate it in place.
const rules = ref<NotificationPreferenceRule[]>([]);
const {
  data: rulesData,
  loading,
  error,
  reload: load,
} = useAsyncData<NotificationPreferenceRule[]>(async () => {
  const prefs = await api.getNotificationPreferences();
  return prefs.rules ?? [];
}, "Failed to load preferences");
watch(
  rulesData,
  (next) => {
    rules.value = next ?? [];
  },
  { immediate: true },
);
const { submitting: saving, withSubmit: withSave } = useFormSubmit();
const { submitting: sending, withSubmit: withTest } = useFormSubmit();
const preferencesSearchInput = ref("");

function addRule() {
  rules.value.push({
    notification_type: "incident_status_change",
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
    await api.updateNotificationPreferences({ rules: rules.value });
  }, "Preferences saved");
}

async function handleTest() {
  await withTest(async () => {
    await api.sendTestNotification();
  }, "Test notification sent");
}

usePageHeaderActions({
  title: "Notification Preferences",
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
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <div class="flex justify-end gap-2">
      <Button variant="outline" :loading="sending" @click="handleTest">
        <Send class="h-3.5 w-3.5" />
        Test
      </Button>
      <Button :loading="saving" @click="handleSave">Save</Button>
    </div>

    <ErrorBanner :message="error" />
    <LoadingSpinner v-if="loading" centered />

    <Card v-else-if="rules.length === 0 && !loading">
      <EmptyState message="No notification rules configured." />
    </Card>

    <template v-else>
      <Card>
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
  </section>
</template>
