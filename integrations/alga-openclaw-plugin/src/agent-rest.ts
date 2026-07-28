import type { AlgaOutboundMessagePayload } from "./types.js";

export type AgentPostMessageResult = {
  status?: string;
  message_id?: string;
  ok?: boolean;
  op?: string;
  investigation_id?: string;
  error?: string;
};

export async function agentPostMessage(
  httpBase: string,
  token: string,
  chatId: string,
  payload: AlgaOutboundMessagePayload,
): Promise<AgentPostMessageResult> {
  const res = await fetch(`${httpBase}/api/v1/agent/messages`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ chat_id: chatId, ...payload }),
  });
  if (!res.ok) {
    throw new Error(`Alga POST /agent/messages failed: ${res.status} ${await res.text()}`);
  }
  return (await res.json()) as AgentPostMessageResult;
}

export async function agentEditMessage(
  httpBase: string,
  token: string,
  messageId: string,
  chatId: string,
  text: string,
): Promise<void> {
  const res = await fetch(`${httpBase}/api/v1/agent/messages/${encodeURIComponent(messageId)}`, {
    method: "PUT",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ chat_id: chatId, kind: "text", text }),
  });
  if (!res.ok) {
    throw new Error(`Alga PUT /agent/messages/${messageId} failed: ${res.status} ${await res.text()}`);
  }
}

export async function agentDeleteMessage(
  httpBase: string,
  token: string,
  messageId: string,
  chatId: string,
): Promise<void> {
  const res = await fetch(`${httpBase}/api/v1/agent/messages/${encodeURIComponent(messageId)}`, {
    method: "DELETE",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ chat_id: chatId }),
  });
  if (!res.ok) {
    throw new Error(`Alga DELETE /agent/messages/${messageId} failed: ${res.status} ${await res.text()}`);
  }
}

export async function agentPostTyping(
  httpBase: string,
  token: string,
  chatId: string,
  active: boolean,
): Promise<void> {
  const res = await fetch(`${httpBase}/api/v1/agent/typing`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ chat_id: chatId, active }),
  });
  if (!res.ok) {
    throw new Error(`Alga POST /agent/typing failed: ${res.status} ${await res.text()}`);
  }
}

export async function agentPostHeartbeat(httpBase: string, token: string): Promise<void> {
  const res = await fetch(`${httpBase}/api/v1/agent/heartbeat`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
  });
  if (!res.ok) {
    throw new Error(`Alga POST /agent/heartbeat failed: ${res.status} ${await res.text()}`);
  }
}

export async function agentGetAlerts(
  httpBase: string,
  token: string,
  params?: {
    status?: string;
    severity?: string;
    search?: string;
    limit?: number;
    skip?: number;
  },
): Promise<{ alerts: unknown[]; total: number }> {
  const sp = new URLSearchParams();
  if (params?.status) sp.set("status", params.status);
  if (params?.severity) sp.set("severity", params.severity);
  if (params?.search) sp.set("search", params.search);
  if (params?.limit) sp.set("limit", String(params.limit));
  if (params?.skip) sp.set("skip", String(params.skip));
  const qs = sp.toString() ? `?${sp.toString()}` : "";
  const res = await fetch(`${httpBase}/api/v1/agent/alerts${qs}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok) {
    throw new Error(`Alga GET /agent/alerts failed: ${res.status} ${await res.text()}`);
  }
  const raw = await res.json();
  if (Array.isArray(raw)) {
    return { alerts: raw, total: raw.length };
  }
  const alerts = (raw.alerts ?? raw.items ?? []) as unknown[];
  const total = (raw.total ?? 0) as number;
  return { alerts, total };
}

export async function agentGetAlert(
  httpBase: string,
  token: string,
  fingerprint: string,
): Promise<unknown> {
  const res = await fetch(
    `${httpBase}/api/v1/agent/alerts/${encodeURIComponent(fingerprint)}`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  if (!res.ok) {
    throw new Error(`Alga GET /agent/alerts/${fingerprint} failed: ${res.status} ${await res.text()}`);
  }
  return await res.json();
}

export async function agentSearchKnowledge(
  httpBase: string,
  token: string,
  params?: {
    query?: string;
    kind?: string;
    tag?: string;
    limit?: number;
    skip?: number;
  },
): Promise<{ notes: unknown[]; total: number }> {
  const sp = new URLSearchParams();
  if (params?.query) sp.set("q", params.query);
  if (params?.kind) sp.set("kind", params.kind);
  if (params?.tag) sp.set("tag", params.tag);
  if (params?.limit) sp.set("limit", String(params.limit));
  if (params?.skip) sp.set("skip", String(params.skip));
  const qs = sp.toString() ? `?${sp.toString()}` : "";
  const res = await fetch(`${httpBase}/api/v1/agent/knowledge${qs}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok) {
    throw new Error(`Alga GET /agent/knowledge failed: ${res.status} ${await res.text()}`);
  }
  const raw = await res.json();
  const items = (raw as Record<string, unknown>).items ?? (raw as Record<string, unknown>).notes ?? [];
  return { notes: items as unknown[], total: ((raw as Record<string, unknown>).total ?? 0) as number };
}

