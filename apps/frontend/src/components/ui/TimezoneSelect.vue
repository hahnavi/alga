<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { ChevronDown, Globe, X } from "@lucide/vue";
import { useDropdownLifecycle } from "@/composables/useDropdownLifecycle";
import { useEscapeKey } from "@/composables/useEscapeKey";

const props = defineProps<{
  id?: string;
  modelValue: string;
  disabled?: boolean;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: string];
}>();

const COMMON_TIMEZONES: { value: string; label: string }[] = [
  { value: "UTC", label: "UTC" },
  { value: "America/New_York", label: "Eastern Time (US & Canada)" },
  { value: "America/Chicago", label: "Central Time (US & Canada)" },
  { value: "America/Denver", label: "Mountain Time (US & Canada)" },
  { value: "America/Los_Angeles", label: "Pacific Time (US & Canada)" },
  { value: "America/Anchorage", label: "Alaska" },
  { value: "Pacific/Honolulu", label: "Hawaii" },
  { value: "America/Sao_Paulo", label: "São Paulo" },
  { value: "America/Argentina/Buenos_Aires", label: "Buenos Aires" },
  { value: "America/Mexico_City", label: "Mexico City" },
  { value: "America/Toronto", label: "Toronto" },
  { value: "America/Vancouver", label: "Vancouver" },
  { value: "Europe/London", label: "London" },
  { value: "Europe/Paris", label: "Paris" },
  { value: "Europe/Berlin", label: "Berlin" },
  { value: "Europe/Amsterdam", label: "Amsterdam" },
  { value: "Europe/Madrid", label: "Madrid" },
  { value: "Europe/Rome", label: "Rome" },
  { value: "Europe/Zurich", label: "Zurich" },
  { value: "Europe/Stockholm", label: "Stockholm" },
  { value: "Europe/Moscow", label: "Moscow" },
  { value: "Europe/Istanbul", label: "Istanbul" },
  { value: "Europe/Warsaw", label: "Warsaw" },
  { value: "Europe/Athens", label: "Athens" },
  { value: "Europe/Helsinki", label: "Helsinki" },
  { value: "Europe/Lisbon", label: "Lisbon" },
  { value: "Asia/Dubai", label: "Dubai" },
  { value: "Asia/Kolkata", label: "Kolkata" },
  { value: "Asia/Bangkok", label: "Bangkok" },
  { value: "Asia/Singapore", label: "Singapore" },
  { value: "Asia/Hong_Kong", label: "Hong Kong" },
  { value: "Asia/Shanghai", label: "Shanghai" },
  { value: "Asia/Tokyo", label: "Tokyo" },
  { value: "Asia/Seoul", label: "Seoul" },
  { value: "Asia/Taipei", label: "Taipei" },
  { value: "Asia/Jakarta", label: "Jakarta" },
  { value: "Asia/Karachi", label: "Karachi" },
  { value: "Asia/Dhaka", label: "Dhaka" },
  { value: "Australia/Sydney", label: "Sydney" },
  { value: "Australia/Melbourne", label: "Melbourne" },
  { value: "Australia/Brisbane", label: "Brisbane" },
  { value: "Australia/Perth", label: "Perth" },
  { value: "Australia/Adelaide", label: "Adelaide" },
  { value: "Pacific/Auckland", label: "Auckland" },
  { value: "Pacific/Fiji", label: "Fiji" },
  { value: "Africa/Cairo", label: "Cairo" },
  { value: "Africa/Lagos", label: "Lagos" },
  { value: "Africa/Johannesburg", label: "Johannesburg" },
  { value: "Africa/Nairobi", label: "Nairobi" },
  { value: "Africa/Casablanca", label: "Casablanca" },
  { value: "Atlantic/Reykjavik", label: "Reykjavik" },
];

const open = ref(false);
const query = ref("");
const rootRef = ref<HTMLElement | null>(null);
const inputRef = ref<HTMLInputElement | null>(null);
const listRef = ref<HTMLElement | null>(null);
const activeIndex = ref(-1);

useDropdownLifecycle(open, rootRef);
useEscapeKey(
  () => {
    open.value = false;
  },
  () => open.value,
);

const filtered = computed(() => {
  const q = query.value.toLowerCase().trim();
  if (!q) return COMMON_TIMEZONES;
  return COMMON_TIMEZONES.filter(
    (tz) => tz.value.toLowerCase().includes(q) || tz.label.toLowerCase().includes(q),
  );
});

const displayLabel = computed(() => {
  const match = COMMON_TIMEZONES.find((tz) => tz.value === props.modelValue);
  if (match) return match.label;
  return props.modelValue || "";
});

watch(open, (is) => {
  if (is) {
    query.value = "";
    activeIndex.value = -1;
    void nextTick(() => inputRef.value?.focus());
  }
});

function toggle() {
  if (props.disabled) return;
  open.value = !open.value;
}

function select(tz: string) {
  emit("update:modelValue", tz);
  open.value = false;
}

function clearValue(e: MouseEvent) {
  e.stopPropagation();
  emit("update:modelValue", "UTC");
}

