import { MessageDedup } from "./dedup.js";
import { AlgaAuthError, AlgaAPIError, AlgaConnectionError } from "./errors.js";
import type {
  ConnectedEvent,
  MessageEvent as AlgaMessageEvent,
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

type Callback<T> = (data: T) => void;

const LOCK_EMOJI = "\u{1F512}"; // 🔒

// ErrHandler is invoked once with a terminal error (auth failure) after the
// SSE + heartbeat loops have stopped. The client must obtain a valid token and
// call start() again to resume.
export type ErrHandler = (err: Error) => void;

export interface SSEOptions {
  heartbeatIntervalMs?: number;
  userAgent?: string;
  fetchImpl?: typeof fetch;
}

export class SSEClient {
  private httpBase: string;
  private token: string;
  private dedup: MessageDedup;
  private abortController: AbortController | null = null;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private reconnectDelayMs = 2000;
  private readonly maxReconnectDelayMs = 60_000;
  private stopped = false;
  private heartbeatIntervalMs: number;
  private userAgent: string;
  private fetchImpl: typeof fetch;
  private errHandler: ErrHandler | null = null;

  onConnected: Callback<ConnectedEvent> | null = null;
  onMessage: Callback<AlgaMessageEvent> | null = null;
  onTyping: Callback<TypingEvent> | null = null;
  onInvestigationResume: Callback<InvestigationSignalEvent> | null = null;
  onPeerFinding: Callback<PeerFindingEvent> | null = null;
  onPeerAsk: Callback<PeerAskEvent> | null = null;
  onPeerReply: Callback<PeerReplyEvent> | null = null;
  onCoordinationTask: Callback<CoordinationTaskEvent> | null = null;
  onSummarizeIncident: Callback<SummarizeIncidentEvent> | null = null;
  onAlertAutoResolved: Callback<AlertAutoResolvedEvent> | null = null;
  onIncidentCommsStale: Callback<IncidentCommsStaleEvent> | null = null;
  // onUnknownEvent receives any SSE event type the SDK has no dedicated
  // handler for, so consumers can react to backend additions without an SDK
  // upgrade.
  onUnknownEvent: ((eventType: string, data: string) => void) | null = null;

  constructor(
    httpBase: string,
    token: string,
    dedup?: MessageDedup,
    heartbeatIntervalMs?: number,
    opts?: SSEOptions,
  ) {
    this.httpBase = httpBase;
    this.token = token;
    this.dedup = dedup ?? new MessageDedup();
    this.heartbeatIntervalMs = Math.max(1_000, heartbeatIntervalMs ?? 30_000);
    this.userAgent = opts?.userAgent ?? "alga-agent-sdk-js";
    this.fetchImpl = opts?.fetchImpl ?? fetch;
  }

  setErrHandler(handler: ErrHandler): void {
    this.errHandler = handler;
  }

  start(): void {
    this.stopped = false;
    this.connect();
    this.heartbeatTimer = setInterval(() => {
      this.sendHeartbeat().catch(() => {});
    }, this.heartbeatIntervalMs);
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

  private fatal(err: Error): void {
    this.stopped = true;
    if (this.heartbeatTimer !== null) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
    if (this.abortController !== null) {
      this.abortController.abort();
      this.abortController = null;
    }
    this.errHandler?.(err);
  }

  private connect(): void {
    if (this.stopped) return;

    const url = `${this.httpBase}/api/v1/agent/events`;
    const ac = new AbortController();
    this.abortController = ac;

    this.fetchImpl(url, {
      headers: {
        Authorization: `Bearer ${this.token}`,
        Accept: "text/event-stream",
        "User-Agent": this.userAgent,
      },
      signal: ac.signal,
    })
      .then(async (response) => {
        if (response.status === 401 || response.status === 403) {
          throw new AlgaAuthError(response.status, "authentication failed");
        }
        if (response.status !== 200) {
          throw new AlgaAPIError(
            response.status,
            "unexpected status code",
            parseRetryAfterMs(response.headers.get("Retry-After")),
          );
        }
        if (!response.body) {
          throw new AlgaConnectionError("response has no body");
        }

        // Any successful connection resets the backoff.
        this.reconnectDelayMs = 2_000;

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
                  this.dispatch(eventType || "message", dataBuf.join("\n"));
                }
                eventType = "";
                dataBuf = [];
                continue;
              }

              if (trimmed.startsWith(":")) continue;

              if (trimmed.startsWith("event:")) {
                eventType = trimmed.slice(6).trimStart();
                continue;
              }

              if (trimmed.startsWith("data:")) {
                // Per SSE spec, strip exactly one leading space when present.
                dataBuf.push(trimmed.slice(trimmed.charAt(5) === " " ? 6 : 5));
                continue;
              }

              // `id:` is intentionally ignored.
              if (trimmed.startsWith("id:")) continue;
            }
          }
        } catch (err: unknown) {
          if (this.stopped || ac.signal.aborted) return;
          throw new AlgaConnectionError(`stream read failed: ${(err as Error).message}`);
        }

        // Stream closed cleanly — treat as a connection error so the outer
        // loop applies backoff and reconnects.
        if (!this.stopped) throw new AlgaConnectionError("sse stream closed");
      })
      .catch((err: unknown) => {
        if (this.stopped) return;
        if (err instanceof AlgaAuthError) {
          this.fatal(err);
          return;
        }
        if (err instanceof DOMException && err.name === "AbortError") return;

        let delay: number;
        if (err instanceof AlgaAPIError && err.retryAfterMs > 0) {
          delay = err.retryAfterMs;
        } else {
          const jitter = 0.9 + Math.random() * 0.2;
          delay = Math.min(this.reconnectDelayMs * jitter, this.maxReconnectDelayMs);
          this.reconnectDelayMs = Math.min(this.reconnectDelayMs * 2, this.maxReconnectDelayMs);
        }
        setTimeout(() => this.connect(), delay);
      });
  }

  private dispatch(eventType: string, data: string): void {
    const trimmed = data.trim();
    let parsed: unknown;
    try {
      parsed = JSON.parse(trimmed);
    } catch {
      return;
    }

    switch (eventType) {
      case "connected":
        this.onConnected?.(parsed as ConnectedEvent);
        break;
      case "message": {
        const msg = parsed as AlgaMessageEvent;
        const id = msg.message_id;
        if (id && this.dedup.isDuplicate(id)) return;
        // Skip internal/system messages (leading lock-emoji per backend convention).
        if (typeof msg.text === "string" && msg.text.startsWith(LOCK_EMOJI)) return;
        this.onMessage?.(msg);
        break;
      }
      case "typing":
        this.onTyping?.(parsed as TypingEvent);
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
      case "coordination_task_dispatched":
        this.onCoordinationTask?.(parsed as CoordinationTaskEvent);
        break;
      case "summarize_incident":
        this.onSummarizeIncident?.(parsed as SummarizeIncidentEvent);
        break;
      case "alert_auto_resolved":
        this.onAlertAutoResolved?.(parsed as AlertAutoResolvedEvent);
        break;
      case "incident_comms_stale":
        this.onIncidentCommsStale?.(parsed as IncidentCommsStaleEvent);
        break;
      default:
        this.onUnknownEvent?.(eventType, trimmed);
        break;
    }
  }

  private async sendHeartbeat(): Promise<void> {
    try {
      const res = await this.fetchImpl(`${this.httpBase}/api/v1/agent/heartbeat`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${this.token}`,
          "User-Agent": this.userAgent,
        },
      });
      if (res.status === 401 || res.status === 403) {
        this.fatal(new AlgaAuthError(res.status, "heartbeat auth failed"));
      }
    } catch {
      // Non-auth heartbeat errors are non-fatal; the SSE loop observes
      // persistent outages on its own cadence.
    }
  }
}

// parseRetryAfterMs parses the HTTP `Retry-After` header into milliseconds.
// Both delta-seconds and HTTP-date forms are accepted. Returns 0 when the
// header is absent or unparseable. Capped at 10 minutes.
export function parseRetryAfterMs(raw: string | null): number {
  if (!raw) return 0;
  const trimmed = raw.trim();
  if (trimmed === "") return 0;
  const secs = Number(trimmed);
  if (Number.isFinite(secs) && secs >= 0) {
    return Math.min(secs * 1000, 10 * 60 * 1000);
  }
  const t = Date.parse(trimmed);
  if (Number.isFinite(t)) {
    const d = t - Date.now();
    if (d > 0) return Math.min(d, 10 * 60 * 1000);
  }
  return 0;
}