export async function agentGetKnowledge(
  httpBase: string,
  token: string,
  id: string,
): Promise<unknown> {
  const res = await fetch(`${httpBase}/api/v1/agent/knowledge/${encodeURIComponent(id)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok) {
    throw new Error(`Alga GET /agent/knowledge/${id} failed: ${res.status} ${await res.text()}`);
  }
  return await res.json();
}

export async function agentCreateKnowledge(
  httpBase: string,
  token: string,
  note: {
    kind: string;
    title: string;
    body_markdown: string;
    tags?: string[];
    selectors?: unknown[];
    source_investigation_id?: string;
    confidence?: number;
  },
): Promise<unknown> {
  const res = await fetch(`${httpBase}/api/v1/agent/knowledge`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify(note),
  });
  if (!res.ok) {
    throw new Error(`Alga POST /agent/knowledge failed: ${res.status} ${await res.text()}`);
  }
  return await res.json();
}

export async function agentGetIncident(
  httpBase: string, token: string, id: string,
): Promise<unknown> {
  const res = await fetch(
    `${httpBase}/api/v1/agent/incidents/${encodeURIComponent(id)}`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  if (!res.ok) throw new Error(`Alga GET /agent/incidents/${id} failed: ${res.status}`);
  return await res.json();
}

export async function agentAddIncidentTimeline(
  httpBase: string, token: string, incidentId: string,
  entry: { type: string; message: string },
): Promise<unknown> {
  const res = await fetch(
    `${httpBase}/api/v1/agent/incidents/${encodeURIComponent(incidentId)}/timeline`,
    {
      method: "POST",
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify(entry),
    },
  );
  if (!res.ok) throw new Error(`Alga POST /agent/incidents/${incidentId}/timeline failed: ${res.status}`);
  return await res.json();
}

export async function agentGetIncidentTimeline(
  httpBase: string, token: string, incidentId: string,
): Promise<unknown> {
  const res = await fetch(
    `${httpBase}/api/v1/agent/incidents/${encodeURIComponent(incidentId)}/timeline`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  if (!res.ok) throw new Error(`Alga GET /agent/incidents/${incidentId}/timeline failed: ${res.status}`);
  return await res.json();
}

export async function agentListTasks(
  httpBase: string, token: string, incidentNumber: number,
  params?: { status?: string; limit?: number; skip?: number },
): Promise<unknown> {
  const sp = new URLSearchParams();
  if (params?.status) sp.set("status", params.status);
  if (params?.limit) sp.set("limit", String(params.limit));
  if (params?.skip) sp.set("skip", String(params.skip));
  const qs = sp.toString() ? `?${sp.toString()}` : "";
  const res = await fetch(
    `${httpBase}/api/v1/agent/incidents/${incidentNumber}/tasks${qs}`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  if (!res.ok) throw new Error(`Alga GET /agent/incidents/${incidentNumber}/tasks failed: ${res.status}`);
  return await res.json();
}

export async function agentGetOnCall(
  httpBase: string, token: string,
): Promise<unknown> {
  const res = await fetch(`${httpBase}/api/v1/agent/on-call/current`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok) throw new Error(`Alga GET /agent/on-call/current failed: ${res.status}`);
  return await res.json();
}

export async function agentListServices(
  httpBase: string, token: string,
): Promise<unknown> {
  const res = await fetch(`${httpBase}/api/v1/agent/services`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok) throw new Error(`Alga GET /agent/services failed: ${res.status}`);
  return await res.json();
}

export async function agentSearchMemories(
  httpBase: string, token: string,
  params?: { query?: string; limit?: number },
): Promise<unknown> {
  const sp = new URLSearchParams();
  if (params?.query) sp.set("query", params.query);
  if (params?.limit) sp.set("limit", String(params.limit));
  const qs = sp.toString() ? `?${sp.toString()}` : "";
  const res = await fetch(`${httpBase}/api/v1/agent/memories${qs}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok) throw new Error(`Alga GET /agent/memories failed: ${res.status}`);
  return await res.json();
}

export async function agentCreateMemory(
  httpBase: string, token: string,
  entry: { content: string; kind?: string; source_investigation_id?: string },
): Promise<unknown> {
  const res = await fetch(`${httpBase}/api/v1/agent/memories`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    body: JSON.stringify(entry),
  });
  if (!res.ok) throw new Error(`Alga POST /agent/memories failed: ${res.status}`);
  return await res.json();
}

export async function agentPeerAsk(
  httpBase: string, token: string,
  req: { target_agent_id: string; question: string },
): Promise<unknown> {
  const res = await fetch(`${httpBase}/api/v1/agent/peer-ask`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) throw new Error(`Alga POST /agent/peer-ask failed: ${res.status}`);
  return await res.json();
}