function onKeydown(e: KeyboardEvent) {
  if (!open.value) {
    if (e.key === "Enter" || e.key === " " || e.key === "ArrowDown") {
      e.preventDefault();
      open.value = true;
    }
    return;
  }

  const items = filtered.value;
  if (e.key === "ArrowDown") {
    e.preventDefault();
    activeIndex.value = items.length === 0 ? -1 : (activeIndex.value + 1) % items.length;
    scrollToActive();
    return;
  }
  if (e.key === "ArrowUp") {
    e.preventDefault();
    activeIndex.value =
      items.length === 0 ? -1 : (activeIndex.value - 1 + items.length) % items.length;
    scrollToActive();
    return;
  }
  if (e.key === "Enter" && activeIndex.value >= 0 && activeIndex.value < items.length) {
    e.preventDefault();
    select(items[activeIndex.value].value);
    return;
  }
}

function scrollToActive() {
  void nextTick(() => {
    const list = listRef.value;
    if (!list) return;
    const active = list.querySelector("[data-active='true']");
    active?.scrollIntoView({ block: "nearest" });
  });
}

function utcOffset(value: string): string {
  try {
    const parts = new Intl.DateTimeFormat("en-US", {
      timeZone: value,
      timeZoneName: "shortOffset",
    }).formatToParts(new Date());
    const tzPart = parts.find((p) => p.type === "timeZoneName");
    return tzPart?.value ?? "";
  } catch {
    return "";
  }
}
</script>

<template>
  <div ref="rootRef" class="relative" @keydown="onKeydown">
    <button
      type="button"
      :id="id"
      :aria-expanded="open"
      aria-haspopup="listbox"
      :disabled="disabled"
      class="field flex cursor-pointer items-center justify-between gap-2 text-left"
      @click="toggle"
    >
      <span class="flex items-center gap-2 truncate">
        <Globe class="h-4 w-4 shrink-0 text-[var(--text-muted)]" />
        <span v-if="modelValue" class="truncate text-[var(--text-input)]">
          {{ displayLabel }}
          <span class="text-[var(--text-muted)]">{{ utcOffset(modelValue) }}</span>
        </span>
        <span v-else class="text-[var(--text-muted)]">Select timezone…</span>
      </span>
      <span class="flex shrink-0 items-center gap-1">
        <button
          v-if="modelValue && modelValue !== 'UTC'"
          type="button"
          class="cursor-pointer rounded p-0.5 text-[var(--text-muted)] transition-colors hover:text-[var(--text-primary)]"
          aria-label="Reset to UTC"
          tabindex="-1"
          @click="clearValue"
        >
          <X class="h-3.5 w-3.5" />
        </button>
        <ChevronDown
          class="h-4 w-4 text-[var(--text-muted)] transition-transform duration-200"
          :class="open ? 'rotate-180' : ''"
        />
      </span>
    </button>

    <Transition
      enter-active-class="transition duration-150 ease-out"
      enter-from-class="opacity-0 -translate-y-1"
      enter-to-class="opacity-100 translate-y-0"
      leave-active-class="transition duration-100 ease-in"
      leave-from-class="opacity-100 translate-y-0"
      leave-to-class="opacity-0 -translate-y-1"
    >
      <div
        v-if="open"
        class="absolute z-50 mt-1 w-full overflow-hidden rounded-lg border border-[var(--border-primary)] bg-[var(--bg-card)] shadow-xl"
      >
        <div class="border-b border-[var(--border-primary)] p-2">
          <input
            ref="inputRef"
            v-model="query"
            type="text"
            class="field w-full text-sm"
            placeholder="Search timezones…"
            autocomplete="off"
          />
        </div>
        <ul ref="listRef" role="listbox" class="max-h-60 overflow-y-auto overscroll-contain py-1">
          <li v-if="filtered.length === 0" class="px-3 py-2 text-sm text-[var(--text-muted)]">
            No matching timezones
          </li>
          <li
            v-for="(tz, i) in filtered"
            :key="tz.value"
            role="option"
            :aria-selected="tz.value === modelValue"
            :data-active="i === activeIndex"
            class="flex cursor-pointer items-center justify-between gap-2 px-3 py-2 text-sm transition-colors"
            :class="[
              tz.value === modelValue
                ? 'bg-[var(--accent-primary)]/10 text-[var(--text-primary)]'
                : i === activeIndex
                  ? 'bg-[var(--bg-secondary)] text-[var(--text-primary)]'
                  : 'text-[var(--text-secondary)]',
            ]"
            @click="select(tz.value)"
            @pointerenter="activeIndex = i"
          >
            <span class="flex items-center gap-2">
              <Globe class="h-3.5 w-3.5 shrink-0 text-[var(--text-muted)]" />
              <span class="truncate">{{ tz.label }}</span>
            </span>
            <span class="shrink-0 text-xs text-[var(--text-muted)]">
              {{ tz.value }}
              <span v-if="utcOffset(tz.value)" class="ml-1">{{ utcOffset(tz.value) }}</span>
            </span>
          </li>
        </ul>
        <div class="border-t border-[var(--border-primary)] px-3 py-2">
          <label class="text-xs text-[var(--text-muted)]">
            Custom IANA timezone
            <input
              :value="modelValue"
              type="text"
              class="field mt-1 w-full text-xs"
              placeholder="e.g. Europe/Berlin"
              @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
            />
          </label>
        </div>
      </div>
    </Transition>
  </div>
</template>
