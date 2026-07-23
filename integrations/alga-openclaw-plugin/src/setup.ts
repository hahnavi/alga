import type { OpenClawConfig } from "openclaw/plugin-sdk/config-contracts";
import { DEFAULT_ACCOUNT_ID } from "./accounts.js";
import type { CoreConfig } from "./types.js";

export function applyAlgaSetup(params: {
  cfg: OpenClawConfig;
  accountId: string;
  input: Record<string, unknown>;
}): OpenClawConfig {
  const nextCfg = structuredClone(params.cfg) as CoreConfig;
  const channels = (nextCfg.channels ?? {}) as Record<string, Record<string, unknown>>;
  const section = (channels.alga ?? {}) as Record<string, unknown>;
  const accounts = { ...((section.accounts as Record<string, unknown> | undefined) ?? {}) };
  const target =
    params.accountId === DEFAULT_ACCOUNT_ID
      ? { ...section }
      : { ...((accounts[params.accountId] as Record<string, unknown> | undefined) ?? {}) };

  if (typeof params.input.serverUrl === "string") {
    (target as { serverUrl?: string }).serverUrl = params.input.serverUrl;
  }
  if (typeof params.input.token === "string") {
    (target as { token?: string }).token = params.input.token;
  }
  if (typeof params.input.name === "string") {
    (target as { name?: string }).name = params.input.name;
  }

  if (params.accountId === DEFAULT_ACCOUNT_ID) {
    channels.alga = {
      ...section,
      ...target,
    };
  } else {
    accounts[params.accountId] = target;
    channels.alga = {
      ...section,
      accounts,
    };
  }
  nextCfg.channels = channels as CoreConfig["channels"];
  return nextCfg as OpenClawConfig;
}
