import { defineChannelPluginEntry } from "openclaw/plugin-sdk/channel-core";
import { algaChannelPlugin } from "./channel.js";
import { setAlgaRuntime } from "./runtime.js";

export const ALGA_TOOL_NAMES = [
  "alga_resolve_alert",
  "alga_reopen_alert",
  "alga_promote_to_incident",
  "alga_set_outcome",
  "alga_cancel_investigation",
  "alga_pause_investigation",
  "alga_search_knowledge",
  "alga_get_knowledge",
  "alga_create_knowledge",
  "alga_list_alerts",
  "alga_triage_feedback",
  "alga_get_incident_context",
  "alga_get_incident_timeline",
  "alga_add_incident_timeline",
  "alga_who_is_on_call",
  "alga_list_services",
  "alga_search_memories",
  "alga_create_memory",
  "alga_peer_ask",
  "alga_assign_investigation",
  "alga_set_incident_priority",
  "alga_set_incident_severity",
  "alga_trigger_escalation",
  "alga_request_status_update",
  "alga_mitigate_incident",
  "alga_resolve_incident",
  "alga_begin_triage",
  "alga_promote_incident",
  "alga_assign_incident_role",
  "alga_post_handoff",
  "alga_publish_status_update",
  "alga_set_incident_resolution_docs",
];

async function ensureAlgaToolsInAllowlist(api: {
  config: Record<string, unknown>;
  runtime: { config: { current: () => Record<string, unknown>; mutateConfigFile: (p: Record<string, unknown>) => Promise<unknown> } };
}) {
  const current = api.runtime.config.current() as {
    tools?: { alsoAllow?: string[] };
  };
  const existing = new Set(current.tools?.alsoAllow ?? []);
  const missing = ALGA_TOOL_NAMES.filter((t) => !existing.has(t));
  if (missing.length === 0) return;

  await api.runtime.config.mutateConfigFile({
    afterWrite: { mode: "auto" },
    mutate(draft: Record<string, unknown>) {
      const tools = (draft.tools ?? {}) as Record<string, unknown>;
      const alsoAllow = [...((tools.alsoAllow as string[]) ?? [])];
      for (const t of missing) {
        if (!alsoAllow.includes(t)) alsoAllow.push(t);
      }
      draft.tools = { ...tools, alsoAllow };
    },
  } as Parameters<typeof api.runtime.config.mutateConfigFile>[0]);
}

export default defineChannelPluginEntry({
  id: "alga",
  name: "Alga",
  description: "Alga investigations: SSE + REST bridge using OpenClaw agent tokens.",
  plugin: algaChannelPlugin,
  setRuntime: setAlgaRuntime,
  registerFull(api) {
    ensureAlgaToolsInAllowlist(api).catch(() => {});
  },
});

// Named exports for downstream tools/extensions that need to post
// structured investigation commands (acknowledge, resolve, set severity,
// set outcome, etc.) through Alga's unified /api/v1/agent/messages
// endpoint without round-tripping through a chat-visible text message.
export { sendAlgaInvestigationCommand, sendAlgaOutboundText, sendAlgaPayload } from "./outbound.js";
export type {
  AlgaChannelData,
  AlgaInvestigationCommand,
  AlgaInvestigationCommandOp,
  AlgaInvestigationSeverity,
  AlgaOutboundMessagePayload,
  InvestigationSignalEventType,
  InvestigationSignalEvent,
} from "./types.js";
