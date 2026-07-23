import { handleAlgaInbound, handleAlgaInvestigationSignal } from "./inbound.js";
import {
  AlgaSSEClient,
  registerAlgaSSESession,
  unregisterAlgaSSESession,
} from "./agent-sse.js";
import type { ChannelGatewayContext } from "openclaw/plugin-sdk/channel-contract";
import type { CoreConfig, ResolvedAlgaAccount } from "./types.js";

const CHANNEL_ID = "alga" as const;

export async function startAlgaGatewayAccount(
  ctx: ChannelGatewayContext<ResolvedAlgaAccount>,
): Promise<void> {
  const account = ctx.account;
  if (!account.configured) {
    throw new Error(
      `Alga is not configured for account "${account.accountId}" (set ALGA_SERVER_URL and ALGA_AGENT_TOKEN, or channels.alga.accounts.*).`,
    );
  }

  ctx.setStatus({
    accountId: account.accountId,
    running: true,
    configured: true,
    enabled: account.enabled,
    connected: false,
  });

  const client = new AlgaSSEClient({
    httpBase: account.httpBase,
    token: account.token,
    log: {
      info: (m) => ctx.log?.info?.(m),
      warn: (m) => ctx.log?.warn?.(m),
      error: (m) => ctx.log?.error?.(m),
    },
    onInboundText: (raw) => {
      void handleAlgaInbound({
        channelId: CHANNEL_ID,
        channelLabel: "Alga",
        account,
        config: ctx.cfg as CoreConfig,
        rawJson: raw,
      }).catch((err) => {
        ctx.log?.error?.(
          `Alga inbound handling failed: ${err instanceof Error ? err.message : String(err)}`,
        );
      });
    },
    onInvestigationSignal: (signalType, data) => {
      handleAlgaInvestigationSignal(signalType, data, ctx.cfg as CoreConfig, account);
    },
  });

  registerAlgaSSESession(account.accountId, client);

  try {
    await client.start(ctx.abortSignal);
  } finally {
    await client.stop();
    unregisterAlgaSSESession(account.accountId);
    ctx.setStatus({
      accountId: account.accountId,
      running: false,
    });
  }
}
