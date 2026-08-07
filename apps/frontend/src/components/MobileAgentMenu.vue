<script setup lang="ts">
import { ref, watch } from "vue";
import { useRoute } from "vue-router";
import DialogCloseButton from "@/components/ui/DialogCloseButton.vue";
import { isActiveRoute } from "@/lib/routing";
import { useRoutePrefetch } from "@/composables/useRoutePrefetch";
import { useDropdownLifecycle } from "@/composables/useDropdownLifecycle";
import { useNavSections } from "@/lib/nav";

const emit = defineEmits<{
  close: [];
}>();

const route = useRoute();
const { prefetch } = useRoutePrefetch();
const { mobileAgentItems: agentItems } = useNavSections();

const sheetOpen = ref(true);
const sheetRef = ref<HTMLElement | null>(null);
useDropdownLifecycle(sheetOpen, sheetRef);
watch(sheetOpen, (open) => {
  if (!open) emit("close");
});

function isActive(to: string) {
  return isActiveRoute(route.path, to);
}

watch(
  () => route.path,
  () => emit("close"),
);
</script>

<template>
  <Teleport to="body">
    <div class="more-menu-backdrop" @click="emit('close')" />
    <div
      ref="sheetRef"
      class="more-menu-sheet flex flex-col"
      role="dialog"
      aria-modal="true"
      aria-label="Agents"
    >
      <div
        class="flex shrink-0 items-center justify-between border-b border-[var(--border-primary)] px-4 py-3"
      >
        <span class="text-sm font-medium text-[var(--text-secondary)]">Agents</span>
        <DialogCloseButton :on-click="() => emit('close')" label="Close menu" size="md" />
      </div>
      <nav class="px-2 py-2" aria-label="Agents navigation">
        <div class="eyebrow px-3 pb-1 pt-1 text-[10px]">AI</div>
        <div class="space-y-0.5">
          <router-link
            v-for="item in agentItems"
            :key="item.to"
            :to="item.to"
            class="flex items-center gap-3 rounded-md px-3 py-2.5 text-sm transition-colors cursor-pointer"
            :class="isActive(item.to) ? 'nav-link-active' : 'nav-link-inactive'"
            @mouseenter="prefetch(item.to)"
            @focus="prefetch(item.to)"
          >
            <component :is="item.icon" class="h-4 w-4 shrink-0" />
            <span>{{ item.label }}</span>
          </router-link>
        </div>
      </nav>
    </div>
  </Teleport>
</template>
