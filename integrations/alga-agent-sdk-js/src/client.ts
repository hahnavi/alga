import { AlgaAuthError, AlgaAPIError, AlgaConnectionError, isRetryableError } from "./errors.js";
import { MessageDedup } from "./dedup.js";
import { SSEClient, parseRetryAfterMs } from "./sse.js";
import type {
  Alert,
  AlertListResponse,
  KnowledgeNote,
  KnowledgeListResponse,
  Memory,
  MemoryListResponse,
  PeerAsk,
  PeerAskListResponse,
  SendMessageResponse,
  CommandResponse,
  ServiceListResponse,
  Incident,
  IncidentContext,
  OnCallEntry,
  SecretValue,
  Playbook,
  ConnectedEvent,
  MessageEvent,
  TypingEvent,
  InvestigationSignalEvent,
  PeerFindingEvent,
  PeerAskEvent,
  PeerReplyEvent,
  CoordinationTaskEvent,
  SummarizeIncidentEvent,
  AlertAutoResolvedEvent,
  IncidentCommsStaleEvent,
} from "./models.js";
import type { InvestigationCommand } from "./commands.js";

type Callback<T> = (data: T) => void;

export interface AlgaClientOptions {
  heartbeatIntervalMs?: number;
  dedup?: MessageDedup;
  userAgent?: string;
  fetchImpl?: typeof fetch;
  // maxRestRetries is the max number of retry attempts for transient REST
  // failures (429, 500, 502, 503, 504, network). 0 disables retries.
  // Negative is invalid and treated as 0. Default 2.
  maxRestRetries?: number;
}

const AGENT_MESSAGES_PATH = "/api/v1/agent/messages";
const IDEMPOTENCY_KEY_HEADER = "Idempotency-Key";
const MAX_RESPONSE_BYTES = 8 * 1024 * 1024;
const MAX_ERROR_MESSAGE_BYTES = 4 * 1024;

export class AlgaClient {
  private serverUrl: string;
  private token: string;
  private dedup: MessageDedup;
  private userAgent: string;
  private fetchImpl: typeof fetch;
  private maxRestRetries: number;
  private heartbeatIntervalMs: number;
  private sse: SSEClient | null = null;
  private errHandler: ((err: Error) => void) | null = null;

  onConnected: Callback<ConnectedEvent> | null = null;
  onMessage: Callback<MessageEvent> | null = null;
  onTyping: Callback<TypingEvent> | null = null;
  onInvestigationResume: Callback<InvestigationSignalEvent> | null = null;
  onPeerFinding: Callback<PeerFindingEvent> | null = null;
  onPeerAsk: Callback<PeerAskEvent> | null = null;
  onPeerReply: Callback<PeerReplyEvent> | null = null;
  onCoordinationTask: Callback<CoordinationTaskEvent> | null = null;
  onSummarizeIncident: Callback<SummarizeIncidentEvent> | null = null;
  onAlertAutoResolved: Callback<AlertAutoResolvedEvent> | null = null;
  onIncidentCommsStale: Callback<IncidentCommsStaleEvent> | null = null;
  onUnknownEvent: ((eventType: string, data: string) => void) | null = null;

  constructor(serverUrl: string, token: string, options?: AlgaClientOptions) {
    this.serverUrl = serverUrl.replace(/\/+$/, "");
    this.token = token;
    this.dedup = options?.dedup ?? new MessageDedup();
    this.userAgent = options?.userAgent ?? "alga-agent-sdk-js";
    this.fetchImpl = options?.fetchImpl ?? fetch;
    this.maxRestRetries = Math.max(0, options?.maxRestRetries ?? 2);
    this.heartbeatIntervalMs = options?.heartbeatIntervalMs ?? 30_000;
  }

  // onErr registers a handler invoked once with a terminal error (auth
  // failure) after the SSE + heartbeat loops have stopped. The caller must
  // obtain a valid token and call connect() again to resume.
  onErr(handler: (err: Error) => void): void {
    this.errHandler = handler;
  }

  connect(): void {
    this.sse = new SSEClient(this.serverUrl, this.token, this.dedup, this.heartbeatIntervalMs, {
      userAgent: this.userAgent,
      fetchImpl: this.fetchImpl,
    });

    this.sse.onConnected = (data) => this.onConnected?.(data);
    this.sse.onMessage = (data) => this.onMessage?.(data);
    this.sse.onTyping = (data) => this.onTyping?.(data);
    this.sse.onInvestigationResume = (data) => this.onInvestigationResume?.(data);
    this.sse.onPeerFinding = (data) => this.onPeerFinding?.(data);
    this.sse.onPeerAsk = (data) => this.onPeerAsk?.(data);
    this.sse.onPeerReply = (data) => this.onPeerReply?.(data);
    this.sse.onCoordinationTask = (data) => this.onCoordinationTask?.(data);
    this.sse.onSummarizeIncident = (data) => this.onSummarizeIncident?.(data);
    this.sse.onAlertAutoResolved = (data) => this.onAlertAutoResolved?.(data);
    this.sse.onIncidentCommsStale = (data) => this.onIncidentCommsStale?.(data);
    this.sse.onUnknownEvent = (eventType, data) => this.onUnknownEvent?.(eventType, data);

    if (this.errHandler) this.sse.setErrHandler(this.errHandler);
    this.sse.start();
  }

