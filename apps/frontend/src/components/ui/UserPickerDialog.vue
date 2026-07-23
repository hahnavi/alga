<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { Bot, Check, Search } from "@lucide/vue";
import type { UserInfo } from "@/lib/api";
import Modal from "@/components/ui/Modal.vue";
import Avatar from "@/components/ui/Avatar.vue";
import { useListFilter } from "@/composables/useListFilter";
import { getAgentAvatarSrc } from "@/lib/agentAvatar";

const props = withDefaults(
  defineProps<{
    open: boolean;
    title?: string;
    users: UserInfo[];
    selectedUserId?: string;
    /** When true, shows an "Agent (auto-dispatch)" option above the user list. */
    showAgentOption?: boolean;
    agentSelected?: boolean;
  }>(),
  {
    title: "Assign to user",
    showAgentOption: false,
    agentSelected: false,
  },
);

const emit = defineEmits<{
  close: [];
  pickUser: [userId: string];
  pickAgent: [];
}>();

const query = ref("");
const usersRef = computed(() => props.users);
const filteredUsers = useListFilter(usersRef, ["full_name", "email"], query);

watch(
  () => props.open,
  (open) => {
    if (open) query.value = "";
  },
);

function userName(u: UserInfo): string {
  return u.full_name?.trim() || u.email;
}

function initial(u: UserInfo): string {
  return userName(u).charAt(0).toUpperCase() || "?";
}
</script>

<template>
  <Modal
    :open="open"
    :title="title"
    max-width="md"
    :show-footer="false"
    @update:open="!$event && emit('close')"
    @close="emit('close')"
  >
    <div class="relative">
      <Search
        class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--text-muted)]"
        aria-hidden="true"
      />
      <input
        v-model="query"
        type="search"
        placeholder="Search by name or email…"
        class="field w-full !pl-9"
        aria-label="Search users"
        autofocus
      />
    </div>

    <div class="mt-3 max-h-72 space-y-1 overflow-y-auto pr-1">
      <button
        v-if="showAgentOption"
        type="button"
        class="flex w-full cursor-pointer items-center gap-3 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2.5 text-left transition-colors hover:bg-[var(--btn-default-hover)]"
        @click="emit('pickAgent')"
      >
        <span
          class="flex h-8 w-8 shrink-0 items-center justify-center overflow-hidden rounded-full bg-[var(--bg-tertiary)]"
        >
          <img
            :src="getAgentAvatarSrc()"
            alt=""
            class="h-full w-full object-cover"
            loading="lazy"
            decoding="async"
          />
        </span>
        <span class="min-w-0 flex-1">
          <span class="flex items-center gap-1.5 text-sm font-medium text-[var(--text-primary)]">
            <Bot class="h-3.5 w-3.5 text-[var(--text-muted)]" aria-hidden="true" />
            Agent (auto-dispatch)
          </span>
          <span class="block truncate text-xs text-[var(--text-muted)]">
            Hand back to the scheduler for automatic agent assignment
          </span>
        </span>
        <Check
          v-if="agentSelected"
          class="h-4 w-4 shrink-0 text-[var(--accent-primary)]"
          aria-hidden="true"
        />
      </button>

      <template v-if="filteredUsers.length > 0">
        <button
          v-for="u in filteredUsers"
          :key="u.id"
          type="button"
          class="flex w-full cursor-pointer items-center gap-3 rounded-md px-3 py-2.5 text-left transition-colors hover:bg-[var(--bg-secondary)]"
          :class="{ 'bg-[var(--bg-secondary)]': u.id === selectedUserId }"
          @click="emit('pickUser', u.id)"
        >
          <Avatar
            :letter="initial(u)"
            bg-class="bg-[var(--accent-primary)]/15 !text-[var(--accent-primary)]"
            :title="userName(u)"
            class="!h-8 !w-8 !text-sm"
          />
          <span class="min-w-0 flex-1">
            <span class="block truncate text-sm font-medium text-[var(--text-primary)]">
              {{ userName(u) }}
            </span>
            <span class="block truncate text-xs text-[var(--text-muted)]">
              {{ u.email }}<template v-if="u.role"> · {{ u.role }}</template>
            </span>
          </span>
          <Check
            v-if="u.id === selectedUserId"
            class="h-4 w-4 shrink-0 text-[var(--accent-primary)]"
            aria-hidden="true"
          />
        </button>
      </template>

      <div v-else class="px-3 py-8 text-center text-sm text-[var(--text-muted)]">
        {{ query.trim() ? "No users match your search." : "No users available." }}
      </div>
    </div>
  </Modal>
</template>
