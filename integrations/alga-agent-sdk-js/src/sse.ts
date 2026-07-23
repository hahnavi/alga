import { MessageDedup } from "./dedup.js";
import type {
  ConnectedEvent,
  MessageEvent as AlgaMessageEvent,
  TypingEvent,
  InvestigationSignalEvent,
  PeerFindingEvent,
  PeerAskEvent,
  PeerReplyEvent,
  AgentPresenceEvent,
} from "./models.js";

type Callback<T> = (data: T) => void;

export class SSEClient {
  private httpBase: string;
  private token: string;
  private dedup: MessageDedup;
  private abortController: AbortController | null = null;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private reconnectDelay = 2000;
  private maxReconnectDelay = 60000;
  private stopped = false;

  onConnected: Callback<ConnectedEvent> | null = null;
  onMessage: Callback<AlgaMessageEvent> | null = null;
  onTyping: Callback<TypingEvent> | null = null;
  onInvestigationCancel: Callback<InvestigationSignalEvent> | null = null;
  onInvestigationPause: Callback<InvestigationSignalEvent> | null = null;
  onInvestigationResume: Callback<InvestigationSignalEvent> | null = null;
  onPeerFinding: Callback<PeerFindingEvent> | null = null;
  onPeerAsk: Callback<PeerAskEvent> | null = null;
  onPeerReply: Callback<PeerReplyEvent> | null = null;
  onAgentPresence: Callback<AgentPresenceEvent> | null = null;

  private heartbeatInterval: number;

  constructor(httpBase: string, token: string, dedup?: MessageDedup, heartbeatInterval?: number) {
    this.httpBase = httpBase;
    this.token = token;
    this.dedup = dedup ?? new MessageDedup();
    this.heartbeatInterval = heartbeatInterval ?? 30_000;
  }

  start(): void {
    this.stopped = false;
    this.connect();
    this.heartbeatTimer = setInterval(() => {
      this.sendHeartbeat().catch(() => {});
    }, this.heartbeatInterval);
  }

  stop(): void {
    this.stopped = true;
    if (this.heartbeatTimer !== null) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
    if (this.abortController !== null) {
      this.abortController.abort();
      this.abortController = null;
    }
  }

  private connect(): void {
    if (this.stopped) return;

    const url = `${this.httpBase}/api/v1/agent/events`;
    const ac = new AbortController();
    this.abortController = ac;

    fetch(url, {
      headers: {
        Authorization: `Bearer ${this.token}`,
        Accept: "text/event-stream",
      },
      signal: ac.signal,
    })
      .then(async (response) => {
        if (response.status === 401 || response.status === 403) {
          throw { authError: true, status: response.status };
        }
        if (!response.ok || !response.body) {
          throw new Error(`HTTP ${response.status}`);
        }

        this.reconnectDelay = 2000;
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        let eventType = "";
        let dataBuf: string[] = [];

        try {
          while (true) {
            const { done, value } = await reader.read();
            if (done || this.stopped) break;

            buffer += decoder.decode(value, { stream: true });
            const lines = buffer.split("\n");
            buffer = lines.pop() || "";

            for (const line of lines) {
              const trimmed = line.replace(/\r$/, "");

              if (trimmed === "") {
                if (dataBuf.length > 0) {
                  this.dispatch(eventType, dataBuf.join("\n"));
                }
                eventType = "";
                dataBuf = [];
                continue;
              }

              if (trimmed.startsWith(":")) continue;

              if (trimmed.startsWith("event:")) {
                eventType = trimmed.slice(6).trim();
                continue;
              }

              if (trimmed.startsWith("data:")) {
                dataBuf.push(trimmed.slice(5));
                continue;
              }
            }
          }
        } catch (err: unknown) {
          if (this.stopped || ac.signal.aborted) return;
          throw err;
        }
      })
      .catch((err: unknown) => {
        if (this.stopped) return;
        if (err && typeof err === "object" && "authError" in err) {
          return;
        }
        if (err instanceof DOMException && err.name === "AbortError") return;
        const jitter = 1 + Math.random() * 0.2 - 0.1;
        const delay = Math.min(this.reconnectDelay * jitter, this.maxReconnectDelay);
        this.reconnectDelay = Math.min(this.reconnectDelay * 2, this.maxReconnectDelay);
        setTimeout(() => this.connect(), delay);
      });
  }

  private dispatch(eventType: string, data: string): void {
    try {
      const parsed = JSON.parse(data);
      switch (eventType) {
        case "connected":
          this.onConnected?.(parsed as ConnectedEvent);
          break;
        case "message": {
          const msg = parsed as AlgaMessageEvent;
          if (msg.id && this.dedup.isDuplicate(msg.id)) return;
          if (msg.kind === "lock_emoji") return;
          this.onMessage?.(msg);
          break;
        }
        case "typing":
          this.onTyping?.(parsed as TypingEvent);
          break;
        case "investigation_cancel":
          this.onInvestigationCancel?.(parsed as InvestigationSignalEvent);
          break;
        case "investigation_pause":
          this.onInvestigationPause?.(parsed as InvestigationSignalEvent);
          break;
        case "investigation_resume":
          this.onInvestigationResume?.(parsed as InvestigationSignalEvent);
          break;
        case "peer_finding":
          this.onPeerFinding?.(parsed as PeerFindingEvent);
          break;
        case "peer_ask":
          this.onPeerAsk?.(parsed as PeerAskEvent);
          break;
        case "peer_reply":
          this.onPeerReply?.(parsed as PeerReplyEvent);
          break;
        case "agent_presence":
          this.onAgentPresence?.(parsed as AgentPresenceEvent);
          break;
      }
    } catch {
      // ignore parse errors
    }
  }

  private async sendHeartbeat(): Promise<void> {
    try {
      await fetch(`${this.httpBase}/api/v1/agent/heartbeat`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${this.token}`,
          "Content-Type": "application/json",
        },
      });
    } catch {
      // heartbeat failure is non-fatal
    }
  }
}
