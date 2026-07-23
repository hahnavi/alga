import { ref, shallowRef, type Ref, type ShallowRef } from "vue";
import { useSSE } from "@/composables/useSSE";
import { MAX_THREAD_MESSAGES } from "@/lib/threadLimits";

/**
 * Owns the SSE→reducer pipeline for a chat thread attached to an entity
 * (alert owner-thread, incident owner-thread, or agent DM).
 *
 * The composable normalizes the three scopes (which differ in SSE event
 * names and payload shape) behind one reducer:
 *   - `messages`: dedup-by-id + sliced at `MAX_THREAD_MESSAGES`
 *   - `drafts`: streaming agent drafts (dedup-by-draft_id)
 *   - agent-sourced messages clear any in-flight drafts (prevents the
 *     "draft and final message both visible" flicker)
 *
 * The caller is responsible for scope-specific payload extraction, draft
 * row construction, and routing the agent-typing state to whatever
 * `useTypingIndicator` instance it owns.
 */

export type ChatThreadScope = "alert" | "incident_inv" | "agent_dm";

export type ChatThreadDraft<TMsg> = TMsg & {
  draft_id: string;
  draft: true;
  /** Text the agent has streamed so far. The composable reads/writes this
   *  field to apply incremental updates without re-building the row. */
  message: string;
};

type ExtractedEdit = { id: string; message: string; edited?: boolean };
type ExtractedDelete = { id: string };
type ExtractedDraft = { draft_id: string; message: string; source?: string };
type ExtractedTyping = { source: string; agent_type?: string };

/**
 * One feed of SSE events that lands in the same `messages` / `drafts`
 * arrays. The composable's main scope contributes the first stream
 * (so a typical caller passes nothing else); callers that need to mix
 * two event sources (e.g. an alert's owner-thread stream + the agent
 * investigation stream) pass `additionalSources`.
 *
 * Each stream owns its own event names, its own match predicate, and
 * its own extractors. The composable registers one SSE handler per
 * event name and walks the streams registered for that name in order
 * until one matches.
 */
export type ChatThreadStream<TMsg extends { id: string }> = {
  /** Returns true if `data` belongs to the current target. */
  matchEvent: (data: unknown, targetId: string) => boolean;
  events: {
    message?: string;
    edited?: string;
    deleted?: string;
    typing?: string;
    typingStop?: string;
    draft?: string;
  };
  extractMessage: (data: unknown) => TMsg | undefined;
  extractEdit: (data: unknown) => ExtractedEdit | null;
  extractDelete: (data: unknown) => ExtractedDelete | null;
  extractDraft?: (data: unknown) => ExtractedDraft | null;
  extractTyping?: (data: unknown) => ExtractedTyping | null;
};

export type ChatThreadOptions<TMsg extends { id: string }> = {
  scope: ChatThreadScope;
  targetId: Ref<string>;
  fetchThread: (id: string) => Promise<TMsg[]>;
  /** Pulls the new message out of the scope-specific SSE event payload. */
  extractMessage: (data: unknown) => TMsg | undefined;
  /** Pulls the { id, message, edited } tuple from the edit event. */
  extractEdit: (data: unknown) => ExtractedEdit | null;
  /** Pulls { id } from the delete event. */
  extractDelete: (data: unknown) => ExtractedDelete | null;
  /** Pulls the draft tuple from the draft event. Defaults assume the
   *  payload is already in `ExtractedDraft` shape. */
  extractDraft?: (data: unknown) => ExtractedDraft | null;
  /** Pulls `{ source, agent_type? }` from the typing event. */
  extractTyping?: (data: unknown) => ExtractedTyping | null;
  /** Build the row stored in `drafts` for a new/updated streaming draft. */
  buildDraft: (
    draftId: string,
    message: string,
    source: string,
    now: string,
  ) => ChatThreadDraft<TMsg>;
  /** True when the message was emitted by an agent; triggers draft clearing. */
  isAgentMessage: (msg: TMsg) => boolean;
  /** Update the message text on a row, returning a new row. The composable
   *  calls this on `editMessage` so message-text updates land on the field
   *  the caller actually reads (`message` for owner-thread, `body` for
   *  AgentDM). */
  setMessage?: (msg: TMsg, text: string) => TMsg;
  /** Invoked for each new message after dedup + slice. */
  onMessageAdded?: (msg: TMsg) => void;
  /** Invoked for each edit. */
  onMessageEdited?: (id: string, message: string, edited: boolean) => void;
  /** Invoked for each delete. */
  onMessageDeleted?: (id: string) => void;
  /** Invoked for typing events that pass scope/match. */
  onTyping?: (source: string, agentType?: string) => void;
  /** Invoked for typing_stop events that pass scope/match. */
  onTypingStop?: () => void;
  /** Invoked after a successful reload, with the freshly fetched items. */
  onLoaded?: (items: TMsg[]) => void;
  /**
   * Optional second/third/… event sources that share the same
   * `messages` / `drafts` state. Each source has its own event
   * names and its own `matchEvent` (the latter typically closes over
   * a different identifier — e.g. the investigation id when the
   * main scope is the alert number).
   */
  additionalSources?: ChatThreadStream<TMsg>[];
};