  disconnect(): void {
    this.sse?.stop();
    this.sse = null;
  }

  // --- REST: Alerts ---

  async listAlerts(params?: Record<string, string>): Promise<AlertListResponse> {
    return this._get("/api/v1/agent/alerts", params);
  }

  async getAlert(fingerprint: string): Promise<Alert> {
    return this._get(`/api/v1/agent/alerts/${encodeURIComponent(fingerprint)}`);
  }

  async resolveAlert(fingerprint: string): Promise<void> {
    await this._post(`/api/v1/agent/alerts/${encodeURIComponent(fingerprint)}/resolve`);
  }

  async reopenAlert(fingerprint: string): Promise<void> {
    await this._post(`/api/v1/agent/alerts/${encodeURIComponent(fingerprint)}/reopen`);
  }

  // --- REST: Incidents ---

  // listIncidentTasks returns the coordination tasks for an incident.
  async listIncidentTasks(
    incidentNumber: number,
    params?: Record<string, string>,
  ): Promise<CoordinationTaskListResponse> {
    return this._get(`/api/v1/agent/incidents/${incidentNumber}/tasks`, params);
  }

  async getIncident(incidentNumber: number): Promise<IncidentContext> {
    return this._get(`/api/v1/agent/incidents/${incidentNumber}`);
  }

  async getIncidentTimeline(incidentNumber: number): Promise<Record<string, unknown>[]> {
    return this._get(`/api/v1/agent/incidents/${incidentNumber}/timeline`);
  }

  async addIncidentTimeline(incidentNumber: number, message: string, eventType: string): Promise<void> {
    await this._post(`/api/v1/agent/incidents/${incidentNumber}/timeline`, { message, event_type: eventType });
  }

  async updateIncidentSummary(incidentNumber: number, summary: string): Promise<Incident> {
    return this._patch(`/api/v1/agent/incidents/${incidentNumber}`, { summary });
  }

  // --- REST: Messages ---

  // sendMessage sends a text message. An Idempotency-Key is auto-generated so
  // retries are replay-safe.
  async sendMessage(chatId: string, text: string, mentions?: string[]): Promise<SendMessageResponse> {
    return this.sendMessageWithKey(chatId, text, mentions);
  }

  // sendMessageWithKey is the explicit-key variant for callers that drive
  // their own outbox.
  async sendMessageWithKey(
    chatId: string,
    text: string,
    mentions?: string[],
    idempotencyKey?: string,
  ): Promise<SendMessageResponse> {
    const body: Record<string, unknown> = { chat_id: chatId, kind: "text", text };
    if (mentions) body.mentions = mentions;
    return this._postIdem(AGENT_MESSAGES_PATH, body, idempotencyKey);
  }

  // sendCommand sends a kind=inv_tool command. The SDK auto-injects an
  // Idempotency-Key so a retry of the same logical command is replayed from
  // the backend cache rather than re-executed.
  async sendCommand(chatId: string, command: InvestigationCommand): Promise<CommandResponse> {
    return this.sendCommandWithKey(chatId, command);
  }

  async sendCommandWithKey(
    chatId: string,
    command: InvestigationCommand,
    idempotencyKey?: string,
  ): Promise<CommandResponse> {
    return this._postIdem(
      AGENT_MESSAGES_PATH,
      { chat_id: chatId, kind: "inv_tool", command },
      idempotencyKey,
    );
  }

  // sendIncidentSummary posts a kind=incident_summary message into the
  // incident coordination thread.
  async sendIncidentSummary(incidentNumber: number, text: string): Promise<void> {
    await this._post(AGENT_MESSAGES_PATH, {
      chat_id: `incident_coord_${incidentNumber}`,
      kind: "incident_summary",
      text,
    });
  }

  // sendDraft streams a partial ("draft") message into a chat.
  async sendDraft(chatId: string, draftId: string, text: string): Promise<void> {
    await this._post("/api/v1/agent/drafts", { chat_id: chatId, draft_id: draftId, text });
  }

  async editMessage(messageId: string, chatId: string, text: string): Promise<void> {
    await this._put(`/api/v1/agent/messages/${encodeURIComponent(messageId)}`, {
      chat_id: chatId,
      kind: "text",
      text,
    });
  }

