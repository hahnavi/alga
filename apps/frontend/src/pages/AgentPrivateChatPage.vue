<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, toRef, watch } from "vue";
import { useRoute } from "vue-router";
import { Bot, ArrowLeft } from "@lucide/vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import { api, type AgentDMMessageRow, type AgentTokenRow, type AgentType } from "@/lib/api";
import MarkdownEditor from "@/components/ui/MarkdownEditor.vue";
import ChatDateSeparator from "@/components/ui/ChatDateSeparator.vue";
import ChatMessageRow from "@/components/ui/ChatMessageRow.vue";
import ChatTypingIndicator from "@/components/ui/ChatTypingIndicator.vue";
import ChatEditorBar from "@/components/ui/ChatEditorBar.vue";
import { useToast } from "@/lib/toast";
import { resolveDisplayName } from "@/lib/userDisplay";
import { clearPageHeader, createSearchActionButton, setPageHeader } from "@/lib/pageHeader";
import { formatDateSeparator, dateSeparatorKey } from "@/lib/time";
import { useChatSearch } from "@/composables/useChatSearch";
import { useChatThread } from "@/composables/useChatThread";
import { useSSE } from "@/composables/useSSE";
import { useTypingIndicator } from "@/composables/useTypingIndicator";
import { useUsers } from "@/composables/useUsers";
import { getAgentAvatarSrc } from "@/lib/agentAvatar";
import { MAX_THREAD_MESSAGES } from "@/lib/threadLimits";

defineOptions({ name: "AgentPrivateChatPage" });

const route = useRoute();
const { push } = useToast();

const agentTokenId = computed(() => route.params.agent_token_id as string);

const agentName = ref("");
const agentBrand = ref<"hermes" | "openclaw" | "other">("hermes");
const agentRevoked = ref(false);
const { users, loadUsers } = useUsers();
const loading = ref(false);
const loadingOlder = ref(false);
let chatLoadSeq = 0;
const hasMore = ref(false);
const draft = ref("");
const sending = ref(false);
let typingPostTimer: number | null = null;

function markAgentRevoked() {
  agentRevoked.value = true;
  agentName.value = "Deleted agent";
  agentBrand.value = "hermes";
  clearAll();
  hasMore.value = false;
  clearAgentTyping();
  setPageHeader("Deleted agent");
}

const {
  isTyping: agentTyping,
  setTyping: setAgentTyping,
  clearTyping: clearAgentTyping,
} = useTypingIndicator({ timeoutMs: 4000 });

const {
  messages,
  drafts: agentDrafts,
  clearAll,
} = useChatThread<AgentDMMessageRow>({
  scope: "agent_dm",
  targetId: toRef(agentTokenId),
  fetchThread: async (id) => {
    const res = await api.listAgentDMMessages(id, { limit: 50 });
    hasMore.value = res.has_more;
    return Array.isArray(res.items) ? res.items : [];
  },
  extractMessage: (data) => (data as { message?: AgentDMMessageRow }).message,
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
    const d = data as { draft_id?: string; message?: string };
    if (!d.draft_id) return null;
    return { draft_id: d.draft_id, message: d.message ?? "", source: "agent" };
  },
  extractTyping: () => ({ source: "agent" }),
  buildDraft: (draftId, message, _source, now) => ({
    id: `draft-${draftId}`,
    draft_id: draftId,
    draft: true,
    agent_token_id: agentTokenId.value,
    chat_id: "alga_dm",
    role: "agent" as const,
    body: message,
    message,
    username: agentName.value || undefined,
    created_at: now,
    updated_at: now,
  }),
  isAgentMessage: (msg) => msg.role === "agent",
  setMessage: (msg, text) => ({ ...msg, body: text }),
  onMessageAdded: () => {
    void nextTick().then(scrollToBottom);
  },
  onTyping: () => {
    setAgentTyping("agent");
    void nextTick().then(scrollToBottom);
  },
  onTypingStop: () => clearAgentTyping(),
});

const {
  query: searchQuery,
  hasQuery: searchHasQuery,
  searchHighlight,
  openSearch: searchOpen,
} = useChatSearch(
  messages,
  (m: AgentDMMessageRow) => m.id,
  (m: AgentDMMessageRow) => m.body,
);

const threadEl = ref<HTMLDivElement | null>(null);

type DmThreadItem =
  | { kind: "date"; key: string; label: string }
  | { kind: "msg"; message: AgentDMMessageRow }
  | { kind: "draft"; message: AgentDMMessageRow & { draft_id: string; draft: true } };

const dmThreadItems = computed((): DmThreadItem[] => {
  const items: DmThreadItem[] = [];
  let lastDate = "";
  const addItem = (m: AgentDMMessageRow, kind: "msg" | "draft") => {
    const dateStr = dateSeparatorKey(m.created_at);
    if (dateStr !== lastDate) {
      items.push({
        kind: "date",
        key: `sep-${dateStr}`,
        label: formatDateSeparator(m.created_at),
      });
      lastDate = dateStr;
    }
    if (kind === "draft") {
      items.push({ kind, message: m as AgentDMMessageRow & { draft_id: string; draft: true } });
    } else {
      items.push({ kind, message: m });
    }
  };
  for (const m of messages.value) {
    addItem(m, "msg");
  }
  for (const m of agentDrafts.value) {
    addItem(m, "draft");
  }
  return items;
});

