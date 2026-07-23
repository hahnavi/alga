<script setup lang="ts" generic="T extends string">
import { computed, nextTick, ref } from "vue";
import type { Component } from "vue";

export type Tab<T extends string> = {
  id: T;
  label: string;
  icon?: Component;
  count?: number;
  dirty?: boolean;
};

const props = withDefaults(
  defineProps<{
    tabs: Tab<T>[];
    ariaLabel?: string;
    idPrefix?: string;
  }>(),
  {
    ariaLabel: "Sections",
    idPrefix: "tabs",
  },
);

const active = defineModel<T>({ required: true });

const tabRefs = ref<HTMLButtonElement[]>([]);

const activeTab = computed(() => props.tabs.find((t) => t.id === active.value));

function selectTab(id: T) {
  active.value = id;
}

function onKeydown(e: KeyboardEvent, idx: number) {
  if (e.key !== "ArrowRight" && e.key !== "ArrowLeft" && e.key !== "Home" && e.key !== "End") {
    return;
  }
  e.preventDefault();
  const n = props.tabs.length;
  let next = idx;
  if (e.key === "ArrowRight") next = (idx + 1) % n;
  else if (e.key === "ArrowLeft") next = (idx - 1 + n) % n;
  else if (e.key === "Home") next = 0;
  else if (e.key === "End") next = n - 1;
  active.value = props.tabs[next].id;
  nextTick(() => tabRefs.value[next]?.focus());
}
</script>

<template>
  <div
    class="flex gap-1 overflow-x-auto border-b border-[var(--border-primary)]"
    role="tablist"
    :aria-label="ariaLabel"
  >
    <button
      v-for="(tab, idx) in tabs"
      :key="tab.id"
      :ref="
        (el) => {
          if (el) tabRefs[idx] = el as HTMLButtonElement;
        }
      "
      type="button"
      role="tab"
      :id="`${idPrefix}-tab-${tab.id}`"
      :aria-selected="active === tab.id"
      :aria-controls="`${idPrefix}-panel-${tab.id}`"
      :tabindex="active === tab.id ? 0 : -1"
      :class="[
        'flex shrink-0 items-center gap-1.5 whitespace-nowrap border-b-2 px-4 py-2.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--bg-primary)]',
        active === tab.id
          ? 'border-[var(--accent)] text-[var(--text-primary)]'
          : 'border-transparent text-[var(--text-muted)] hover:text-[var(--text-primary)]',
      ]"
      @click="selectTab(tab.id)"
      @keydown="onKeydown($event, idx)"
    >
      <component :is="tab.icon" v-if="tab.icon" class="h-4 w-4" />
      <span>{{ tab.label }}</span>
      <span
        v-if="tab.count !== undefined && tab.count > 0"
        class="rounded-full border border-[var(--border-secondary)] px-1.5 py-0.5 text-xs font-medium text-[var(--text-badge-muted)]"
      >
        {{ tab.count }}
      </span>
      <span
        v-else-if="tab.dirty"
        class="ml-0.5 inline-block h-1.5 w-1.5 rounded-full bg-[var(--accent)]"
        :title="`Unsaved changes in ${tab.label}`"
        aria-hidden="true"
      />
      <slot :name="`tab-${tab.id}`" :tab="tab" />
    </button>
  </div>

  <div
    v-if="activeTab"
    :id="`${idPrefix}-panel-${activeTab.id}`"
    role="tabpanel"
    :aria-labelledby="`${idPrefix}-tab-${activeTab.id}`"
    tabindex="0"
    class="focus:outline-none"
  >
    <slot :name="`panel-${activeTab.id}`" :tab="activeTab" />
  </div>
</template>
