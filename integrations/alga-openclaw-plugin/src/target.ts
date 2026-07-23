import { normalizeInvestigationChatId, stripInvestigationChatPrefix } from "./chat-ids.js";

/** OpenClaw session kind: operator private chat vs investigation thread. */
export type AlgaOpenClawChatType = "group" | "direct";

export type ParsedAlgaTarget = {
  to: string;
  chatType: AlgaOpenClawChatType;
  investigationSessionId: string;
};

/** Map normalized Alga messaging target to OpenClaw chat kind (`alga_dm` is direct). */
export function inferAlgaChatTypeFromTarget(raw: string): AlgaOpenClawChatType {
  const n = normalizeAlgaMessagingTarget(raw);
  return /^alga_dm$/i.test(n) ? "direct" : "group";
}

/** Normalize user-facing target to canonical `investigation_<id>` or fixed operator–agent DM `alga_dm`. */
export function normalizeAlgaMessagingTarget(raw: string): string {
  let s = raw.trim();
  if (!s) {
    return s;
  }
  if (/^alga_dm$/i.test(s)) {
    return "alga_dm";
  }
  if (/^alga:/i.test(s)) {
    s = s.replace(/^alga:/i, "").trim();
  }
  return normalizeInvestigationChatId(s);
}

export function parseAlgaExplicitTarget(raw: string): ParsedAlgaTarget | null {
  const normalized = normalizeAlgaMessagingTarget(raw);
  if (!normalized) {
    return null;
  }
  return {
    to: normalized,
    chatType: inferAlgaChatTypeFromTarget(normalized),
    investigationSessionId: normalized,
  };
}

export function looksLikeAlgaTargetId(raw: string): boolean {
  const t = raw.trim();
  if (!t) {
    return false;
  }
  if (/^alga:/i.test(t)) {
    return true;
  }
  if (/^investigation_/i.test(t)) {
    return true;
  }
  if (/^alga_dm$/i.test(t)) {
    return true;
  }
  if (/^(alert|incident_coord|incident_inv)_\d+$/i.test(t)) {
    return true;
  }
  // ObjectId-like hex (24 chars)
  if (/^[a-f0-9]{24}$/i.test(t)) {
    return true;
  }
  return false;
}

export function outboundChatIdForAlga(targetTo: string): string {
  const n = normalizeAlgaMessagingTarget(targetTo);
  if (/^alga_dm$/i.test(n)) {
    return "alga_dm";
  }
  return stripInvestigationChatPrefix(n);
}
