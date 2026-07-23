<script setup lang="ts">
import { computed, nextTick, onMounted, ref, toRef } from "vue";
import { Bot, Copy, HatGlasses, Link2, Reply, X } from "@lucide/vue";
import {
  api,
  type OwnerThread,
  type OwnerThreadMessage,
  type UserInfo,
  type AgentTokenRow,
} from "@/lib/api";
import { getErrorMessage } from "@/lib/error";
import { getAgentAvatarSrc } from "@/lib/agentAvatar";
import {
  displayName,
  messagePermalink,
  shouldShowAgentAvatar,
  sourceAvatarBg,
  sourceColor,
} from "@/lib/chatMessage";
import { useChatThread } from "@/composables/useChatThread";
import { useClipboard } from "@/composables/useClipboard";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { useStickToBottom } from "@/composables/useStickToBottom";
import { useTypingIndicator } from "@/composables/useTypingIndicator";
import ChatDateSeparator from "@/components/ui/ChatDateSeparator.vue";
import ChatEditorBar from "@/components/ui/ChatEditorBar.vue";
import ChatMessageRow from "@/components/ui/ChatMessageRow.vue";
import ChatTypingIndicator from "@/components/ui/ChatTypingIndicator.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import IconBtn from "@/components/ui/IconBtn.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import MarkdownEditor from "@/components/ui/MarkdownEditor.vue";
import MessageContextMenu, { type MessageAction } from "@/components/ui/MessageContextMenu.vue";
import TypingIndicator from "@/components/ui/TypingIndicator.vue";
import { formatDateSeparator, dateSeparatorKey } from "@/lib/time";

const avatarSrc = (msg: OwnerThreadMessage): string | undefined =>
  shouldShowAgentAvatar(msg) ? getAgentAvatarSrc(msg.agent_type) : undefined;

const props = defineProps<{
  ownerType: "alert" | "incident_inv";
  ownerId: string;
  title: string;
  canWrite: boolean;
  emptyTitle?: string;
  emptyDescription?: string;
  users?: UserInfo[];
  agents?: AgentTokenRow[];
}>();

const emit = defineEmits<{ close: []; updated: [thread: OwnerThread] }>();

const ownerIdRef = toRef(props, "ownerId");
const commentText = ref("");
const replyingTo = ref<OwnerThreadMessage | null>(null);
const threadEl = ref<HTMLDivElement | null>(null);
const { stickToBottom, scrollToBottom } = useStickToBottom(threadEl);
const editorRef = ref<InstanceType<typeof MarkdownEditor> | null>(null);
const { copyToClipboard } = useClipboard();
const { submitting, formError: error, withSubmit } = useFormSubmit();

const {
  isTyping: agentTyping,
  typingSource: agentTypingSource,
  typingAgentType: agentTypingAgentType,
  setTyping: setAgentTyping,
  clearTyping: clearAgentTyping,
} = useTypingIndicator({ timeoutMs: 6000 });

const {
  messages: threadMessages,
  drafts: agentDrafts,
  loading,
  reload,
} = useChatThread<OwnerThreadMessage>({
  scope: props.ownerType,
  targetId: ownerIdRef,
  fetchThread: async (id) => {
    const fresh =
      props.ownerType === "alert"
        ? await api.getAlertThread(Number(id))
        : await api.getIncidentThread(id);
    return fresh.messages ?? [];
  },
  extractMessage: (data) => (data as { message?: OwnerThreadMessage }).message,
  extractEdit: (data) => {
    const d = data as { message_id?: string; message?: string; edited?: boolean };
    if (!d.message_id || typeof d.message !== "string") return null;
    return { id: d.message_id, message: d.message, edited: d.edited };
  },
  extractDelete: (data) => {
    const d = data as { message_id?: string };
    return d.message_id ? { id: d.message_id } : null;
  },
  extractDraft: (data) => {
    const d = data as {
      draft_id?: string;
      message?: string;
      source?: OwnerThreadMessage["source"];
    };
    if (!d.draft_id) return null;
    return { draft_id: d.draft_id, message: d.message ?? "", source: d.source };
  },
  extractTyping: (data) => {
    const d = data as { source?: string; agent_type?: string };
    return { source: d.source ?? "agent", agent_type: d.agent_type };
  },
  buildDraft: (draftId, message, source, now) => ({
    id: `draft-${draftId}`,
    draft_id: draftId,
    draft: true,
    type: "comment" as const,
    source: source as OwnerThreadMessage["source"],
    message,
    username: "Agent",
    created_at: now,
    updated_at: now,
  }),
  isAgentMessage: (msg) => msg.source === "agent",
  onMessageAdded: () => stickToBottom(),
  onMessageEdited: () => stickToBottom(),
  onTyping: (source, agentType) => {
    setAgentTyping(source, agentType);
    stickToBottom();
  },
  onTypingStop: () => clearAgentTyping(),
});