// `agent_revoked` is not part of the chat-thread scope; it stays here as
// a top-level SSE concern handled by the page directly.
useSSE("/api/v1/events", {
  agent_revoked: (data: unknown) => {
    const d = data as { agent_token_id?: string };
    if (d.agent_token_id !== agentTokenId.value) return;
    markAgentRevoked();
  },
});

async function bootstrapChat() {
  const seq = ++chatLoadSeq;
  loading.value = true;
  messages.value = [];
  agentDrafts.value = [];
  try {
    await resolveAgentTitle();
    if (seq !== chatLoadSeq) return;
    syncAgentPageHeader();
    if (agentRevoked.value) {
      return;
    }

    const id = agentTokenId.value?.trim();
    if (!id) {
      hasMore.value = false;
      return;
    }

    const res = await api.listAgentDMMessages(id, { limit: 50 });
    if (seq !== chatLoadSeq) return;
    messages.value = Array.isArray(res.items) ? res.items : [];
    hasMore.value = res.has_more;
  } catch (e) {
    if (seq === chatLoadSeq) {
      push(getErrorMessage(e, "Failed to load chat"), "error");
    }
  } finally {
    if (seq === chatLoadSeq) {
      loading.value = false;
      await nextTick();
      scrollToBottom();
    }
  }
}

async function loadOlder() {
  if (!hasMore.value || loadingOlder.value || messages.value.length === 0) return;
  const first = messages.value[0];
  if (!first?.id) return;
  loadingOlder.value = true;
  const prevHeight = threadEl.value?.scrollHeight ?? 0;
  try {
    const res = await api.listAgentDMMessages(agentTokenId.value, {
      before: first.id,
      limit: 50,
    });
    const older = Array.isArray(res.items) ? res.items : [];
    messages.value = [...older, ...messages.value];
    hasMore.value = res.has_more;
    await nextTick();
    if (threadEl.value) {
      const h = threadEl.value.scrollHeight;
      threadEl.value.scrollTop = h - prevHeight;
    }
  } catch (e) {
    push(getErrorMessage(e, "Failed to load older messages"), "error");
  } finally {
    loadingOlder.value = false;
  }
}

function scrollToBottom() {
  const el = threadEl.value;
  if (!el) return;
  el.scrollTop = el.scrollHeight;
}

async function sendMessage() {
  const text = draft.value.trim();
  if (!text || sending.value) return;
  sending.value = true;
  try {
    const msg = await api.postAgentDMMessage(agentTokenId.value, text);
    draft.value = "";
    if (!messages.value.some((x) => x.id === msg.id)) {
      messages.value = [...messages.value, msg].slice(-MAX_THREAD_MESSAGES);
    }
    await nextTick();
    scrollToBottom();
  } catch (e) {
    push(getErrorMessage(e, "Failed to send"), "error");
  } finally {
    sending.value = false;
  }
}

function scheduleTypingNotify() {
  if (typingPostTimer) clearTimeout(typingPostTimer);
  typingPostTimer = setTimeout(() => {
    typingPostTimer = null;
    void api.postAgentDMTyping(agentTokenId.value).catch(() => {
      /* intentional: typing indicator is best-effort */
    });
  }, 600);
}

function onThreadScroll() {
  const el = threadEl.value;
  if (!el || loadingOlder.value || !hasMore.value) return;
  if (el.scrollTop < 48) {
    void loadOlder();
  }
}

function normalizeHeaderAgentBrand(t?: AgentType | string): "hermes" | "openclaw" | "other" {
  const s = String(t ?? "hermes").toLowerCase();
  if (s === "openclaw") return "openclaw";
  if (s === "other") return "other";
  return "hermes";
}

function syncAgentPageHeader() {
  if (agentRevoked.value) {
    setPageHeader("Deleted agent");
    return;
  }
  setPageHeader(agentName.value || "Agent chat", undefined, {
    headerAgentBrand: agentBrand.value,
    actions: [createSearchActionButton(() => searchOpen())],
  });
}

async function resolveAgentTitle() {
  try {
    const tokens = await api.getAgentTokens();
    const row = tokens.find((x: AgentTokenRow) => x.id === agentTokenId.value);
    if (!row) {
      agentRevoked.value = true;
      agentName.value = "Deleted agent";
      agentBrand.value = "hermes";
      return;
    }
    agentRevoked.value = false;
    agentName.value = row.name;
    agentBrand.value = normalizeHeaderAgentBrand(row.agent_type);
  } catch {
    agentRevoked.value = true;
    agentName.value = "Deleted agent";
    agentBrand.value = "hermes";
  }
}

