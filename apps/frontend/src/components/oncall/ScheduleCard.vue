<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import { User } from "@lucide/vue";
import type { OnCallCurrent, OnCallScheduleRecord } from "@/lib/api";
import { formatTime } from "@/lib/time";
import InteractiveCard from "@/components/ui/InteractiveCard.vue";
defineOptions({ name: "ScheduleCard" });

const props = defineProps<{
  schedule: OnCallScheduleRecord;
  current?: OnCallCurrent | null;
  loading?: boolean;
}>();

const router = useRouter();

function goToSchedule() {
  router.push(`/on-call/schedules/${props.schedule.id}`);
}

const rotationLabel = (type: string) => {
  switch (type) {
    case "daily":
      return "Daily";
    case "weekly":
      return "Weekly";
    default:
      return type;
  }
};

const currentLabel = computed(() => {
  const c = props.current;
  if (!c) return "No one on call";
  return c.user_display_name || c.user_id;
});

const untilLabel = computed(() => {
  const c = props.current;
  if (!c?.until) return "";
  return `until ${formatTime(c.until)}`;
});
</script>

<template>
  <InteractiveCard @navigate="goToSchedule">
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0 flex-1">
        <h3 class="text-sm font-medium text-[var(--text-primary)]">{{ schedule.team_name }}</h3>
        <div class="mt-2 flex flex-wrap items-center gap-2">
          <span
            v-for="layer in schedule.layers"
            :key="layer.id"
            class="inline-flex items-center rounded-full border border-[var(--text-badge-info)] px-2 py-0.5 text-xs text-[var(--text-badge-info)]"
          >
            {{ layer.name }} &middot; {{ rotationLabel(layer.rotation_type) }}
          </span>
        </div>
        <div class="mt-2 flex flex-wrap items-center gap-2 text-xs text-[var(--text-muted)]">
          <span class="inline-flex items-center gap-1">
            <User class="h-3 w-3" />
            <span class="font-medium text-[var(--text-secondary)]">{{ currentLabel }}</span>
            <span>currently on call</span>
          </span>
          <span v-if="untilLabel">&middot; {{ untilLabel }}</span>
        </div>
      </div>
    </div>
  </InteractiveCard>
</template>
