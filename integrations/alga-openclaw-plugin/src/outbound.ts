import { attachChannelToResult } from "openclaw/plugin-sdk/channel-send-result";
import { agentPostMessage, agentEditMessage, agentDeleteMessage, agentPostTyping } from "./agent-rest.js";
import { outboundChatIdForAlga } from "./target.js";
import { normalizeInvestigationChatId } from "./chat-ids.js";
import { resolveAlgaAccount } from "./accounts.js";
import type { AlgaChannelData, AlgaInvestigationCommand, AlgaReplyPayload, CoreConfig } from "./types.js";

const CHANNEL = "alga" as const;

type AlgaPayloadContext = {
  cfg: CoreConfig;
  to: string;
  payload: AlgaReplyPayload;
  accountId?: string | null;
};

// `/stop` handling: once an operator types /stop in a thread, outbound text and
// typing indicators for that thread are suppressed until a non-/stop message
// clears the flag (see inbound.ts). Keyed by the canonical investigation chat
// id so the inbound SSE chat_id ("alert_83") and the outbound target
// ("investigation_alert_83") resolve to the same entry.
const stoppedChats = new Set<string>();

function normalizeChatIdForStop(chatId: string): string {
  if (chatId === "alga_dm") {
    return chatId;
  }
  return normalizeInvestigationChatId(chatId);
}

export function markChatStopped(chatId: string): void {
  stoppedChats.add(normalizeChatIdForStop(chatId));
}

export function clearChatStopped(chatId: string): void {
  stoppedChats.delete(normalizeChatIdForStop(chatId));
}

export function isChatStopped(chatId: string): boolean {
  return stoppedChats.has(normalizeChatIdForStop(chatId));
}

function resolveAlgaCommandFromPayload(payload: AlgaReplyPayload): AlgaInvestigationCommand | null {
  const algaData = payload.channelData?.alga as AlgaChannelData | undefined;
  if (algaData?.command?.op) {
    return algaData.command;
  }
  return null;
}

export async function sendAlgaPayload(ctx: AlgaPayloadContext) {
  const account = resolveAlgaAccount({ cfg: ctx.cfg, accountId: ctx.accountId });
  if (!account.configured) {
    throw new Error("Alga account is not configured");
  }
  const chatId = outboundChatIdForAlga(ctx.to);

  const command = resolveAlgaCommandFromPayload(ctx.payload);
  if (command) {
    const result = await agentPostMessage(account.httpBase, account.token, chatId, {
      kind: "inv_tool",
      command,
    });
    if (result.ok === false) {
      throw new Error(
        `Alga inv_tool "${command.op}" failed: ${result.error ?? "unknown error"}`,
      );
    }
    if (ctx.payload.text?.trim()) {
      await agentPostMessage(account.httpBase, account.token, chatId, {
        kind: "text",
        text: ctx.payload.text,
      });
    }
    return attachChannelToResult(CHANNEL, {
      messageId: "",
      conversationId: chatId,
    });
  }

  const text = ctx.payload.text ?? "";
  if (!text.trim()) {
    return attachChannelToResult(CHANNEL, {
      messageId: "",
      conversationId: chatId,
    });
  }
  const result = await agentPostMessage(account.httpBase, account.token, chatId, {
    kind: "text",
    text,
  });
  return attachChannelToResult(CHANNEL, {
    messageId: result.message_id || "",
    conversationId: chatId,
  });
}

export async function sendAlgaText(params: {
  cfg: CoreConfig;
  accountId?: string | null;
  to: string;
  text: string;
}): Promise<void> {
  const account = resolveAlgaAccount({ cfg: params.cfg, accountId: params.accountId });
  if (!account.configured) {
    throw new Error("Alga account is not configured");
  }
  const chatId = outboundChatIdForAlga(params.to);
  if (isChatStopped(chatId)) {
    return;
  }
  await agentPostMessage(account.httpBase, account.token, chatId, {
    kind: "text",
    text: params.text,
  });
}

