import { Type } from "@sinclair/typebox";
import type { ChannelAgentTool } from "openclaw/plugin-sdk/channel-contract";
import { resolveAlgaAccount } from "./accounts.js";
import { agentPostMessage } from "./agent-rest.js";
import type { AgentPostMessageResult } from "./agent-rest.js";
import type { AlgaInvestigationCommand, AlgaInvestigationSeverity, CoreConfig, ResolvedAlgaAccount } from "./types.js";

const InvestigationIdParam = Type.String({
  description:
    "Investigation identifier. This is the Alga chat id of the investigation thread: an alert investigation is `alert_<number>` (e.g. `alert_42`), an incident investigation thread is `incident_coord_<number>` or `incident_inv_<number>`. A bare number is treated as an alert number.",
});

const IncidentIdParam = Type.Optional(
  Type.String({
    description:
      "Incident number when the op targets an incident and the investigation_id is not already an `incident_*` chat id. Used to build the incident coordination chat id `incident_coord_<number>`.",
  }),
);

const INCIDENT_TOOL_OPS = new Set<AlgaInvestigationCommand["op"]>([
  "set_incident_priority",
  "set_incident_severity",
  "trigger_escalation",
  "request_status_update",
  "mitigate_incident",
  "resolve_incident",
  "begin_triage",
  "promote_incident",
  "assign_incident_role",
  "post_handoff",
  "publish_status_update",
  "set_incident_resolution_docs",
]);