  async deleteMessage(messageId: string, chatId: string): Promise<void> {
    await this._delete(`/api/v1/agent/messages/${encodeURIComponent(messageId)}`, { chat_id: chatId });
  }

  async sendTyping(chatId: string, active = true): Promise<void> {
    await this._post("/api/v1/agent/typing", { chat_id: chatId, active });
  }

  async sendHeartbeat(): Promise<void> {
    await this._post("/api/v1/agent/heartbeat");
  }

  // --- REST: Knowledge ---

  async listKnowledge(params?: Record<string, string>): Promise<KnowledgeListResponse> {
    return this._get("/api/v1/agent/knowledge", params);
  }

  async getKnowledge(id: string): Promise<KnowledgeNote> {
    return this._get(`/api/v1/agent/knowledge/${encodeURIComponent(id)}`);
  }

  async createKnowledge(params: Record<string, unknown>): Promise<KnowledgeNote> {
    return this._post("/api/v1/agent/knowledge", params);
  }

  // --- REST: Memories ---

  async listMemories(params?: Record<string, string>): Promise<MemoryListResponse> {
    return this._get("/api/v1/agent/memories", params);
  }

  async createMemory(params: Record<string, unknown>): Promise<Memory> {
    return this._post("/api/v1/agent/memories", params);
  }

  async getMemory(id: string): Promise<Memory> {
    return this._get(`/api/v1/agent/memories/${encodeURIComponent(id)}`);
  }

  async deleteMemory(id: string): Promise<void> {
    await this._delete(`/api/v1/agent/memories/${encodeURIComponent(id)}`);
  }

  // --- REST: Peer Ask ---

  async listPeerAsks(params?: Record<string, string>): Promise<PeerAskListResponse> {
    return this._get("/api/v1/agent/peer-ask", params);
  }

  async createPeerAsk(params: Record<string, unknown>): Promise<PeerAsk> {
    return this._post("/api/v1/agent/peer-ask", params);
  }

  async getPeerAsk(id: string): Promise<PeerAsk> {
    return this._get(`/api/v1/agent/peer-ask/${encodeURIComponent(id)}`);
  }

  async replyPeerAsk(id: string, reply: string): Promise<PeerAsk> {
    return this._post(`/api/v1/agent/peer-ask/${encodeURIComponent(id)}/reply`, { reply });
  }

  async cancelPeerAsk(id: string): Promise<void> {
    await this._post(`/api/v1/agent/peer-ask/${encodeURIComponent(id)}/cancel`);
  }

  // --- REST: Reference data ---

  async listServices(params?: Record<string, string>): Promise<ServiceListResponse> {
    return this._get("/api/v1/agent/services", params);
  }

  async whoIsOnCall(): Promise<OnCallEntry[]> {
    return this._get("/api/v1/agent/on-call/current");
  }

  async getPlaybooks(alertFingerprint: string): Promise<Playbook[]> {
    return this._get("/api/v1/agent/playbooks", { alert_fingerprint: alertFingerprint });
  }

  // getSecret fetches an allow-listed shared secret value. Not-found and
  // not-allow-listed both surface as 404 by backend design.
  async getSecret(secretId: string): Promise<SecretValue> {
    return this._get(`/api/v1/agent/secrets/${encodeURIComponent(secretId)}`);
  }

  // --- HTTP plumbing ---

  private _get(path: string, params?: Record<string, string>): Promise<any> {
    return this._doJson("GET", withQuery(path, params), undefined, "");
  }

  private _post(path: string, body?: unknown): Promise<any> {
    return this._doJson("POST", path, body, "");
  }

  private _put(path: string, body?: unknown): Promise<any> {
    return this._doJson("PUT", path, body, "");
  }

  private _patch(path: string, body?: unknown): Promise<any> {
    return this._doJson("PATCH", path, body, "");
  }

  private _delete(path: string, body?: unknown): Promise<any> {
    return this._doJson("DELETE", path, body, "");
  }

  // _postIdem is POST with an auto-generated Idempotency-Key on
  // /api/v1/agent/messages — the only path the backend honors the key on.
  private _postIdem(path: string, body?: unknown, idempotencyKey?: string): Promise<any> {
    return this._doJson("POST", path, body, idempotencyKey ?? "");
  }

