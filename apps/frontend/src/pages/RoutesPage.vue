<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { h, onMounted, ref, computed } from "vue";
import { ArrowRight, Route } from "@lucide/vue";
import { api, type RouteCondition, type RouteConfig, type RouteTarget } from "@/lib/api";
import {
  summarizeCondition,
  CONDITION_SOURCE_OPTIONS,
  CONDITION_OPERATOR_OPTIONS,
} from "@/lib/routeConditions";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import Input from "@/components/ui/Input.vue";
import Checkbox from "@/components/ui/Checkbox.vue";
import Select from "@/components/ui/Select.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import { useToast } from "@/lib/toast";
import { useDelete } from "@/composables/useDelete";
import { useFormSubmit } from "@/composables/useFormSubmit";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import Modal from "@/components/ui/Modal.vue";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { useListFilter } from "@/composables/useListFilter";

defineOptions({ name: "RoutesPage" });

type ConditionEntry = Omit<RouteCondition, "value"> & { value: string };
type TargetForm = { provider: "mattermost" | "slack"; channel: string };
type RouteForm = {
  matchMode: "all" | "any";
  conditions: ConditionEntry[];
  targets: TargetForm[];
  silenced: boolean;
};

const routes = ref<RouteForm[]>([]);
const defaultDestinations = ref<RouteTarget[]>([]);
const loading = ref(false);
const { submitting: saving, formError, withSubmit } = useFormSubmit();
const error = ref("");
const editingIndex = ref<number | null>(null);
const routesSearchInput = ref("");
const { push } = useToast();
const { canWrite } = useEntityPermissions("routes");
const channelOptions = ref<Record<string, { name: string; display_name?: string }[]>>({});

const routeEditorOpen = computed({
  get: () => editingIndex.value !== null,
  set: (val: boolean) => {
    if (!val) editingIndex.value = null;
  },
});

const {
  showDeleteConfirm,
  confirmDelete: confirmDeleteRoute,
  doDelete: doDeleteRoute,
} = useDelete(async (idx: number) => {
  routes.value.splice(idx, 1);
}, "Route");

function fromRouteConfig(route: RouteConfig): RouteForm {
  const conditions: ConditionEntry[] =
    (route.conditions as RouteCondition[] | undefined)?.map((condition) => ({
      source: condition.source,
      field: condition.field,
      operator: condition.operator,
      value: condition.value ?? "",
    })) ?? [];

  const targets: TargetForm[] =
    route.targets && route.targets.length > 0
      ? route.targets.map((t) => ({
          provider: (t.provider ?? "mattermost") as "mattermost" | "slack",
          channel: t.channel,
        }))
      : [{ provider: "mattermost", channel: "alerts" }];

  return {
    matchMode: route.match_mode ?? "all",
    conditions: conditions.length
      ? conditions
      : [{ source: "labels", field: "severity", operator: "exact", value: "critical" }],
    targets,
    silenced: route.silenced ?? false,
  };
}

function toRouteConfig(route: RouteForm): RouteConfig {
  const conditions = route.conditions.map((condition) => ({
    source: condition.source,
    field: condition.field.trim(),
    operator: condition.operator,
    value: condition.value?.trim(),
  }));
  if (route.silenced) {
    return {
      match_mode: route.matchMode,
      conditions,
      silenced: true,
    };
  }
  const targets = route.targets.map((t) => ({
    provider: t.provider,
    channel: t.channel.trim(),
  }));
  return {
    match_mode: route.matchMode,
    conditions,
    targets,
    silenced: false,
  };
}

function targetSummary(targets: TargetForm[]) {
  return "Forward to " + targets.map((t) => `${t.provider}:${t.channel}`).join(", ");
}

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [data, mmChannels, slackDests] = await Promise.all([
      api.getRoutes(),
      api.getChannels().catch(() => [] as { name: string; display_name?: string }[]),
      api.getDestinations("slack").catch(() => [] as { name: string; id?: string }[]),
    ]);
    routes.value = data.routes.map(fromRouteConfig);
    defaultDestinations.value = data.default_destinations ?? [];
    channelOptions.value = {
      mattermost: mmChannels,
      slack: slackDests.map((d) => ({ name: d.name, display_name: d.name })),
    };
  } catch (err) {
    error.value = getErrorMessage(err, "Failed to load routes");
  } finally {
    loading.value = false;
  }
}

function addRoute() {
  routes.value.push({
    matchMode: "all",
    conditions: [{ source: "labels", field: "severity", operator: "exact", value: "critical" }],
    targets: [{ provider: "mattermost", channel: "alerts" }],
    silenced: false,
  });
  editingIndex.value = routes.value.length - 1;
}

