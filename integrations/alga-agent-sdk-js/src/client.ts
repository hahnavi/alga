import { AlgaAuthError, AlgaAPIError, AlgaConnectionError } from "./errors.js";
import { MessageDedup } from "./dedup.js";
import { SSEClient } from "./sse.js";
import type {
  Alert,
  AlertListResponse,
  Investigation,
  InvestigationListResponse,
  KnowledgeNote,
  KnowledgeListResponse,
  Memory,
  MemoryListResponse,
  PeerAsk,
  PeerAskListResponse,
  SendMessageResponse,
  CommandResponse,
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
} from "./models.js";
import type { InvestigationCommand } from "./commands.js";
import type { Playbook, Capability } from "./models.js";

export interface AlgaClientOptions {
  heartbeatInterval?: number;
  dedup?: MessageDedup;
}

type Callback<T> = (data: T) => void;

export class AlgaClient {
  private serverUrl: string;
  private token: string;
  private dedup: MessageDedup;
  private heartbeatInterval?: number;
  private sse: SSEClient | null = null;
  private stopped = false;
  private stopResolve: (() => void) | null = null;

  onConnected: Callback<ConnectedEvent> | null = null;
  onMessage: Callback<MessageEvent> | null = null;
  onTyping: Callback<TypingEvent> | null = null;
  onInvestigationCancel: Callback<InvestigationSignalEvent> | null = null;
  onInvestigationPause: Callback<InvestigationSignalEvent> | null = null;
  onInvestigationResume: Callback<InvestigationSignalEvent> | null = null;
  onPeerFinding: Callback<PeerFindingEvent> | null = null;
  onPeerAsk: Callback<PeerAskEvent> | null = null;
  onPeerReply: Callback<PeerReplyEvent> | null = null;
  onAgentPresence: Callback<AgentPresenceEvent> | null = null;

  constructor(serverUrl: string, token: string, options?: AlgaClientOptions) {
    this.serverUrl = serverUrl.replace(/\/+$/, "");
    this.token = token;
    this.dedup = options?.dedup ?? new MessageDedup();
    this.heartbeatInterval = options?.heartbeatInterval;
  }

  connect(): void {
    this.stopped = false;
    this.sse = new SSEClient(this.serverUrl, this.token, this.dedup, this.heartbeatInterval);

    this.sse.onConnected = (data) => this.onConnected?.(data);
    this.sse.onMessage = (data) => this.onMessage?.(data);
    this.sse.onTyping = (data) => this.onTyping?.(data);
    this.sse.onInvestigationCancel = (data) => this.onInvestigationCancel?.(data);
    this.sse.onInvestigationPause = (data) => this.onInvestigationPause?.(data);
    this.sse.onInvestigationResume = (data) => this.onInvestigationResume?.(data);
    this.sse.onPeerFinding = (data) => this.onPeerFinding?.(data);
    this.sse.onPeerAsk = (data) => this.onPeerAsk?.(data);
    this.sse.onPeerReply = (data) => this.onPeerReply?.(data);
    this.sse.onAgentPresence = (data) => this.onAgentPresence?.(data);

    this.sse.start();
  }

  disconnect(): void {
    this.stopped = true;
    this.sse?.stop();
    this.sse = null;
    this.stopResolve?.();
    this.stopResolve = null;
  }

  wait(): Promise<void> {
    if (this.stopped) return Promise.resolve();
    return new Promise<void>((resolve) => {
      this.stopResolve = resolve;
    });
  }

  async listAlerts(params?: Record<string, unknown>): Promise<AlertListResponse> {
    return this._get("/api/v1/agent/alerts", params);
  }

  async getAlert(fingerprint: string): Promise<Alert> {
    return this._get(`/api/v1/agent/alerts/${encodeURIComponent(fingerprint)}`);
  }

  async resolveAlert(fingerprint: string): Promise<Alert> {
    return this._post(`/api/v1/agent/alerts/${encodeURIComponent(fingerprint)}/resolve`);
  }

  async reopenAlert(fingerprint: string): Promise<Alert> {
    return this._post(`/api/v1/agent/alerts/${encodeURIComponent(fingerprint)}/reopen`);
  }

  async listInvestigations(params?: Record<string, unknown>): Promise<InvestigationListResponse> {
    return this._get("/api/v1/agent/investigations", params);
  }

  async getInvestigation(id: string): Promise<Investigation> {
    return this._get(`/api/v1/agent/investigations/${encodeURIComponent(id)}`);
  }

  async postUpdate(investigationId: string, type: string, message: string): Promise<Investigation> {
    return this._post(`/api/v1/agent/investigations/${encodeURIComponent(investigationId)}/updates`, {
      type,
      message,
    });
  }

  async sendMessage(chatId: string, text: string, mentions?: string[]): Promise<SendMessageResponse> {
    const body: Record<string, unknown> = { chat_id: chatId, kind: "text", text };
    if (mentions) body.mentions = mentions;
    return this._post("/api/v1/agent/messages", body);
  }

  async sendCommand(chatId: string, command: InvestigationCommand): Promise<CommandResponse> {
    return this._post("/api/v1/agent/messages", {
      chat_id: chatId,
      kind: "inv_tool",
      command,
    });
  }

  async sendIncidentSummary(incidentId: string, text: string): Promise<void> {
    await this._post("/api/v1/agent/messages", {
      chat_id: `incident_coord_${incidentId}`,
      kind: "incident_summary",
      text,
    });
  }

  async sendTriageResponse(investigationId: string, text: string, mentions?: string[]): Promise<void> {
    const body: Record<string, unknown> = {
      chat_id: `investigation_${investigationId}`,
      kind: "triage_response",
      text,
    };
    if (mentions) body.mentions = mentions;
    await this._post("/api/v1/agent/messages", body);
  }

