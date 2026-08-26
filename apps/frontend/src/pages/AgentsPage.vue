<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { computed, onMounted, ref } from "vue";
import { Plus, MessageSquare, Bot, Wifi, WifiOff, Clock, Shield } from "@lucide/vue";
import {
  api,
  type AgentTokenRow,
  type AgentType,
  type AgentCapability,
  type RouteCondition,
} from "@/lib/api";
import AgentActionsMenu from "@/components/ui/AgentActionsMenu.vue";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import Select from "@/components/ui/Select.vue";
import Modal from "@/components/ui/Modal.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import SecretDisplay from "@/components/ui/SecretDisplay.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import Card from "@/components/ui/Card.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import ConditionEditor from "@/components/ui/ConditionEditor.vue";
import DateTimePicker from "@/components/ui/DateTimePicker.vue";
import { useToast } from "@/lib/toast";
import { useDelete } from "@/composables/useDelete";
import { useClipboard } from "@/composables/useClipboard";
import { useSSE } from "@/composables/useSSE";
import { useEscapeKey } from "@/composables/useEscapeKey";
import { useListFilter } from "@/composables/useListFilter";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { formatExpires, localDatetimeToRFC3339 } from "@/lib/time";
import { getAgentAvatarSrc } from "@/lib/agentAvatar";
defineOptions({ name: "AgentsPage" });

const { push } = useToast();
const { copyToClipboard } = useClipboard();

const agentTokens = ref<AgentTokenRow[]>([]);
const agentTokensLoading = ref(false);
const agentSubmitting = ref(false);
const togglingAgentId = ref<string | null>(null);
const agentError = ref("");
const agentSearchQuery = ref("");
const agentNewName = ref("");
const agentNewExpiresLocal = ref("");
const agentNewType = ref<AgentType>("alga");
const agentNewScope = ref<"all" | "labels">("all");
const agentNewLabelSelectors = ref<RouteCondition[]>([]);
const agentNewCapabilities = ref<AgentCapability[]>(["investigate"]);
const capabilityFilter = ref<AgentCapability | "">("");
const agentCreatedSecret = ref<string | null>(null);

const agentCapabilities: Record<AgentCapability, { label: string; color: string }> = {
  investigate: {
    label: "Investigate",
    color: "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
  },
  communicate: {
    label: "Communicate",
    color: "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
  },
  command: {
    label: "Command",
    color: "bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200",
  },
  secrets: {
    label: "Secrets",
    color: "bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200",
  },
};
const showAgentDialog = ref(false);

useSSE("/api/v1/events", {
  agent_presence: (data: unknown) => {
    const d = data as { agent_id?: string; online?: boolean };
    const id = d.agent_id;
    if (!id) return;
    const row = agentTokens.value.find((x) => x.id === id);
    if (row) row.online = !!d.online;
  },
});

const {
  deleteTarget: agentTokenToDelete,
  showDeleteConfirm: showAgentDeleteConfirm,
  confirmDelete: confirmDeleteAgentToken,
  doDelete: doDeleteAgentToken,
} = useDelete<AgentTokenRow>(async (token) => {
  await api.revokeAgentToken(token.id);
  agentTokens.value = await api.getAgentTokens();
}, "Agent");

const showAgentScopeDialog = ref(false);
const agentScopeEditToken = ref<AgentTokenRow | null>(null);
const agentScopeEditScope = ref<"all" | "labels">("all");
const agentScopeEditLabelSelectors = ref<RouteCondition[]>([]);
const agentScopeEditCapabilities = ref<string[]>([]);
const agentScopeSubmitting = ref(false);

const showAgentRegenerateConfirm = ref(false);
const agentTokenToRegenerate = ref<AgentTokenRow | null>(null);
const regeneratingAgentId = ref<string | null>(null);
const showAgentRegeneratedSecret = ref(false);
const agentRegeneratedSecret = ref<string | null>(null);
const agentRegeneratedName = ref("");

const regenerateConfirmMessage = computed(() => {
  const t = agentTokenToRegenerate.value;
  if (!t) return "";
  return `Replace the bot token for "${t.name}"? The current token stops working immediately. Update every integration, agent, or script that still uses the old token before you continue.`;
});