function addCondition(routeIdx: number) {
  routes.value[routeIdx].conditions.push({
    source: "labels",
    field: "",
    operator: "exact",
    value: "",
  });
}

function removeCondition(routeIdx: number, matchIdx: number) {
  const entries = routes.value[routeIdx].conditions;
  entries.splice(matchIdx, 1);
  if (entries.length === 0) {
    entries.push({ source: "labels", field: "severity", operator: "exact", value: "critical" });
  }
}

function addTarget(routeIdx: number) {
  routes.value[routeIdx].targets.push({ provider: "mattermost", channel: "" });
}

function removeTarget(routeIdx: number, targetIdx: number) {
  const list = routes.value[routeIdx].targets;
  list.splice(targetIdx, 1);
  if (list.length === 0) {
    list.push({ provider: "mattermost", channel: "alerts" });
  }
}

async function save() {
  error.value = "";
  const normalized = routes.value.map(toRouteConfig);
  if (
    normalized.some(
      (route) =>
        !route.silenced &&
        (!route.targets ||
          route.targets.length === 0 ||
          route.targets.some((t) => !t.channel.trim())),
    )
  ) {
    error.value = "Each non-silenced route needs at least one non-empty destination channel.";
    push("Fix destination targets before saving", "error");
    return;
  }
  if (normalized.some((route) => (route.conditions?.length ?? 0) === 0)) {
    error.value = "Each route must include at least one matcher.";
    push("Fix route fields before saving", "error");
    return;
  }

  await withSubmit(async () => {
    await api.updateRoutes({ routes: normalized });
    await load();
  }, "Routes updated successfully");
}

function onEditorClosed(open: boolean) {
  if (!open) editingIndex.value = null;
}

usePageHeaderActions({
  title: "Routes",
  titleIcon: h(Route, {
    class: "h-5 w-5 shrink-0 text-[var(--text-muted)]",
    "aria-hidden": "true",
  }),
  searchInput: routesSearchInput,
  searchPlaceholder: "Search routes...",
  showAdd: canWrite,
  onAdd: addRoute,
  addLabel: "Add rule",
});

const filteredRoutes = useListFilter(
  routes,
  [(r) => targetSummary(r.targets), (r) => summarizeCondition(r.conditions[0])],
  routesSearchInput,
);

function routeIndex(item: RouteForm): number {
  return routes.value.indexOf(item);
}