  async sendCommandDecision(investigationId: string, text: string, mentions?: string[]): Promise<void> {
    const body: Record<string, unknown> = {
      chat_id: `investigation_${investigationId}`,
      kind: "command_decision",
      text,
    };
    if (mentions) body.mentions = mentions;
    await this._post("/api/v1/agent/messages", body);
  }

  async sendStatusUpdate(chatId: string, text: string, mentions?: string[]): Promise<void> {
    const body: Record<string, unknown> = {
      chat_id: chatId,
      kind: "status_update",
      text,
    };
    if (mentions) body.mentions = mentions;
    await this._post("/api/v1/agent/messages", body);
  }

  async editMessage(messageId: string, chatId: string, text: string): Promise<void> {
    await this._put(`/api/v1/agent/messages/${encodeURIComponent(messageId)}`, {
      chat_id: chatId,
      kind: "text",
      text,
    });
  }

  async deleteMessage(messageId: string, chatId: string): Promise<void> {
    await this._delete(`/api/v1/agent/messages/${encodeURIComponent(messageId)}`, {
      chat_id: chatId,
    });
  }

  async sendTyping(chatId: string, active = true): Promise<void> {
    await this._post("/api/v1/agent/typing", { chat_id: chatId, active });
  }

  async sendHeartbeat(): Promise<void> {
    await this._post("/api/v1/agent/heartbeat");
  }

  async listKnowledge(params?: Record<string, unknown>): Promise<KnowledgeListResponse> {
    return this._get("/api/v1/agent/knowledge", params);
  }

  async createKnowledge(params: Record<string, unknown>): Promise<KnowledgeNote> {
    return this._post("/api/v1/agent/knowledge", params);
  }

  async listMemories(params?: Record<string, unknown>): Promise<MemoryListResponse> {
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

  async listPeerAsks(params?: Record<string, unknown>): Promise<PeerAskListResponse> {
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

  async getIncident(id: string): Promise<Incident> {
    return this._get(`/api/v1/agent/incidents/${encodeURIComponent(id)}`);
  }

  async addIncidentTimeline(id: string, message: string, eventType?: string): Promise<unknown> {
    const body: Record<string, unknown> = { message };
    if (eventType) body.event_type = eventType;
    return this._post(`/api/v1/agent/incidents/${encodeURIComponent(id)}/timeline`, body);
  }

  async listServices(): Promise<Service[]> {
    return this._get("/api/v1/agent/services");
  }

  async whoIsOnCall(): Promise<Record<string, unknown>[]> {
    const res = await this._get("/api/v1/agent/on-call/current");
    return Array.isArray(res) ? res : [];
  }

  async getCapabilities(): Promise<Capability[]> {
    return this._get("/api/v1/agent/capabilities");
  }

  async getPlaybooks(alertFingerprint?: string): Promise<Playbook[]> {
    const params: Record<string, unknown> = {};
    if (alertFingerprint) {
      params.alert_fingerprint = alertFingerprint;
    }
    return this._get("/api/v1/agent/playbooks", params);
  }

  async uploadMedia(filePath: string): Promise<unknown> {
    const { readFileSync } = await import("node:fs");
    const { basename } = await import("node:path");
    const buffer = readFileSync(filePath);
    const filename = basename(filePath);

    const formData = new FormData();
    formData.append("file", new Blob([buffer]), filename);

    const url = `${this.serverUrl}/api/v1/agent/media`;
    const res = await fetch(url, {
      method: "POST",
      headers: { Authorization: `Bearer ${this.token}` },
      body: formData,
    });

    if (!res.ok) {
      const text = await res.text().catch(() => "");
      throw new AlgaAPIError(res.status, text || `upload failed: ${res.status}`);
    }

    return res.json();
  }

  private async _request(method: string, path: string, body?: unknown): Promise<any> {
    const url = `${this.serverUrl}${path}`;
    const headers: Record<string, string> = {
      Authorization: `Bearer ${this.token}`,
      "Content-Type": "application/json",
    };

    let res: Response;
    try {
      res = await fetch(url, {
        method,
        headers,
        body: body !== undefined ? JSON.stringify(body) : undefined,
      });
    } catch (err) {
      throw new AlgaConnectionError(`request failed: ${(err as Error).message}`);
    }

    if (res.status === 401 || res.status === 403) {
      const text = await res.text().catch(() => "");
      throw new AlgaAuthError(res.status, text || `authentication error: ${res.status}`);
    }

    if (res.status >= 400) {
      const text = await res.text().catch(() => "");
      throw new AlgaAPIError(res.status, text || `API error: ${res.status}`);
    }

    if (res.status === 204 || res.headers.get("content-length") === "0") {
      return undefined;
    }

    return res.json();
  }

  private _get(path: string, params?: Record<string, unknown>): Promise<any> {
    let fullPath = path;
    if (params) {
      const qs = new URLSearchParams();
      for (const [k, v] of Object.entries(params)) {
        if (v !== undefined && v !== null) {
          qs.set(k, String(v));
        }
      }
      const str = qs.toString();
      if (str) fullPath += `?${str}`;
    }
    return this._request("GET", fullPath);
  }

  private _post(path: string, body?: unknown): Promise<any> {
    return this._request("POST", path, body);
  }

  private _put(path: string, body?: unknown): Promise<any> {
    return this._request("PUT", path, body);
  }

  private _delete(path: string, body?: unknown): Promise<any> {
    return this._request("DELETE", path, body);
  }
}
