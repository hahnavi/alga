import {
  createStandardChannelSetupStatus,
  DEFAULT_ACCOUNT_ID,
  patchChannelConfigForAccount,
  setSetupChannelEnabled,
  type ChannelSetupWizard,
  type ChannelSetupWizardCredential,
  type OpenClawConfig,
} from "openclaw/plugin-sdk/setup";
import { hasConfiguredSecretInput } from "openclaw/plugin-sdk/secret-input";
import { normalizeOptionalString } from "openclaw/plugin-sdk/string-coerce-runtime";
import { resolveAlgaAccount } from "./accounts.js";

const channel = "alga" as const;

function enableAlgaAccount(cfg: OpenClawConfig, accountId: string): OpenClawConfig {
  return patchChannelConfigForAccount({
    cfg,
    channel,
    accountId,
    patch: { enabled: true },
  });
}

const serverUrlCredential: ChannelSetupWizardCredential = {
  inputKey: "serverUrl",
  providerHint: "alga-server",
  credentialLabel: "Alga server URL",
  preferredEnvVar: "ALGA_SERVER_URL",
  envPrompt: "ALGA_SERVER_URL detected. Use env var?",
  keepPrompt: "Alga server URL already configured. Keep it?",
  inputPrompt: "Enter Alga server URL (e.g. http://localhost:8080)",
  allowEnv: ({ accountId }: { accountId: string }) => accountId === DEFAULT_ACCOUNT_ID,
  inspect: ({ cfg, accountId }: { cfg: OpenClawConfig; accountId: string }) => {
    const resolved = resolveAlgaAccount({ cfg, accountId });
    return {
      accountConfigured: Boolean(resolved.serverUrl),
      hasConfiguredValue: Boolean(resolved.config.serverUrl),
      resolvedValue: normalizeOptionalString(resolved.serverUrl),
      envValue:
        accountId === DEFAULT_ACCOUNT_ID
          ? normalizeOptionalString(process.env.ALGA_SERVER_URL)
          : undefined,
    };
  },
  applyUseEnv: ({ cfg, accountId }: { cfg: OpenClawConfig; accountId: string }) =>
    enableAlgaAccount(cfg, accountId),
  applySet: ({
    cfg,
    accountId,
    value,
  }: {
    cfg: OpenClawConfig;
    accountId: string;
    value: unknown;
  }) =>
    patchChannelConfigForAccount({
      cfg,
      channel,
      accountId,
      patch: {
        enabled: true,
        serverUrl: value,
      },
    }),
};

const tokenCredential: ChannelSetupWizardCredential = {
  inputKey: "token",
  providerHint: "alga-token",
  credentialLabel: "Alga agent token",
  preferredEnvVar: "ALGA_AGENT_TOKEN",
  envPrompt: "ALGA_AGENT_TOKEN detected. Use env var?",
  keepPrompt: "Alga agent token already configured. Keep it?",
  inputPrompt: "Enter Alga agent token",
  allowEnv: ({ accountId }: { accountId: string }) => accountId === DEFAULT_ACCOUNT_ID,
  inspect: ({ cfg, accountId }: { cfg: OpenClawConfig; accountId: string }) => {
    const resolved = resolveAlgaAccount({ cfg, accountId });
    const configuredValue = resolved.config.token;
    return {
      accountConfigured: Boolean(resolved.token) || hasConfiguredSecretInput(configuredValue),
      hasConfiguredValue: hasConfiguredSecretInput(configuredValue),
      resolvedValue: normalizeOptionalString(resolved.token),
      envValue:
        accountId === DEFAULT_ACCOUNT_ID
          ? normalizeOptionalString(process.env.ALGA_AGENT_TOKEN)
          : undefined,
    };
  },
  applyUseEnv: ({ cfg, accountId }: { cfg: OpenClawConfig; accountId: string }) =>
    enableAlgaAccount(cfg, accountId),
  applySet: ({
    cfg,
    accountId,
    value,
  }: {
    cfg: OpenClawConfig;
    accountId: string;
    value: unknown;
  }) =>
    patchChannelConfigForAccount({
      cfg,
      channel,
      accountId,
      patch: {
        enabled: true,
        token: value,
      },
    }),
};

export const algaSetupWizard: ChannelSetupWizard = {
  channel,
  status: createStandardChannelSetupStatus({
    channelLabel: "Alga",
    configuredLabel: "configured",
    unconfiguredLabel: "needs server URL and token",
    configuredHint: "server URL + token set",
    unconfiguredHint: "needs server URL and token",
    configuredScore: 2,
    unconfiguredScore: 1,
    resolveConfigured: ({ cfg, accountId }) =>
      resolveAlgaAccount({ cfg, accountId }).configured,
  }),
  introNote: {
    title: "Alga investigations gateway",
    lines: [
      "Connect OpenClaw to your Alga instance for automated SRE investigations.",
      "",
      "You need:",
      "  1. Alga server URL (the HTTP(S) address of your Alga backend)",
      "  2. Alga agent token (from: alga agent-tokens create, or the Alga console)",
      "",
      "Environment variables: ALGA_SERVER_URL, ALGA_AGENT_TOKEN",
    ],
  },
  envShortcut: {
    prompt: "ALGA_SERVER_URL + ALGA_AGENT_TOKEN detected. Use env vars?",
    preferredEnvVar: "ALGA_SERVER_URL",
    isAvailable: ({ cfg, accountId }) =>
      accountId === DEFAULT_ACCOUNT_ID &&
      Boolean(process.env.ALGA_SERVER_URL?.trim()) &&
      Boolean(process.env.ALGA_AGENT_TOKEN?.trim()) &&
      !resolveAlgaAccount({ cfg, accountId }).configured,
    apply: ({ cfg, accountId }) => enableAlgaAccount(cfg, accountId),
  },
  credentials: [serverUrlCredential, tokenCredential],
  completionNote: {
    title: "Alga configured",
    lines: [
      "Alga is ready. Investigations will be bridged over SSE + REST.",
      "Use the Alga operations console to manage alerts and routes.",
    ],
  },
  disable: (cfg: OpenClawConfig) => setSetupChannelEnabled(cfg, channel, false),
};
