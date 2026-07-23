import {
  createAccountListHelpers,
  DEFAULT_ACCOUNT_ID,
  normalizeAccountId,
  resolveMergedAccountConfig,
} from "openclaw/plugin-sdk/account-core";
import { normalizeOptionalString } from "openclaw/plugin-sdk/string-coerce-runtime";
import { algaUrlsFromBase } from "./endpoints.js";
import type { AlgaAccountConfig, CoreConfig, ResolvedAlgaAccount } from "./types.js";

const { listAccountIds: listAlgaAccountIds, resolveDefaultAccountId: resolveDefaultAlgaAccountId } =
  createAccountListHelpers("alga", { normalizeAccountId });

export { listAlgaAccountIds, resolveDefaultAlgaAccountId, DEFAULT_ACCOUNT_ID };

function resolveMergedAlgaAccountConfig(cfg: CoreConfig, accountId: string): AlgaAccountConfig {
  const algaSection = (cfg.channels as Record<string, unknown> | undefined)?.alga as
    | (AlgaAccountConfig & { accounts?: Record<string, Partial<AlgaAccountConfig>> })
    | undefined;
  return resolveMergedAccountConfig({
    channelConfig: algaSection,
    accounts: algaSection?.accounts,
    accountId,
    omitKeys: ["defaultAccount"],
    normalizeAccountId,
  }) as AlgaAccountConfig;
}

function envServerUrl(): string {
  return (
    process.env.ALGA_SERVER_URL?.trim() ||
    process.env.ALGA_URL?.trim() ||
    ""
  );
}

function envToken(): string {
  return process.env.ALGA_AGENT_TOKEN?.trim() || "";
}

export function resolveAlgaAccount(params: {
  cfg: CoreConfig;
  accountId?: string | null;
}): ResolvedAlgaAccount {
  const accountId = normalizeAccountId(params.accountId);
  const merged = resolveMergedAlgaAccountConfig(params.cfg, accountId);
  const baseEnabled =
    (params.cfg.channels as Record<string, { enabled?: boolean }> | undefined)?.alga?.enabled !==
    false;
  const enabled = baseEnabled && merged.enabled !== false;

  const serverUrl = (merged.serverUrl?.trim() || envServerUrl()).replace(/\/$/, "");
  const configToken = merged.token?.trim();
  const envTokenVal = envToken();
  const token = configToken || envTokenVal;
  const tokenSource: "config" | "env" | "none" = configToken
    ? "config"
    : envTokenVal
      ? "env"
      : "none";
  const { httpBase } = algaUrlsFromBase(serverUrl);

  return {
    accountId,
    enabled,
    configured: Boolean(serverUrl && token && httpBase),
    name: normalizeOptionalString(merged.name),
    serverUrl,
    httpBase,
    token,
    tokenSource,
    tokenStatus: token ? "available" : "missing",
    config: {
      ...merged,
      allowFrom: merged.allowFrom ?? ["*"],
    },
  };
}

export function isAlgaConfigured(account: ResolvedAlgaAccount): boolean {
  return account.configured;
}
