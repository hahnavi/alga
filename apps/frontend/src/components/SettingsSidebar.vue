<script setup lang="ts">
import { ref, useAttrs } from "vue";
import { useRoute } from "vue-router";
import { ArrowLeft, PanelLeftClose, PanelLeftOpen } from "@lucide/vue";
import { isActiveRoute } from "@/lib/routing";
import { safeGetItem, safeSetItem } from "@/lib/storage";
import { useNavSections } from "@/lib/nav";
import { useRoutePrefetch } from "@/composables/useRoutePrefetch";
import { CARD_ICON_BTN_CLASS } from "@/lib/uiClasses";
import UserMenuBar from "@/components/UserMenuBar.vue";

defineOptions({ inheritAttrs: false });

const attrs = useAttrs();
const route = useRoute();
const { prefetch } = useRoutePrefetch();
const { settingsSections } = useNavSections();

const collapsed = ref(safeGetItem("sidebar_collapsed") === "true");

function toggleCollapse() {
  collapsed.value = !collapsed.value;
  safeSetItem("sidebar_collapsed", String(collapsed.value));
}

function isActive(to: string) {
  return isActiveRoute(route.path, to);
}
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
      <span v-if="!collapsed" class="ml-2 text-lg font-semibold">Alga</span>
    </div>

    <div class="shrink-0 px-2 pt-2" :class="collapsed ? 'flex justify-center' : ''">
      <router-link
        to="/"
        class="flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors cursor-pointer text-[var(--text-muted)] hover:bg-[var(--sidebar-hover,rgb(148_163_184/0.1))] hover:text-[var(--text-primary)]"
        :title="collapsed ? 'Back to Dashboard' : undefined"
        @mouseenter="prefetch('/')"
        @focus="prefetch('/')"
      >
        <ArrowLeft class="h-4 w-4 shrink-0" />
        <span v-if="!collapsed">Back to Dashboard</span>
      </router-link>
    </div>

    <nav class="flex-1 overflow-y-auto px-2 py-2">
      <template v-for="(section, si) in settingsSections" :key="section.label">
        <div
          v-if="si > 0"
          class="mx-1 border-t border-[var(--border-primary)]"
          :class="collapsed ? 'my-2' : 'my-1'"
        />
        <div
          v-if="!collapsed"
          class="px-3 pb-1 text-[10px] font-semibold tracking-wider uppercase text-[var(--text-muted)]"
          :class="si === 0 ? 'pt-1' : 'pt-3'"
        >
          {{ section.label }}
        </div>
        <div class="space-y-0.5">
          <router-link
            v-for="item in section.items"
            :key="item.to"
            :to="item.to"
            class="flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors cursor-pointer"
            :title="collapsed ? item.label : undefined"
            @mouseenter="prefetch(item.to)"
            @focus="prefetch(item.to)"
            :class="
              isActive(item.to)
                ? 'bg-[var(--sidebar-active,rgb(255_255_255/0.12))] text-[var(--text-primary)] font-medium'
                : 'text-[var(--text-muted)] hover:bg-[var(--sidebar-hover,rgb(148_163_184/0.1))] hover:text-[var(--text-primary)]'
            "
          >
            <component :is="item.icon" class="h-4 w-4 shrink-0" />
            <span v-if="!collapsed">{{ item.label }}</span>
          </router-link>
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
    </div>
  </aside>
</template>
