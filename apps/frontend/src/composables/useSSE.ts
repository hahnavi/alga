import { onBeforeUnmount, ref, type Ref } from "vue";
import { redirectToLogin } from "@/lib/authRedirect";

type SSEState = "connecting" | "open" | "reconnecting";

type SSEOptions = {
  reconnectMs?: number;
  withCredentials?: boolean;
  maxRetries?: number;
  onReconnect?: () => void;
};

const DEFAULT_MAX_RETRIES = 20;
const BACKOFF_MULTIPLIER = 1.5;
const MAX_BACKOFF_MS = 30_000;
const AUTH_PROBE_PATH = "/api/v1/notifications?limit=1";

type SharedSSE = {
  es: EventSource | null;
  refCount: number;
  handlers: Map<string, Set<(data: unknown) => void>>;
  reconnectTimer: number | null;
  retryCount: number;
  disposed: boolean;
  options: SSEOptions;
  eventListeners: Array<{ event: string; fn: (e: MessageEvent) => void }>;
  hasConnectedBefore: boolean;
  state: Ref<SSEState>;
};

const sharedConnections = new Map<string, SharedSSE>();

async function probeAuthAndReconnect(path: string, shared: SharedSSE) {
  if (shared.disposed) return;
  const base = import.meta.env.VITE_API_BASE_URL ?? "";
  try {
    const res = await fetch(`${base}${AUTH_PROBE_PATH}`, { credentials: "same-origin" });
    // Only treat 401 as an auth failure. 403 may be a permission flip for
    // an otherwise valid session; redirecting there would kick the user
    // out and force a hard re-login on transient policy changes.
    if (res.status === 401) {
      shared.state.value = "reconnecting";
      redirectToLogin();
      return;
    }
  } catch {
    // Network error, not an auth failure - fall through to normal reconnect.
  }
  scheduleReconnect(path, shared);
}

function attachHandler(shared: SharedSSE, event: string) {
  if (!shared.es || event === "message") return;
  const existing = shared.eventListeners.find((l) => l.event === event);
  if (existing) return;
  const fn = (e: MessageEvent) => {
    try {
      dispatch(shared, event, JSON.parse(e.data as string));
    } catch {
      /* ignore */
    }
  };
  shared.es.addEventListener(event, fn);
  shared.eventListeners.push({ event, fn });
}

function connectShared(path: string, shared: SharedSSE) {
  if (shared.reconnectTimer != null) {
    clearTimeout(shared.reconnectTimer);
    shared.reconnectTimer = null;
  }
  if (shared.es) {
    shared.es.close();
    shared.es = null;
  }

  const { withCredentials = true } = shared.options;
  const base = import.meta.env.VITE_API_BASE_URL ?? "";

  try {
    shared.es = new EventSource(`${base}${path}`, { withCredentials });
  } catch {
    scheduleReconnect(path, shared);
    return;
  }

  shared.es.onmessage = (e: MessageEvent) => {
    try {
      dispatch(shared, "message", JSON.parse(e.data as string));
    } catch {
      /* ignore */
    }
  };

  // Attach tracked listeners for every registered event so close()/eventListeners
  // accounting matches what is actually attached to the EventSource. Tracked
  // listeners reference the previous (now closed) EventSource, so reset the
  // list first or named events silently stop arriving after a reconnect.
  shared.eventListeners = [];
  for (const event of shared.handlers.keys()) {
    attachHandler(shared, event);
  }

  shared.es.onerror = () => {
    if (shared.es) {
      shared.es.close();
      shared.es = null;
    }
    shared.state.value = "reconnecting";
    // After the first failure of each burst, probe auth. EventSource hides HTTP
    // status, so a dead session looks identical to a transient network blip and
    // would otherwise hammer the server with 401s forever. The probe hits a
    // non-safe path so a 401 here triggers the global login redirect.
    if (shared.retryCount === 0 && !shared.disposed) {
      void probeAuthAndReconnect(path, shared);
    } else {
      scheduleReconnect(path, shared);
    }
  };

  shared.es.onopen = () => {
    shared.state.value = "open";
    if (shared.hasConnectedBefore) {
      shared.options.onReconnect?.();
    } else {
      shared.hasConnectedBefore = true;
    }
    shared.retryCount = 0;
  };
}

