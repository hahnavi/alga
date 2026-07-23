<script setup lang="ts">
import { computed, ref } from "vue";
import { Bot, ChevronDown, ChevronRight, Lock, SearchCheck } from "@lucide/vue";
import type { IncidentCoordinationMessage } from "@/lib/api";
import ChatMessageRow from "@/components/ui/ChatMessageRow.vue";
import {
  avatarBg,
  avatarLetter,
  borderClass,
  childrenOf,
  displayName,
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

defineOptions({ name: "CoordinationThreadNode" });

const props = defineProps<{
  message: IncidentCoordinationMessage;
  messagesByParent: Map<string, IncidentCoordinationMessage[]>;
  depth: number;
}>();

const emit = defineEmits<{
  "context-menu": [payload: { id: string; clientX: number; clientY: number }];
}>();

const collapsed = ref(false);

// Cap visual indentation; the tree depth is uncapped structurally.
const MAX_INDENT_DEPTH = 6;
const indentRem = computed(() => Math.min(props.depth, MAX_INDENT_DEPTH) * 1.5);
const indentStyle = computed(() => ({ marginLeft: `${indentRem.value}rem` }));

const children = computed(() => childrenOf(props.message.id, props.messagesByParent));
const hasChildren = computed(() => children.value.length > 0);

function onContextMenu(payload: { id: string; clientX: number; clientY: number }) {
  emit("context-menu", payload);
}
</script>

<template>
  <div :style="depth > 0 ? indentStyle : undefined">
    <div class="flex items-start gap-0.5">
      <button
        v-if="hasChildren"
        type="button"
        class="mt-2 flex h-5 w-5 shrink-0 cursor-pointer items-center justify-center rounded text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
        :aria-label="collapsed ? 'Expand thread' : 'Collapse thread'"
        :aria-expanded="!collapsed"
        @click="collapsed = !collapsed"
      >
        <ChevronDown v-if="!collapsed" class="h-3.5 w-3.5" />
        <ChevronRight v-else class="h-3.5 w-3.5" />
      </button>
      <div v-else class="w-5 shrink-0" aria-hidden="true" />
      <div class="min-w-0 flex-1">
        <ChatMessageRow
          :id="message.id"
          variant="thread"
          :border-class="borderClass(message)"
          :avatar-src="avatarSrc(message)"
          :avatar-letter="message.actor_type === 'agent' ? undefined : avatarLetter(message)"
          :avatar-bg="avatarBg(message)"
          :avatar-title="displayName(message)"
          :display-name="displayName(message)"
          :created-at="message.created_at"
          :content="message.body"
          :internal="message.internal"
          @context-menu="onContextMenu"
        >
          <template #meta-extras>
            <Lock
              v-if="message.internal"
              class="h-3.5 w-3.5 text-amber-500"
              title="Internal note"
            />
            <Bot
              v-if="message.actor_type === 'agent'"
              class="h-3.5 w-3.5 text-purple-500"
              title="Agent"
            />
            <SearchCheck
              v-if="message.kind === 'investigation_summary'"
              class="h-3.5 w-3.5 text-cyan-500"
              title="Investigation summary"
            />
            <span
              v-if="message.kind !== 'chat' && message.kind !== 'agent_reply'"
              class="text-xs font-medium text-[var(--text-secondary)]"
            >
              {{ message.kind.replace("_", " ") }}
            </span>
          </template>
        </ChatMessageRow>

        <template v-if="!collapsed">
          <CoordinationThreadNode
            v-for="child in children"
            :key="child.id"
            :message="child"
            :messages-by-parent="messagesByParent"
            :depth="depth + 1"
            @context-menu="onContextMenu"
          />
        </template>
      </div>
    </div>
  </div>
</template>
