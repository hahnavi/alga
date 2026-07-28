<script setup lang="ts">
import { computed, onMounted } from "vue";
import { Merge, SlidersHorizontal } from "@lucide/vue";
import Card from "@/components/ui/Card.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Input from "@/components/ui/Input.vue";
import NumberInput from "@/components/ui/NumberInput.vue";
import Select from "@/components/ui/Select.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import SkeletonRows from "@/components/ui/SkeletonRows.vue";
import SystemFormFooter from "@/components/system/SystemFormFooter.vue";
import { useSystemConfigForm, type SystemForm } from "@/composables/useSystemConfigForm";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { formatTimeAgo } from "@/lib/time";

defineOptions({ name: "SystemGeneralPage" });

const FIELDS: ReadonlyArray<keyof SystemForm> = [
  "log_level",
  "session_expiry_hours",
  "correlation_window",
  "correlation_cooldown_ttl",
];

const {
  form,
  original,
  environment,
  updatedAt,
  loading,
  error,
  loadConfig,
  saving,
  saveError,
  isDirty,
  changedFields,
  discard,
  save,
} = useSystemConfigForm();

usePageHeaderActions({
  title: "General",
  titleIcon: SlidersHorizontal,
  showAdd: false,
});

const dirty = computed(() => isDirty(FIELDS));

onMounted(() => {
  void loadConfig();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <ErrorBanner :message="error" />

    <SkeletonRows v-if="loading" :count="6" />

    <template v-else-if="original">
      <div class="flex flex-col gap-1">
        <p class="text-sm text-[var(--text-muted)]">
          Runtime system configuration. Changes take effect immediately and persist across restarts.
        </p>
        <p v-if="updatedAt" class="text-xs text-[var(--text-muted)]">
          Last saved {{ formatTimeAgo(updatedAt) }}
        </p>
      </div>

      <ErrorBanner :message="saveError" />

      <Card class="space-y-4">
        <header class="flex items-start gap-3">
          <span
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)]"
          >
            <SlidersHorizontal class="h-4 w-4" />
          </span>
          <div class="min-w-0">
            <h3 class="text-sm font-semibold text-[var(--text-primary)]">General</h3>
            <p class="text-xs text-[var(--text-muted)]">Core runtime and session behavior.</p>
          </div>
        </header>
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <FormLabel for="system-log-level">Log Level</FormLabel>
            <Select id="system-log-level" v-model="form.log_level">
              <option value="debug">Debug</option>
              <option value="info">Info</option>
              <option value="warn">Warn</option>
              <option value="error">Error</option>
              <option value="fatal">Fatal</option>
            </Select>
            <p class="mt-1 text-xs text-[var(--text-muted)]">Controls backend log verbosity.</p>
          </div>
          <div>
            <FormLabel for="system-session-expiry">Session Expiry (hours)</FormLabel>
            <NumberInput
              id="system-session-expiry"
              v-model.number="form.session_expiry_hours"
              min="1"
              max="720"
              placeholder="24"
            />
            <p class="mt-1 text-xs text-[var(--text-muted)]">
              How long user sessions remain valid (1–720 hours).
            </p>
          </div>
          <div>
            <FormLabel for="system-environment">Environment</FormLabel>
            <Input
              id="system-environment"
              :model-value="environment || '(not set)'"
              disabled
              class="opacity-60"
            />
            <p class="mt-1 text-xs text-[var(--text-muted)]">Set via server config; read-only.</p>
          </div>
          <div v-if="updatedAt">
            <FormLabel>Last saved</FormLabel>
            <Input :model-value="formatTimeAgo(updatedAt)" disabled class="opacity-60" />
            <p class="mt-1 text-xs text-[var(--text-muted)]">
              When the configuration was last persisted.
            </p>
          </div>
        </div>
      </Card>

      <Card class="space-y-4">
        <header class="flex items-start gap-3">
          <span
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)]"
          >
            <Merge class="h-4 w-4" />
          </span>
          <div class="min-w-0">
            <h3 class="text-sm font-semibold text-[var(--text-primary)]">Alert Correlation</h3>
            <p class="text-xs text-[var(--text-muted)]">
              Group related alerts into shared investigations.
            </p>
          </div>
        </header>
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <FormLabel for="system-correlation-window">Correlation Window</FormLabel>
            <Input
              id="system-correlation-window"
              v-model="form.correlation_window"
              placeholder="5m"
            />
            <p class="mt-1 text-xs text-[var(--text-muted)]">
              Go duration (e.g. 5m, 10m). Set 0 to disable correlation.
            </p>
          </div>
          <div>
            <FormLabel for="system-cooldown-ttl">Correlation Cooldown TTL</FormLabel>
            <Input
              id="system-cooldown-ttl"
              v-model="form.correlation_cooldown_ttl"
              placeholder="30m"
            />
            <p class="mt-1 text-xs text-[var(--text-muted)]">
              Wait before the same fingerprint can start a new investigation.
            </p>
          </div>
        </div>
      </Card>

      <SystemFormFooter
        :dirty="dirty"
        :saving="saving"
        @save="save(changedFields(FIELDS), 'General')"
        @discard="discard(FIELDS)"
      />
    </template>
  </section>
</template>