const onlineCount = computed(
  () => agentTokens.value.filter((t) => t.online === true && !t.expired).length,
);
const offlineCount = computed(
  () => agentTokens.value.filter((t) => t.online !== true && !t.expired).length,
);
const disabledCount = computed(
  () => agentTokens.value.filter((t) => !t.enabled && !t.expired).length,
);

const searchFilteredAgents = useListFilter(
  agentTokens,
  ["name", (t) => t.agent_type ?? "hermes"],
  agentSearchQuery,
  { sort: (a, b) => a.name.localeCompare(b.name) },
);

const filteredAgents = computed(() => {
  if (!capabilityFilter.value) return searchFilteredAgents.value;
  return searchFilteredAgents.value.filter((t) =>
    t.capabilities?.includes(capabilityFilter.value as AgentCapability),
  );
});

const capabilityCounts = computed(() => {
  const counts: Record<AgentCapability, number> = { investigate: 0, communicate: 0, command: 0 };
  for (const t of agentTokens.value) {
    for (const cap of t.capabilities ?? []) {
      if (cap in counts) counts[cap] += 1;
    }
  }
  return counts;
});

const emptyFilterMessage = computed(() => {
  if (capabilityFilter.value) {
    const label = agentCapabilities[capabilityFilter.value].label;
    return agentSearchQuery.value.trim()
      ? `No agents with the ${label} capability match your search.`
      : `No agents have the ${label} capability.`;
  }
  return "No agents match your search.";
});

function toggleCapabilityFilter(cap: AgentCapability | "") {
  capabilityFilter.value = capabilityFilter.value === cap ? "" : cap;
}

function capabilityPillClass(active: boolean): string {
  return active
    ? "border-[var(--btn-default-border)] bg-[var(--btn-default-bg)] text-[var(--btn-default-text)]"
    : "border-[var(--border-primary)] bg-transparent text-[var(--text-muted)] hover:border-[var(--border-secondary)] hover:text-[var(--text-secondary)]";
}

async function loadAgentTokens() {
  agentTokensLoading.value = true;
  try {
    agentTokens.value = await api.getAgentTokens();
  } catch (err) {
    agentError.value = getErrorMessage(err, "Failed to load agents");
  } finally {
    agentTokensLoading.value = false;
  }
}

function openAgentDialog() {
  resetAgentForm();
  showAgentDialog.value = true;
}

function closeAgentDialog() {
  showAgentDialog.value = false;
  resetAgentForm();
}

function resetAgentForm() {
  agentError.value = "";
  agentCreatedSecret.value = null;
  agentNewName.value = "";
  agentNewExpiresLocal.value = "";
  agentNewType.value = "alga";
  agentNewScope.value = "all";
  agentNewLabelSelectors.value = [];
  agentNewCapabilities.value = ["investigate"];
}

function resolvedAgentType(t: AgentTokenRow): AgentType {
  const s = t.agent_type ?? "hermes";
  if (s === "alga" || s === "openclaw" || s === "other") return s;
  return "hermes";
}

function agentIconForType(t: AgentTokenRow) {
  return getAgentAvatarSrc(resolvedAgentType(t));
}

function agentOnlineLabel(t: AgentTokenRow): string {
  const type = resolvedAgentType(t);
  if (type === "alga") return "Alga Agent connected via SSE";
  if (type === "openclaw") return "OpenClaw adapter connected via SSE";
  if (type === "other") return "Agent connected via SSE";
  return "Hermes adapter connected via SSE";
}

function agentTypeLabel(t: AgentTokenRow): string {
  const type = resolvedAgentType(t);
  if (type === "alga") return "Alga Agent";
  if (type === "openclaw") return "OpenClaw";
  if (type === "other") return "Other";
  return "Hermes";
}

