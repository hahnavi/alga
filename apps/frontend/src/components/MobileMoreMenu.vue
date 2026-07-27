<script setup lang="ts">
import { useRoute, useRouter } from "vue-router";
import { LogOut, Settings, X } from "@lucide/vue";
import { useAuthStore } from "@/stores/auth";
import UserLabel from "@/components/ui/UserLabel.vue";
import { isActiveRoute } from "@/lib/routing";
import { useRoutePrefetch } from "@/composables/useRoutePrefetch";
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

function isActive(to: string) {
  return isActiveRoute(route.path, to);
}

function handleNav() {
  emit("close");
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
    <div class="more-menu-sheet">
      <div class="flex items-center justify-between px-5 py-3">
        <span class="text-sm font-medium text-[var(--text-secondary)]">Menu</span>
        <button
          class="flex h-7 w-7 cursor-pointer items-center justify-center rounded-full text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
          aria-label="Close menu"
          @click="emit('close')"
        >
          <X class="h-4 w-4" />
        </button>
      </div>

      <div class="max-h-[60vh] overflow-y-auto px-5 pb-5">
        <template v-for="(section, si) in sections" :key="section.label">
          <div v-if="si > 0" class="my-3 border-t border-[var(--border-primary)]" />
          <div
            class="mb-2 text-[10px] font-semibold tracking-wider uppercase text-[var(--text-muted)]"
          >
            {{ section.label }}
          </div>
          <div class="grid grid-cols-3 gap-2">
            <router-link
              v-for="item in section.items"
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
              @click="handleNav"
            >
              <component :is="item.icon" class="h-5 w-5" />
              <span>{{ item.label }}</span>
            </router-link>
          </div>
        </template>

        <div class="mt-3 border-t border-[var(--border-primary)] pt-3">
          <div
            class="mb-2 text-[10px] font-semibold tracking-wider uppercase text-[var(--text-muted)]"
          >
            User
          </div>
          <div class="mb-3 flex items-center gap-3 rounded-xl bg-[rgb(35_35_42_/_0.9)] px-3 py-3">
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
    </div>
  </Teleport>
</template>
