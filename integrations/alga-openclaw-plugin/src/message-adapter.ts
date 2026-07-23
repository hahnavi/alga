import { createAttachedChannelResultAdapter } from "openclaw/plugin-sdk/channel-send-result";
import { chunkText, resolveTextChunkLimit } from "openclaw/plugin-sdk/reply-chunking";
import { agentPostMessage } from "./agent-rest.js";
import { sendAlgaPayload } from "./outbound.js";
import { outboundChatIdForAlga } from "./target.js";
import { resolveAlgaAccount } from "./accounts.js";
import type { AlgaReplyPayload, CoreConfig } from "./types.js";
import type { OpenClawConfig } from "openclaw/plugin-sdk/config-runtime";

const CHANNEL = "alga" as const;

export const algaOutboundAdapter = {
  deliveryMode: "direct" as const,
  chunker: chunkText,
  chunkerMode: "text" as const,
  textChunkLimit: 4000,
  resolveEffectiveTextChunkLimit: ({ cfg, accountId, fallbackLimit }: {
    cfg: OpenClawConfig; accountId?: string | null; fallbackLimit?: number;
  }) => resolveTextChunkLimit(cfg as OpenClawConfig, CHANNEL, accountId ?? undefined, {
    fallbackLimit: fallbackLimit ?? 4000,
  }),
  shouldTreatDeliveredTextAsVisible: ({ kind }: { kind: string }) => kind === "final",
  deliveryCapabilities: { durableFinal: { text: true } },
  ...createAttachedChannelResultAdapter({
    channel: CHANNEL,
    sendText: async (ctx: { cfg: CoreConfig; to: string; text: string; accountId?: string | null }) => {
      const account = resolveAlgaAccount({ cfg: ctx.cfg, accountId: ctx.accountId });
      if (!account.configured) throw new Error("Alga account is not configured");
      const chatId = outboundChatIdForAlga(ctx.to);
      const result = await agentPostMessage(account.httpBase, account.token, chatId, {
        kind: "text", text: ctx.text,
      });
      return { messageId: result.message_id || "", conversationId: chatId };
    },
  }),
  sendPayload: async (ctx: { cfg: CoreConfig; to: string; payload: AlgaReplyPayload; accountId?: string | null }) => {
    return await sendAlgaPayload(ctx);
  },
  resolveTarget: ({ to }: { to?: string }) => {
    const trimmed = to?.trim();
    if (!trimmed) return { ok: false as const, error: new Error("Delivering to Alga requires --to <investigation_id>") };
    return { ok: true as const, to: trimmed };
  },
};

export const algaMessageAdapter = {
  id: CHANNEL,
  durableFinal: { capabilities: { text: true } },
  send: {
    text: async (ctx: any) => {
      const account = resolveAlgaAccount({ cfg: ctx.cfg, accountId: ctx.accountId });
      if (!account.configured) throw new Error("Alga account is not configured");
      const chatId = outboundChatIdForAlga(ctx.to);
      const result = await agentPostMessage(account.httpBase, account.token, chatId, {
        kind: "text", text: ctx.text,
      });
      return {
        receipt: { messageId: result.message_id || "", chatId, channelId: CHANNEL },
        attachChannel: { channel: CHANNEL, messageId: result.message_id || "", conversationId: chatId },
      };
    },
    media: async () => ({
      receipt: { messageId: "", chatId: "", channelId: CHANNEL },
      attachChannel: { channel: CHANNEL, messageId: "", conversationId: "" },
    }),
    payload: async (ctx: any) => {
      const r = await sendAlgaPayload(ctx);
      const chatId = outboundChatIdForAlga(ctx.to);
      return {
        receipt: { messageId: r?.messageId || "", chatId: r?.conversationId || chatId, channelId: CHANNEL },
        attachChannel: { channel: CHANNEL, messageId: r?.messageId || "", conversationId: r?.conversationId || chatId },
      };
    },
  },
  // No `live`/`finalizer` capability claims: this channel does NOT implement
  // the message-adapter draft-preview lifecycle (deliverFinalizableLivePreview).
  // Tool calls and the final summary are posted directly as one-shot messages
  // through the inbound delivery path (see inbound.ts): onToolStart posts each
  // tool call immediately, and delivery.deliver posts the final text once at
  // turn end. Advertising draftPreview/previewFinalization/finalEdit here
  // without backing methods would make OpenClaw core believe it could drive an
  // in-place edit/finalize flow that does not exist, which previously competed
  // with the inbound path and produced duplicate/divergent thread messages.
  receive: { defaultAckPolicy: "manual", supportedAckPolicies: ["manual"] },
};
