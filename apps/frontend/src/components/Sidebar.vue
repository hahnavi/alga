<script setup lang="ts">
import { computed, ref, useAttrs, watch, type ComponentPublicInstance } from "vue";
import { useRoute } from "vue-router";
import { ChevronDown, ChevronRight, PanelLeftClose, PanelLeftOpen, Settings } from "@lucide/vue";
import { isActiveRoute } from "@/lib/routing";
import { safeGetItem, safeSetItem } from "@/lib/storage";
import { isNavGroup, useNavSections, type NavEntry } from "@/lib/nav";
import { useDropdownLifecycle } from "@/composables/useDropdownLifecycle";
import { useRoutePrefetch } from "@/composables/useRoutePrefetch";
import { CARD_ICON_BTN_CLASS } from "@/lib/uiClasses";
import UserMenuBar from "@/components/UserMenuBar.vue";
import LogoMark from "@/components/ui/LogoMark.vue";

defineOptions({ inheritAttrs: false });

const attrs = useAttrs();
const route = useRoute();
const { prefetch } = useRoutePrefetch();
const { sidebarSections } = useNavSections();

const collapsed = ref(safeGetItem("sidebar_collapsed") === "true");
const openPopup = ref<string | null>(null);
// Mirror of `openPopup !== null` so `useDropdownLifecycle` (boolean-only) can
// drive click-outside + Escape. The watcher below keeps the two in sync and
// also nulls `openPopup` when the user clicks away or presses Escape.
const isPopupOpen = ref(false);
const popupMenuRef = ref<HTMLElement | null>(null);
const popupPos = ref<{ top: number; left: number } | null>(null);
function setPopupMenuRef(el: Element | ComponentPublicInstance | null) {
  popupMenuRef.value = (el as HTMLElement | null) ?? null;
}
watch(isPopupOpen, (open) => {
  if (!open) openPopup.value = null;
});
watch(openPopup, (label) => {
  isPopupOpen.value = label !== null;
});

function toggleCollapse() {
  collapsed.value = !collapsed.value;
  safeSetItem("sidebar_collapsed", String(collapsed.value));
  openPopup.value = null;
}

const expandedGroups = ref<Set<string>>(new Set(["Agents"]));

function toggleGroup(label: string) {
  if (expandedGroups.value.has(label)) {
    expandedGroups.value.delete(label);
  } else {
    expandedGroups.value.add(label);
  }
}

function togglePopup(label: string, event: MouseEvent) {
  if (openPopup.value === label) {
    openPopup.value = null;
    popupPos.value = null;
  } else {
    const btn = event.currentTarget as HTMLElement;
    const rect = btn.getBoundingClientRect();
    popupPos.value = { top: rect.top, left: rect.right + 4 };
    openPopup.value = label;
  }
}

function closePopup() {
  openPopup.value = null;
  popupPos.value = null;
}

useDropdownLifecycle(isPopupOpen, popupMenuRef);

const navSections = sidebarSections;

function isActive(to: string) {
  return isActiveRoute(route.path, to);
}

function isEntryActive(entry: NavEntry): boolean {
  if (isNavGroup(entry)) {
    return entry.children.some((c) => isActive(c.to));
  }
  return isActive(entry.to);
}

watch(
  () => route.path,
  () => {
    openPopup.value = null;
    for (const section of navSections.value) {
      for (const entry of section.items) {
        if (isNavGroup(entry) && entry.children.some((c) => isActive(c.to))) {
          expandedGroups.value.add(entry.label);
        }
      }
    }
  },
  { immediate: true },
);

watch(collapsed, (val) => {
  if (!val) openPopup.value = null;
});

const popupStyle = computed(() => {
  if (!popupPos.value) return { display: "none" };
  return {
    top: `${popupPos.value.top}px`,
    left: `${popupPos.value.left}px`,
  };
});
</script>