export function createAlgaCommandTools(cfg?: CoreConfig): ChannelAgentTool[] {
  const tools: ChannelAgentTool[] = [
    {
      label: "Promote Alert to Incident",
      name: "alga_promote_to_incident",
      description:
        "Promote an alert to an incident from alert investigation. Borrow the SRE investigation summary as the incident description.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        title: Type.Optional(
          Type.String({ description: "Optional custom title for the created incident." })
        ),
        severity: Type.Optional(
          Type.String({ description: "Optional severity (critical, high, warning, info; defaults to warning)." })
        ),
        priority: Type.Optional(
          Type.String({ description: "Optional priority (P1, P2, P3, P4, P5; computed automatically if omitted)." })
        ),
      }),
      execute: async (_id, args) => {
        const p = args as {
          investigation_id?: string;
          title?: string;
          severity?: string;
          priority?: string;
        };
        if (!p.investigation_id) return errText("promote_to_incident", "missing investigation_id");
        const cmd: AlgaInvestigationCommand = { op: "promote_to_incident" };
        if (p.title) cmd.title = p.title;
        if (p.severity) cmd.severity = p.severity as AlgaInvestigationSeverity;
        if (p.priority) cmd.priority = p.priority;
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("promote_to_incident", r.error ?? "unknown");
          return okText("promote_to_incident", p.investigation_id);
        } catch (e) {
          return catchErr("promote_to_incident", e);
        }
      },
    },
    {
      label: "Resolve Alert",
      name: "alga_resolve_alert",
      description:
        "Resolve a single alert linked to the investigation. Optionally include root cause and resolution. " +
        "The investigation transitions to 'complete' only when ALL linked alerts are resolved.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        fingerprint: Type.Optional(
          Type.String({
            description: "Alert fingerprint to resolve. If omitted, resolves the primary alert.",
          }),
        ),
        root_cause: Type.Optional(
          Type.String({ description: "Root cause analysis for this alert." }),
        ),
        resolution: Type.Optional(
          Type.String({ description: "Steps taken to remediate the issue." }),
        ),
      }),
      execute: async (_id, args) => {
        const p = args as {
          investigation_id?: string;
          fingerprint?: string;
          root_cause?: string;
          resolution?: string;
        };
        if (!p.investigation_id) return errText("resolve_alert", "missing investigation_id");
        const cmd: AlgaInvestigationCommand = { op: "resolve_alert" };
        if (p.fingerprint) cmd.fingerprint = p.fingerprint;
        if (p.root_cause) cmd.root_cause = p.root_cause;
        if (p.resolution) cmd.resolution = p.resolution;
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("resolve_alert", r.error ?? "unknown");
          return okText("resolve_alert", p.investigation_id);
        } catch (e) {
          return catchErr("resolve_alert", e);
        }
      },
    },
    {
      label: "Reopen Alert",
      name: "alga_reopen_alert",
      description:
        "Reopen a previously resolved alert. Use this if the issue recurs or the resolution was premature. " +
        "If the investigation was completed, the system will re-activate it and re-delegate to an agent.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        fingerprint: Type.Optional(
          Type.String({
            description: "Alert fingerprint to reopen. If omitted, reopens the primary alert.",
          }),
        ),
      }),
      execute: async (_id, args) => {
        const p = args as { investigation_id?: string; fingerprint?: string };
        if (!p.investigation_id) return errText("reopen_alert", "missing investigation_id");
        const cmd: AlgaInvestigationCommand = { op: "reopen_alert" };
        if (p.fingerprint) cmd.fingerprint = p.fingerprint;
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("reopen_alert", r.error ?? "unknown");
          return okText("reopen_alert", p.investigation_id);
        } catch (e) {
          return catchErr("reopen_alert", e);
        }
      },
    },
    {
      label: "Set Outcome",
      name: "alga_set_outcome",
      description:
        "Record root cause and/or resolution on the investigation without resolving alerts or closing it. " +
        "Use this to progressively document findings during the investigation. " +
        "When done investigating, use alga_resolve_alert to finalize and close.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        root_cause: Type.Optional(
          Type.String({ description: "Root cause analysis. What caused the issue?" }),
        ),
        resolution: Type.Optional(
          Type.String({ description: "Steps taken or recommended to fix the issue." }),
        ),
      }),
      execute: async (_id, args) => {
        const p = args as {
          investigation_id?: string;
          root_cause?: string;
          resolution?: string;
        };
        if (!p.investigation_id) return errText("set_outcome", "missing investigation_id");
        const cmd: AlgaInvestigationCommand = { op: "set_outcome" };
        if (p.root_cause) cmd.root_cause = p.root_cause;
        if (p.resolution) cmd.resolution = p.resolution;
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("set_outcome", r.error ?? "unknown");
          return okText("set_outcome", p.investigation_id);
        } catch (e) {
          return catchErr("set_outcome", e);
        }
      },
    },
    {
      label: "Cancel Investigation",
      name: "alga_cancel_investigation",
      description:
        "Cancel the investigation. Use this when the alert is a false positive, not actionable, " +
        "or when investigation cannot proceed (e.g. insufficient data, transient issue that resolved itself). " +
        "Provide a clear reason for cancellation. The investigation status becomes 'cancelled'.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        reason: Type.String({
          description: "Reason for cancellation (e.g. 'false positive', 'transient issue resolved on its own').",
        }),
      }),
      execute: async (_id, args) => {
        const p = args as { investigation_id?: string; reason?: string };
        if (!p.investigation_id) return errText("cancel_investigation", "missing investigation_id");
        try {
          const r = await execInvTool(cfg, p.investigation_id, {
            op: "cancel_investigation",
            reason: p.reason ?? "",
          });
          if (r.ok === false) return errText("cancel_investigation", r.error ?? "unknown");
          return okText("cancel_investigation", p.investigation_id, "Investigation cancelled.");
        } catch (e) {
          return catchErr("cancel_investigation", e);
        }
      },
    },
    {
      label: "Pause Investigation",
      name: "alga_pause_investigation",
      description:
        "Pause the investigation. Use when waiting for human input, external events, or additional data " +
        "before continuing. The investigation can be resumed later by an operator or via a reopen_alert command. " +
        "Combine with mentioning an admin group in a chat message to escalate for human review.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        reason: Type.String({
          description: "Reason for pausing (e.g. 'waiting for deployment rollup', 'need human approval').",
        }),
      }),
      execute: async (_id, args) => {
        const p = args as { investigation_id?: string; reason?: string };
        if (!p.investigation_id) return errText("pause_investigation", "missing investigation_id");
        try {
          const r = await execInvTool(cfg, p.investigation_id, {
            op: "pause_investigation",
            reason: p.reason ?? "",
          });
          if (r.ok === false) return errText("pause_investigation", r.error ?? "unknown");
          return okText("pause_investigation", p.investigation_id, "Investigation paused.");
        } catch (e) {
          return catchErr("pause_investigation", e);
        }
      },
    },
    {
      label: "Search Knowledge",
      name: "alga_search_knowledge",
      description:
        "Search Alga's shared knowledge base for runbooks, known issues, service owner contacts, and facts. " +
        "Query by text, kind, or tag. Use this early in the investigation to find existing operational knowledge " +
        "that may explain the alert or provide remediation steps. Returns short previews with each note's id; " +
        "call alga_get_knowledge with an id to read the full body.",
      parameters: Type.Object({
        query: Type.Optional(Type.String({ description: "Free-text search query." })),
        kind: Type.Optional(
          Type.Unsafe({
            type: "string",
            enum: ["runbook", "known_issue", "service_owner", "fact"],
            description: "Filter by note kind.",
          }),
        ),
        tag: Type.Optional(Type.String({ description: "Filter by tag." })),
        limit: Type.Optional(Type.Number({ description: "Max results (default 10)." })),
      }),
      execute: async (_id, args) => {
        const p = args as { query?: string; kind?: string; tag?: string; limit?: number };
        try {
          const account = resolveAccount(cfg);
          const { agentSearchKnowledge } = await import("./agent-rest.js");
          const result = await agentSearchKnowledge(account.httpBase, account.token, {
            query: p.query,
            kind: p.kind,
            tag: p.tag,
            limit: p.limit,
          });
          const notes = result.notes as Array<{
            id?: string;
            title?: string;
            body_markdown?: string;
            kind?: string;
          }>;
          if (!notes.length) {
            return {
              content: [{ type: "text" as const, text: "No matching knowledge notes found." }],
            };
          }
          const lines = notes.map((n, i) => {
            const fullBody = n.body_markdown ?? "";
            const truncated = fullBody.length > 200;
            const preview = fullBody.slice(0, 200) + (truncated ? " …[truncated]" : "");
            return `${i + 1}. [${n.kind ?? "note"}] ${n.title ?? "untitled"}\nid: ${n.id ?? "?"}\n${preview}`;
          });
          return {
            content: [
              {
                type: "text" as const,
                text:
                  `Found ${notes.length} notes (previews truncated to 200 chars; ` +
                  `call alga_get_knowledge with an id for the full body):\n\n${lines.join("\n\n")}`,
              },
            ],
          };
        } catch (e) {
          return catchErr("search_knowledge", e);
        }
      },
    },
    {
      label: "Get Knowledge",
      name: "alga_get_knowledge",
      description:
        "Fetch the full body of a single knowledge note by its id. Use this after alga_search_knowledge when a note " +
        "looks relevant and you need the complete runbook or known-issue content (search only returns a 200-char preview).",
      parameters: Type.Object({
        id: Type.String({
          description: "Knowledge note id (UUID) returned by alga_search_knowledge.",
        }),
      }),
      execute: async (_id, args) => {
        const p = args as { id?: string };
        const noteId = (p.id ?? "").trim();
        if (!noteId) return errText("get_knowledge", "id is required");
        try {
          const account = resolveAccount(cfg);
          const { agentGetKnowledge } = await import("./agent-rest.js");
          const note = (await agentGetKnowledge(account.httpBase, account.token, noteId)) as {
            title?: string;
            kind?: string;
            body_markdown?: string;
          };
          return {
            content: [
              {
                type: "text" as const,
                text: `[${note.kind ?? "note"}] ${note.title ?? "untitled"}\n\n${note.body_markdown ?? ""}`,
              },
            ],
          };
        } catch (e) {
          return catchErr("get_knowledge", e);
        }
      },
    },
    {
      label: "Create Knowledge Note",
      name: "alga_create_knowledge",
      description:
        "Create a reusable knowledge note from investigation findings. Capture runbooks, known issues, " +
        "service owner information, or facts that future investigations can reference. " +
        "Always create a note when you discover something that could help diagnose or resolve similar issues.",
      parameters: Type.Object({
        kind: Type.Unsafe({
          type: "string",
          enum: ["runbook", "known_issue", "service_owner", "fact"],
          description:
            "Note kind: 'runbook' (step-by-step fix), 'known_issue' (recurring problem), " +
            "'service_owner' (team/contact info), 'fact' (general operational knowledge).",
        }),
        title: Type.String({ description: "Descriptive note title." }),
        body_markdown: Type.String({ description: "Note content in Markdown format." }),
        tags: Type.Optional(
          Type.Array(Type.String(), { description: "Tags for categorization and discovery." }),
        ),
        source_investigation_id: Type.Optional(
          Type.String({
            description: "Link to the investigation this note was derived from.",
          }),
        ),
        confidence: Type.Optional(
          Type.Number({ description: "Confidence score 0-1. How certain is this knowledge?" }),
        ),
      }),
      execute: async (_id, args) => {
        const p = args as {
          kind?: string;
          title?: string;
          body_markdown?: string;
          tags?: string[];
          source_investigation_id?: string;
          confidence?: number;
        };
        if (!p.title || !p.body_markdown) {
          return errText("create_knowledge", "title and body_markdown are required");
        }
        try {
          const account = resolveAccount(cfg);
          const { agentCreateKnowledge } = await import("./agent-rest.js");
          await agentCreateKnowledge(account.httpBase, account.token, {
            kind: p.kind ?? "fact",
            title: p.title,
            body_markdown: p.body_markdown,
            tags: p.tags,
            source_investigation_id: p.source_investigation_id,
            confidence: p.confidence,
          });
          return {
            content: [{ type: "text" as const, text: `Knowledge note "${p.title}" created.` }],
          };
        } catch (e) {
          return catchErr("create_knowledge", e);
        }
      },
    },
    {
      label: "List Alerts",
      name: "alga_list_alerts",
      description:
        "Query alerts from Alga. Use this to find related or correlated alerts beyond the current " +
        "investigation's linked alerts. Filter by status (firing/resolved/acknowledged), severity, or search text. " +
        "Useful for understanding the broader alert landscape during an investigation.",
      parameters: Type.Object({
        status: Type.Optional(
          Type.String({
            description: "Filter by status: 'firing', 'resolved', or 'acknowledged'.",
          }),
        ),
        severity: Type.Optional(
          Type.String({ description: "Filter by severity label (e.g. 'critical', 'warning')." }),
        ),
        search: Type.Optional(Type.String({ description: "Search term for alert labels/annotations." })),
        limit: Type.Optional(Type.Number({ description: "Max results (default 20)." })),
      }),
      execute: async (_id, args) => {
        const p = args as { status?: string; severity?: string; search?: string; limit?: number };
        try {
          const account = resolveAccount(cfg);
          const { agentGetAlerts } = await import("./agent-rest.js");
          const result = await agentGetAlerts(account.httpBase, account.token, {
            status: p.status,
            severity: p.severity,
            search: p.search,
            limit: p.limit ?? 20,
          });
          const alerts = result.alerts as Array<{
            fingerprint?: string;
            labels?: Record<string, string>;
            status?: string;
          }>;
          if (!alerts.length) {
            return { content: [{ type: "text" as const, text: "No alerts found." }] };
          }
          const lines = alerts.map(
            (a, i) =>
              `${i + 1}. ${a.labels?.alertname ?? a.fingerprint} [${a.status ?? "unknown"}]`,
          );
          return {
            content: [
              { type: "text" as const, text: `${alerts.length} alerts:\n${lines.join("\n")}` },
            ],
          };
        } catch (e) {
          return catchErr("list_alerts", e);
        }
      },
    },
    {
      label: "Triage Feedback",
      name: "alga_triage_feedback",
      description:
        "Provide feedback on an AI triage classification. If the triage system correctly classified the alert, " +
        "confirm with agreed=true. If it was wrong, set agreed=false and provide the correct decision and severity. " +
        "This improves future triage accuracy. Only use this when the investigation includes triage context.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        triage_result_id: Type.String({
          description: "ID of the triage result to provide feedback on.",
        }),
        agreed: Type.Boolean({
          description:
            "Whether you agree with the triage classification. true = confirmed, false = override.",
        }),
        correct_decision: Type.Optional(
          Type.String({
            description:
              "The correct triage decision if you disagree (e.g. 'investigate', 'ignore', 'escalate').",
          }),
        ),
        correct_severity: Type.Optional(
          Type.String({
            description:
              "The correct severity if you disagree with the original (e.g. 'critical', 'warning', 'info').",
          }),
        ),
        note: Type.Optional(
          Type.String({ description: "Optional explanation for the feedback." }),
        ),
      }),
      execute: async (_id, args) => {
        const p = args as {
          investigation_id?: string;
          triage_result_id?: string;
          agreed?: boolean;
          correct_decision?: string;
          correct_severity?: string;
          note?: string;
        };
        if (!p.investigation_id)
          return errText("triage_feedback", "missing investigation_id");
        if (!p.triage_result_id)
          return errText("triage_feedback", "missing triage_result_id");
        const cmd: AlgaInvestigationCommand = {
          op: "triage_feedback",
          triage_result_id: p.triage_result_id,
          agreed: p.agreed ?? true,
        };
        if (p.correct_decision) cmd.correct_decision = p.correct_decision;
        if (p.correct_severity) cmd.correct_severity = p.correct_severity;
        if (p.note) cmd.note = p.note;
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("triage_feedback", r.error ?? "unknown");
          return okText(
            "triage_feedback",
            p.investigation_id,
            p.agreed ? "Triage confirmed." : "Triage overridden.",
          );
        } catch (e) {
          return catchErr("triage_feedback", e);
        }
      },
    },
    {
      label: "Get Incident Context",
      name: "alga_get_incident_context",
      description:
        "Get incident context including status, severity, timeline, roles, and linked alerts/investigations. " +
        "Available to assigned incident roles and investigate-capable responders; use this for internal coordination context, not public status wording.",
      parameters: Type.Object({
        incident_id: Type.String({ description: "Incident ID (e.g. 42)." }),
      }),
      execute: async (_id, args) => {
        const p = args as { incident_id?: string };
        if (!p.incident_id) return errText("get_incident_context", "missing incident_id");
        try {
          const account = resolveAccount(cfg);
          const { agentGetIncident } = await import("./agent-rest.js");
          const data = await agentGetIncident(account.httpBase, account.token, p.incident_id);
          return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
        } catch (e) { return catchErr("get_incident_context", e); }
      },
    },
    {
      label: "Add Incident Timeline",
      name: "alga_add_incident_timeline",
      description:
        "Add a timeline entry to an incident (type: progress, finding, action, resolution, comment).",
      parameters: Type.Object({
        incident_id: Type.String({ description: "Incident ID." }),
        type: Type.String({ description: "Entry type: progress, finding, action, resolution, comment." }),
        message: Type.String({ description: "Timeline message content." }),
      }),
      execute: async (_id, args) => {
        const p = args as { incident_id?: string; type?: string; message?: string };
        if (!p.incident_id) return errText("add_incident_timeline", "missing incident_id");
        if (!p.type || !p.message) return errText("add_incident_timeline", "type and message required");
        try {
          const account = resolveAccount(cfg);
          const { agentAddIncidentTimeline } = await import("./agent-rest.js");
          await agentAddIncidentTimeline(account.httpBase, account.token, p.incident_id, {
            type: p.type, message: p.message,
          });
          return okText("add_incident_timeline", p.incident_id);
        } catch (e) { return catchErr("add_incident_timeline", e); }
      },
    },
    {
      label: "Get Incident Timeline",
      name: "alga_get_incident_timeline",
      description:
        "Get the timeline of events for an incident.",
      parameters: Type.Object({
        incident_id: Type.String({ description: "Incident ID (e.g. 42)." }),
      }),
      execute: async (_id, args) => {
        const p = args as { incident_id?: string };
        if (!p.incident_id) return errText("get_incident_timeline", "missing incident_id");
        try {
          const account = resolveAccount(cfg);
          const { agentGetIncidentTimeline } = await import("./agent-rest.js");
          const data = await agentGetIncidentTimeline(account.httpBase, account.token, p.incident_id);
          return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
        } catch (e) { return catchErr("get_incident_timeline", e); }
      },
    },
    {
      label: "Who Is On Call",
      name: "alga_who_is_on_call",
      description:
        "Get the current on-call person for each schedule.",
      parameters: Type.Object({}),
      execute: async (_id, _args) => {
        try {
          const account = resolveAccount(cfg);
          const { agentGetOnCall } = await import("./agent-rest.js");
          const data = await agentGetOnCall(account.httpBase, account.token);
          return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
        } catch (e) { return catchErr("who_is_on_call", e); }
      },
    },
    {
      label: "List Services",
      name: "alga_list_services",
      description:
        "List services in the service catalog with their current status.",
      parameters: Type.Object({}),
      execute: async (_id, _args) => {
        try {
          const account = resolveAccount(cfg);
          const { agentListServices } = await import("./agent-rest.js");
          const data = await agentListServices(account.httpBase, account.token);
          return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
        } catch (e) { return catchErr("list_services", e); }
      },
    },
    {
      label: "Search Memories",
      name: "alga_search_memories",
      description:
        "Search agent memories from past investigations. Useful for finding past solutions.",
      parameters: Type.Object({
        query: Type.Optional(Type.String({ description: "Search query." })),
        limit: Type.Optional(Type.Number({ description: "Max results (default 10)." })),
      }),
      execute: async (_id, args) => {
        const p = args as { query?: string; limit?: number };
        try {
          const account = resolveAccount(cfg);
          const { agentSearchMemories } = await import("./agent-rest.js");
          const data = await agentSearchMemories(account.httpBase, account.token, {
            query: p.query, limit: p.limit,
          });
          return { content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }] };
        } catch (e) { return catchErr("search_memories", e); }
      },
    },
    {
      label: "Create Memory",
      name: "alga_create_memory",
      description:
        "Create an agent memory for future investigations (useful findings, fixes, insights).",
      parameters: Type.Object({
        content: Type.String({ description: "Memory content to persist." }),
        kind: Type.Optional(Type.String({ description: "Memory kind (optional)." })),
        source_investigation_id: Type.Optional(Type.String({ description: "Source investigation ID." })),
      }),
      execute: async (_id, args) => {
        const p = args as { content?: string; kind?: string; source_investigation_id?: string };
        if (!p.content) return errText("create_memory", "missing content");
        try {
          const account = resolveAccount(cfg);
          const { agentCreateMemory } = await import("./agent-rest.js");
          await agentCreateMemory(account.httpBase, account.token, {
            content: p.content, kind: p.kind, source_investigation_id: p.source_investigation_id,
          });
          return okText("create_memory", "memory created");
        } catch (e) { return catchErr("create_memory", e); }
      },
    },
    {
      label: "Ask Peer Agent",
      name: "alga_peer_ask",
      description:
        "Ask another agent for help. Useful when you need expertise from a specialized agent.",
      parameters: Type.Object({
        target_agent_id: Type.String({ description: "Target agent ID to ask." }),
        question: Type.String({ description: "Question to ask the peer agent." }),
      }),
      execute: async (_id, args) => {
        const p = args as { target_agent_id?: string; question?: string };
        if (!p.target_agent_id) return errText("peer_ask", "missing target_agent_id");
        if (!p.question) return errText("peer_ask", "missing question");
        try {
          const account = resolveAccount(cfg);
          const { agentPeerAsk } = await import("./agent-rest.js");
          await agentPeerAsk(account.httpBase, account.token, {
            target_agent_id: p.target_agent_id, question: p.question,
          });
          return okText("peer_ask", p.target_agent_id);
        } catch (e) { return catchErr("peer_ask", e); }
      },
    },
    {
      label: "Assign Investigation",
      name: "alga_assign_investigation",
      description:
        "Reassign the current investigation to a different agent. Only the currently assigned agent can reassign. " +
        "Use when you determine a specialist agent is better suited for this investigation.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        target_agent_id: Type.String({ description: "Agent ID to reassign the investigation to." }),
      }),
      execute: async (_id, args) => {
        const p = args as { investigation_id?: string; target_agent_id?: string };
        if (!p.investigation_id) return errText("assign_investigation", "missing investigation_id");
        if (!p.target_agent_id) return errText("assign_investigation", "missing target_agent_id");
        const cmd: AlgaInvestigationCommand = { op: "assign_investigation", target_agent_id: p.target_agent_id };
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("assign_investigation", r.error ?? "unknown");
          return okText("assign_investigation", p.investigation_id, `Reassigned to agent ${p.target_agent_id}.`);
        } catch (e) { return catchErr("assign_investigation", e); }
      },
    },
    {
      label: "Set Incident Priority",
      name: "alga_set_incident_priority",
      description:
        "Set the priority (P1–P5) of an incident. P1 is the highest priority. " +
        "Can be used from an alert investigation chat if the alert was promoted to an incident, " +
        "or from an incident chat directly. Affects SLA targets.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        priority: Type.Unsafe({
          type: "string",
          enum: ["P1", "P2", "P3", "P4", "P5"],
          description: "Priority level. P1 = critical/highest, P5 = lowest.",
        }),
        incident_id: IncidentIdParam,
      }),
      execute: async (_id, args) => {
        const p = args as { investigation_id?: string; priority?: string; incident_id?: string };
        if (!p.investigation_id) return errText("set_incident_priority", "missing investigation_id");
        if (!p.priority) return errText("set_incident_priority", "missing priority");
        const cmd: AlgaInvestigationCommand = { op: "set_incident_priority", priority: p.priority };
        if (p.incident_id) cmd.incident_id = p.incident_id;
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("set_incident_priority", r.error ?? "unknown");
          return okText("set_incident_priority", p.investigation_id, `Priority set to ${p.priority}.`);
        } catch (e) { return catchErr("set_incident_priority", e); }
      },
    },
    {
      label: "Set Incident Severity",
      name: "alga_set_incident_severity",
      description:
        "Set the severity of an incident (critical, high, warning, info). " +
        "Can be used from an alert investigation chat if the alert was promoted to an incident.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        severity: Type.Unsafe({
          type: "string",
          enum: ["critical", "high", "warning", "info"],
          description: "New severity level.",
        }),
        incident_id: IncidentIdParam,
      }),
      execute: async (_id, args) => {
        const p = args as { investigation_id?: string; severity?: string; incident_id?: string };
        if (!p.investigation_id) return errText("set_incident_severity", "missing investigation_id");
        if (!p.severity) return errText("set_incident_severity", "missing severity");
        const cmd: AlgaInvestigationCommand = { op: "set_incident_severity", severity: p.severity as AlgaInvestigationSeverity };
        if (p.incident_id) cmd.incident_id = p.incident_id;
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("set_incident_severity", r.error ?? "unknown");
          return okText("set_incident_severity", p.investigation_id, `Severity set to ${p.severity}.`);
        } catch (e) { return catchErr("set_incident_severity", e); }
      },
    },
    {
      label: "Trigger Escalation",
      name: "alga_trigger_escalation",
      description:
        "Trigger an escalation for an incident. This notifies on-call responders and escalation contacts " +
        "that the incident requires urgent attention.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        incident_id: IncidentIdParam,
      }),
      execute: async (_id, args) => {
        const p = args as { investigation_id?: string; incident_id?: string };
        if (!p.investigation_id) return errText("trigger_escalation", "missing investigation_id");
        const cmd: AlgaInvestigationCommand = { op: "trigger_escalation" };
        if (p.incident_id) cmd.incident_id = p.incident_id;
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("trigger_escalation", r.error ?? "unknown");
          return okText("trigger_escalation", p.investigation_id, "Escalation triggered.");
        } catch (e) { return catchErr("trigger_escalation", e); }
      },
    },
    {
      label: "Request Status Update",
      name: "alga_request_status_update",
      description:
        "Request a status update from incident responders. Sends a notification asking for a progress " +
        "report on the incident.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        incident_id: IncidentIdParam,
      }),
      execute: async (_id, args) => {
        const p = args as { investigation_id?: string; incident_id?: string };
        if (!p.investigation_id) return errText("request_status_update", "missing investigation_id");
        const cmd: AlgaInvestigationCommand = { op: "request_status_update" };
        if (p.incident_id) cmd.incident_id = p.incident_id;
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("request_status_update", r.error ?? "unknown");
          return okText("request_status_update", p.investigation_id, "Status update requested.");
        } catch (e) { return catchErr("request_status_update", e); }
      },
    },
    {
      label: "Mitigate Incident",
      name: "alga_mitigate_incident",
      description:
        "Mark an incident as mitigated. Use when the immediate impact has been contained but " +
        "the root cause may not yet be fully resolved.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        reason: Type.Optional(Type.String({ description: "Reason or description of the mitigation action taken." })),
        incident_id: IncidentIdParam,
      }),
      execute: async (_id, args) => {
        const p = args as { investigation_id?: string; reason?: string; incident_id?: string };
        if (!p.investigation_id) return errText("mitigate_incident", "missing investigation_id");
        const cmd: AlgaInvestigationCommand = { op: "mitigate_incident" };
        if (p.reason) cmd.reason = p.reason;
        if (p.incident_id) cmd.incident_id = p.incident_id;
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("mitigate_incident", r.error ?? "unknown");
          return okText("mitigate_incident", p.investigation_id, "Incident mitigated.");
        } catch (e) { return catchErr("mitigate_incident", e); }
      },
    },
    {
      label: "Resolve Incident",
      name: "alga_resolve_incident",
      description:
        "Resolve an incident. Use when the root cause has been addressed and the incident is fully closed.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        reason: Type.Optional(Type.String({ description: "Resolution description or post-fix summary." })),
        incident_id: IncidentIdParam,
      }),
      execute: async (_id, args) => {
        const p = args as { investigation_id?: string; reason?: string; incident_id?: string };
        if (!p.investigation_id) return errText("resolve_incident", "missing investigation_id");
        const cmd: AlgaInvestigationCommand = { op: "resolve_incident" };
        if (p.reason) cmd.reason = p.reason;
        if (p.incident_id) cmd.incident_id = p.incident_id;
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("resolve_incident", r.error ?? "unknown");
          return okText("resolve_incident", p.investigation_id, "Incident resolved.");
        } catch (e) { return catchErr("resolve_incident", e); }
      },
    },
    {
      label: "Begin Triage",
      name: "alga_begin_triage",
      description:
        "Move an incident from the detected state into active triaging. " +
        "Use this when you begin the structured triage process for an incident.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        incident_id: IncidentIdParam,
      }),
      execute: async (_id, args) => {
        const p = args as { investigation_id?: string; incident_id?: string };
        if (!p.investigation_id) return errText("begin_triage", "missing investigation_id");
        const cmd: AlgaInvestigationCommand = { op: "begin_triage" };
        if (p.incident_id) cmd.incident_id = p.incident_id;
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("begin_triage", r.error ?? "unknown");
          return okText("begin_triage", p.investigation_id, "Triage started.");
        } catch (e) { return catchErr("begin_triage", e); }
      },
    },
    {
      label: "Promote Incident",
      name: "alga_promote_incident",
      description:
        "Promote an incident to the next lifecycle stage (e.g. from triaging to active). " +
        "Use this to advance the incident through the standard progression.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        incident_id: IncidentIdParam,
      }),
      execute: async (_id, args) => {
        const p = args as { investigation_id?: string; incident_id?: string };
        if (!p.investigation_id) return errText("promote_incident", "missing investigation_id");
        const cmd: AlgaInvestigationCommand = { op: "promote_incident" };
        if (p.incident_id) cmd.incident_id = p.incident_id;
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("promote_incident", r.error ?? "unknown");
          return okText("promote_incident", p.investigation_id, "Incident promoted.");
        } catch (e) { return catchErr("promote_incident", e); }
      },
    },
    {
      label: "Assign Incident Role",
      name: "alga_assign_incident_role",
      description:
        "Assign an ICS (Incident Command System) role to a user or agent on an incident. " +
        "Role types include: incident_commander, operations_lead, communications_lead, scribe, and others. " +
        "Provide either user_id or agent_token_id, not both.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        role_type: Type.String({ description: "ICS role type (e.g. incident_commander, operations_lead, scribe)." }),
        user_id: Type.Optional(Type.String({ description: "User ID to assign to the role." })),
        agent_token_id: Type.Optional(Type.String({ description: "Agent token ID to assign to the role." })),
        scope_description: Type.Optional(Type.String({ description: "Optional scope or context for this role assignment." })),
        incident_id: IncidentIdParam,
      }),
      execute: async (_id, args) => {
        const p = args as {
          investigation_id?: string;
          role_type?: string;
          user_id?: string;
          agent_token_id?: string;
          scope_description?: string;
          incident_id?: string;
        };
        if (!p.investigation_id) return errText("assign_incident_role", "missing investigation_id");
        if (!p.role_type) return errText("assign_incident_role", "missing role_type");
        if (!p.user_id && !p.agent_token_id) return errText("assign_incident_role", "provide user_id or agent_token_id");
        const cmd: AlgaInvestigationCommand = { op: "assign_incident_role", role_type: p.role_type };
        if (p.user_id) cmd.user_id = p.user_id;
        if (p.agent_token_id) cmd.agent_token_id = p.agent_token_id;
        if (p.scope_description) cmd.scope_description = p.scope_description;
        if (p.incident_id) cmd.incident_id = p.incident_id;
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("assign_incident_role", r.error ?? "unknown");
          return okText("assign_incident_role", p.investigation_id, `Role ${p.role_type} assigned.`);
        } catch (e) { return catchErr("assign_incident_role", e); }
      },
    },
    {
      label: "Post Handoff",
      name: "alga_post_handoff",
      description:
        "Commander-facing coordination tool for the FINAL handoff after all recovery and verification work is complete. " +
        "Set audience='commander' or audience='command' when commander review, approval, or a decision is needed. " +
        "WARNING: calling this tool ACTIVATES other agents (commander, communicator) by forwarding the message to them — " +
        "every call wakes up teammate agents and can interrupt their current work, causing ping-pong loops. " +
        "Do NOT call this tool during investigation, identification, mitigation, or verification phases. " +
        "For status milestones during active work (identified, monitoring, resolved), use alga_publish_status_update instead — " +
        "it does NOT activate other agents and is the only path that creates a Status Updates card entry. " +
        "Do NOT use this tool to publish milestone updates, post investigation findings, send progress notes, or share interim summaries; " +
        "those belong in alga_publish_status_update (for status) or the alert investigation thread (for technical findings). " +
        "Reserve this tool for the single structured commander handoff that happens AFTER recovery is verified AND a " +
        "status_level='monitoring' update has already been published via alga_publish_status_update.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        message: Type.String({ description: "Short commander-facing coordination update, decision request, summary, or handoff." }),
        audience: Type.Optional(
          Type.Unsafe({
            type: "string",
            enum: ["none", "commander", "communicator", "command"],
            description: "Role intent for backend mention resolution.",
          }),
        ),
        urgency: Type.Optional(
          Type.Unsafe({
            type: "string",
            enum: ["info", "needs_attention", "decision_needed"],
            description: "How loud the handoff should ring for the audience.",
          }),
        ),
        incident_id: IncidentIdParam,
      }),
      execute: async (_id, args) => {
        const p = args as {
          investigation_id?: string;
          message?: string;
          audience?: "none" | "commander" | "communicator" | "command";
          urgency?: "info" | "needs_attention" | "decision_needed";
          incident_id?: string;
        };
        if (!p.investigation_id) return errText("post_handoff", "missing investigation_id");
        if (!p.message) return errText("post_handoff", "missing message");
        const cmd: AlgaInvestigationCommand = {
          op: "post_handoff",
          message: p.message,
        };
        if (p.audience) cmd.audience = p.audience;
        if (p.urgency) cmd.urgency = p.urgency;
        if (p.incident_id) cmd.incident_id = p.incident_id;
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("post_handoff", r.error ?? "unknown");
          return okText("post_handoff", p.investigation_id, "Handoff posted.");
        } catch (e) { return catchErr("post_handoff", e); }
      },
    },
    {
      label: "Publish Status Update",
      name: "alga_publish_status_update",
      description:
        "Publish a single status-level milestone (identified, monitoring, resolved, ...) on the incident. " +
        "This is the ONLY path that creates a Status Updates card entry and does NOT activate teammate agents. " +
        "Use this for every status milestone during active work; reserve alga_post_handoff for the final structured " +
        "commander handoff after recovery is verified. source_coordination_message_id can pin this update to an " +
        "existing coordination message.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        status_level: Type.String({
          description: "Status level for this milestone: investigating, identified, mitigated, monitoring, resolved.",
        }),
        message: Type.String({ description: "Short public-facing status text for this milestone." }),
        source_coordination_message_id: Type.Optional(
          Type.String({ description: "Optional coordination message id this update is anchored to." }),
        ),
        incident_id: IncidentIdParam,
      }),
      execute: async (_id, args) => {
        const p = args as {
          investigation_id?: string;
          status_level?: string;
          message?: string;
          source_coordination_message_id?: string;
          incident_id?: string;
        };
        if (!p.investigation_id) return errText("publish_status_update", "missing investigation_id");
        if (!p.status_level) return errText("publish_status_update", "missing status_level");
        if (!p.message) return errText("publish_status_update", "missing message");
        const cmd: AlgaInvestigationCommand = {
          op: "publish_status_update",
          status_level: p.status_level as AlgaInvestigationCommand["status_level"],
          message: p.message,
        };
        if (p.source_coordination_message_id) cmd.source_coordination_message_id = p.source_coordination_message_id;
        if (p.incident_id) cmd.incident_id = p.incident_id;
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("publish_status_update", r.error ?? "unknown");
          return okText("publish_status_update", p.investigation_id, `Status set to ${p.status_level}.`);
        } catch (e) { return catchErr("publish_status_update", e); }
      },
    },
    {
      label: "Set Incident Resolution Docs",
      name: "alga_set_incident_resolution_docs",
      description:
        "Attach structured resolution documents (root cause, impact, actions taken, postmortem) to an incident " +
        "as part of the resolution flow. The backend requires both a root_cause and a resolution section before " +
        "the incident can be marked resolved.",
      parameters: Type.Object({
        investigation_id: InvestigationIdParam,
        summary: Type.String({ description: "Short summary of the incident outcome." }),
        impact_assessment: Type.String({ description: "Impact assessment: who/what was affected and for how long." }),
        actions_taken: Type.String({ description: "What was done to mitigate and resolve the incident." }),
        root_cause: Type.Optional(Type.String({ description: "Root cause; if omitted the existing value is kept." })),
        resolution: Type.Optional(Type.String({ description: "Resolution narrative; if omitted the existing value is kept." })),
        incident_id: IncidentIdParam,
      }),
      execute: async (_id, args) => {
        const p = args as {
          investigation_id?: string;
          summary?: string;
          impact_assessment?: string;
          actions_taken?: string;
          root_cause?: string;
          resolution?: string;
          incident_id?: string;
        };
        if (!p.investigation_id) return errText("set_incident_resolution_docs", "missing investigation_id");
        if (!p.summary) return errText("set_incident_resolution_docs", "missing summary");
        if (!p.impact_assessment) return errText("set_incident_resolution_docs", "missing impact_assessment");
        if (!p.actions_taken) return errText("set_incident_resolution_docs", "missing actions_taken");
        const cmd: AlgaInvestigationCommand = {
          op: "set_incident_resolution_docs",
          summary: p.summary,
          impact_assessment: p.impact_assessment,
          actions_taken: p.actions_taken,
        };
        if (p.root_cause) cmd.root_cause = p.root_cause;
        if (p.resolution) cmd.resolution = p.resolution;
        if (p.incident_id) cmd.incident_id = p.incident_id;
        try {
          const r = await execInvTool(cfg, p.investigation_id, cmd);
          if (r.ok === false) return errText("set_incident_resolution_docs", r.error ?? "unknown");
          return okText("set_incident_resolution_docs", p.investigation_id, "Resolution docs recorded.");
        } catch (e) { return catchErr("set_incident_resolution_docs", e); }
      },
    },
  ];
  return tools;
}

