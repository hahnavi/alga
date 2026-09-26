<script setup lang="ts">
import { RouterLink } from "vue-router";
import { CircleDot } from "@lucide/vue";
import type { RelatedAlert, RelatedIncident, UserInfo } from "@/lib/api";
import { incidentPriorityBorderColor } from "@/lib/alertLabels";
import Card from "@/components/ui/Card.vue";
import AlertStatusBadge from "@/components/ui/AlertStatusBadge.vue";
import DeletedBadge from "@/components/ui/DeletedBadge.vue";
import AlertDetailsSidebar from "@/components/AlertDetailsSidebar.vue";

// The sidebar + related-incident/alerts column rendered by the alert detail
// page in both layout states (drawer-open and default). Extracted so the
// ~100-line block isn't duplicated per layout branch.
//
// Mirrors AlertDetailsSidebar's local TimelineEntry shape (icon is rendered
// via <component :is>, so the page's richer icon type assigns structurally).
type TimelineEntry = {
  type: string;
  timestamp: string;
  label: string;
  subline: string;
  dotClass: string;
  lineClass: string;
  iconClass: string;
  icon: unknown;
};

type ResolvedDeliveryTarget = {
  channel: string;
  channelLabel: string;
  icon: string;
  href: string | null;
};

type Assignee = { name: string; isAgent: boolean; agentType?: string };

defineProps<{
  runbookHref: string | null;
  deliveryTargets: ResolvedDeliveryTarget[];
  timeline: TimelineEntry[];
  assignee: Assignee | null;
  users?: UserInfo[];
  canAssign?: boolean;
  assigneeId?: string;
  relatedIncident: RelatedIncident | null;
  relatedAlerts: RelatedAlert[];
}>();

const emit = defineEmits<{
  openDeliveryThread: [target: ResolvedDeliveryTarget];
  assign: [assigneeType: "user" | "agent", assigneeId?: string];
}>();

function incidentBadgeClass(status: string): string {
  if (status === "resolved" || status === "closed") return "badge-green";
  if (status === "mitigated") return "badge-yellow";
  return "badge-red";
}
</script>

<template>
  <AlertDetailsSidebar
    :runbook-href="runbookHref"
    :delivery-targets="deliveryTargets"
    :timeline="timeline"
    :assignee="assignee"
    :users="users"
    :can-assign="canAssign"
    :assignee-id="assigneeId"
    @open-delivery-thread="(t) => emit('openDeliveryThread', t)"
    @assign="(type, id) => emit('assign', type, id)"
  >
    <template #after-notifications>
      <!-- Related Incident -->
      <Card v-if="relatedIncident">
        <div class="mb-3 flex items-center gap-2">
          <h3 class="field-label mb-0">Incident</h3>
        </div>
        <RouterLink
          v-if="!relatedIncident.deleted_at"
          :to="`/incidents/${relatedIncident.incident_number}`"
          class="flex cursor-pointer items-center gap-3 rounded-md border border-[var(--border-primary)] px-3 py-2 transition-colors hover:bg-[var(--bg-secondary)]"
        >
          <CircleDot
            class="h-4 w-4 shrink-0"
            :style="{ color: incidentPriorityBorderColor(relatedIncident.priority) }"
          />
          <div class="min-w-0 flex-1">
            <span class="text-sm font-medium text-[var(--text-primary)]">
              #{{ relatedIncident.incident_number }}
              {{ relatedIncident.title }}
            </span>
          </div>
          <span :class="['badge shrink-0', incidentBadgeClass(relatedIncident.status)]">
            {{ relatedIncident.status.replace("_", " ") }}
          </span>
        </RouterLink>
        <div
          v-else
          class="flex cursor-default items-center gap-3 rounded-md border border-[var(--border-primary)] px-3 py-2 opacity-50 italic"
        >
          <CircleDot
            class="h-4 w-4 shrink-0"
            :style="{ color: incidentPriorityBorderColor(relatedIncident.priority) }"
          />
          <div class="min-w-0 flex-1">
            <span class="text-sm font-medium text-[var(--text-primary)]">
              #{{ relatedIncident.incident_number }}
              {{ relatedIncident.title }}
            </span>
          </div>
          <DeletedBadge class="shrink-0" title="This incident was deleted" />
          <span :class="['badge shrink-0', incidentBadgeClass(relatedIncident.status)]">
            {{ relatedIncident.status.replace("_", " ") }}
          </span>
        </div>
      </Card>
    </template>
  </AlertDetailsSidebar>

  <!-- Related Alerts -->
  <Card v-if="relatedAlerts.length > 0">
    <div class="mb-3">
      <h3 class="field-label mb-0">Related Alerts</h3>
      <p class="mt-0.5 text-xs text-[var(--text-muted)]">
        Correlated alerts from the same investigation
      </p>
    </div>
    <div class="space-y-2">
      <template v-for="ra in relatedAlerts" :key="ra.fingerprint">
        <!-- alert_number is the canonical identity and the only routable one;
             a row without it renders inert instead of linking to a dead path. -->
        <RouterLink
          v-if="ra.alert_number"
          :to="`/alerts/${ra.alert_number}`"
          class="flex cursor-pointer items-center gap-3 rounded-md border border-[var(--border-primary)] px-3 py-2 transition-colors hover:bg-[var(--bg-secondary)]"
        >
          <CircleDot class="h-4 w-4 shrink-0 text-[var(--text-muted)]" />
          <div class="min-w-0 flex-1">
            <span class="text-sm text-[var(--text-primary)]">
              {{ ra.labels?.alertname || ra.fingerprint }}
            </span>
            <span class="ml-1 text-xs text-[var(--text-muted)]">#{{ ra.alert_number }}</span>
          </div>
          <AlertStatusBadge :status="ra.status" class="shrink-0" />
        </RouterLink>
        <div
          v-else
          class="flex cursor-default items-center gap-3 rounded-md border border-[var(--border-primary)] px-3 py-2"
          aria-disabled="true"
        >
          <CircleDot class="h-4 w-4 shrink-0 text-[var(--text-muted)]" />
          <div class="min-w-0 flex-1">
            <span class="text-sm text-[var(--text-primary)]">
              {{ ra.labels?.alertname || ra.fingerprint }}
            </span>
          </div>
          <AlertStatusBadge :status="ra.status" class="shrink-0" />
        </div>
      </template>
    </div>
  </Card>
</template>
