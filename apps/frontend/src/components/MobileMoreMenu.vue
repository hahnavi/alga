<script setup lang="ts">
import { ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { LogOut, Settings } from "@lucide/vue";
import { useAuthStore } from "@/stores/auth";
import UserLabel from "@/components/ui/UserLabel.vue";
import LogoMark from "@/components/ui/LogoMark.vue";
import DialogCloseButton from "@/components/ui/DialogCloseButton.vue";
import { isActiveRoute } from "@/lib/routing";
import { useRoutePrefetch } from "@/composables/useRoutePrefetch";
import { useDropdownLifecycle } from "@/composables/useDropdownLifecycle";
import { useNavSections } from "@/lib/nav";
import { MOBILE_MORE_USER_ACTION_CLASS } from "@/lib/uiClasses";

const emit = defineEmits<{
  close: [];
}>();

const auth = useAuthStore();
const route = useRoute();
const router = useRouter();
const { prefetch } = useRoutePrefetch();
const { mobileMoreSections: sections } = useNavSections();

const sheetOpen = ref(true);
const sheetRef = ref<HTMLElement | null>(null);
useDropdownLifecycle(sheetOpen, sheetRef);
watch(sheetOpen, (open) => {
  if (!open) emit("close");
});
watch(
  () => route.path,
  () => emit("close"),
);

function isActive(to: string) {
  return isActiveRoute(route.path, to);
}

function openSettings() {
  emit("close");
  router.push("/settings/general");
}

async function handleLogout() {
  emit("close");
  await auth.logout();
  router.push("/login");
}
</script>

<template>
  <Teleport to="body">
    <div class="more-menu-backdrop" @click="emit('close')" />
    <div
      ref="sheetRef"
      class="more-menu-sheet flex flex-col"
      role="dialog"
      aria-modal="true"
      aria-label="Menu"
    >
      <div
        class="flex shrink-0 items-center justify-between border-b border-[var(--border-primary)] px-4 py-3"
      >
        <div class="flex min-w-0 items-center gap-2">
          <LogoMark class="h-7 w-7" />
          <span class="flex min-w-0 flex-col">
            <span class="text-[15px] font-semibold leading-tight tracking-tight">Alga</span>
            <span
              class="font-mono text-[9px] font-medium leading-none tracking-[0.24em] text-[var(--accent)]"
            >
              OPS CONSOLE
            </span>
          </span>
        </div>
        <DialogCloseButton :on-click="() => emit('close')" label="Close menu" size="md" />
      </div>

      <nav class="max-h-[52vh] overflow-y-auto px-2 py-2" aria-label="More navigation">
        <template v-for="(section, si) in sections" :key="section.label">
          <div v-if="si > 0" class="mx-1 my-2 border-t border-[var(--border-primary)]" />
          <div class="eyebrow px-3 pb-1 pt-2 text-[10px]">
            {{ section.label }}
          </div>
          <div class="space-y-0.5">
            <router-link
              v-for="item in section.items"
              :key="item.to"
              :to="item.to"
              class="flex items-center gap-3 rounded-md px-3 py-2.5 text-sm transition-colors cursor-pointer"
              :class="isActive(item.to) ? 'nav-link-active' : 'nav-link-inactive'"
              @mouseenter="prefetch(item.to)"
              @focus="prefetch(item.to)"
              @click="emit('close')"
            >
              <component :is="item.icon" class="h-4 w-4 shrink-0" />
              <span>{{ item.label }}</span>
            </router-link>
          </div>
        </template>
      </nav>

      <div class="shrink-0 border-t border-[var(--border-primary)] px-3 py-3">
        <div class="mb-2 flex items-center gap-3 rounded-xl bg-[var(--bg-secondary)] px-3 py-2.5">
          <div
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-[var(--btn-default-bg)] text-sm font-medium text-[var(--text-primary)]"
          >
            {{ (auth.user?.full_name || auth.user?.email || "?").charAt(0).toUpperCase() }}
          </div>
          <div class="min-w-0 flex-1">
            <p class="truncate text-sm font-medium text-[var(--text-primary)]">
              <UserLabel :user="auth.user" />
            </p>
            <p v-if="auth.user?.full_name" class="truncate text-xs text-[var(--text-muted)]">
              {{ auth.user?.email }}
            </p>
          </div>
        </div>
        <button :class="MOBILE_MORE_USER_ACTION_CLASS" @click="openSettings">
          <Settings class="h-4 w-4" />
          Settings
        </button>
        <button :class="MOBILE_MORE_USER_ACTION_CLASS" @click="handleLogout">
          <LogOut class="h-4 w-4" />
          Logout
        </button>
      </div>
    </div>
  </Teleport>
</template>
