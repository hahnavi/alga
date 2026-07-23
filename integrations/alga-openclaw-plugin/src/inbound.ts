import type { OpenClawConfig } from "openclaw/plugin-sdk/config-runtime";
import type { GetReplyOptions } from "openclaw/plugin-sdk/reply-runtime";
import {
  deliverFormattedTextWithAttachments,
  type OutboundReplyPayload,
} from "openclaw/plugin-sdk/reply-payload";
import { dispatchChannelInboundReply } from "openclaw/plugin-sdk/channel-inbound";
import { dispatchInboundReplyWithBase } from "openclaw/plugin-sdk/inbound-reply-dispatch";
import { getAlgaRuntime } from "./runtime.js";
import { normalizeInvestigationChatId } from "./chat-ids.js";
import { normalizeAlgaMessagingTarget, outboundChatIdForAlga } from "./target.js";
import {
  sendAlgaText,
  sendAlgaTyping,
  sendAlgaTypingStop,
  markChatStopped,
  clearChatStopped,
} from "./outbound.js";
import { agentPostMessage } from "./agent-rest.js";
import type { CoreConfig, ResolvedAlgaAccount, InvestigationSignalEventType, InvestigationSignalEvent } from "./types.js";

const DEDUP_TTL_MS = 300_000;
const DEDUP_MAX = 1000;
const TYPING_KEEPALIVE_MS = 3_000;
const TYPING_MAX_DURATION_MS = 120_000;
const SILENT_DELIVER = new Set(["NO_REPLY", "HEARTBEAT_OK"]);

const TOOL_EMOJI: Record<string, string> = {
  exec: "💻",
  shell: "💻",
  bash: "💻",
  run: "💻",
  apply_patch: "🔧",
  edit_file: "✏️",
  write_file: "✏️",
  create_file: "✏️",
  read_file: "📄",
  view_file: "📄",
  cat: "📄",
  web_search: "🔍",
  search: "🔍",
  grep: "🔍",
  ripgrep: "🔍",
  list: "📋",
  ls: "📋",
  glob: "📋",
  fetch: "🌐",
  curl: "🌐",
  http: "🌐",
};

const ARG_PRIORITY = [
  "command", "cmd", "query", "q", "path", "file", "filepath",
  "url", "pattern", "search", "expr", "expression", "code",
];

const seen = new Map<string, number>();

function pruneSeen(now: number) {
  for (const [k, t] of seen) {
    if (now - t > DEDUP_TTL_MS) {
      seen.delete(k);
    }
  }
}

function parseAllowedUsers(): Set<string> | null {
  const raw = process.env.ALGA_ALLOWED_USERS?.trim();
  if (!raw) {
    return null;
  }
  return new Set(raw.split(",").map((s: string) => s.trim()).filter(Boolean));
}

function truncate(s: string, max: number): string {
  return s.length > max ? s.slice(0, max) + "…" : s;
}

function shortenArgs(args?: Record<string, unknown>): string {
  if (!args) return "";
  for (const key of ARG_PRIORITY) {
    const val = args[key];
    if (typeof val === "string" && val.trim()) {
      return truncate(val.trim(), 48);
    }
  }
  for (const val of Object.values(args)) {
    if (typeof val === "string" && val.trim()) {
      return truncate(val.trim(), 48);
    }
  }
  return "";
}

function formatToolMessage(name: string, args?: Record<string, unknown>): string {
  const emoji = TOOL_EMOJI[name] ?? (name.startsWith("alga_") ? "🧩" : "🔧");
  const summary = shortenArgs(args);
  return summary ? `${emoji} ${name}: ${summary}` : `${emoji} ${name}`;
}