const visibleMessages = computed(() => [...threadMessages.value, ...agentDrafts.value]);
const emptyTitle = computed(
  () => props.emptyTitle ?? `No ${props.title.toLowerCase()} messages yet`,
);
const emptyDescription = computed(
  () => props.emptyDescription ?? "Messages and notes will appear here once the thread starts.",
);

type ThreadItem =
  | { kind: "date"; key: string; label: string }
  | { kind: "msg"; message: OwnerThreadMessage }
  | { kind: "draft"; message: OwnerThreadMessage & { draft_id: string; draft: true } };

const threadItems = computed((): ThreadItem[] => {
  const items: ThreadItem[] = [];
  let lastDate = "";
  for (const message of visibleMessages.value) {
    const date = dateSeparatorKey(message.created_at);
    if (date !== lastDate) {
      items.push({
        kind: "date",
        key: `sep-${date}`,
        label: formatDateSeparator(message.created_at),
      });
      lastDate = date;
    }
    if ("draft" in message && message.draft) {
      items.push({
        kind: "draft",
        message: message as ThreadItem extends { kind: "draft"; message: infer M } ? M : never,
      });
    } else {
      items.push({ kind: "msg", message });
    }
  }
  return items;
});

const contextMenu = ref<{
  message: OwnerThreadMessage;
  position: { clientX: number; clientY: number };
} | null>(null);

function onMessageContextMenu(payload: { id: string; clientX: number; clientY: number }) {
  const message = threadMessages.value.find((m) => m.id === payload.id);
  if (!message) return;
  contextMenu.value = {
    message,
    position: { clientX: payload.clientX, clientY: payload.clientY },
  };
}

function closeContextMenu() {
  contextMenu.value = null;
}

const contextMenuActions = computed<MessageAction[]>(() => {
  const open = contextMenu.value;
  if (!open) return [];
  const actions: MessageAction[] = [];
  if (props.canWrite) {
    actions.push({
      key: "reply",
      label: "Reply",
      icon: Reply,
      onSelect: () => startReply(open.message),
    });
  }
  actions.push({
    key: "copy-text",
    label: "Copy text",
    icon: Copy,
    onSelect: () => copyToClipboard(open.message.message, "Message copied"),
  });
  actions.push({
    key: "copy-link",
    label: "Copy link",
    icon: Link2,
    onSelect: () => copyToClipboard(messagePermalink(open.message.id), "Link copied"),
  });
  return actions;
});

function startReply(msg: OwnerThreadMessage) {
  replyingTo.value = msg;
  nextTick(() => editorRef.value?.focus());
}

function cancelReply() {
  replyingTo.value = null;
}

function extractMentions(): string[] {
  return editorRef.value?.getMentionIds() ?? [];
}

async function loadThread() {
  error.value = "";
  try {
    await reload();
    emit("updated", { messages: threadMessages.value });
    await scrollToBottom();
  } catch (err: unknown) {
    const message = getErrorMessage(err, "Failed to load thread");
    if (!/thread not found/i.test(message)) {
      error.value = message;
    }
  }
}

async function submitComment() {
  const message = commentText.value.trim();
  if (!message || submitting.value) return;
  await withSubmit(async () => {
    const result =
      props.ownerType === "alert"
        ? await api.addAlertThreadMessage(Number(props.ownerId), {
            message,
            type: "comment",
            mentions: extractMentions(),
            reply_to_message_id: replyingTo.value?.id,
          })
        : await api.addIncidentThreadMessage(props.ownerId, {
            message,
            type: "comment",
            mentions: extractMentions(),
            reply_to_message_id: replyingTo.value?.id,
          });
    // The server returns the full thread; sync our reducer state to it
    // without going through SSE (which would race the addMessage dedup).
    if (result?.messages) {
      threadMessages.value = result.messages;
    }
    commentText.value = "";
    replyingTo.value = null;
    emit("updated", { messages: threadMessages.value });
    await scrollToBottom();
    editorRef.value?.focus();
  });
}

function replyContextFor(message: OwnerThreadMessage): {
  replyToText: string;
  replyToAuthor: string;
} {
  const qid = message.reply_to_message_id;
  if (!qid) return { replyToText: "", replyToAuthor: "" };
  const found = threadMessages.value.find((m) => m.id === qid);
  if (!found) return { replyToText: "", replyToAuthor: "" };
  return { replyToText: found.message, replyToAuthor: displayName(found) };
}

defineExpose({ reload: loadThread });

onMounted(loadThread);
</script>