function dispatch(shared: SharedSSE, event: string, data: unknown) {
  const set = shared.handlers.get(event);
  if (set) {
    for (const fn of set) {
      try {
        fn(data);
      } catch {
        /* swallow handler errors */
      }
    }
  }
}

function scheduleReconnect(path: string, shared: SharedSSE) {
  if (shared.disposed) return;
  const { reconnectMs = 3000, maxRetries = DEFAULT_MAX_RETRIES } = shared.options;
  if (shared.retryCount >= maxRetries) return;
  if (shared.reconnectTimer != null) clearTimeout(shared.reconnectTimer);
  const delay = Math.min(
    reconnectMs * Math.pow(BACKOFF_MULTIPLIER, shared.retryCount),
    MAX_BACKOFF_MS,
  );
  shared.retryCount++;
  shared.reconnectTimer = setTimeout(() => {
    shared.reconnectTimer = null;
    connectShared(path, shared);
  }, delay);
}

function createShared(
  options: SSEOptions,
  initialState: Ref<SSEState> = ref<SSEState>("connecting"),
): SharedSSE {
  return {
    es: null,
    refCount: 0,
    handlers: new Map(),
    reconnectTimer: null,
    retryCount: 0,
    disposed: false,
    options,
    eventListeners: [],
    hasConnectedBefore: false,
    state: initialState,
  };
}

function registerHandlers(shared: SharedSSE, handlers: Record<string, (data: unknown) => void>) {
  for (const [event, handler] of Object.entries(handlers)) {
    let set = shared.handlers.get(event);
    const isNewEvent = !set;
    if (!set) {
      set = new Set();
      shared.handlers.set(event, set);
    }
    set.add(handler);
    if (isNewEvent) {
      attachHandler(shared, event);
    }
  }
}

export function useSSE(
  path: string,
  handlers: Record<string, (data: unknown) => void>,
  options: SSEOptions = {},
) {
  let shared = sharedConnections.get(path);
  if (!shared || shared.disposed) {
    shared = createShared(options);
    sharedConnections.set(path, shared);
    connectShared(path, shared);
  }

  const entries = Object.entries(handlers);
  registerHandlers(shared, handlers);
  shared.refCount++;

  function close() {
    for (const [evt, handler] of entries) {
      const set = shared!.handlers.get(evt);
      if (set) {
        set.delete(handler);
        if (set.size === 0) shared!.handlers.delete(evt);
      }
    }
    if (shared!.es) {
      const removed = shared!.eventListeners.filter((l) => !shared!.handlers.has(l.event));
      for (const { event, fn } of removed) {
        shared!.es.removeEventListener(event, fn);
      }
    }
    shared!.eventListeners = shared!.eventListeners.filter((l) => shared!.handlers.has(l.event));
    shared!.refCount--;
    if (shared!.refCount <= 0) {
      shared!.disposed = true;
      if (shared!.reconnectTimer != null) {
        clearTimeout(shared!.reconnectTimer);
        shared!.reconnectTimer = null;
      }
      if (shared!.es) {
        shared!.es.close();
        shared!.es = null;
      }
      shared!.eventListeners = [];
      sharedConnections.delete(path);
    }
  }

  function reconnect() {
    if (!shared) return;
    if (shared.disposed) {
      const prevState = shared.state;
      const prevRefCount = shared.refCount;
      shared = createShared(options, prevState);
      shared.refCount = prevRefCount || 1;
      sharedConnections.set(path, shared);
      registerHandlers(shared, handlers);
    }
    shared.retryCount = 0;
    shared.state.value = "connecting";
    connectShared(path, shared);
  }

  onBeforeUnmount(() => {
    close();
  });

  return { close, reconnect, state: shared.state };
}
