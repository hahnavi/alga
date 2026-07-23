<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { h, onBeforeUnmount, onMounted, ref } from "vue";
import { Clock } from "@lucide/vue";
import { api, type OnCallScheduleRecord, type OnCallCurrent } from "@/lib/api";
import { useToast } from "@/lib/toast";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import ScheduleCard from "@/components/oncall/ScheduleCard.vue";
import { setPageHeader, clearPageHeader } from "@/lib/pageHeader";

defineOptions({ name: "OnCallPage" });

const { push } = useToast();

const schedules = ref<OnCallScheduleRecord[]>([]);
const loading = ref(false);
const error = ref("");

const scheduleCurrentOnCall = ref<Record<string, OnCallCurrent | null>>({});

async function loadCurrentOnCall() {
  const results = await Promise.allSettled(
    schedules.value.map((s) =>
      api
        .getScheduleCurrent(s.id)
        .then((c) => ({ id: s.id, current: c }))
        .catch(() => ({ id: s.id, current: null as OnCallCurrent | null })),
    ),
  );
  for (const r of results) {
    if (r.status === "fulfilled") {
      scheduleCurrentOnCall.value[r.value.id] = r.value.current;
    }
  }
}

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const schedData = await api.getSchedules();
    schedules.value = schedData.items || [];
    await loadCurrentOnCall();
  } catch (err) {
    const msg = getErrorMessage(err, "Failed to load schedules");
    error.value = msg;
    push(msg, "error");
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  setPageHeader("On-Call", undefined, {
    titleIcon: h(Clock, {
      class: "h-5 w-5 shrink-0 text-[var(--text-muted)]",
      "aria-hidden": "true",
    }),
  });
  load();
});

onBeforeUnmount(() => {
  clearPageHeader();
});
</script>

<template>
  <section class="space-y-6 px-4 py-4 md:space-y-8 md:px-6 md:py-6">
    <ErrorBanner :message="error" />
    <LoadingSpinner v-if="loading" centered />

    <template v-if="!loading">
      <div>
        <h2 class="mb-3 text-base font-medium text-[var(--text-primary)]">Schedules</h2>
        <EmptyState v-if="schedules.length === 0" message="No schedules found.">
          <template #icon>
            <Clock class="mb-2 h-6 w-6 opacity-40" />
          </template>
        </EmptyState>
        <div v-else class="space-y-3">
          <div v-for="schedule in schedules" :key="schedule.id" :id="schedule.id">
            <ScheduleCard
              :schedule="schedule"
              :current="scheduleCurrentOnCall[schedule.id] ?? null"
            />
          </div>
        </div>
      </div>
    </template>
  </section>
</template>
