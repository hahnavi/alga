export { AlgaError, AlgaAuthError, AlgaAPIError, AlgaConnectionError, isAuthError, isRetryableError } from "./errors.js";
export { MessageDedup } from "./dedup.js";
export {
  // alert investigation
  resolveAlert,
  reopenAlert,
  setOutcome,
  cancelInvestigation,
  pauseInvestigation,
  triageFeedback,
  assignInvestigation,
  promoteToIncident,
  // incident
  setIncidentPriority,
  setIncidentSeverity,
  triggerEscalation,
  mitigateIncident,
  resolveIncident,
  beginTriage,
  promoteIncident,
  assignIncidentRoleToUser,
  assignIncidentRoleToAgent,
  // coordination / status
  postHandoff,
  publishStatusUpdate,
  setIncidentResolutionDocs,
  // coordination tasks
  dispatchTask,
  dispatchTaskToAgent,
  claimTask,
  completeTask,
  synthesizeFindings,
  TASK_KIND_INVESTIGATE,
  TASK_KIND_COMMUNICATE,
  TASK_KIND_VERIFY,
  TASK_KIND_MITIGATE,
} from "./commands.js";
export type { InvestigationCommand } from "./commands.js";
export { SSEClient, parseRetryAfterMs } from "./sse.js";
export type { ErrHandler, SSEOptions } from "./sse.js";
export { AlgaClient } from "./client.js";
export type { AlgaClientOptions, CoordinationTaskListResponse } from "./client.js";
export type {
  Alert,
  AlertEvent,
  DeliveryTarget,
  CoordinationTask,
  KnowledgeNote,
  Memory,
  PeerAsk,
  Service,
  Incident,
  IncidentRole,
  IncidentContext,
  OnCallEntry,
  SecretValue,
  Playbook,
  PlaybookStep,
  Capability,
  ConnectedEvent,
  MessageEvent,
  TypingEvent,
  InvestigationSignalEvent,
  PeerFindingEvent,
  PeerAskEvent,
  PeerReplyEvent,
  AgentPresenceEvent,
  CoordinationTaskEvent,
  SummarizeIncidentEvent,
  AlertAutoResolvedEvent,
  IncidentCommsStaleEvent,
  AlertListResponse,
  KnowledgeListResponse,
  MemoryListResponse,
  PeerAskListResponse,
  ServiceListResponse,
  SendMessageResponse,
  CommandResponse,
} from "./models.js";