  // _doJson performs a JSON REST call with retry on transient errors.
  // Mutations on /api/v1/agent/messages get an auto-injected Idempotency-Key
  // making retries replay-safe; other mutations are performed exactly once.
  private async _doJson(
    method: string,
    path: string,
    body: unknown,
    idempotencyKey: string,
  ): Promise<any> {
    const mutating = method !== "GET" && method !== "HEAD";
    let bodyData: string | undefined;
    if (body !== undefined) {
      bodyData = JSON.stringify(body);
    }

    if (mutating && !idempotencyKey && path === AGENT_MESSAGES_PATH) {
      idempotencyKey = newIdempotencyKey();
    }

    let attempts = this.maxRestRetries;
    if (mutating && !idempotencyKey) {
      // Non-replay-safe mutation: execute exactly once.
      attempts = 0;
    }

    let lastErr: Error | null = null;
    for (let attempt = 0; attempt <= attempts; attempt++) {
      const res = await this.rawRequest(method, path, bodyData, idempotencyKey);
      if (res instanceof Error) {
        lastErr = res;
        if (!isRetryableError(res) || attempt === attempts) throw res;
        await this.sleep(backoffMs(attempt, 0));
        continue;
      }

      if (res.status === 401 || res.status === 403) {
        throw new AlgaAuthError(res.status, truncate(res.body, MAX_ERROR_MESSAGE_BYTES));
      }

      if (res.status >= 400) {
        const apiErr = new AlgaAPIError(
          res.status,
          truncate(res.body, MAX_ERROR_MESSAGE_BYTES),
          parseRetryAfterMs(res.retryAfter),
        );
        if (!apiErr.isRetryable() || attempt === attempts) throw apiErr;
        lastErr = apiErr;
        await this.sleep(backoffMs(attempt, apiErr.retryAfterMs));
        continue;
      }

      if (res.body.length === 0) return undefined;
      return unwrapEnvelope(res.body);
    }
    throw lastErr ?? new AlgaConnectionError("exhausted retries");
  }

  private async rawRequest(
    method: string,
    path: string,
    bodyData: string | undefined,
    idempotencyKey: string,
  ): Promise<{ status: number; body: string; retryAfter: string | null } | Error> {
    const url = `${this.serverUrl}${path}`;
    const headers: Record<string, string> = {
      Authorization: `Bearer ${this.token}`,
      "User-Agent": this.userAgent,
    };
    if (bodyData !== undefined) headers["Content-Type"] = "application/json";
    if (idempotencyKey) headers[IDEMPOTENCY_KEY_HEADER] = idempotencyKey;

    let res: Response;
    try {
      res = await this.fetchImpl(url, {
        method,
        headers,
        body: bodyData,
      });
    } catch (err) {
      return new AlgaConnectionError(`request failed: ${(err as Error).message}`);
    }

    const text = await res.text().catch(() => "");
    const truncated = text.length > MAX_RESPONSE_BYTES ? text.slice(0, MAX_RESPONSE_BYTES) : text;
    return { status: res.status, body: truncated, retryAfter: res.headers.get("Retry-After") };
  }

  private sleep(ms: number): Promise<void> {
    if (ms <= 0) return Promise.resolve();
    return new Promise((resolve) => setTimeout(resolve, ms));
  }
}

// --- helpers ---

// withQuery appends url-encoded params to path. Empty values are skipped.
function withQuery(path: string, params?: Record<string, string>): string {
  if (!params) return path;
  const sp = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") sp.set(k, v);
  }
  const str = sp.toString();
  return str ? `${path}?${str}` : path;
}

// unwrapEnvelope decodes a backend response, unwrapping the standard
// {"data": ...} success envelope when present. Some endpoints write flat
// bodies; those fall through to a plain parse.
function unwrapEnvelope(body: string): unknown {
  try {
    const parsed = JSON.parse(body);
    if (
      parsed !== null &&
      typeof parsed === "object" &&
      "data" in parsed &&
      parsed.data !== null &&
      parsed.data !== undefined
    ) {
      return parsed.data;
    }
    return parsed;
  } catch {
    return undefined;
  }
}

function truncate(s: string, n: number): string {
  if (s.length <= n) return s;
  return s.slice(0, n);
}

function newIdempotencyKey(): string {
  const bytes = new Uint8Array(16);
  if (typeof crypto !== "undefined" && crypto.getRandomValues) {
    crypto.getRandomValues(bytes);
    return "alga-" + Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
  }
  return "alga-" + Date.now().toString(16);
}

// backoffMs returns exponential backoff for an attempt index, capped at 30s
// plus up to 20% additive jitter, honoring server-supplied retryAfterMs when
// present.
function backoffMs(attempt: number, retryAfterMs: number): number {
  if (retryAfterMs > 0) return Math.min(retryAfterMs, 10 * 60 * 1000);
  let base = 1000 * Math.pow(2, attempt);
  base = Math.min(base, 30_000);
  const jitter = Math.random() * base * 0.2;
  return base + jitter;
}

// CoordinationTaskListResponse is re-exported here to avoid a circular model
// dependency; it mirrors the backend paginated envelope.
export interface CoordinationTaskListResponse {
  items?: import("./models.js").CoordinationTask[];
  total?: number;
}