onMounted(() => {
  load();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <p class="text-sm text-[var(--text-muted)]">
      Alerts are always sent to the default channels from Integration Settings. Routes forward
      matching alerts to <strong>additional</strong> channels. Match <strong>all</strong> means
      every condition must match (AND); match <strong>any</strong> means at least one (OR).
    </p>

    <LoadingSpinner v-if="loading" centered />

    <div v-if="defaultDestinations.length > 0 && !loading" class="space-y-3">
      <div class="flex items-center gap-2 text-sm font-medium text-[var(--text-muted)]">
        <span>Primary Channels</span>
        <span
          class="rounded bg-[var(--bg-code)] px-1.5 py-0.5 text-xs font-semibold text-[var(--text-muted)]"
          >Always active</span
        >
      </div>
      <Card v-for="(dest, didx) in defaultDestinations" :key="didx">
        <div class="flex items-center gap-3">
          <span class="text-sm font-medium capitalize">{{ dest.provider ?? "mattermost" }}</span>
          <ArrowRight class="h-3.5 w-3.5 text-[var(--text-muted)]" />
          <span class="text-sm text-[var(--text-secondary)]">{{ dest.channel }}</span>
          <router-link
            to="/communication-channels"
            class="ml-auto text-xs text-[var(--text-muted)] underline-offset-2 transition-colors hover:text-[var(--text-primary)] hover:underline focus-visible:rounded-sm focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
          >
            Configure in Channel Settings
          </router-link>
        </div>
      </Card>
    </div>

    <div
      v-if="defaultDestinations.length > 0 && routes.length > 0 && !loading"
      class="mt-2 text-sm font-medium text-[var(--text-muted)]"
    >
      Additional Forwarding Rules
    </div>

    <Card v-if="!loading && routes.length === 0">
      <EmptyState message="No forwarding rules configured.">
        <template #footer>
          <p class="mt-1 text-xs text-[var(--text-muted)]">
            Add a rule to forward specific alerts to additional channels.
          </p>
        </template>
      </EmptyState>
    </Card>

    <div class="space-y-3">
      <Card v-for="item in filteredRoutes" :key="routeIndex(item)">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="space-y-1">
            <p class="text-sm font-medium">Route {{ routeIndex(item) + 1 }}</p>
            <p class="text-sm text-[var(--text-muted)]">
              <span v-if="item.silenced" class="mr-2 badge-yellow">Silenced</span>
              <span v-else>{{ targetSummary(item.targets) }}</span>
            </p>
            <p class="text-xs text-[var(--text-muted)]">
              {{ item.matchMode === "all" ? "Match all" : "Match any" }} of
              {{ item.conditions.length }} matcher(s)
            </p>
            <p class="text-xs text-[var(--text-muted)]">
              {{ summarizeCondition(item.conditions[0]) }}
              <span v-if="item.conditions.length > 1">
                (+{{ item.conditions.length - 1 }} more)</span
              >
            </p>
          </div>
          <div class="flex gap-2">
            <Button size="sm" @click="editingIndex = routeIndex(item)">Edit</Button>
            <Button size="sm" variant="destructive" @click="confirmDeleteRoute(routeIndex(item))"
              >Delete</Button
            >
          </div>
        </div>
      </Card>
    </div>

    <div class="flex gap-3">
      <Button :loading="saving" @click="save">Save</Button>
    </div>

    <ErrorBanner :message="formError || error" />
  </section>

  <Modal
    v-model:open="routeEditorOpen"
    title="Add route"
    maxWidth="5xl"
    :loading="saving"
    confirmLabel="Save"
    @confirm="save"
    @update:open="onEditorClosed"
  >
    <template v-if="editingIndex !== null && routes[editingIndex]">
      <div class="mb-3 flex items-center gap-4">
        <label class="flex items-center gap-2 text-sm">
          <Checkbox v-model="routes[editingIndex].silenced" />
          <span>Silenced</span>
        </label>
      </div>

      <div class="mb-2 grid gap-2 md:grid-cols-2">
        <Select id="match-mode" v-model="routes[editingIndex].matchMode">
          <option value="all">Match all conditions (AND)</option>
          <option value="any">Match any condition (OR)</option>
        </Select>
      </div>

      <div v-if="!routes[editingIndex].silenced" class="mb-4 space-y-2">
        <p class="text-sm font-medium">Destinations</p>
        <p class="text-xs text-[var(--text-muted)]">
          Send matching alerts to every integration below.
        </p>
        <div
          v-for="(tgt, tidx) in routes[editingIndex].targets"
          :key="`${editingIndex}-t-${tidx}`"
          class="grid gap-2 md:grid-cols-[1fr_1fr_auto]"
        >
          <Select v-model="tgt.provider">
            <option value="mattermost">mattermost</option>
            <option value="slack">slack</option>
          </Select>
          <Select v-model="tgt.channel">
            <option value="">
              {{ tgt.provider === "slack" ? "Select a channel" : "Select a channel" }}
            </option>
            <option
              v-for="ch in channelOptions[tgt.provider] || []"
              :key="ch.name"
              :value="ch.name"
            >
              {{ ch.display_name || ch.name }}
            </option>
          </Select>
          <Button size="sm" variant="destructive" @click="removeTarget(editingIndex, tidx)"
            >Remove</Button
          >
        </div>
        <Button size="sm" @click="addTarget(editingIndex)">Add destination</Button>
      </div>

      <div class="space-y-2">
        <p class="text-sm font-medium">Matchers</p>
        <div
          v-for="(entry, matchIdx) in routes[editingIndex].conditions"
          :key="`${editingIndex}-${matchIdx}`"
          class="grid gap-2 md:grid-cols-[1fr_1fr_1fr_1fr_auto]"
        >
          <Select v-model="entry.source">
            <option v-for="s in CONDITION_SOURCE_OPTIONS" :key="s.value" :value="s.value">
              {{ s.label }}
            </option>
          </Select>
          <Input v-model="entry.field" placeholder="field (e.g. severity, status)" />
          <Select v-model="entry.operator">
            <option v-for="o in CONDITION_OPERATOR_OPTIONS" :key="o.value" :value="o.value">
              {{ o.label }}
            </option>
          </Select>
          <Input
            v-model="entry.value"
            :placeholder="
              entry.operator === 'exists' || entry.operator === 'not_exists'
                ? 'No value needed'
                : 'value'
            "
            :disabled="entry.operator === 'exists' || entry.operator === 'not_exists'"
          />
          <Button size="sm" variant="destructive" @click="removeCondition(editingIndex, matchIdx)"
            >Remove</Button
          >
        </div>
        <Button size="sm" @click="addCondition(editingIndex)">Add matcher</Button>
      </div>
    </template>
  </Modal>

  <ConfirmDialog
    v-model:open="showDeleteConfirm"
    title="Delete route"
    :message="'Are you sure you want to delete this route?'"
    confirm-label="Delete"
    :destructive="true"
    @confirm="doDeleteRoute"
  />
</template>