async function submitAgentToken() {
  const name = agentNewName.value.trim();
  if (!name) {
    agentError.value = "Agent name is required.";
    push("Agent name is required", "error");
    return;
  }
  let expiresAt: string | undefined;
  if (agentNewExpiresLocal.value.trim()) {
    const iso = localDatetimeToRFC3339(agentNewExpiresLocal.value);
    if (!iso) {
      push("Invalid expiration date", "error");
      return;
    }
    expiresAt = iso;
  }
  agentSubmitting.value = true;
  agentError.value = "";
  try {
    const res = await api.createAgentToken(
      name,
      expiresAt,
      agentNewType.value,
      agentNewScope.value,
      agentNewScope.value === "labels" ? agentNewLabelSelectors.value : undefined,
      agentNewCapabilities.value,
    );
    agentCreatedSecret.value = res.token;
    push("Agent bot token created", "success");
    agentTokens.value = await api.getAgentTokens();
  } catch (err) {
    agentError.value = getErrorMessage(err, "Failed to create agent bot token");
  } finally {
    agentSubmitting.value = false;
  }
}

async function copyAgentSecret() {
  if (!agentCreatedSecret.value) return;
  await copyToClipboard(agentCreatedSecret.value, "Secret copied");
}

function openAgentEditScope(token: AgentTokenRow) {
  agentScopeEditToken.value = token;
  agentScopeEditScope.value = token.scope === "labels" ? "labels" : "all";
  agentScopeEditLabelSelectors.value = token.label_selectors?.length
    ? token.label_selectors.map((c) => ({ ...c }))
    : [];
  agentScopeEditCapabilities.value = token.capabilities?.length ? [...token.capabilities] : [];
  showAgentScopeDialog.value = true;
}

function openAgentRegenerate(token: AgentTokenRow) {
  agentTokenToRegenerate.value = token;
  showAgentRegenerateConfirm.value = true;
}

function deleteAgent(token: AgentTokenRow) {
  confirmDeleteAgentToken(token);
}

function onAgentRegenerateCancel() {
  agentTokenToRegenerate.value = null;
}

async function doRegenerateAgentToken() {
  const t = agentTokenToRegenerate.value;
  if (!t || regeneratingAgentId.value) return;
  regeneratingAgentId.value = t.id;
  try {
    const res = await api.regenerateAgentToken(t.id);
    showAgentRegenerateConfirm.value = false;
    agentTokenToRegenerate.value = null;
    agentRegeneratedSecret.value = res.token;
    agentRegeneratedName.value = res.name;
    showAgentRegeneratedSecret.value = true;
    push("Bot token regenerated", "success");
    agentTokens.value = await api.getAgentTokens();
  } catch (err) {
    push(getErrorMessage(err, "Failed to regenerate token"), "error");
  } finally {
    regeneratingAgentId.value = null;
  }
}

function closeAgentRegeneratedSecretDialog() {
  showAgentRegeneratedSecret.value = false;
  agentRegeneratedSecret.value = null;
  agentRegeneratedName.value = "";
}

async function copyAgentRegeneratedSecret() {
  if (!agentRegeneratedSecret.value) return;
  await copyToClipboard(agentRegeneratedSecret.value, "Secret copied");
}

async function toggleAgentEnabled(token: AgentTokenRow) {
  if (token.expired) return;
  togglingAgentId.value = token.id;
  try {
    const newEnabled = !token.enabled;
    await api.updateAgentToken(token.id, token.scope ?? "all", token.label_selectors, newEnabled);
    push(newEnabled ? "Agent enabled" : "Agent disabled", "success");
    agentTokens.value = await api.getAgentTokens();
  } catch (err) {
    push(getErrorMessage(err, "Failed to toggle agent"), "error");
  } finally {
    togglingAgentId.value = null;
  }
}

function closeAgentScopeDialog() {
  showAgentScopeDialog.value = false;
  agentScopeEditToken.value = null;
}

async function submitAgentScopeEdit() {
  const token = agentScopeEditToken.value;
  if (!token || agentScopeSubmitting.value) return;
  agentScopeSubmitting.value = true;
  try {
    const selectors =
      agentScopeEditScope.value === "labels" ? agentScopeEditLabelSelectors.value : [];
    await api.updateAgentToken(
      token.id,
      agentScopeEditScope.value,
      selectors,
      undefined,
      agentScopeEditCapabilities.value as AgentCapability[],
    );
    push("Investigation scope updated", "success");
    closeAgentScopeDialog();
    agentTokens.value = await api.getAgentTokens();
  } catch (err) {
    push(getErrorMessage(err, "Failed to update scope"), "error");
  } finally {
    agentScopeSubmitting.value = false;
  }
}

