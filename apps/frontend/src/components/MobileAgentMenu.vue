<script setup lang="ts">
import { watch } from "vue";
import { useRoute } from "vue-router";
import DialogCloseButton from "@/components/ui/DialogCloseButton.vue";
import { isActiveRoute } from "@/lib/routing";
import { useRoutePrefetch } from "@/composables/useRoutePrefetch";
import { useNavSections } from "@/lib/nav";

const emit = defineEmits<{
  close: [];
}>();

const route = useRoute();
const { prefetch } = useRoutePrefetch();
const { mobileAgentItems: agentItems } = useNavSections();

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
    <div class="more-menu-sheet">
      <div class="flex items-center justify-between px-5 py-3">
        <span class="text-sm font-medium text-[var(--text-secondary)]">Agents</span>
        <DialogCloseButton :on-click="() => emit('close')" label="Close menu" size="md" />
      </div>
      <div class="grid grid-cols-3 gap-2 px-5 pb-5 pt-1">
        <router-link
          v-for="item in agentItems"
          :key="item.to"
          :to="item.to"
          class="flex flex-col items-center gap-1.5 rounded-xl px-2 py-3 text-[11px] transition-colors cursor-pointer"
          @mouseenter="prefetch(item.to)"
          @focus="prefetch(item.to)"
          :class="
            isActive(item.to)
              ? 'bg-[var(--sidebar-active,rgb(255_255_255/0.12))] text-[var(--text-primary)] font-medium'
              : 'text-[var(--text-secondary)] hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)]'
          "
        >
          <component :is="item.icon" class="h-5 w-5" />
          <span>{{ item.label }}</span>
        </router-link>
      </div>
    </div>
  </Teleport>
</template>