export async function handleAlgaInbound(params: {
  channelId: string;
  channelLabel: string;
  account: ResolvedAlgaAccount;
  config: CoreConfig;
  rawJson: string;
}): Promise<void> {
  let data: Record<string, unknown>;
  try {
    data = JSON.parse(params.rawJson) as Record<string, unknown>;
  } catch {
    return;
  }
  const text = typeof data.text === "string" ? data.text : "";
  const chatIdRaw = typeof data.chat_id === "string" ? data.chat_id : "";
  if (!text.trim() || !chatIdRaw.trim()) {
    return;
  }
  if (text.codePointAt(0) === 0x1f512) {
    return;
  }

  // Non-mention room messages arrive with trigger:"observe". OpenClaw maps an
  // `InboundEventKind:"room_event"` inbound to `sourceReplyDeliveryMode:
  // "message_tool_only"`, which suppresses automatic source delivery. Tool-call
  // narration is therefore emitted from the agent's own message tool, and there
  // is no streaming final text to forward. Observe turns skip tool-call
  // forwarding entirely and let the message tool be the single delivery.
  const isObserve = typeof data.trigger === "string" && data.trigger === "observe";

  if (text.trim() === "/stop") {
    markChatStopped(chatIdRaw);
  } else {
    clearChatStopped(chatIdRaw);
  }

  const allowed = parseAllowedUsers();
  const senderId = typeof data.sender_id === "string" ? data.sender_id : "";
  if (allowed && !allowed.has(senderId)) {
    return;
  }

  const msgId =
    (typeof data.message_id === "string" && data.message_id) ||
    (typeof data.id === "string" && data.id) ||
    `${chatIdRaw}:${text.slice(0, 64)}:${senderId}`;
  const now = Date.now();
  pruneSeen(now);
  if (seen.has(msgId)) {
    return;
  }
  seen.set(msgId, now);

  const trimmedChat = chatIdRaw.trim();
  const sessionChatId = /^alga_dm$/i.test(trimmedChat)
    ? "alga_dm"
    : normalizeInvestigationChatId(chatIdRaw);
  const target = normalizeAlgaMessagingTarget(sessionChatId);

  const runtime = getAlgaRuntime();
  const route = runtime.channel.routing.resolveAgentRoute({
    cfg: params.config as OpenClawConfig,
    channel: params.channelId,
    accountId: params.account.accountId,
    peer: { kind: "channel", id: target },
  });
  const storePath = runtime.channel.session.resolveStorePath(params.config.session?.store, {
    agentId: route.agentId,
  });
  const previousTimestamp = runtime.channel.session.readSessionUpdatedAt({
    storePath,
    sessionKey: route.sessionKey,
  });
  const senderName =
    (typeof data.sender_name === "string" && data.sender_name.trim()) || "User";

  const dmThread = sessionChatId === "alga_dm";
  const threadTitle = dmThread
    ? "Agent DM"
    : `Investigation ${sessionChatId.replace(/^investigation_/, "")}`;

  const body = runtime.channel.reply.formatAgentEnvelope({
    channel: params.channelLabel,
    from: senderName || senderId || "user",
    timestamp: now,
    previousTimestamp,
    envelope: runtime.channel.reply.resolveEnvelopeFormatOptions(params.config as OpenClawConfig),
    body: text,
  });

  const ctxPayload = runtime.channel.reply.finalizeInboundContext({
    Body: body,
    BodyForAgent: text,
    RawBody: text,
    CommandBody: text,
    From: senderId || "user",
    To: target,
    SessionKey: route.sessionKey,
    AccountId: route.accountId ?? params.account.accountId,
    ChatType: dmThread ? "direct" : "group",
    ConversationLabel: threadTitle,
    GroupSubject: threadTitle,
    GroupChannel: sessionChatId,
    NativeChannelId: chatIdRaw,
    MessageThreadId: sessionChatId,
    ThreadLabel: undefined,
    ThreadParentId: undefined,
    SenderName: senderName,
    SenderId: senderId,
    Provider: params.channelId,
    Surface: params.channelId,
    MessageSid: msgId,
    MessageSidFull: msgId,
    ReplyToId: undefined,
    Timestamp: now,
    OriginatingChannel: params.channelId,
    OriginatingTo: target,
    CommandAuthorized: true,
    ...(isObserve && !dmThread ? { InboundEventKind: "room_event" as const } : {}),
  });

  const replyTo = target;
  const skipAmbientReply = isObserve;
  const httpBase = params.account.httpBase;
  const token = params.account.token;
  const algaChatId = outboundChatIdForAlga(replyTo);

  // Per-segment assistant text delivery.
  //
  // OpenClaw's delivery.deliver only receives the LAST assistant message's text
  // for a turn (buildEmbeddedRunPayloads collapses the whole turn to the
  // canonical final answer). The intermediate "explain before executing"
  // reasoning segments are dropped. To reproduce Hermes-style per-step
  // narration, we capture each assistant text segment via onPartialReply and
  // post it as its OWN fresh message the moment the segment completes — either
  // when the next assistant segment starts (payload.replace === true) or when a
  // tool call begins (so the explanation lands before the tool that follows it).
  //
  // onPartialReply semantics (from embedded-agent-subscribe.handlers.messages):
  //   payload.text    = accumulated cleaned text for the CURRENT assistant msg
  //   payload.delta   = new slice since the last emit (empty when replace)
  //   payload.replace = true when a NEW assistant message started (previous
  //                     segment is complete); the previous segment's final text
  //                     is the value we tracked before this replace.
  //
  // Every message here is a fresh one-shot POST. There is no preview/edit
  // mechanism, so each message keeps its true creation timestamp and the
  // thread reads top-to-bottom: reason → tool → reason → tool → final summary.
  let currentSegmentText = "";
  let lastPostedText = "";

  const flushSegment = (): void => {
    const text = currentSegmentText.trim();
    currentSegmentText = "";
    if (!text || text === lastPostedText) return;
    lastPostedText = text;
    void agentPostMessage(httpBase, token, algaChatId, {
      kind: "text",
      text,
    }).catch(() => {
      // Best-effort: a failed reasoning post must never abort the run.
    });
  };

  const replyOptions: GetReplyOptions | undefined = skipAmbientReply
    ? undefined
    : {
      sourceReplyDeliveryMode: "automatic",
      suppressDefaultToolProgressMessages: true,
      allowProgressCallbacksWhenSourceDeliverySuppressed: true,
      // OpenClaw appends `⚠️ <tool> failed` payloads for non-acknowledged tool
      // errors (payloads.ts buildEmbeddedRunPayloads). In a per-message thread
      // model these surface as spurious trailing messages the agent never
      // authored, and — because they post with an empty user_id — they also
      // inflate the participant count. The agent already sees tool errors in
      // tool results and narrates them; suppress the synthetic warning.
      suppressToolErrorWarnings: true,
      onPartialReply: (payload) => {
        const text = (payload.text ?? "").trim();
        if (!text || SILENT_DELIVER.has(text)) return;
        // A new assistant message just started: the previous segment is done.
        // Flush it before we begin tracking the new one.
        if (payload.replace) flushSegment();
        currentSegmentText = text;
      },
      onToolStart: (payload) => {
        if (payload.phase && payload.phase !== "start") return;
        // The reasoning that precedes this tool call is complete; land it now
        // so it appears before the tool_call message in the thread.
        flushSegment();
        const name = payload.name?.trim();
        if (!name) return;
        void agentPostMessage(httpBase, token, algaChatId, {
          kind: "tool_call",
          text: formatToolMessage(name, payload.args),
        }).catch(() => {
          // Best-effort: a failed tool_call narration must never abort the run.
        });
      },
    };

  const typingKeepalive = skipAmbientReply
    ? null
    : startTypingKeepalive({
      cfg: params.config,
      accountId: params.account.accountId,
      to: replyTo,
    });

  let delivered = false;

  try {
    await dispatchChannelInboundReply({
      cfg: params.config as OpenClawConfig,
      channel: params.channelId,
      accountId: params.account.accountId,
      agentId: route.agentId,
      routeSessionKey: route.sessionKey,
      storePath,
      ctxPayload,
      recordInboundSession: runtime.channel.session.recordInboundSession,
      dispatchReplyWithBufferedBlockDispatcher: runtime.channel.reply.dispatchReplyWithBufferedBlockDispatcher,
      ...(replyOptions ? { replyOptions } : {}),
      delivery: {
        deliver: async (payload: OutboundReplyPayload) => {
          typingKeepalive?.stop();
          delivered = true;
          const finalText = (payload as { text?: string }).text?.trim() ?? "";
          if (!finalText || SILENT_DELIVER.has(finalText)) {
            // Flush any trailing reasoning that wasn't followed by a tool call.
            flushSegment();
            return;
          }
          // delivery.deliver carries only the LAST assistant message's text,
          // which is exactly what the final onPartialReply captured for the
          // current segment. If we already posted it via flushSegment (e.g. it
          // was followed by a silent tail), skip to avoid duplication;
          // otherwise post it now as the turn's final summary. Either way it
          // lands last, after all tools and intermediate reasoning.
          if (finalText === lastPostedText) {
            currentSegmentText = "";
            return;
          }
          currentSegmentText = "";
          await deliverFormattedTextWithAttachments({
            payload,
            send: async ({ text }: { text: string }) => {
              await sendAlgaText({
                cfg: params.config,
                accountId: params.account.accountId,
                to: replyTo,
                text,
              });
            },
          });
        },
      },
    });
  } finally {
    typingKeepalive?.stop();
    // If the turn ended without a final delivery (e.g. NO_REPLY), still flush
    // any accumulated reasoning so it is not silently lost.
    if (!delivered) flushSegment();
  }
}