type ScopeConfig = {
  events: {
    message: string;
    edited: string;
    deleted: string;
    typing: string;
    typingStop: string;
    draft: string;
  };
  matchEvent: (data: unknown, scopeId: string) => boolean;
};

const SCOPES: Record<ChatThreadScope, ScopeConfig> = {
  alert: {
    events: {
      message: "owner_thread_message",
      edited: "owner_thread_message_edited",
      deleted: "owner_thread_message_deleted",
      typing: "owner_thread_typing",
      typingStop: "owner_thread_typing_stop",
      draft: "owner_thread_draft",
    },
    matchEvent: (data, id) => {
      const d = data as { owner_type?: unknown; owner_id?: unknown } | null;
      return !!d && d.owner_type === "alert" && String(d.owner_id) === id;
    },
  },
  incident_inv: {
    events: {
      message: "owner_thread_message",
      edited: "owner_thread_message_edited",
      deleted: "owner_thread_message_deleted",
      typing: "owner_thread_typing",
      typingStop: "owner_thread_typing_stop",
      draft: "owner_thread_draft",
    },
    matchEvent: (data, id) => {
      const d = data as { owner_type?: unknown; owner_id?: unknown } | null;
      return !!d && d.owner_type === "incident_inv" && String(d.owner_id) === id;
    },
  },
  agent_dm: {
    events: {
      message: "agent_dm_message",
      edited: "agent_dm_message_edited",
      deleted: "agent_dm_message_deleted",
      typing: "agent_dm_typing",
      typingStop: "agent_dm_typing_stop",
      draft: "agent_dm_draft",
    },
    matchEvent: (data, id) => {
      const d = data as { agent_token_id?: unknown } | null;
      return !!d && d.agent_token_id === id;
    },
  },
};

type StreamAction = "message" | "edited" | "deleted" | "typing" | "typingStop" | "draft";

function streamEventName(
  stream: ChatThreadStream<{ id: string }>,
  action: StreamAction,
): string | undefined {
  switch (action) {
    case "message":
      return stream.events.message;
    case "edited":
      return stream.events.edited;
    case "deleted":
      return stream.events.deleted;
    case "typing":
      return stream.events.typing;
    case "typingStop":
      return stream.events.typingStop;
    case "draft":
      return stream.events.draft;
  }
}

