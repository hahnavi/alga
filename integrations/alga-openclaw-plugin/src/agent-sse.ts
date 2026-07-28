import EventSource from "eventsource2";
import type { InvestigationSignalEvent, InvestigationSignalEventType } from "./types.js";

const HEARTBEAT_INTERVAL_MS = 30_000;
const RECONNECT_BASE_MS = 2_000;
const RECONNECT_MAX_MS = 60_000;

export type AlgaSSEInboundHandler = (raw: string) => void;

export type AlgaSSESignalHandler = (
  signalType: InvestigationSignalEventType,
  data: InvestigationSignalEvent,
) => void;

export type AlgaSSELogSink = {
  info: (m: string) => void;
  warn: (m: string) => void;
  error: (m: string) => void;
};

export class AlgaSSEClient {
  private es: EventSource | null = null;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectDelay = RECONNECT_BASE_MS;
  private stopped = false;

  constructor(
    private readonly opts: {
      httpBase: string;
      token: string;
      onInboundText: AlgaSSEInboundHandler;
      onInvestigationSignal?: AlgaSSESignalHandler;
      log?: AlgaSSELogSink;
    },
  ) {}

  isConnected(): boolean {
    return this.es !== null && this.es.readyState === EventSource.OPEN;
  }

  start(abortSignal: AbortSignal): Promise<void> {
    this.stopped = false;
    this.connect();

    abortSignal.addEventListener(
      "abort",
      () => {
        this.stopped = true;
        void this.stop();
      },
      { once: true },
    );

    return new Promise((resolve) => {
      if (abortSignal.aborted) {
        resolve();
        return;
      }
      abortSignal.addEventListener("abort", () => resolve(), { once: true });
    });
  }

  async stop(): Promise<void> {
    this.stopped = true;
    this.stopHeartbeat();
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.es) {
      this.es.close();
      this.es = null;
    }
  }

  private connect(): void {
    const url = `${this.opts.httpBase}/api/v1/agent/events`;
    this.opts.log?.info(`Alga SSE connecting (${url})`);
    try {
      this.es = new EventSource(url, {
        headers: { Authorization: `Bearer ${this.opts.token}` },
      });
    } catch (err) {
      this.opts.log?.error(`Alga SSE init failed: ${err instanceof Error ? err.message : String(err)}`);
      this.scheduleReconnect();
      return;
    }
    this.setupEventListeners();
  }

  private scheduleReconnect(): void {
    if (this.stopped) return;
    if (this.reconnectTimer !== null) clearTimeout(this.reconnectTimer);
    const jittered = this.reconnectDelay * (0.9 + Math.random() * 0.2);
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      if (!this.stopped) this.connect();
    }, jittered);
    this.reconnectDelay = Math.min(this.reconnectDelay * 2, RECONNECT_MAX_MS);
  }

  private setupEventListeners(): void {
    const es = this.es;
    if (!es) return;

    es.onopen = () => {
      this.reconnectDelay = RECONNECT_BASE_MS;
      this.opts.log?.info(`Alga SSE connected (${this.opts.httpBase})`);
      this.startHeartbeat();
    };

    es.onerror = (err: unknown) => {
      this.opts.log?.warn(
        `Alga SSE error: ${err instanceof Error ? err.message : String(err)} (reconnecting)`,
      );
      this.stopHeartbeat();
      this.scheduleReconnect();
    };

    es.addEventListener("connected", () => {});

    es.addEventListener("message", (ev: MessageEvent) => {
      if (typeof ev.data === "string") this.opts.onInboundText(ev.data);
    });

    es.addEventListener("typing", () => {});
    es.addEventListener("peer_finding", () => {});
    es.addEventListener("peer_ask", () => {});
    es.addEventListener("peer_reply", () => {});

    es.addEventListener("investigation_resume", (ev: MessageEvent) => {
      this.dispatchSignal("investigation_resume", ev.data);
    });

    es.addEventListener("investigation_abort", (ev: MessageEvent) => {
      this.dispatchSignal("investigation_abort", ev.data);
    });

    es.addEventListener("coordination_task_dispatched", (ev: MessageEvent) => {
      this.opts.log?.info(
        `Alga SSE coordination_task_dispatched: ${typeof ev.data === "string" ? ev.data : ""}`,
      );
    });

    es.addEventListener("summarize_incident", (ev: MessageEvent) => {
      this.opts.log?.info(
        `Alga SSE summarize_incident: ${typeof ev.data === "string" ? ev.data : ""}`,
      );
    });

    es.addEventListener("alert_auto_resolved", (ev: MessageEvent) => {
      this.opts.log?.info(
        `Alga SSE alert_auto_resolved: ${typeof ev.data === "string" ? ev.data : ""}`,
      );
    });

    es.addEventListener("incident_comms_stale", (ev: MessageEvent) => {
      this.opts.log?.info(
        `Alga SSE incident_comms_stale: ${typeof ev.data === "string" ? ev.data : ""}`,
      );
    });
  }

  private dispatchSignal(
    signalType: InvestigationSignalEventType,
    rawData: unknown,
  ): void {
    if (!this.opts.onInvestigationSignal) return;
    if (typeof rawData !== "string" || !rawData) return;
    try {
      const data = JSON.parse(rawData) as InvestigationSignalEvent;
      if (!data?.investigation_id) return;
      this.opts.onInvestigationSignal(signalType, data);
    } catch {
      this.opts.log?.warn(`Failed to parse ${signalType} event: ${rawData}`);
    }
  }

  private startHeartbeat(): void {
    this.stopHeartbeat();
    this.heartbeatTimer = setInterval(() => {
      void fetch(`${this.opts.httpBase}/api/v1/agent/heartbeat`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${this.opts.token}`,
          "Content-Type": "application/json",
        },
      }).catch(() => {});
    }, HEARTBEAT_INTERVAL_MS);
  }

  private stopHeartbeat(): void {
    if (this.heartbeatTimer !== null) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
  }
}

const sessions = new Map<string, AlgaSSEClient>();

export function registerAlgaSSESession(accountId: string, session: AlgaSSEClient): void {
  sessions.set(accountId, session);
}

export function unregisterAlgaSSESession(accountId: string): void {
  sessions.delete(accountId);
}

export function getAlgaSSESession(accountId: string): AlgaSSEClient | undefined {
  return sessions.get(accountId);
}
