<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { Check, CircleDot, Loader2, Search } from "@lucide/vue";
import Modal from "@/components/ui/Modal.vue";
import Button from "@/components/ui/Button.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import AlertStatusBadge from "@/components/ui/AlertStatusBadge.vue";
import { api, type AlertRecord } from "@/lib/api";
import { alertSeverityLabel, severityBorderColor } from "@/lib/alertLabels";
import { getErrorMessage } from "@/lib/error";
import { useSearchDebounce } from "@/composables/useSearchDebounce";

const props = withDefaults(
  defineProps<{
    open: boolean;
    submitting: boolean;
    error: string;
    linkedAlertNumbers: number[];
    searchPlaceholder?: string;
  }>(),
  {
    searchPlaceholder: "Search by alert name or #N…",
  },
);

const emit = defineEmits<{
  "update:open": [value: boolean];
  pickAlert: [alertNumber: number];
  submit: [];
}>();

const query = ref("");
const results = ref<AlertRecord[]>([]);
const loading = ref(false);
const fetchError = ref("");
const staged = ref<AlertRecord | null>(null);
const selectedIndex = ref(0);
let fetchSeq = 0;

const linkedSet = computed(() => new Set(props.linkedAlertNumbers));

function resetState() {
  query.value = "";
  results.value = [];
  loading.value = false;
  fetchError.value = "";
  staged.value = null;
  selectedIndex.value = 0;
}

watch(
  () => props.open,
  (open) => {
    if (open) resetState();
  },
);

async function fetchResults() {
  const seq = ++fetchSeq;
  const q = query.value.trim();
  if (!q) {
    results.value = [];
    loading.value = false;
    fetchError.value = "";
    return;
  }
  loading.value = true;
  fetchError.value = "";
  try {
    const data = await api.getAlerts({
      search: q,
      sort: "-updated_at",
      limit: 12,
    });
    if (seq !== fetchSeq) return;
    const filtered = (data ?? []).filter(
      (a) => a.alert_number !== undefined && !a.deleted_at && !linkedSet.value.has(a.alert_number),
    );
    results.value = filtered;
    selectedIndex.value = 0;
    if (staged.value && !filtered.some((a) => a.alert_number === staged.value?.alert_number)) {
      staged.value = null;
    }
  } catch (err) {
    if (seq !== fetchSeq) return;
    results.value = [];
    fetchError.value = getErrorMessage(err, "Failed to search alerts");
  } finally {
    if (seq === fetchSeq) loading.value = false;
  }
}

const { scheduleSearchReload } = useSearchDebounce(fetchResults, 300);

watch(query, () => scheduleSearchReload());

function isStaged(alert: AlertRecord): boolean {
  return staged.value?.alert_number === alert.alert_number;
}

function pickFromResults(index: number) {
  const target = results.value[index];
  if (!target) return;
  staged.value = target;
  emit("pickAlert", target.alert_number as number);
}

function pickStaged() {
  if (staged.value?.alert_number == null) return;
  emit("pickAlert", staged.value.alert_number);
  emit("submit");
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === "ArrowDown") {
    e.preventDefault();
    if (results.value.length === 0) return;
    selectedIndex.value = (selectedIndex.value + 1) % results.value.length;
    return;
  }
  if (e.key === "ArrowUp") {
    e.preventDefault();
    if (results.value.length === 0) return;
    selectedIndex.value = (selectedIndex.value - 1 + results.value.length) % results.value.length;
    return;
  }
  if (e.key === "Enter") {
    e.preventDefault();
    pickFromResults(selectedIndex.value);
    return;
  }
  if (e.key === "Escape") {
    e.stopPropagation();
    return;
  }
}

const showEmptyState = computed(
  () =>
    !loading.value &&
    !fetchError.value &&
    query.value.trim().length > 0 &&
    results.value.length === 0,
);

const canLink = computed(() => staged.value?.alert_number != null && !props.submitting);
</script>