<template>
  <aside
    v-bind="attrs"
    class="flex h-screen shrink-0 flex-col overflow-visible border-r border-[var(--border-primary)] bg-[var(--bg-sidebar)] transition-[width] duration-200 ease-in-out"
    :style="{ width: collapsed ? '56px' : '240px' }"
  >
    <div
      class="flex h-14 shrink-0 items-center border-b border-[var(--border-primary)] px-2"
      :class="collapsed ? 'justify-center' : ''"
    >
      <button
        :class="[CARD_ICON_BTN_CLASS, 'inline-flex h-8 w-8']"
        :aria-label="collapsed ? 'Expand sidebar' : 'Collapse sidebar'"
        :title="collapsed ? 'Expand sidebar' : 'Collapse sidebar'"
        @click="toggleCollapse"
      >
        <PanelLeftOpen v-if="collapsed" class="h-4 w-4" />
        <PanelLeftClose v-else class="h-4 w-4" />
      </button>
      <div v-if="!collapsed" class="ml-2 flex min-w-0 items-center gap-2">
        <LogoMark class="h-7 w-7 shrink-0" />
        <span class="flex min-w-0 flex-col">
          <span class="text-[15px] font-semibold leading-tight tracking-tight">Alga</span>
          <span
            class="font-mono text-[9px] font-medium leading-none tracking-[0.24em] text-[var(--accent)]"
          >
            OPS CONSOLE
          </span>
        </span>
      </div>
    </div>

    <nav class="flex-1 overflow-y-auto px-2 py-2">
      <template v-for="(section, si) in navSections" :key="section.label">
        <div
          v-if="si > 0"
          class="mx-1 border-t border-[var(--border-primary)]"
          :class="collapsed ? 'my-2' : 'my-1'"
        />
        <div
          v-if="!collapsed"
          class="eyebrow px-3 pb-1 text-[10px]"
          :class="si === 0 ? 'pt-1' : 'pt-3'"
        >
          {{ section.label }}
        </div>
        <div class="space-y-0.5">
          <template v-for="entry in section.items" :key="'to' in entry ? entry.to : entry.label">
            <router-link
              v-if="!isNavGroup(entry)"
              :to="entry.to"
              class="flex items-center gap-3 rounded-sm px-3 py-2 text-sm transition-colors cursor-pointer"
              @mouseenter="prefetch(entry.to)"
              @focus="prefetch(entry.to)"
              :class="isActive(entry.to) ? 'nav-link-active' : 'nav-link-inactive'"
              :title="collapsed ? entry.label : undefined"
            >
              <component :is="entry.icon" class="h-4 w-4 shrink-0" />
              <span v-if="!collapsed">{{ entry.label }}</span>
            </router-link>

            <div v-else class="relative">
              <button
                :data-popup-trigger="entry.label"
                type="button"
                class="flex w-full items-center gap-3 rounded-sm px-3 py-2 text-sm transition-colors cursor-pointer"
                :class="[
                  isEntryActive(entry)
                    ? 'text-[var(--text-primary)] font-medium'
                    : 'text-[var(--text-muted)] hover:bg-[var(--sidebar-hover,rgb(148_163_184/0.1))] hover:text-[var(--text-primary)]',
                  collapsed && openPopup === entry.label
                    ? 'bg-[var(--sidebar-hover,rgb(148_163_184/0.1))]'
                    : '',
                ]"
                :title="collapsed ? entry.label : undefined"
                @click="collapsed ? togglePopup(entry.label, $event) : toggleGroup(entry.label)"
              >
                <component :is="entry.icon" class="h-4 w-4 shrink-0" />
                <span v-if="!collapsed" class="flex-1 text-left">
                  {{ entry.label }}
                </span>
                <ChevronDown
                  v-if="!collapsed && expandedGroups.has(entry.label)"
                  class="h-3.5 w-3.5 shrink-0 text-[var(--text-muted)]"
                />
                <ChevronRight
                  v-else-if="!collapsed"
                  class="h-3.5 w-3.5 shrink-0 text-[var(--text-muted)]"
                />
              </button>

              <div
                v-if="!collapsed && expandedGroups.has(entry.label)"
                class="ml-4 mt-0.5 space-y-0.5 border-l border-[var(--border-primary)] pl-2"
              >
                <router-link
                  v-for="child in entry.children"
                  :key="child.to"
                  :to="child.to"
                  class="flex items-center gap-3 rounded-sm px-3 py-1.5 text-sm transition-colors cursor-pointer"
                  @mouseenter="prefetch(child.to)"
                  @focus="prefetch(child.to)"
                  :class="isActive(child.to) ? 'nav-link-active' : 'nav-link-inactive'"
                >
                  <component :is="child.icon" class="h-3.5 w-3.5 shrink-0" />
                  <span>{{ child.label }}</span>
                </router-link>
              </div>

              <Teleport to="body">
                <div
                  v-if="collapsed && openPopup === entry.label"
                  :ref="setPopupMenuRef"
                  class="fixed z-50 min-w-[160px] rounded-lg border border-[var(--border-primary)] bg-[var(--bg-sidebar)] py-1 shadow-lg"
                  :style="popupStyle"
                >
                  <div class="px-3 py-1.5 text-xs font-medium text-[var(--text-muted)]">
                    {{ entry.label }}
                  </div>
                  <router-link
                    v-for="child in entry.children"
                    :key="child.to"
                    :to="child.to"
                    class="flex items-center gap-3 px-3 py-1.5 text-sm transition-colors cursor-pointer"
                    @mouseenter="prefetch(child.to)"
                    @focus="prefetch(child.to)"
                    :class="isActive(child.to) ? 'nav-link-active' : 'nav-link-inactive'"
                    @click="closePopup"
                  >
                    <component :is="child.icon" class="h-3.5 w-3.5 shrink-0" />
                    <span>{{ child.label }}</span>
                  </router-link>
                </div>
              </Teleport>
            </div>
          </template>
        </div>
      </template>
    </nav>

    <div
      class="shrink-0 border-t border-[var(--border-primary)] px-2 py-3"
      :class="collapsed ? 'flex flex-col items-center gap-1.5' : 'flex items-center gap-1'"
    >
      <UserMenuBar
        :show-name="!collapsed"
        :compact="collapsed"
        :class="collapsed ? '' : 'min-w-0 flex-1'"
      />
      <router-link
        to="/settings/general"
        aria-label="Settings"
        :title="collapsed ? 'Settings' : undefined"
        :class="[CARD_ICON_BTN_CLASS, 'inline-flex h-8 w-8']"
        @mouseenter="prefetch('/settings/general')"
        @focus="prefetch('/settings/general')"
      >
        <Settings class="h-4 w-4" aria-hidden="true" />
      </router-link>
    </div>
  </aside>
</template>