export async function sendAlgaTyping(params: {
  cfg: CoreConfig;
  accountId?: string | null;
  to: string;
}): Promise<void> {
  const account = resolveAlgaAccount({ cfg: params.cfg, accountId: params.accountId });
  if (!account.configured) {
    return;
  }
  const chatId = outboundChatIdForAlga(params.to);
  if (isChatStopped(chatId)) {
    return;
  }
  try {
    await agentPostTyping(account.httpBase, account.token, chatId, true);
  } catch {
    /* ignore */
  }
}

export function sendAlgaTypingStop(params: {
  cfg: CoreConfig;
  accountId?: string | null;
  to: string;
}): void {
  const account = resolveAlgaAccount({ cfg: params.cfg, accountId: params.accountId });
  if (!account.configured) {
    return;
  }
  const chatId = outboundChatIdForAlga(params.to);
  void agentPostTyping(account.httpBase, account.token, chatId, false).catch(() => {});
}

export async function sendAlgaOutboundText(ctx: {
  cfg: CoreConfig;
  to: string;
  text: string;
  accountId?: string | null;
}) {
  const account = resolveAlgaAccount({ cfg: ctx.cfg, accountId: ctx.accountId });
  if (!account.configured) {
    throw new Error("Alga account is not configured");
  }
  const chatId = outboundChatIdForAlga(ctx.to);
  const result = await agentPostMessage(account.httpBase, account.token, chatId, {
    kind: "text",
    text: ctx.text,
  });
  return attachChannelToResult(CHANNEL, {
    messageId: result.message_id || "",
    conversationId: chatId,
  });
}

export async function editAlgaMessage(params: {
  cfg: CoreConfig;
  accountId?: string | null;
  to: string;
  messageId: string;
  text: string;
}): Promise<void> {
  const account = resolveAlgaAccount({ cfg: params.cfg, accountId: params.accountId });
  if (!account.configured) {
    throw new Error("Alga account is not configured");
  }
  const chatId = outboundChatIdForAlga(params.to);
  await agentEditMessage(account.httpBase, account.token, params.messageId, chatId, params.text);
}

export async function deleteAlgaMessage(params: {
  cfg: CoreConfig;
  accountId?: string | null;
  to: string;
  messageId: string;
}): Promise<void> {
  const account = resolveAlgaAccount({ cfg: params.cfg, accountId: params.accountId });
  if (!account.configured) return;
  const chatId = outboundChatIdForAlga(params.to);
  try {
    await agentDeleteMessage(account.httpBase, account.token, params.messageId, chatId);
  } catch {
    // Draft cleanup is best-effort
  }
}

// sendAlgaInvestigationCommand posts an investigation command (ack,
// resolve, set severity, set outcome, etc.) as an `inv_tool` message
// via Alga's unified POST /api/v1/agent/messages endpoint. Commands do
// not produce a user-visible message in the investigation thread, so the
// returned result carries an empty messageId.
export async function sendAlgaInvestigationCommand(ctx: {
  cfg: CoreConfig;
  to: string;
  command: AlgaInvestigationCommand;
  accountId?: string | null;
}) {
  const account = resolveAlgaAccount({ cfg: ctx.cfg, accountId: ctx.accountId });
  if (!account.configured) {
    throw new Error("Alga account is not configured");
  }
  const chatId = outboundChatIdForAlga(ctx.to);
  const result = await agentPostMessage(account.httpBase, account.token, chatId, {
    kind: "inv_tool",
    command: ctx.command,
  });
  if (result.ok === false) {
    throw new Error(
      `Alga inv_tool "${ctx.command.op}" failed: ${result.error ?? "unknown error"}`,
    );
  }
  return attachChannelToResult(CHANNEL, {
    messageId: "",
    conversationId: chatId,
  });
}