function startTypingKeepalive(params: {
  cfg: CoreConfig;
  accountId: string;
  to: string;
}): { stop: () => void } {
  void sendAlgaTyping(params);

  const timer = setInterval(() => {
    void sendAlgaTyping(params);
  }, TYPING_KEEPALIVE_MS);

  const ttl = setTimeout(() => {
    clearInterval(timer);
  }, TYPING_MAX_DURATION_MS);

  let stopped = false;
  return {
    stop() {
      if (stopped) return;
      stopped = true;
      clearInterval(timer);
      clearTimeout(ttl);
      sendAlgaTypingStop(params);
    },
  };
}

export function handleAlgaInvestigationSignal(
  signalType: InvestigationSignalEventType,
  data: InvestigationSignalEvent,
  cfg: CoreConfig,
  account: ResolvedAlgaAccount,
): void {
  const runtime = getAlgaRuntime();
  const investigationId = data.alert_investigation_id;
  const actor = data.actor ?? "system";

  let actionWord: string;
  if (signalType === "investigation_resume") {
    actionWord = "resumed";
  } else {
    const status = (data.status ?? "").toLowerCase();
    actionWord = status === "cancelled" || status === "canceled"
      ? "cancelled"
      : status === "investigating"
        ? "re-activated"
        : "paused";
    if (status === "investigating") {
      return;
    }
  }

  const chatId = `alert_${investigationId}`;
  const target = normalizeAlgaMessagingTarget(normalizeInvestigationChatId(chatId));
  const route = runtime.channel.routing.resolveAgentRoute({
    cfg: cfg as OpenClawConfig,
    channel: "alga",
    accountId: account.accountId,
    peer: { kind: "channel", id: target },
  });

  const reasonText = data.reason ? `: ${data.reason}` : "";
  const body = runtime.channel.reply.formatAgentEnvelope({
    channel: "Alga",
    from: "system",
    timestamp: Date.now(),
    previousTimestamp: undefined,
    envelope: runtime.channel.reply.resolveEnvelopeFormatOptions(cfg as OpenClawConfig),
    body: `Investigation ${actionWord} by ${actor}${reasonText}`,
  });

  const ctxPayload = runtime.channel.reply.finalizeInboundContext({
    Body: body,
    BodyForAgent: `[SYSTEM] Investigation ${investigationId} ${actionWord} by ${actor}${reasonText}`,
    RawBody: "",
    CommandBody: "",
    From: actor,
    To: target,
    SessionKey: route.sessionKey,
    AccountId: route.accountId ?? account.accountId,
    ChatType: "group",
    ConversationLabel: `Investigation ${investigationId}`,
    GroupSubject: `Investigation ${investigationId}`,
    GroupChannel: normalizeInvestigationChatId(chatId),
    NativeChannelId: chatId,
    MessageThreadId: normalizeInvestigationChatId(chatId),
    ThreadLabel: undefined,
    ThreadParentId: undefined,
    SenderName: actor,
    SenderId: "system",
    Provider: "alga",
    Surface: "alga",
    MessageSid: `signal:${signalType}:${investigationId}:${Date.now()}`,
    MessageSidFull: `signal:${signalType}:${investigationId}:${Date.now()}`,
    ReplyToId: undefined,
    Timestamp: Date.now(),
    OriginatingChannel: "alga",
    OriginatingTo: target,
    CommandAuthorized: true,
  });

  void dispatchInboundReplyWithBase({
    cfg: cfg as OpenClawConfig,
    channel: "alga",
    accountId: account.accountId,
    route,
    storePath: runtime.channel.session.resolveStorePath((cfg as OpenClawConfig).session?.store, {
      agentId: route.agentId,
    }),
    ctxPayload,
    core: runtime,
    deliver: async () => {},
    onRecordError: (error: unknown) => {
      throw error instanceof Error ? error : new Error(String(error));
    },
    onDispatchError: (error: unknown) => {
      throw error instanceof Error ? error : new Error(String(error));
    },
  }).catch(() => {});
}
