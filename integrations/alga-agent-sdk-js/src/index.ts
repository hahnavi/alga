export { AlgaError, AlgaAuthError, AlgaAPIError, AlgaConnectionError } from "./errors.js";
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
  requestStatusUpdate,
  mitigateIncident,
  resolveIncident,
  beginTriage,
  promoteIncident,
  assignIncidentRole,
} from "./commands.js";
export type { InvestigationCommand } from "./commands.js";
export {
  SSEClient,
} from "./sse.js";
export { AlgaClient } from "./client.js";
export type { AlgaClientOptions } from "./client.js";
export type {
  Alert,
  AlertEvent,
  DeliveryTarget,
  CorrelatedAlert,
  InvestigationResult,
  InvestigationUpdate,
  Investigation,
  KnowledgeNote,
  Memory,
  PeerAsk,
  Service,
  Incident,
  ConnectedEvent,
  MessageEvent,
  TypingEvent,
  InvestigationSignalEvent,
  PeerFindingEvent,
  PeerAskEvent,
  PeerReplyEvent,
  AgentPresenceEvent,
  AlertListResponse,
  InvestigationListResponse,
  KnowledgeListResponse,
  MemoryListResponse,
  PeerAskListResponse,
  SendMessageResponse,
  CommandResponse,
} from "./models.js";
