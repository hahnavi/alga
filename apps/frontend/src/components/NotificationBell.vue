<script setup lang="ts">
import { ref, watch } from "vue";
import { useRouter } from "vue-router";
import { Bell, Users, Check, CheckCheck } from "@lucide/vue";
import { useNotificationStore } from "@/stores/notifications";
import { useAuthStore } from "@/stores/auth";
import type { NotificationRecord } from "@/lib/api";
import { formatTimeAgo } from "@/lib/time";
import { useDropdownLifecycle } from "@/composables/useDropdownLifecycle";
import { usePopoverPosition } from "@/composables/usePopoverPosition";
import { handleNotificationClick } from "@/lib/notificationClick";
import { HEADER_ICON_BTN_CLASS } from "@/lib/uiClasses";

const props = defineProps<{ showBadge?: boolean }>();

const auth = useAuthStore();
const store = useNotificationStore();
const router = useRouter();

const rootRef = ref<HTMLElement | null>(null);
const contentRef = ref<HTMLElement | null>(null);
const open = ref(false);

useDropdownLifecycle(open, rootRef, contentRef);

const position = usePopoverPosition({
  trigger: rootRef,
  contentRef,
  isOpen: open,
  placement: "bottom-right",
});

function toggle() {
  open.value = !open.value;
  if (open.value && store.notifications.length === 0) {
    store.fetchNotifications(15);
  }
}

function close() {
  open.value = false;
}

async function clickNotification(n: NotificationRecord) {
  await handleNotificationClick(n, store, router, close);
}

async function markAllRead() {
  await store.markAllRead();
}

function viewAll() {
  close();
  router.push("/notifications");
}

watch(
  () => auth.user,
  (user) => {
    if (!user) {
      store.reset();
    }
  },
);
</script>

<template>
  <div v-if="auth.user" ref="rootRef" class="relative">
    <button
      type="button"
      :class="HEADER_ICON_BTN_CLASS"
      :aria-label="`Notifications${store.hasUnread ? ` (${store.unreadCount} unread)` : ''}`"
      :title="`Notifications${store.hasUnread ? ` (${store.unreadCount} unread)` : ''}`"
      aria-haspopup="menu"
      :aria-expanded="open"
      @click="toggle"
    >
      <Bell class="h-5 w-5" aria-hidden="true" />
      <span
        v-if="props.showBadge !== false && store.hasUnread"
        class="absolute top-1 right-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-[var(--accent)] px-1 text-[10px] font-bold leading-none text-white"
      >
        {{ store.unreadCount > 99 ? "99+" : store.unreadCount }}
      </span>
    </button>

    <Teleport to="body">
      <Transition
        enterActiveClass="transition duration-150 ease-out"
        enterFromClass="opacity-0 scale-95"
        enterToClass="opacity-100 scale-100"
        leaveActiveClass="transition duration-100 ease-in"
        leaveFromClass="opacity-100 scale-100"
        leaveToClass="opacity-0 scale-95"
      >
        <div
          v-if="open"
          ref="contentRef"
          class="fixed z-50 w-80 overflow-hidden rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-lg"
          :style="{
            top: position.top ? `${position.top}px` : undefined,
            right: position.right ? `${position.right}px` : undefined,
            bottom: position.bottom ? `${position.bottom}px` : undefined,
            left: position.left ? `${position.left}px` : undefined,
          }"
          role="menu"
          aria-label="Notifications"
        >
          <div
            class="flex items-center justify-between border-b border-[var(--border-primary)] px-3 py-2.5"
          >
            <span class="text-sm font-semibold uppercase tracking-wide text-[var(--text-muted)]"
              >Notifications</span
            >
            <button
              v-if="store.hasUnread"
              type="button"
              class="inline-flex cursor-pointer items-center gap-1 rounded px-1.5 py-0.5 text-xs text-[var(--text-muted)] transition-colors hover:bg-[var(--sidebar-hover,rgb(148_163_184/0.12))] hover:text-[var(--text-primary)]"
              @click="markAllRead"
            >
              <CheckCheck class="h-3 w-3" />
              Mark all read
            </button>
          </div>

          <div class="max-h-80 overflow-y-auto">
            <template v-if="store.notifications.length > 0">
              <button
                v-for="n in store.notifications.slice(0, 8)"
                :key="n.id"
                type="button"
                class="flex w-full cursor-pointer gap-2.5 border-b border-[var(--border-primary)] px-3 py-2.5 text-left transition-colors last:border-b-0 hover:bg-[var(--sidebar-hover,rgb(148_163_184/0.08))]"
                :class="n.read ? '' : 'bg-[var(--focus-ring)]/[0.08]'"
                @click="clickNotification(n)"
              >
                <div class="mt-0.5 shrink-0">
                  <div
                    class="flex h-7 w-7 items-center justify-center rounded-full"
                    :class="
                      n.type === 'mention'
                        ? 'bg-[var(--bg-badge-info)] text-[var(--text-badge-info)]'
                        : 'bg-[var(--bg-primary)] text-[var(--text-muted)]'
                    "
                  >
                    <Users v-if="n.type === 'mention'" class="h-3.5 w-3.5" />
                    <Bell v-else class="h-3.5 w-3.5" />
                  </div>
                </div>
                <div class="min-w-0 flex-1">
                  <p
                    class="truncate text-sm text-[var(--text-primary)]"
                    :class="n.read ? 'font-normal' : 'font-medium'"
                  >
                    {{ n.title }}
                  </p>
                  <p v-if="n.message" class="mt-0.5 truncate text-xs text-[var(--text-muted)]">
                    {{ n.message }}
                  </p>
                  <p class="mt-0.5 text-[11px] text-[var(--text-muted)]">
                    {{ formatTimeAgo(n.created_at) }}
                  </p>
                </div>
                <div v-if="!n.read" class="mt-2 shrink-0">
                  <span class="inline-block h-2 w-2 rounded-full bg-[var(--accent)]" />
                </div>
              </button>
            </template>
            <div v-else class="px-3 py-8 text-center text-sm text-[var(--text-muted)]">
              <Bell class="mx-auto mb-2 h-6 w-6 opacity-40" />
              No notifications yet
            </div>
          </div>

          <div class="border-t border-[var(--border-primary)]">
            <button
              type="button"
              class="flex w-full cursor-pointer items-center justify-center gap-1.5 px-3 py-2.5 text-xs font-medium text-[var(--text-muted)] transition-colors hover:bg-[var(--sidebar-hover,rgb(148_163_184/0.08))] hover:text-[var(--text-primary)]"
              @click="viewAll"
            >
              <Check class="h-3 w-3" />
              View all notifications
            </button>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>
