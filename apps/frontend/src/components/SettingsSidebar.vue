<script setup lang="ts">
import { useAttrs } from "vue";
import { useRoute } from "vue-router";
import { ArrowLeft } from "@lucide/vue";
import { isActiveRoute } from "@/lib/routing";
import { useNavSections } from "@/lib/nav";
import { useRoutePrefetch } from "@/composables/useRoutePrefetch";
import UserMenuBar from "@/components/UserMenuBar.vue";

defineOptions({ inheritAttrs: false });

const attrs = useAttrs();
const route = useRoute();
const { prefetch } = useRoutePrefetch();
const { settingsSections } = useNavSections();

function isActive(to: string) {
  return isActiveRoute(route.path, to);
}
</script>

<template>
  <aside
    v-bind="attrs"
    class="flex h-screen w-[240px] shrink-0 flex-col overflow-visible border-r border-[var(--border-primary)] bg-[var(--bg-sidebar)]"
  >
    <div class="flex h-14 shrink-0 items-center gap-2 border-b border-[var(--border-primary)] px-2">
      <router-link
        to="/"
        class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm text-[var(--text-muted)] transition-colors hover:bg-[var(--sidebar-hover,rgb(148_163_184/0.1))] hover:text-[var(--text-primary)]"
        @mouseenter="prefetch('/')"
        @focus="prefetch('/')"
      >
        <ArrowLeft class="h-4 w-4 shrink-0" />
        <span>Back to Dashboard</span>
      </router-link>
    </div>

    <div class="px-5 pb-1 pt-4 text-base font-semibold">Settings</div>

    <nav class="flex-1 overflow-y-auto px-2 py-2">
      <template v-for="(section, si) in settingsSections" :key="section.label">
        <div v-if="si > 0" class="mx-1 my-1 border-t border-[var(--border-primary)]" />
        <div
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
            @mouseenter="prefetch(item.to)"
            @focus="prefetch(item.to)"
            :class="
              isActive(item.to)
                ? 'bg-[var(--sidebar-active,rgb(255_255_255/0.12))] text-[var(--text-primary)] font-medium'
                : 'text-[var(--text-muted)] hover:bg-[var(--sidebar-hover,rgb(148_163_184/0.1))] hover:text-[var(--text-primary)]'
            "
          >
            <component :is="item.icon" class="h-4 w-4 shrink-0" />
            <span>{{ item.label }}</span>
          </router-link>
        </div>
      </template>
    </nav>

    <div class="shrink-0 border-t border-[var(--border-primary)] px-2 py-3">
      <UserMenuBar show-name />
    </div>
  </aside>
</template>
