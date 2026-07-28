<script setup lang="ts">
import { computed, ref, useId } from "vue";
import { useRouter } from "vue-router";
import { ChevronDown, LogOut } from "@lucide/vue";
import { useAuthStore } from "@/stores/auth";
import UserLabel from "@/components/ui/UserLabel.vue";
import { useDropdownLifecycle } from "@/composables/useDropdownLifecycle";
import { usePopoverPosition, type PopoverPlacement } from "@/composables/usePopoverPosition";
import { ACCOUNT_MENU_ITEM_CLASS } from "@/lib/uiClasses";

defineOptions({ inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    showName?: boolean;
    compact?: boolean;
  }>(),
  {
    showName: false,
    compact: false,
  },
);

const menuId = useId();
const auth = useAuthStore();
const router = useRouter();
const rootRef = ref<HTMLElement | null>(null);
const contentRef = ref<HTMLElement | null>(null);
const open = ref(false);

useDropdownLifecycle(open, rootRef, contentRef);

const placement = computed<PopoverPlacement>(() => (props.compact ? "right" : "top-left"));

const menuPosition = usePopoverPosition({
  trigger: rootRef,
  contentRef,
  isOpen: open,
  placement,
});

function close() {
  open.value = false;
}

async function handleLogout() {
  close();
  await auth.logout();
  router.push("/login");
}

function toggle() {
  open.value = !open.value;
}
</script>

<template>
  <div v-if="auth.user" ref="rootRef" class="relative" v-bind="$attrs">
    <button
      type="button"
      class="inline-flex w-full cursor-pointer items-center gap-1 rounded-md p-1.5 text-sm text-[var(--text-primary)] transition-colors hover:bg-[var(--sidebar-hover,rgb(148_163_184/0.15))] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)] md:gap-1.5 md:px-2 md:py-1.5"
      :aria-expanded="open"
      aria-haspopup="menu"
      :aria-controls="menuId"
      :aria-label="`Account menu for ${auth.user.full_name || auth.user.email}`"
      :title="`Account menu for ${auth.user.full_name || auth.user.email}`"
      @click="toggle"
    >
      <div
        class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-[var(--btn-default-bg)] text-sm font-medium"
      >
        {{ (auth.user.full_name || auth.user.email).charAt(0).toUpperCase() }}
      </div>
      <span
        v-if="showName && !compact"
        class="ml-1 hidden truncate text-sm text-[var(--text-secondary)] md:inline"
      >
        <UserLabel :user="auth.user" />
      </span>
      <ChevronDown
        v-if="!compact"
        class="hidden h-4 w-4 shrink-0 text-[var(--text-muted)] md:block"
        :class="{ 'rotate-180': open }"
        aria-hidden="true"
      />
    </button>

    <Teleport to="body">
      <div
        v-if="open"
        ref="contentRef"
        :id="menuId"
        class="fixed z-50 min-w-[11rem] rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-1 text-[var(--text-primary)] shadow-lg"
        :style="{
          top: menuPosition.top ? `${menuPosition.top}px` : undefined,
          right: menuPosition.right ? `${menuPosition.right}px` : undefined,
          bottom: menuPosition.bottom ? `${menuPosition.bottom}px` : undefined,
          left: menuPosition.left ? `${menuPosition.left}px` : undefined,
        }"
        role="menu"
        aria-label="Account menu"
      >
        <div
          class="pointer-events-none border-b border-[var(--border-primary)] px-3 py-2.5"
          aria-hidden="true"
        >
          <p class="truncate text-sm font-medium text-[var(--text-primary)]">
            <UserLabel :user="auth.user" />
          </p>
          <p v-if="auth.user.full_name" class="mt-0.5 truncate text-xs text-[var(--text-muted)]">
            {{ auth.user.email }}
          </p>
        </div>
        <button
          type="button"
          :class="ACCOUNT_MENU_ITEM_CLASS"
          role="menuitem"
          @click="handleLogout"
        >
          <LogOut class="h-4 w-4 shrink-0" />
          Logout
        </button>
      </div>
    </Teleport>
  </div>
</template>