<template>
  <Modal
    :open="open"
    title="Link Alert"
    max-width="lg"
    :show-footer="false"
    :prevent-close="submitting"
    @update:open="(v: boolean) => emit('update:open', v)"
    @close="emit('update:open', false)"
  >
    <ErrorBanner v-if="error" :message="error" class="mb-3" />
    <ErrorBanner v-else-if="fetchError" :message="fetchError" class="mb-3" />

    <div class="relative">
      <Search
        class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--text-muted)]"
        aria-hidden="true"
      />
      <Loader2
        v-if="loading"
        class="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 animate-spin text-[var(--text-muted)]"
        aria-hidden="true"
      />
      <input
        v-model="query"
        type="search"
        role="combobox"
        aria-label="Search alerts"
        aria-controls="link-alert-results"
        :aria-expanded="results.length > 0"
        :aria-activedescendant="
          results[selectedIndex]
            ? `link-alert-result-${results[selectedIndex].alert_number}`
            : undefined
        "
        :placeholder="searchPlaceholder"
        class="field w-full !pl-9"
        :class="loading ? '!pr-9' : ''"
        autocomplete="off"
        autofocus
        :disabled="submitting"
        @keydown="onKeydown"
      />
    </div>

    <div
      id="link-alert-results"
      role="listbox"
      aria-label="Alert search results"
      class="mt-3 max-h-72 space-y-1 overflow-y-auto pr-1"
    >
      <div v-if="!query.trim()" class="px-3 py-8 text-center text-sm text-[var(--text-muted)]">
        Search by alert name, fingerprint, or <code>#N</code>.
      </div>
      <div
        v-else-if="loading && results.length === 0"
        class="px-3 py-8 text-center text-sm text-[var(--text-muted)]"
      >
        Searching…
      </div>
      <div
        v-else-if="showEmptyState"
        class="px-3 py-8 text-center text-sm text-[var(--text-muted)]"
      >
        No alerts match your search.
      </div>
      <template v-else>
        <button
          v-for="(alert, index) in results"
          :id="`link-alert-result-${alert.alert_number}`"
          :key="alert.fingerprint"
          type="button"
          role="option"
          :aria-selected="isStaged(alert)"
          class="flex w-full cursor-pointer items-center gap-3 rounded-md px-3 py-2.5 text-left transition-colors hover:bg-[var(--bg-secondary)] disabled:cursor-not-allowed disabled:opacity-60"
          :class="{
            'bg-[var(--bg-secondary)]': index === selectedIndex || isStaged(alert),
          }"
          :disabled="submitting"
          @click="pickFromResults(index)"
          @mouseenter="selectedIndex = index"
        >
          <CircleDot
            class="h-4 w-4 shrink-0"
            :style="{
              color: severityBorderColor(alertSeverityLabel(alert.labels)),
            }"
            aria-hidden="true"
          />
          <div class="min-w-0 flex-1">
            <span
              v-if="alert.alert_number != null && alert.alert_number > 0"
              class="font-mono text-xs text-[var(--text-muted)] mr-1.5"
            >
              #{{ alert.alert_number }}
            </span>
            <span class="text-sm text-[var(--text-primary)]">
              {{ alert.labels?.alertname || alert.fingerprint }}
            </span>
          </div>
          <AlertStatusBadge
            :status="alert.status"
            :acknowledged="alert.acknowledged"
            class="shrink-0"
          />
          <Check
            v-if="isStaged(alert)"
            class="h-4 w-4 shrink-0 text-[var(--accent-primary)]"
            aria-hidden="true"
          />
        </button>
      </template>
    </div>

    <div class="mt-4 flex justify-end gap-2 border-t border-[var(--border-primary)] pt-4">
      <Button variant="outline" :disabled="submitting" @click="emit('update:open', false)">
        Cancel
      </Button>
      <Button variant="primary" :loading="submitting" :disabled="!canLink" @click="pickStaged">
        {{ staged ? `Link #${staged.alert_number}` : "Link" }}
      </Button>
    </div>
  </Modal>
</template>
