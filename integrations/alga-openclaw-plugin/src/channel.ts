import {
  buildChannelOutboundSessionRoute,
  createChatChannelPlugin,
} from "openclaw/plugin-sdk/channel-core";
import { getChatChannelMeta } from "openclaw/plugin-sdk/channel-plugin-common";
import { describeAccountSnapshot } from "openclaw/plugin-sdk/account-core";
import {
  listAlgaAccountIds,
  resolveAlgaAccount,
  resolveDefaultAlgaAccountId,
  isAlgaConfigured,
  DEFAULT_ACCOUNT_ID,
} from "./accounts.js";
import { algaChannelPluginConfigSchema } from "./config-schema.js";
import { startAlgaGatewayAccount } from "./gateway.js";
import { applyAlgaSetup } from "./setup.js";
import { algaSetupWizard } from "./setup-wizard.js";
import { algaChannelStatus } from "./status.js";
import {
  inferAlgaChatTypeFromTarget,
  looksLikeAlgaTargetId,
  normalizeAlgaMessagingTarget,
  parseAlgaExplicitTarget,
} from "./target.js";
import { createAlgaCommandTools } from "./agent-tools.js";
import { algaMessageAdapter, algaOutboundAdapter } from "./message-adapter.js";
import type { CoreConfig } from "./types.js";

const CHANNEL_ID = "alga" as const;
const meta = {
  ...(getChatChannelMeta(CHANNEL_ID) ?? {
    id: CHANNEL_ID,
    label: "Alga",
    selectionLabel: "Alga (investigations)",
    docsPath: "https://docs.openclaw.ai/alga",
    blurb: "Alga investigations over SSE + REST.",
    order: 70,
  }),
};

const _plugin = createChatChannelPlugin({
  base: {
    id: CHANNEL_ID,
    meta,
    setupWizard: algaSetupWizard,
    capabilities: {
      chatTypes: ["group", "direct"],
    },
    agentTools: (params) => createAlgaCommandTools(params?.cfg as CoreConfig | undefined),
    agentPrompt: {
      messageToolHints: () => [
        "You are an SRE investigation agent in an Alga investigation chat. Operators are reading your output in real time.",
        "Follow the backend-provided 'Investigation Instructions' and 'Incident Instructions' in the chat context. They are the authoritative source for your role, tool usage rules, and workflow.",
        "CRITICAL: In EVERY assistant turn, write a visible text block BEFORE and AFTER every tool call. Thinking blocks are invisible to operators. Never collapse the whole investigation into one final message — operators need to see your work as it happens.",
        "CRITICAL: After every tool call, narrate it in your visible text on its own line using the format `🧩 tool_name [key arg]`. Example: `🧩 alga_list_alerts` or `🧩 alga_search_knowledge query=\"TestAlert\"` or `🧩 alga_resolve_alert`. You may batch multiple tool-call narrations into a single text block (one per line). This is the operator-visible tool-call history.",
        "CRITICAL: After your last action (resolve_alert, promote_to_incident, cancel_investigation, pause_investigation, or set_outcome), write a final assistant text message in markdown summarizing the investigation. Include: alert/fingerprint, status, findings, root cause, evidence, resolution, and any runbook references. Use `##` headings and bullet lists. This is your last message in the thread.",
        "CRITICAL: NEVER output 'NO_REPLY' or 'HEARTBEAT_OK' as your text. Always write a real message.",
        "Interleave analysis and tool calls: brief analysis → tool call(s) → analysis of results → next tool call(s) → ... → final markdown summary.",
        "You may call multiple independent tools in one turn; narrate each one. Don't add artificial sequencing for tools that have no data dependency.",
        "Format your output with markdown headings, bullet points, and code blocks for readability.",
        "Do NOT use exec/curl to call the Alga API — use the dedicated tools instead.",
        "If exec/shell commands fail (timeout, no cluster access), do NOT retry — analyze from available context instead.",
      ],
    },
    streaming: {
      blockStreamingCoalesceDefaults: { minChars: 1, idleMs: 1000 },
    },
    reload: { configPrefixes: ["channels.alga"] },
    configSchema: algaChannelPluginConfigSchema,
    setup: {
      applyAccountConfig: ({ cfg, accountId, input }) =>
        applyAlgaSetup({
          cfg,
          accountId,
          input: input as Record<string, unknown>,
        }),
    },
    config: {
      listAccountIds: (cfg) => listAlgaAccountIds(cfg as CoreConfig),
      resolveAccount: (cfg, accountId) => resolveAlgaAccount({ cfg: cfg as CoreConfig, accountId }),
      defaultAccountId: (cfg) => resolveDefaultAlgaAccountId(cfg as CoreConfig),
      isConfigured: (account) => isAlgaConfigured(account),
      describeAccount: (account) =>
        describeAccountSnapshot({
          account,
          configured: account.configured,
          extra: { serverUrl: account.serverUrl },
        }),
      resolveAllowFrom: ({ cfg, accountId }) =>
        resolveAlgaAccount({ cfg: cfg as CoreConfig, accountId }).config.allowFrom,
      resolveDefaultTo: ({ cfg, accountId }) =>
        resolveAlgaAccount({ cfg: cfg as CoreConfig, accountId }).config.defaultTo,
    },
    messaging: {
      normalizeTarget: (raw) => normalizeAlgaMessagingTarget(raw),
      parseExplicitTarget: ({ raw }) => {
        const parsed = parseAlgaExplicitTarget(raw);
        if (!parsed) {
          return null;
        }
        return {
          to: parsed.to,
          threadId: parsed.investigationSessionId,
          chatType: parsed.chatType,
        };
      },
      inferTargetChatType: (input: { raw?: string; to?: string; target?: string } | undefined) => {
        const raw =
          (typeof input?.raw === "string" && input.raw) ||
          (typeof input?.to === "string" && input.to) ||
          (typeof input?.target === "string" && input.target) ||
          "";
        return inferAlgaChatTypeFromTarget(raw);
      },
      targetResolver: {
        looksLikeId: looksLikeAlgaTargetId,
        hint: "<investigation_id|investigation_<id>|alga:...>",
      },
      resolveOutboundSessionRoute: ({ cfg, agentId, accountId, target, threadId }) => {
        const parsed = parseAlgaExplicitTarget(target);
        if (!parsed) {
          return null;
        }
        return buildChannelOutboundSessionRoute({
          cfg,
          agentId,
          channel: CHANNEL_ID,
          accountId,
          peer: { kind: "channel", id: parsed.to },
          chatType: parsed.chatType,
          from: `${CHANNEL_ID}:${accountId ?? DEFAULT_ACCOUNT_ID}`,
          to: parsed.to,
          threadId: threadId ?? parsed.investigationSessionId,
        });
      },
    },
    status: algaChannelStatus,
    gateway: {
      startAccount: async (ctx) => {
        await startAlgaGatewayAccount(ctx);
      },
    },
    message: algaMessageAdapter,
  },
  outbound: algaOutboundAdapter,
});

export const algaChannelPlugin = _plugin;