<template>
  <section class="flex min-h-0 flex-1 flex-col bg-[var(--bg-primary)]">
    <header
      class="flex shrink-0 items-center justify-between gap-3 border-b border-[var(--border-primary)] px-4 py-2"
    >
      <div class="flex min-w-0 items-center gap-2">
        <h2 class="field-label mb-0">
          <HatGlasses class="inline h-4 w-4 align-text-bottom text-[var(--text-muted)]" />
          {{ title.toUpperCase() }}
        </h2>
        <TypingIndicator v-if="agentTyping" />
      </div>
      <IconBtn :icon="X" label="Close thread" size="md" @click="emit('close')" />
    </header>
    <ErrorBanner :message="error" />
    <LoadingSpinner v-if="loading" centered />
    <div
      v-else
      ref="threadEl"
      class="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-3 pt-2 pb-28 md:pb-2"
    >
      <div
        v-if="visibleMessages.length === 0"
        class="rounded-lg border border-dashed border-[var(--border-primary)] py-10 text-center"
      >
        <HatGlasses class="mx-auto mb-2 h-6 w-6 text-[var(--text-muted)]" />
        <p class="text-sm font-medium text-[var(--text-primary)]">{{ emptyTitle }}</p>
        <p class="mt-1 text-xs text-[var(--text-muted)]">
          {{ emptyDescription }}
        </p>
      </div>
      <template
        v-for="item in threadItems"
        :key="item.kind === 'date' ? item.key : item.message.id"
      >
        <ChatDateSeparator v-if="item.kind === 'date'" :label="item.label" />
        <ChatMessageRow
          v-else
          :id="item.message.id"
          :border-class="sourceColor(item.message.source)"
          :avatar-src="avatarSrc(item.message)"
          :avatar-letter="
            item.message.source === 'agent' ? undefined : displayName(item.message).charAt(0)
          "
          :avatar-bg="sourceAvatarBg(item.message.source)"
          :avatar-title="displayName(item.message)"
          :display-name="displayName(item.message)"
          :created-at="item.message.created_at"
          :content="item.message.message"
          :internal="item.message.internal"
          :edited="item.message.edited"
          :reply-to-text="replyContextFor(item.message).replyToText"
          :reply-to-author="replyContextFor(item.message).replyToAuthor"
          @context-menu="onMessageContextMenu"
        >
          <template #meta-extras>
            <Bot
              v-if="item.message.source === 'agent'"
              class="h-3.5 w-3.5 text-purple-500"
              title="Agent"
            />
          </template>
          <template #actions>
            <button
              v-if="canWrite"
              type="button"
              class="rounded p-1 text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-hover)] hover:text-[var(--text-primary)]"
              title="Reply"
              :aria-label="'Reply to ' + displayName(item.message)"
              @click="startReply(item.message)"
            >
              <Reply class="h-4 w-4" />
            </button>
          </template>
        </ChatMessageRow>
      </template>
      <ChatTypingIndicator
        v-if="agentTyping"
        :avatar-src="getAgentAvatarSrc(agentTypingAgentType ?? undefined)"
        avatar-bg="bg-transparent"
        :avatar-title="agentTypingSource ?? 'Agent'"
        :display-name="agentTypingSource ?? 'Agent'"
        class="mt-1"
      />
    </div>
    <div
      v-if="replyingTo"
      class="flex items-center gap-2 rounded border border-[var(--border-primary)] bg-[var(--bg-muted)]/50 px-3 py-1.5 text-sm"
    >
      <Reply class="h-4 w-4 shrink-0 text-[var(--text-muted)]" />
      <div class="min-w-0 flex-1">
        <span class="font-medium text-[var(--text-secondary)]">{{ displayName(replyingTo) }}</span>
        <p class="line-clamp-1 text-xs text-[var(--text-muted)]">{{ replyingTo.message }}</p>
      </div>
      <button
        type="button"
        class="rounded p-1 text-[var(--text-muted)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-primary)]"
        title="Cancel reply"
        aria-label="Cancel reply"
        @click="cancelReply"
      >
        <X class="h-4 w-4" />
      </button>
    </div>
    <ChatEditorBar v-if="canWrite" class="!mt-0 !pt-0">
      <MarkdownEditor
        ref="editorRef"
        v-model="commentText"
        :disabled="submitting"
        :users="users"
        :agents="agents"
        placeholder="Write a thread message..."
        @submit="submitComment"
      />
    </ChatEditorBar>
    <MessageContextMenu
      :open="contextMenu !== null"
      :position="contextMenu?.position ?? null"
      :actions="contextMenuActions"
      :aria-label="'Message actions'"
      @close="closeContextMenu"
    />
  </section>
</template>