export function useChatThread<TMsg extends { id: string }>(
  opts: ChatThreadOptions<TMsg>,
): {
  messages: ShallowRef<TMsg[]>;
  drafts: ShallowRef<ChatThreadDraft<TMsg>[]>;
  loading: Ref<boolean>;
  addMessage: (msg: TMsg) => void;
  editMessage: (id: string, message: string, edited?: boolean) => void;
  deleteMessage: (id: string) => void;
  upsertDraft: (input: { draft_id?: string; message?: string; source?: string }) => void;
  clearAll: () => void;
  reload: () => Promise<void>;
} {
  const cfg = SCOPES[opts.scope];
  const messages: ShallowRef<TMsg[]> = shallowRef([]);
  const drafts: ShallowRef<ChatThreadDraft<TMsg>[]> = shallowRef([]);
  const loading = ref(false);
  let loadSeq = 0;

  // The main scope is the first stream; additionalSources contribute
  // any extra feeds (e.g. an alert page that also wants to ingest
  // investigation_update events into the same messages array).
  const streams: ChatThreadStream<TMsg>[] = [
    {
      matchEvent: cfg.matchEvent,
      events: { ...cfg.events },
      extractMessage: opts.extractMessage,
      extractEdit: opts.extractEdit,
      extractDelete: opts.extractDelete,
      extractDraft: opts.extractDraft,
      extractTyping: opts.extractTyping,
    },
    ...(opts.additionalSources ?? []),
  ];

  async function reload() {
    const seq = ++loadSeq;
    const id = opts.targetId.value;
    if (!id) {
      messages.value = [];
      drafts.value = [];
      return;
    }
    loading.value = true;
    try {
      const items = await opts.fetchThread(id);
      if (seq !== loadSeq) return;
      // Merge strategy: keep any current messages that the server fetch
      // doesn't yet know about (SSE event arrived before the server-side
      // write was visible). This avoids the brief "message disappeared"
      // flicker when a freshly posted message collides with a reload.
      const freshIds = new Set(items.map((m) => m.id));
      const preserved = messages.value.filter((m) => !freshIds.has(m.id));
      messages.value = [...items, ...preserved].slice(-MAX_THREAD_MESSAGES);
      drafts.value = [];
      opts.onLoaded?.(items);
    } finally {
      if (seq === loadSeq) loading.value = false;
    }
  }

  function addMessage(msg: TMsg) {
    if (opts.isAgentMessage(msg)) drafts.value = [];
    if (messages.value.some((m) => m.id === msg.id)) return;
    messages.value = [...messages.value, msg].slice(-MAX_THREAD_MESSAGES);
    opts.onMessageAdded?.(msg);
  }

  function editMessage(id: string, message: string, edited = true) {
    const idx = messages.value.findIndex((m) => m.id === id);
    if (idx < 0) return;
    const existing = messages.value[idx];
    if (opts.isAgentMessage(existing)) drafts.value = [];
    const updated = opts.setMessage
      ? { ...opts.setMessage(existing, message), edited }
      : { ...existing, message, edited };
    messages.value = [
      ...messages.value.slice(0, idx),
      updated as TMsg,
      ...messages.value.slice(idx + 1),
    ];
    opts.onMessageEdited?.(id, message, edited);
  }

  function deleteMessage(id: string) {
    const before = messages.value.length;
    messages.value = messages.value.filter((m) => m.id !== id);
    if (messages.value.length !== before) opts.onMessageDeleted?.(id);
  }

  function upsertDraft(input: { draft_id?: string; message?: string; source?: string }) {
    const draftId = input.draft_id?.trim();
    if (!draftId) return;
    const now = new Date().toISOString();
    const newRow = opts.buildDraft(draftId, input.message ?? "", input.source ?? "agent", now);
    const idx = drafts.value.findIndex((m) => m.draft_id === draftId);
    if (idx >= 0) {
      // Merge: the caller's buildDraft owns the field shape (e.g. AgentDM
      // drafts use `body`, owner-thread drafts use `message`), so we spread
      // its output over the existing row rather than touching one field.
      drafts.value = [
        ...drafts.value.slice(0, idx),
        { ...drafts.value[idx], ...newRow, updated_at: now },
        ...drafts.value.slice(idx + 1),
      ];
    } else {
      drafts.value = [...drafts.value, newRow].slice(-MAX_THREAD_MESSAGES);
    }
  }

  function clearAll() {
    messages.value = [];
    drafts.value = [];
  }

  // Build the SSE handler map. One handler is registered per unique
  // event name across all streams. A handler walks the streams that
  // registered that event and applies the first match. Streams with
  // a `false` `matchEvent` are ignored.
  type StreamHandler = (data: unknown) => void;
  const handlers = new Map<string, StreamHandler[]>();
  function register(event: string, handler: StreamHandler) {
    const list = handlers.get(event);
    if (list) list.push(handler);
    else handlers.set(event, [handler]);
  }

  for (const stream of streams) {
    const messageEvent = streamEventName(stream, "message");
    if (messageEvent) {
      register(messageEvent, (data) => {
        if (!stream.matchEvent(data, opts.targetId.value)) return;
        const m = stream.extractMessage(data);
        if (m) addMessage(m);
      });
    }
    const editedEvent = streamEventName(stream, "edited");
    if (editedEvent) {
      register(editedEvent, (data) => {
        if (!stream.matchEvent(data, opts.targetId.value)) return;
        const e = stream.extractEdit(data);
        if (e) editMessage(e.id, e.message, e.edited);
      });
    }
    const deletedEvent = streamEventName(stream, "deleted");
    if (deletedEvent) {
      register(deletedEvent, (data) => {
        if (!stream.matchEvent(data, opts.targetId.value)) return;
        const d = stream.extractDelete(data);
        if (d) deleteMessage(d.id);
      });
    }
    const typingEvent = streamEventName(stream, "typing");
    if (typingEvent && stream.extractTyping) {
      register(typingEvent, (data) => {
        if (!stream.matchEvent(data, opts.targetId.value)) return;
        const t = stream.extractTyping!(data);
        if (t && opts.onTyping) opts.onTyping(t.source, t.agent_type);
      });
    }
    const typingStopEvent = streamEventName(stream, "typingStop");
    if (typingStopEvent) {
      register(typingStopEvent, (data) => {
        if (!stream.matchEvent(data, opts.targetId.value)) return;
        if (opts.onTypingStop) opts.onTypingStop();
      });
    }
    const draftEvent = streamEventName(stream, "draft");
    if (draftEvent && stream.extractDraft) {
      register(draftEvent, (data) => {
        if (!stream.matchEvent(data, opts.targetId.value)) return;
        const d = stream.extractDraft!(data);
        if (d) upsertDraft(d);
      });
    }
  }

  const sseHandlers: Record<string, (data: unknown) => void> = {};
  for (const [event, list] of handlers) {
    sseHandlers[event] = (data: unknown) => {
      for (const h of list) h(data);
    };
  }

  useSSE("/api/v1/events", sseHandlers, {
    onReconnect: () => {
      void reload();
    },
  });

  return {
    messages,
    drafts,
    loading,
    addMessage,
    editMessage,
    deleteMessage,
    upsertDraft,
    clearAll,
    reload,
  };
}
