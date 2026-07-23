import { DEFAULT_ACCOUNT_ID } from "./accounts.js";
import {
  createComputedAccountStatusAdapter,
  createDefaultChannelRuntimeState,
} from "openclaw/plugin-sdk/status-helpers";

export const algaChannelStatus = createComputedAccountStatusAdapter({
  defaultRuntime: createDefaultChannelRuntimeState(DEFAULT_ACCOUNT_ID),
  buildChannelSummary: ({ snapshot }) => ({
    configured: Boolean(snapshot.configured),
    running: Boolean(snapshot.running),
  }),
  resolveAccountSnapshot: ({ account, runtime }) => ({
    accountId: account.accountId,
    name: account.name,
    enabled: account.enabled,
    configured: account.configured,
    extra: {
      serverUrl: account.serverUrl || "[missing]",
      connected: runtime?.connected ?? false,
    },
  }),
});