type AlgaToolResult = { content: [{ type: "text"; text: string }] };

function resolveAccount(cfg?: CoreConfig): ResolvedAlgaAccount {
  const account = resolveAlgaAccount({ cfg: cfg ?? ({} as CoreConfig), accountId: null });
  if (!account.configured) {
    throw new Error(
      "Alga is not configured (set ALGA_SERVER_URL and ALGA_AGENT_TOKEN, or channels.alga config).",
    );
  }
  return account;
}

/**
 * Resolve the Alga chat_id for an inv_tool command. The backend addresses
 * alert investigations as `alert_<number>`, incident investigation threads as
 * `incident_coord_<number>` / `incident_inv_<number>`, and the operator DM as
 * a fixed value. If `investigationId` is already a valid chat id it is used
 * verbatim; a bare number is treated as an alert number; incident ops without
 * an explicit incident chat id fall back to `incident_coord_<incidentId>`.
 */
function resolveInvToolChatId(
  investigationId: string,
  cmd: AlgaInvestigationCommand,
): string {
  const id = investigationId.trim();
  if (cmd.incident_id && !/^(alert_|incident_)/.test(id)) {
    const inc = String(cmd.incident_id).trim();
    return /^(incident_coord_|incident_inv_)/.test(inc) ? inc : `incident_coord_${inc}`;
  }
  if (/^(alert_|incident_coord_|incident_inv_|alga_dm)/i.test(id)) {
    return id;
  }
  if (INCIDENT_TOOL_OPS.has(cmd.op)) {
    return `incident_coord_${id}`;
  }
  return `alert_${id}`;
}

async function execInvTool(
  cfg: CoreConfig | undefined,
  investigationId: string,
  cmd: AlgaInvestigationCommand,
): Promise<AgentPostMessageResult> {
  const account = resolveAccount(cfg);
  const chatId = resolveInvToolChatId(investigationId, cmd);
  return agentPostMessage(account.httpBase, account.token, chatId, {
    kind: "inv_tool",
    command: cmd,
  });
}

function errText(op: string, msg: string): AlgaToolResult {
  return { content: [{ type: "text", text: `alga ${op} failed: ${msg}` }] };
}

function okText(op: string, id: string, msg?: string): AlgaToolResult {
  const detail = msg ? ` ${msg}` : "";
  return {
    content: [{ type: "text", text: `alga ${op} succeeded for ${id}.${detail}` }],
  };
}

function catchErr(op: string, err: unknown): AlgaToolResult {
  const text = err instanceof Error ? err.message : String(err);
  return { content: [{ type: "text", text: `alga ${op} error: ${text}` }] };
}