function handleEscape() {
  if (showAgentRegeneratedSecret.value) {
    closeAgentRegeneratedSecretDialog();
    return;
  }
  if (showAgentRegenerateConfirm.value) {
    showAgentRegenerateConfirm.value = false;
    onAgentRegenerateCancel();
    return;
  }
  if (showAgentDeleteConfirm.value) return;
  if (showAgentScopeDialog.value) {
    closeAgentScopeDialog();
    return;
  }
  if (showAgentDialog.value) {
    closeAgentDialog();
  }
}

useEscapeKey(handleEscape, () => true);

usePageHeaderActions({
  title: "Agents",
  titleIcon: Bot,
  searchInput: agentSearchQuery,
  searchPlaceholder: "Search agents...",
  onAdd: openAgentDialog,
  addLabel: "Add agent",
});

onMounted(() => {
  loadAgentTokens();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <ErrorBanner :message="agentError" />

    <div class="flex items-center justify-between gap-4">
      <p class="text-sm text-[var(--text-muted)]">
        Manage investigation agents. Each agent has a secret token (shown once) for authenticating
        to Alga and running automated investigations.
      </p>
    </div>

    <LoadingSpinner v-if="agentTokensLoading" centered />

    <template v-else-if="agentTokens.length">
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Card class="flex items-center gap-3">
          <div
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--bg-secondary)]"
          >
            <Bot class="h-4 w-4 text-[var(--text-secondary)]" />
          </div>
          <div>
            <div class="text-xl font-bold text-[var(--text-primary)]">{{ agentTokens.length }}</div>
            <div class="text-xs text-[var(--text-muted)]">Total</div>
          </div>
        </Card>
        <Card class="flex items-center gap-3">
          <div
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--bg-badge-resolved)]"
          >
            <Wifi class="h-4 w-4 text-[var(--text-online)]" />
          </div>
          <div>
            <div class="text-xl font-bold text-[var(--text-primary)]">{{ onlineCount }}</div>
            <div class="text-xs text-[var(--text-muted)]">Online</div>
          </div>
        </Card>
        <Card class="flex items-center gap-3">
          <div
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--bg-secondary)]"
          >
            <WifiOff class="h-4 w-4 text-[var(--text-muted)]" />
          </div>
          <div>
            <div class="text-xl font-bold text-[var(--text-primary)]">{{ offlineCount }}</div>
            <div class="text-xs text-[var(--text-muted)]">Offline</div>
          </div>
        </Card>
        <Card class="flex items-center gap-3">
          <div
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--bg-badge-warning)]"
          >
            <Shield class="h-4 w-4 text-[var(--text-badge-warning)]" />
          </div>
          <div>
            <div class="text-xl font-bold text-[var(--text-primary)]">{{ disabledCount }}</div>
            <div class="text-xs text-[var(--text-muted)]">Disabled</div>
          </div>
        </Card>
      </div>

      <div class="flex flex-wrap items-center justify-between gap-2">
        <div
          class="flex flex-wrap items-center gap-1.5"
          role="group"
          aria-label="Filter by capability"
        >
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium transition-colors cursor-pointer focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
            :class="capabilityPillClass(!capabilityFilter)"
            :aria-pressed="!capabilityFilter"
            @click="toggleCapabilityFilter('')"
          >
            All
            <span class="tabular-nums opacity-60">{{ agentTokens.length }}</span>
          </button>
          <button
            v-for="(info, key) in agentCapabilities"
            :key="key"
            type="button"
            class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium transition-colors cursor-pointer focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
            :class="capabilityPillClass(capabilityFilter === key)"
            :aria-pressed="capabilityFilter === key"
            @click="toggleCapabilityFilter(key)"
          >
            {{ info.label }}
            <span class="tabular-nums opacity-60">{{ capabilityCounts[key] }}</span>
          </button>
        </div>
        <p v-if="capabilityFilter" class="text-xs text-[var(--text-muted)]">
          {{ filteredAgents.length }} of {{ agentTokens.length }} agents
        </p>
      </div>

      <div class="space-y-3">
        <Card
          v-for="t in filteredAgents"
          :key="t.id"
          class="relative transition-colors"
          :class="{
            'opacity-50': t.expired,
            'border-[var(--border-secondary)]': t.online === true && !t.expired,
          }"
        >
          <div class="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
            <div class="flex min-w-0 items-start gap-3">
              <div
                class="shrink-0 rounded-full p-0.5"
                :class="t.online === true && !t.expired ? 'ring-2 ring-[var(--bg-online)]' : ''"
                :title="t.online === true && !t.expired ? agentOnlineLabel(t) : undefined"
              >
                <div
                  class="flex h-10 w-10 items-center justify-center overflow-hidden rounded-full bg-[var(--bg-secondary)]"
                >
                  <img
                    :src="agentIconForType(t)"
                    :alt="agentTypeLabel(t)"
                    class="h-full w-full rounded-full object-cover"
                    loading="lazy"
                    decoding="async"
                  />
                </div>
              </div>
              <div class="min-w-0 pr-8 md:pr-0">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="text-sm font-semibold text-[var(--text-primary)]">{{ t.name }}</span>
                  <span
                    v-if="t.online === true && !t.expired"
                    class="inline-block h-2 w-2 rounded-full bg-[var(--bg-online)] shrink-0"
                    :title="agentOnlineLabel(t)"
                  />
                  <span
                    v-else-if="!t.expired"
                    class="inline-block h-2 w-2 rounded-full bg-[var(--text-muted)] shrink-0"
                    title="No SSE connection for this agent"
                  />
                  <span v-if="!t.enabled && !t.expired" class="badge-yellow">Disabled</span>
                  <span v-if="t.expired" class="badge-red">Expired</span>
                  <span class="badge-muted">{{ t.scope === "labels" ? "Labels" : "All" }}</span>
                </div>
                <div
                  class="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--text-muted)]"
                >
                  <span class="inline-flex items-center gap-1">
                    <Bot class="h-3 w-3" />
                    {{ agentTypeLabel(t) }}
                  </span>
                  <span class="inline-flex items-center gap-1">
                    <Clock class="h-3 w-3" />
                    Expires {{ formatExpires(t) }}
                  </span>
                  <span v-if="t.last_used" class="inline-flex items-center gap-1">
                    Last used {{ formatExpires({ expires_at: t.last_used }) }}
                  </span>
                </div>
                <div
                  v-if="t.scope === 'labels' && t.label_selectors?.length"
                  class="mt-2 flex flex-wrap gap-1.5"
                >
                  <span
                    v-for="(c, ci) in t.label_selectors"
                    :key="ci"
                    class="rounded-md bg-[var(--bg-secondary)] px-2 py-0.5 text-[11px] text-[var(--text-secondary)]"
                  >
                    {{ c.field }}
                    <span class="opacity-50">{{ c.operator === "exact" ? "=" : c.operator }}</span>
                    {{ c.operator !== "exists" && c.operator !== "not_exists" ? c.value : "" }}
                  </span>
                </div>
                <div v-if="t.capabilities?.length" class="flex flex-wrap gap-1 mt-1">
                  <span
                    v-for="cap in t.capabilities"
                    :key="cap"
                    :class="agentCapabilities[cap]?.color ?? 'bg-gray-100 text-gray-800'"
                    class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium uppercase"
                  >
                    {{ agentCapabilities[cap]?.label ?? cap }}
                  </span>
                </div>
              </div>
            </div>
            <div class="flex shrink-0 items-center justify-end gap-2 md:mt-0">
              <router-link :to="`/agents/${t.id}/chat`" class="inline-flex" @click.stop>
                <Button size="sm" variant="outline" :disabled="!!t.expired">
                  <MessageSquare class="h-3.5 w-3.5" />
                  Chat
                </Button>
              </router-link>
              <div class="absolute right-3 top-3 md:relative md:right-auto md:top-auto">
                <AgentActionsMenu
                  :agent="t"
                  :toggling="togglingAgentId === t.id"
                  @toggle-enabled="toggleAgentEnabled(t)"
                  @edit-scope="openAgentEditScope(t)"
                  @regenerate="openAgentRegenerate(t)"
                  @delete="deleteAgent(t)"
                />
              </div>
            </div>
          </div>
        </Card>
        <EmptyState v-if="filteredAgents.length === 0" :message="emptyFilterMessage" />
      </div>
    </template>

    <EmptyState v-else message="No agents configured yet.">
      <template #icon>
        <Bot class="mb-2 h-6 w-6 opacity-40" />
      </template>
      <template #footer>
        <p class="text-xs text-[var(--text-muted)]">
          Add an agent to start running automated investigations.
        </p>
        <Button size="sm" class="mt-2" @click="openAgentDialog">
          <Plus class="h-4 w-4" />
          Add agent
        </Button>
      </template>
    </EmptyState>

    <Modal v-model:open="showAgentDialog" title="Add integration agent" @close="closeAgentDialog">
      <ErrorBanner :message="agentError" />
      <template v-if="!agentCreatedSecret">
        <div class="space-y-3">
          <label class="block text-sm">
            <span class="mb-1 block text-[var(--text-muted)]">Agent name</span>
            <Input
              id="agent-token-name-input"
              v-model="agentNewName"
              placeholder="e.g. sre-agent-prod"
            />
          </label>
          <label class="block text-sm">
            <span class="mb-1 block text-[var(--text-muted)]">Agent type</span>
            <Select
              id="agent-new-type-select"
              :model-value="agentNewType"
              class="w-full"
              @update:model-value="agentNewType = $event as AgentType"
            >
              <option value="alga">Alga Agent</option>
              <option value="hermes">Hermes Agent</option>
              <option value="openclaw">OpenClaw</option>
              <option value="other">Other (Agent SDK / Self-developed)</option>
            </Select>
          </label>
          <label class="block text-sm">
            <span class="mb-1 block text-[var(--text-muted)]">Expiration (optional)</span>
            <DateTimePicker
              id="agent-new-expiry"
              v-model="agentNewExpiresLocal"
              placeholder="Pick expiry date & time"
            />
          </label>
          <fieldset class="block text-sm">
            <legend class="mb-1 text-[var(--text-muted)]">Investigation scope</legend>
            <div class="flex gap-4">
              <label class="flex items-center gap-1.5">
                <input
                  v-model="agentNewScope"
                  type="radio"
                  value="all"
                  class="accent-[var(--accent)]"
                />
                <span>All investigations</span>
              </label>
              <label class="flex items-center gap-1.5">
                <input
                  v-model="agentNewScope"
                  type="radio"
                  value="labels"
                  class="accent-[var(--accent)]"
                />
                <span>Match by labels</span>
              </label>
            </div>
          </fieldset>
          <template v-if="agentNewScope === 'labels'">
            <ConditionEditor v-model="agentNewLabelSelectors" />
            <p class="text-xs text-[var(--text-muted)]">
              Only investigations whose alert labels match <strong>all</strong> conditions will be
              assigned to this agent.
            </p>
          </template>
          <fieldset class="space-y-2">
            <legend class="text-sm font-medium text-[var(--text-secondary)]">Capabilities</legend>
            <div class="flex flex-wrap gap-2">
              <label
                v-for="(info, key) in agentCapabilities"
                :key="key"
                class="inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-sm cursor-pointer transition-colors"
                :class="
                  agentNewCapabilities.includes(key)
                    ? 'border-blue-500 bg-blue-50 text-blue-700 dark:border-blue-400 dark:bg-blue-900 dark:text-blue-200'
                    : 'border-gray-300 text-gray-600 dark:border-gray-600 dark:text-gray-400'
                "
              >
                <input
                  type="checkbox"
                  :value="key"
                  v-model="agentNewCapabilities"
                  class="sr-only"
                />
                {{ info.label }}
              </label>
            </div>
          </fieldset>
          <p class="text-xs text-[var(--text-muted)]">
            After creation you will receive a bot token (like a Slack bot token). Store it securely;
            it is not shown again.
          </p>
        </div>
      </template>
      <template v-else>
        <p class="mb-3 text-sm text-[var(--text-secondary)]">
          Save this bot token now. It will not be shown again. Use as
          <code class="rounded bg-[var(--bg-code)] px-1 text-xs">Authorization: Bearer ...</code>
          for agent API routes and SSE authentication.
        </p>
        <SecretDisplay :secret="agentCreatedSecret" @copy="copyAgentSecret" />
      </template>

      <template #footer>
        <template v-if="!agentCreatedSecret">
          <Button variant="outline" :disabled="agentSubmitting" @click="closeAgentDialog">
            Cancel
          </Button>
          <Button variant="primary" :loading="agentSubmitting" @click="submitAgentToken">
            Create
          </Button>
        </template>
        <template v-else>
          <Button id="agent-token-done" variant="primary" @click="closeAgentDialog">Done</Button>
        </template>
      </template>
    </Modal>

    <Modal
      v-if="agentScopeEditToken"
      v-model:open="showAgentScopeDialog"
      :title="`Edit investigation scope (${agentScopeEditToken.name})`"
      :loading="agentScopeSubmitting"
      confirm-label="Save"
      @confirm="submitAgentScopeEdit"
      @close="closeAgentScopeDialog"
    >
      <div class="space-y-3">
        <fieldset class="block text-sm">
          <legend class="mb-1 text-[var(--text-muted)]">Investigation scope</legend>
          <div class="flex gap-4">
            <label class="flex items-center gap-1.5">
              <input
                v-model="agentScopeEditScope"
                type="radio"
                value="all"
                class="accent-[var(--accent)]"
              />
              <span>All investigations</span>
            </label>
            <label class="flex items-center gap-1.5">
              <input
                v-model="agentScopeEditScope"
                type="radio"
                value="labels"
                class="accent-[var(--accent)]"
              />
              <span>Match by labels</span>
            </label>
          </div>
        </fieldset>
        <template v-if="agentScopeEditScope === 'labels'">
          <ConditionEditor v-model="agentScopeEditLabelSelectors" />
          <p class="text-xs text-[var(--text-muted)]">
            Only investigations whose alert labels match <strong>all</strong> conditions will be
            assigned to this agent.
          </p>
        </template>
        <fieldset class="space-y-2">
          <legend class="text-sm font-medium text-[var(--text-secondary)]">Capabilities</legend>
          <div class="flex flex-wrap gap-2">
            <label
              v-for="(info, key) in agentCapabilities"
              :key="key"
              class="inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-sm cursor-pointer transition-colors"
              :class="
                agentScopeEditCapabilities.includes(key)
                  ? 'border-blue-500 bg-blue-50 text-blue-700 dark:border-blue-400 dark:bg-blue-900 dark:text-blue-200'
                  : 'border-gray-300 text-gray-600 dark:border-gray-600 dark:text-gray-400'
              "
            >
              <input
                type="checkbox"
                :value="key"
                v-model="agentScopeEditCapabilities"
                class="sr-only"
              />
              {{ info.label }}
            </label>
          </div>
        </fieldset>
      </div>
    </Modal>

    <ConfirmDialog
      v-model:open="showAgentDeleteConfirm"
      title="Delete agent"
      :message="`Are you sure you want to delete agent '${agentTokenToDelete?.name}'? This action cannot be undone.`"
      confirm-label="Delete"
      :destructive="true"
      @confirm="doDeleteAgentToken"
    />

    <ConfirmDialog
      v-model:open="showAgentRegenerateConfirm"
      title="Regenerate bot token"
      :message="regenerateConfirmMessage"
      confirm-label="Regenerate"
      @cancel="onAgentRegenerateCancel"
      @confirm="doRegenerateAgentToken"
    />

    <Modal
      v-model:open="showAgentRegeneratedSecret"
      :title="`New bot token${agentRegeneratedName ? ` (${agentRegeneratedName})` : ''}`"
      :show-footer="false"
      @close="closeAgentRegeneratedSecretDialog"
    >
      <p v-if="agentRegeneratedSecret" class="mb-3 text-sm text-[var(--text-secondary)]">
        Copy this token now. The previous secret no longer works for this agent.
      </p>
      <SecretDisplay
        v-if="agentRegeneratedSecret"
        :secret="agentRegeneratedSecret"
        @copy="copyAgentRegeneratedSecret"
      />
      <div v-if="agentRegeneratedSecret" class="flex justify-end">
        <Button type="button" @click="closeAgentRegeneratedSecretDialog">Done</Button>
      </div>
    </Modal>
  </section>
</template>
