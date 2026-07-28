<script setup lang="ts">
import { computed, onMounted } from "vue";
import { Bot, CalendarClock, Workflow } from "@lucide/vue";
import Card from "@/components/ui/Card.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Input from "@/components/ui/Input.vue";
import NumberInput from "@/components/ui/NumberInput.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import SkeletonRows from "@/components/ui/SkeletonRows.vue";
import SystemFormFooter from "@/components/system/SystemFormFooter.vue";
import { useSystemConfigForm, type SystemForm } from "@/composables/useSystemConfigForm";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";

defineOptions({ name: "SystemInvestigationsPage" });

const FIELDS: ReadonlyArray<keyof SystemForm> = [
  "investigation_timeout",
  "max_concurrent_investigations",
  "agent_presence_ttl",
  "agent_disconnect_grace",
  "scheduler_leader_ttl",
];

const {
  form,
  original,
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
  title: "Investigations",
  titleIcon: Workflow,
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
      <p class="text-sm text-[var(--text-muted)]">
        Throughput, timeouts, and scheduler behavior for investigations.
      </p>

      <ErrorBanner :message="saveError" />

      <Card class="space-y-4">
        <header class="flex items-start gap-3">
          <span
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)]"
          >
            <Workflow class="h-4 w-4" />
          </span>
          <div class="min-w-0">
            <h3 class="text-sm font-semibold text-[var(--text-primary)]">Investigation Pipeline</h3>
            <p class="text-xs text-[var(--text-muted)]">
              Throughput and timeouts for investigations.
            </p>
          </div>
        </header>
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <FormLabel for="system-investigation-timeout">Investigation Timeout</FormLabel>
            <Input
              id="system-investigation-timeout"
              v-model="form.investigation_timeout"
              placeholder="10m"
            />
            <p class="mt-1 text-xs text-[var(--text-muted)]">
              Max Go duration before an investigation is force-completed.
            </p>
          </div>
          <div>
            <FormLabel for="system-max-investigations">Max Concurrent Investigations</FormLabel>
            <NumberInput
              id="system-max-investigations"
              v-model.number="form.max_concurrent_investigations"
              min="1"
              placeholder="3"
            />
            <p class="mt-1 text-xs text-[var(--text-muted)]">
              Cap of investigations the scheduler runs in parallel.
            </p>
          </div>
        </div>
      </Card>

      <Card class="space-y-4">
        <header class="flex items-start gap-3">
          <span
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)]"
          >
            <Bot class="h-4 w-4" />
          </span>
          <div class="min-w-0">
            <h3 class="text-sm font-semibold text-[var(--text-primary)]">Agent Settings</h3>
            <p class="text-xs text-[var(--text-muted)]">
              Presence and liveness detection for agents.
            </p>
          </div>
        </header>
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <FormLabel for="system-agent-presence-ttl">Agent Presence TTL</FormLabel>
            <Input
              id="system-agent-presence-ttl"
              v-model="form.agent_presence_ttl"
              placeholder="90s"
            />
            <p class="mt-1 text-xs text-[var(--text-muted)]">How long presence heartbeats count.</p>
          </div>
          <div>
            <FormLabel for="system-agent-disconnect-grace">Agent Disconnect Grace</FormLabel>
            <Input
              id="system-agent-disconnect-grace"
              v-model="form.agent_disconnect_grace"
              placeholder="45s"
            />
            <p class="mt-1 text-xs text-[var(--text-muted)]">
              Buffer before a missed heartbeat marks an agent offline.
            </p>
          </div>
        </div>
      </Card>

      <Card class="space-y-4">
        <header class="flex items-start gap-3">
          <span
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)]"
          >
            <CalendarClock class="h-4 w-4" />
          </span>
          <div class="min-w-0">
            <h3 class="text-sm font-semibold text-[var(--text-primary)]">Scheduler</h3>
            <p class="text-xs text-[var(--text-muted)]">
              Leader election for the investigation scheduler.
            </p>
          </div>
        </header>
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <FormLabel for="system-leader-ttl">Leader Lease TTL</FormLabel>
            <Input id="system-leader-ttl" v-model="form.scheduler_leader_ttl" placeholder="15s" />
            <p class="mt-1 text-xs text-[var(--text-muted)]">
              Go duration. Set 0 for single-replica deployments.
            </p>
          </div>
        </div>
      </Card>

      <SystemFormFooter
        :dirty="dirty"
        :saving="saving"
        @save="save(changedFields(FIELDS), 'Investigations')"
        @discard="discard(FIELDS)"
      />
    </template>
  </section>
</template>
