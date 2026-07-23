<script setup lang="ts">
import { computed, ref } from "vue";
import { Bot, Copy, Link2, List, ListTree, Lock, MessageSquare, SearchCheck } from "@lucide/vue";
import type { IncidentCoordinationMessage } from "@/lib/api";
import ChatDateSeparator from "@/components/ui/ChatDateSeparator.vue";
import ChatMessageRow from "@/components/ui/ChatMessageRow.vue";
import MessageContextMenu, { type MessageAction } from "@/components/ui/MessageContextMenu.vue";
import { formatDateSeparator, dateSeparatorKey } from "@/lib/time";
import { useClipboard } from "@/composables/useClipboard";
import CoordinationThreadNode from "./CoordinationThreadNode.vue";
import {
  avatarBg,
  avatarLetter,
  borderClass,
  displayName,
  groupMessagesByParent,
  rootMessages,
} from "./coordinationHelpers";
import { getAgentAvatarSrc } from "@/lib/agentAvatar";

const avatarSrc = (msg: {
  source?: string;
  metadata?: { agent_type?: unknown };
  agent_type?: string;
}) =>
  msg.source === "agent"
    ? getAgentAvatarSrc((msg.metadata?.agent_type as string | undefined) ?? msg.agent_type)
    : undefined;

const props = defineProps<{
  messages: IncidentCoordinationMessage[];
}>();

const { copyToClipboard } = useClipboard();

const threaded = ref(true);

const contextMenu = ref<{
  message: IncidentCoordinationMessage;
  position: { clientX: number; clientY: number };
} | null>(null);

function onMessageContextMenu(payload: { id: string; clientX: number; clientY: number }) {
  const message = props.messages.find((m) => m.id === payload.id);
  if (!message) return;
  contextMenu.value = {
    message,
    position: { clientX: payload.clientX, clientY: payload.clientY },
  };
}

function closeContextMenu() {
  contextMenu.value = null;
}

function messagePermalink(messageId: string): string {
  if (typeof window === "undefined") return `#msg-${messageId}`;
  return `${window.location.origin}${window.location.pathname}#msg-${messageId}`;
}

const contextMenuActions = computed<MessageAction[]>(() => {
  const open = contextMenu.value;
  if (!open) return [];
  return [
    {
      key: "copy-text",
      label: "Copy text",
      icon: Copy,
      onSelect: () => copyToClipboard(open.message.body, "Message copied"),
    },
    {
      key: "copy-link",
      label: "Copy link",
      icon: Link2,
      onSelect: () => copyToClipboard(messagePermalink(open.message.id), "Link copied"),
    },
  ];
});

// --- Threaded mode -------------------------------------------------------

const messagesByParent = computed(() => groupMessagesByParent(props.messages));
const roots = computed(() => rootMessages(props.messages, messagesByParent.value));

// --- Chronological (flat) mode ------------------------------------------

type StreamItem =
  | { kind: "date"; key: string; label: string }
  | { kind: "msg"; message: IncidentCoordinationMessage };

const streamItems = computed((): StreamItem[] => {
  const items: StreamItem[] = [];
  let lastDate = "";
  for (const message of props.messages) {
    const date = dateSeparatorKey(message.created_at);
    if (date !== lastDate) {
      items.push({
        kind: "date",
        key: `sep-${date}`,
        label: formatDateSeparator(message.created_at),
      });
      lastDate = date;
    }
    items.push({ kind: "msg", message });
  }
  return items;
});

const isEmpty = computed(() => props.messages.length === 0);
</script>

<template>
  <div class="flex flex-col gap-3">
    <div
      v-if="isEmpty"
      class="rounded-lg border border-dashed border-[var(--border-primary)] py-10 text-center"
    >
      <MessageSquare class="mx-auto mb-2 h-6 w-6 text-[var(--text-muted)]" />
      <p class="text-sm font-medium text-[var(--text-primary)]">No coordination messages yet</p>
      <p class="mt-1 text-xs text-[var(--text-muted)]">
        Start with a status update, decision, or agent request.
      </p>
    </div>

    <template v-else>
      <!-- View toggle -->
      <div class="flex justify-end">
        <div
          class="inline-flex items-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-card)] p-0.5"
          role="group"
          aria-label="Stream view"
        >
          <button
            type="button"
            class="inline-flex items-center gap-1 rounded px-2 py-1 text-xs font-medium transition-colors"
            :class="
              threaded
                ? 'bg-[var(--bg-secondary)] text-[var(--text-primary)]'
                : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'
            "
            :aria-pressed="threaded"
            title="Threaded view"
            @click="threaded = true"
          >
            <ListTree class="h-3.5 w-3.5" />
            Threaded
          </button>
          <button
            type="button"
            class="inline-flex items-center gap-1 rounded px-2 py-1 text-xs font-medium transition-colors"
            :class="
              !threaded
                ? 'bg-[var(--bg-secondary)] text-[var(--text-primary)]'
                : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'
            "
            :aria-pressed="!threaded"
            title="Chronological view"
            @click="threaded = false"
          >
            <List class="h-3.5 w-3.5" />
            Chronological
          </button>
        </div>
      </div>

      <!-- Threaded -->
      <template v-if="threaded">
        <CoordinationThreadNode
          v-for="root in roots"
          :key="root.id"
          :message="root"
          :messages-by-parent="messagesByParent"
          :depth="0"
          @context-menu="onMessageContextMenu"
        />
      </template>

      <!-- Chronological (flat) -->
      <template v-else>
        <template
          v-for="item in streamItems"
          :key="item.kind === 'date' ? item.key : item.message.id"
        >
          <ChatDateSeparator v-if="item.kind === 'date'" :label="item.label" />
          <ChatMessageRow
            v-else
            :id="item.message.id"
            variant="thread"
            :border-class="borderClass(item.message)"
            :avatar-src="avatarSrc(item.message)"
            :avatar-letter="
              item.message.actor_type === 'agent' ? undefined : avatarLetter(item.message)
            "
            :avatar-bg="avatarBg(item.message)"
            :avatar-title="displayName(item.message)"
            :display-name="displayName(item.message)"
            :created-at="item.message.created_at"
            :content="item.message.body"
            :internal="item.message.internal"
            @context-menu="onMessageContextMenu"
          >
            <template #meta-extras>
              <Lock
                v-if="item.message.internal"
                class="h-3.5 w-3.5 text-amber-500"
                title="Internal note"
              />
              <Bot
                v-if="item.message.actor_type === 'agent'"
                class="h-3.5 w-3.5 text-purple-500"
                title="Agent"
              />
              <SearchCheck
                v-if="item.message.kind === 'investigation_summary'"
                class="h-3.5 w-3.5 text-cyan-500"
                title="Investigation summary"
              />
              <span
                v-if="item.message.kind !== 'chat' && item.message.kind !== 'agent_reply'"
                class="text-xs font-medium text-[var(--text-secondary)]"
              >
                {{ item.message.kind.replace("_", " ") }}
              </span>
            </template>
          </ChatMessageRow>
        </template>
      </template>
    </template>

    <MessageContextMenu
      :open="contextMenu !== null"
      :position="contextMenu?.position ?? null"
      :actions="contextMenuActions"
      :aria-label="'Coordination message actions'"
      @close="closeContextMenu"
    />
  </div>
</template>