function dmBorderClass(role: AgentDMMessageRow["role"]): string {
  return role === "agent" ? "border-l-[var(--accent)]" : "border-l-[var(--bg-online)]";
}

function dmAvatarClass(role: AgentDMMessageRow["role"]): string {
  return role === "agent" ? "bg-transparent" : "bg-[var(--bg-online)]";
}

function displayNameForMessage(m: AgentDMMessageRow): string {
  return resolveDisplayName({
    userId: m.user_id,
    username: m.username,
    users: users.value,
    role: m.role,
    agentName: agentName.value || "Agent",
    fallback: "You",
  });
}

function userAvatarLetter(m: AgentDMMessageRow): string {
  if (m.username?.trim()) return m.username.trim().charAt(0).toUpperCase();
  return "U";
}

watch(
  agentTokenId,
  (id) => {
    // Param becomes undefined while navigating away; don't re-bootstrap then.
    if (!id) return;
    void bootstrapChat();
  },
  { immediate: true },
);

watch(draft, () => {
  if (draft.value.trim()) scheduleTypingNotify();
});

onMounted(() => {
  void loadUsers();
});

onBeforeUnmount(() => {
  chatLoadSeq++;
  clearPageHeader();
  clearAgentTyping();
  if (typingPostTimer) {
    clearTimeout(typingPostTimer);
    typingPostTimer = null;
  }
  agentDrafts.value = [];
});
</script>

<template>
  <div class="flex h-full min-h-0 min-w-0 flex-1 flex-col">
    <div v-if="agentRevoked" class="flex min-h-0 flex-1 items-center justify-center px-4 py-10">
      <Card class="max-w-md">
        <div class="flex flex-col items-center gap-4 text-center">
          <div
            class="flex h-14 w-14 items-center justify-center rounded-full bg-[var(--bg-secondary)] opacity-60 grayscale"
          >
            <Bot class="h-7 w-7 text-[var(--text-muted)]" />
          </div>
          <div class="space-y-1">
            <h2 class="text-base font-semibold text-[var(--text-primary)]">
              This agent was deleted
            </h2>
            <p class="text-sm text-[var(--text-muted)]">
              The integration agent
              <code class="rounded bg-[var(--bg-code)] px-1 text-xs">{{ agentTokenId }}</code>
              has been removed and its messages are no longer available here. You can no longer send
              messages to it.
            </p>
          </div>
          <router-link to="/agents" class="inline-flex">
            <Button variant="outline" size="sm">
              <ArrowLeft class="h-4 w-4" />
              Back to agents
            </Button>
          </router-link>
        </div>
      </Card>
    </div>
    <template v-else>
      <div
        ref="threadEl"
        class="min-h-0 flex-1 space-y-2 overflow-y-auto px-2 pb-28 md:pb-0"
        @scroll="onThreadScroll"
      >
        <LoadingSpinner v-if="loadingOlder" label="" />

        <LoadingSpinner v-if="loading && messages.length === 0" centered />

        <template v-else>
          <template
            v-for="item in dmThreadItems"
            :key="item.kind === 'date' ? item.key : item.message.id"
          >
            <ChatDateSeparator v-if="item.kind === 'date'" :label="item.label" />
            <ChatMessageRow
              v-else
              :id="item.message.id"
              :border-class="dmBorderClass(item.message.role)"
              :highlight-class="searchHighlight(item.message.id)"
              :avatar-src="
                item.message.role === 'agent'
                  ? getAgentAvatarSrc(agentBrand === 'openclaw' ? 'openclaw' : undefined)
                  : undefined
              "
              :avatar-letter="
                item.message.role !== 'agent' ? userAvatarLetter(item.message) : undefined
              "
              :avatar-bg="dmAvatarClass(item.message.role)"
              :avatar-title="item.message.role === 'agent' ? agentName || 'Agent' : 'User'"
              :display-name="displayNameForMessage(item.message)"
              :created-at="item.message.created_at"
              :edited="item.message.edited"
              :content="item.message.body"
              :search-query="searchHasQuery ? searchQuery : undefined"
            />
          </template>

          <ChatTypingIndicator
            v-if="agentTyping"
            :avatar-src="getAgentAvatarSrc(agentBrand === 'openclaw' ? 'openclaw' : undefined)"
            avatar-bg="bg-transparent"
            :avatar-title="agentName || 'Agent'"
            :display-name="agentName || 'Agent'"
            :dimmed="searchHasQuery"
          />

          <div
            v-if="messages.length === 0 && agentDrafts.length === 0 && !agentTyping"
            class="py-8 text-center text-[var(--text-muted)]"
          >
            No messages yet
          </div>
        </template>
      </div>

      <ChatEditorBar>
        <MarkdownEditor
          v-model="draft"
          :disabled="sending"
          :users="users"
          :enable-internal-note="false"
          placeholder="Message the agent (Markdown). @ to mention teammates. Ctrl+Enter or toolbar to send — text is forwarded verbatim."
          @submit="sendMessage"
        />
      </ChatEditorBar>
    </template>
  </div>
</template>
