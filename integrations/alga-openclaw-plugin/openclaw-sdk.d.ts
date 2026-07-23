/**
 * Minimal ambient typings so this package typechecks before `openclaw` is built to `dist/`.
 * At runtime the real `openclaw` package (or workspace link) supplies implementations.
 */

declare module "eventsource2" {
  declare class EventSource {
    static readonly CONNECTING: number;
    static readonly OPEN: number;
    static readonly CLOSED: number;
    readonly readyState: number;
    readonly url: string;
    onopen: ((ev: Event) => void) | null;
    onmessage: ((ev: MessageEvent) => void) | null;
    onerror: ((ev: Event) => void) | null;
    constructor(url: string, eventSourceInitDict?: { headers?: Record<string, string>; withCredentials?: boolean; proxy?: string });
    addEventListener(type: string, listener: (ev: MessageEvent) => void): void;
    removeEventListener(type: string, listener: (ev: MessageEvent) => void): void;
    close(): void;
  }
  export default EventSource;
}

declare module "openclaw/plugin-sdk/account-core" {
  export const createAccountListHelpers: any;
  export const DEFAULT_ACCOUNT_ID: string;
  export const normalizeAccountId: any;
  export const resolveMergedAccountConfig: any;
  export const describeAccountSnapshot: any;
}

declare module "openclaw/plugin-sdk/text-runtime" {
  export const normalizeOptionalString: any;
}

declare module "openclaw/plugin-sdk/channel-core" {
  export const defineChannelPluginEntry: any;
  export const defineSetupPluginEntry: any;
  export const createChatChannelPlugin: any;
  export const buildChannelOutboundSessionRoute: any;
  export type ChannelPlugin = any;
}

declare module "openclaw/plugin-sdk/channel-contract" {
  export type ChannelGatewayContext<T = unknown> = any;
  export type ChannelAgentTool = {
    label?: string;
    name: string;
    description: string;
    parameters: any;
    execute: (toolCallId: string, params: unknown, signal?: AbortSignal, onUpdate?: any) => Promise<any>;
  };
}

declare module "openclaw/plugin-sdk/channel-plugin-common" {
  export const getChatChannelMeta: any;
}

declare module "openclaw/plugin-sdk/channel-config-schema" {
  export const buildChannelConfigSchema: any;
}

declare module "openclaw/plugin-sdk/config-runtime" {
  export type OpenClawConfig = any;
}

declare module "openclaw/plugin-sdk/reply-chunking" {
  export const chunkText: any;
  export const resolveTextChunkLimit: any;
}

declare module "openclaw/plugin-sdk/reply-payload" {
  export const deliverFormattedTextWithAttachments: any;
  export const hasOutboundText: any;
  export type OutboundReplyPayload = any;
}

declare module "openclaw/plugin-sdk/channel-send-result" {
  export const attachChannelToResult: any;
  export const createAttachedChannelResultAdapter: any;
}

declare module "openclaw/plugin-sdk/runtime-store" {
  export const createPluginRuntimeStore: any;
  export type PluginRuntime = any;
}

declare module "openclaw/plugin-sdk/inbound-reply-dispatch" {
  export const dispatchInboundReplyWithBase: any;
}

declare module "openclaw/plugin-sdk/channel-inbound" {
  export const dispatchChannelInboundReply: any;
}

declare module "openclaw/plugin-sdk/status-helpers" {
  export const createComputedAccountStatusAdapter: any;
  export const createDefaultChannelRuntimeState: any;
}

declare module "openclaw/plugin-sdk/setup" {
  export const createStandardChannelSetupStatus: any;
  export const DEFAULT_ACCOUNT_ID: string;
  export const patchChannelConfigForAccount: any;
  export const setSetupChannelEnabled: any;
  export type ChannelSetupWizard = any;
  export type ChannelSetupWizardCredential = any;
  export type OpenClawConfig = any;
}

declare module "openclaw/plugin-sdk/secret-input" {
  export const hasConfiguredSecretInput: any;
}

declare module "openclaw/plugin-sdk/reply-runtime" {
  export type GetReplyOptions = {
    onToolStart?: (payload: {
      name?: string; phase?: string; args?: Record<string, unknown>; detailMode?: "explain" | "raw";
    }) => Promise<void> | void;
    onItemEvent?: (payload: {
      itemId?: string; kind?: string; title?: string; name?: string; phase?: string;
      status?: string; summary?: string; progressText?: string; meta?: string;
      approvalId?: string; approvalSlug?: string;
    }) => Promise<void> | void;
    onToolResult?: (payload: any) => Promise<void> | void;
    onPlanUpdate?: (payload: {
      phase?: string; title?: string; explanation?: string; steps?: string[]; source?: string;
    }) => Promise<void> | void;
    onApprovalEvent?: (payload: {
      phase?: string; kind?: string; status?: string; title?: string; itemId?: string;
      toolCallId?: string; approvalId?: string; approvalSlug?: string; command?: string;
      host?: string; reason?: string; scope?: string; message?: string;
    }) => Promise<void> | void;
    onCommandOutput?: (payload: {
      itemId?: string; phase?: string; title?: string; toolCallId?: string; name?: string;
      output?: string; status?: string; exitCode?: number | null; durationMs?: number; cwd?: string;
    }) => Promise<void> | void;
    onPatchSummary?: (payload: {
      itemId?: string; phase?: string; title?: string; toolCallId?: string; name?: string;
      added?: string[]; modified?: string[]; deleted?: string[]; summary?: string;
    }) => Promise<void> | void;
    onPartialReply?: (payload: any) => Promise<void> | void;
    onReasoningStream?: (payload: any) => Promise<void> | void;
    onReasoningEnd?: () => Promise<void> | void;
    onAssistantMessageStart?: () => Promise<void> | void;
    onCompactionStart?: () => Promise<void> | void;
    onCompactionEnd?: () => Promise<void> | void;
    suppressDefaultToolProgressMessages?: boolean;
    allowProgressCallbacksWhenSourceDeliverySuppressed?: boolean;
    sourceReplyDeliveryMode?: "automatic" | "message_tool_only";
  };
}
