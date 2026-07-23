<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { ChevronDown, Search } from "@lucide/vue";
import { useDropdownLifecycle } from "@/composables/useDropdownLifecycle";
import { useEscapeKey } from "@/composables/useEscapeKey";
import { flagEmoji, getPhoneCountries, type PhoneCountry } from "@/lib/phoneCountries";

const props = withDefaults(
  defineProps<{
    modelValue?: string;
    disabled?: boolean;
    error?: string;
  }>(),
  {
    modelValue: "US",
    disabled: false,
    error: "",
  },
);

const emit = defineEmits<{
  "update:modelValue": [value: string];
}>();

const countries = computed<PhoneCountry[]>(() => getPhoneCountries("en"));

const selected = computed(
  () => countries.value.find((c) => c.code === props.modelValue) ?? countries.value[0],
);

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
  if (!q) return countries.value;
  return countries.value.filter(
    (c) =>
      c.name.toLowerCase().includes(q) ||
      c.code.toLowerCase().includes(q) ||
      c.dial.includes(q) ||
      ("+" + c.dial).includes(q),
  );
});

watch(open, (is) => {
  if (is) {
    query.value = "";
    const idx = filtered.value.findIndex((c) => c.code === selected.value.code);
    activeIndex.value = idx >= 0 ? idx : 0;
    void nextTick(() => inputRef.value?.focus());
  }
});

function toggle() {
  if (props.disabled) return;
  open.value = !open.value;
}

function select(c: PhoneCountry) {
  emit("update:modelValue", c.code);
  open.value = false;
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
    select(items[activeIndex.value]);
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
</script>

<template>
  <div ref="rootRef" class="relative" @keydown="onKeydown">
    <button
      type="button"
      :aria-expanded="open"
      aria-haspopup="listbox"
      :disabled="disabled"
      :aria-invalid="error ? 'true' : undefined"
      class="field flex w-auto shrink-0 cursor-pointer items-center justify-between gap-1.5 text-left"
      @click="toggle"
    >
      <span class="flex items-center gap-1.5">
        <span aria-hidden="true">{{ flagEmoji(selected.code) }}</span>
        <span class="font-medium">+{{ selected.dial }}</span>
      </span>
      <ChevronDown
        class="h-4 w-4 text-[var(--text-muted)] transition-transform duration-200"
        :class="open ? 'rotate-180' : ''"
      />
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
        class="absolute left-0 z-50 mt-1 w-72 overflow-hidden rounded-lg border border-[var(--border-primary)] bg-[var(--bg-card)] shadow-xl"
      >
        <div class="flex items-center gap-2 border-b border-[var(--border-primary)] p-2">
          <Search class="h-4 w-4 shrink-0 text-[var(--text-muted)]" />
          <input
            ref="inputRef"
            v-model="query"
            type="text"
            class="field w-full text-sm"
            placeholder="Search country or code…"
            autocomplete="off"
            @input="activeIndex = 0"
          />
        </div>
        <ul ref="listRef" role="listbox" class="max-h-60 overflow-y-auto overscroll-contain py-1">
          <li v-if="filtered.length === 0" class="px-3 py-2 text-sm text-[var(--text-muted)]">
            No matching countries
          </li>
          <li
            v-for="(c, i) in filtered"
            :key="c.code"
            role="option"
            :aria-selected="c.code === modelValue"
            :data-active="i === activeIndex"
            class="flex cursor-pointer items-center justify-between gap-2 px-3 py-2 text-sm transition-colors"
            :class="[
              c.code === modelValue
                ? 'bg-[var(--accent-primary)]/10 text-[var(--text-primary)]'
                : i === activeIndex
                  ? 'bg-[var(--bg-secondary)] text-[var(--text-primary)]'
                  : 'text-[var(--text-secondary)]',
            ]"
            @click="select(c)"
            @pointerenter="activeIndex = i"
          >
            <span class="flex min-w-0 items-center gap-2">
              <span aria-hidden="true" class="text-base">{{ flagEmoji(c.code) }}</span>
              <span class="truncate">{{ c.name }}</span>
              <span class="shrink-0 text-xs text-[var(--text-muted)]">{{ c.code }}</span>
            </span>
            <span class="shrink-0 font-medium">+{{ c.dial }}</span>
          </li>
        </ul>
      </div>
    </Transition>
  </div>
</template>
