import { describe, expect, it } from "vitest";
import {
  displayName,
  sourceAvatarBg,
  sourceColor,
  borderClass,
  groupMessagesByParent,
  rootMessages,
  childrenOf,
  avatarBg,
  avatarLetter,
  messagePermalink,
  shouldShowAgentAvatar,
} from "../src/lib/chatMessage.ts";
import type { IncidentCoordinationMessage, OwnerThreadMessage } from "../src/lib/api.ts";

const agentMsg = {
  id: "m1",
  type: "comment",
  source: "agent",
  message: "investigated",
  created_at: "2026-06-07T00:00:00Z",
  updated_at: "2026-06-07T00:00:00Z",
} satisfies OwnerThreadMessage;

const userMsg = {
  id: "m2",
  type: "comment",
  source: "user",
  message: "ack",
  username: "alice",
  created_at: "2026-06-07T00:00:01Z",
  updated_at: "2026-06-07T00:00:01Z",
} satisfies OwnerThreadMessage;

const slackMsg = {
  id: "m3",
  type: "comment",
  source: "slack",
  message: "ping",
  created_at: "2026-06-07T00:00:02Z",
  updated_at: "2026-06-07T00:00:02Z",
} satisfies OwnerThreadMessage;

const incidentMsg = (
  overrides: Partial<IncidentCoordinationMessage> = {},
): IncidentCoordinationMessage =>
  ({
    id: "i1",
    incident_number: 1,
    kind: "agent_reply",
    actor_type: "agent",
    body: "thinking",
    internal: false,
    source: "agent",
    created_at: "2026-06-07T00:00:00Z",
    updated_at: "2026-06-07T00:00:00Z",
    ...overrides,
  }) as IncidentCoordinationMessage;

describe("displayName", () => {
  it("prefers actor_display_name then username then role fallback", () => {
    expect(displayName(incidentMsg({ actor_display_name: "Hermes" }))).toBe("Hermes");
    expect(displayName(userMsg)).toBe("alice");
    expect(displayName(agentMsg)).toBe("Agent");
    expect(displayName(slackMsg)).toBe("Slack user");
    expect(displayName({ ...userMsg, source: "system" as const, username: undefined })).toBe(
      "System",
    );
  });
});

describe("sourceAvatarBg", () => {
  it("returns tinted background per source", () => {
    expect(sourceAvatarBg("agent")).toBe("bg-transparent");
    expect(sourceAvatarBg("system")).toBe("bg-blue-100 dark:bg-blue-900/30");
    expect(sourceAvatarBg("slack")).toBe("bg-emerald-600");
    expect(sourceAvatarBg("unknown")).toBe("bg-[var(--bg-secondary)]");
  });
});

describe("avatarBg", () => {
  it("for incident messages uses actor_type palette", () => {
    expect(avatarBg(incidentMsg({ actor_type: "agent" }))).toBe("bg-transparent");
    expect(avatarBg(incidentMsg({ actor_type: "system" }))).toBe("bg-slate-600");
    expect(avatarBg(incidentMsg({ actor_type: "user", source: "slack" }))).toBe("bg-emerald-600");
    expect(avatarBg(incidentMsg({ actor_type: "user" }))).toBe("bg-blue-600");
  });
});

describe("avatarLetter", () => {
  it("is the first letter of display name (uppercased)", () => {
    expect(avatarLetter(userMsg)).toBe("A");
    expect(avatarLetter(agentMsg)).toBe("A");
    expect(avatarLetter(slackMsg)).toBe("S");
  });
});

describe("sourceColor", () => {
  it("returns the per-source left border class", () => {
    expect(sourceColor("agent")).toBe("border-l-purple-500");
    expect(sourceColor("system")).toBe("border-l-blue-500");
    expect(sourceColor("slack")).toBe("border-l-emerald-500");
    expect(sourceColor("mattermost")).toBe("border-l-indigo-500");
    expect(sourceColor("user")).toBe("border-l-[var(--border-primary)]");
  });
});

describe("borderClass", () => {
  it("falls back to sourceColor except for known kinds", () => {
    expect(borderClass(incidentMsg({ kind: "decision" }))).toBe("border-l-emerald-500");
    expect(borderClass(incidentMsg({ kind: "action" }))).toBe("border-l-amber-500");
    expect(borderClass(incidentMsg({ kind: "agent_reply" }))).toBe("border-l-purple-500");
    expect(borderClass(incidentMsg({ kind: "investigation_summary" }))).toBe("border-l-cyan-500");
    expect(borderClass(incidentMsg({ kind: "chat", source: "slack" }))).toBe(
      "border-l-emerald-500",
    );
  });
});

describe("messagePermalink", () => {
  it("anchors the message id to the current page", () => {
    expect(messagePermalink("abc")).toContain("#msg-abc");
  });
});

describe("shouldShowAgentAvatar", () => {
  it("is true for agent sources only", () => {
    expect(shouldShowAgentAvatar(agentMsg)).toBe(true);
    expect(shouldShowAgentAvatar(userMsg)).toBe(false);
    expect(shouldShowAgentAvatar(slackMsg)).toBe(false);
  });
});

describe("groupMessagesByParent", () => {
  it("puts orphan roots under the empty key", () => {
    const root = incidentMsg({ id: "r1", parent_message_id: undefined });
    const child = incidentMsg({ id: "c1", parent_message_id: "r1" });
    const grouped = groupMessagesByParent([root, child]);
    expect(grouped.get("")?.length).toBe(1);
    expect(grouped.get("")?.[0]?.id).toBe("r1");
    expect(grouped.get("r1")?.length).toBe(1);
    expect(grouped.get("r1")?.[0]?.id).toBe("c1");
  });
});

describe("rootMessages", () => {
  it("includes orphans whose parent id is unknown", () => {
    const root = incidentMsg({ id: "r1", parent_message_id: undefined });
    const orphan = incidentMsg({ id: "o1", parent_message_id: "missing" });
    const grouped = groupMessagesByParent([root, orphan]);
    const roots = rootMessages([root, orphan], grouped);
    expect(roots.length).toBe(2);
    expect(roots[0]?.id).toBe("r1");
    expect(roots[1]?.id).toBe("o1");
  });
});

describe("childrenOf", () => {
  it("returns children sorted by created_at ascending", () => {
    const child1 = incidentMsg({
      id: "c1",
      parent_message_id: "r1",
      created_at: "2026-06-07T00:00:02Z",
    });
    const child2 = incidentMsg({
      id: "c2",
      parent_message_id: "r1",
      created_at: "2026-06-07T00:00:01Z",
    });
    const grouped = groupMessagesByParent([child1, child2]);
    const ordered = childrenOf("r1", grouped);
    expect(ordered.map((m) => m.id)).toEqual(["c2", "c1"]);
  });
});
