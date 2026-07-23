import assert from "node:assert/strict";
import { test } from "node:test";
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
): IncidentCoordinationMessage => ({
  id: "i1",
  kind: "agent_reply",
  source: "agent",
  actor_type: "agent",
  message: "thinking",
  created_at: "2026-06-07T00:00:00Z",
  ...overrides,
});

test("displayName prefers actor_display_name then username then role fallback", () => {
  assert.equal(displayName(incidentMsg({ actor_display_name: "Hermes" })), "Hermes");
  assert.equal(displayName(userMsg), "alice");
  assert.equal(displayName(agentMsg), "Agent");
  assert.equal(displayName(slackMsg), "Slack user");
  assert.equal(
    displayName({ ...userMsg, source: "system" as const, username: undefined }),
    "System",
  );
});

test("sourceAvatarBg returns tinted background per source", () => {
  assert.equal(sourceAvatarBg("agent"), "bg-transparent");
  assert.equal(sourceAvatarBg("system"), "bg-blue-100 dark:bg-blue-900/30");
  assert.equal(sourceAvatarBg("slack"), "bg-emerald-600");
  assert.equal(sourceAvatarBg("unknown"), "bg-[var(--bg-secondary)]");
});

test("avatarBg for incident messages uses actor_type palette", () => {
  assert.equal(avatarBg(incidentMsg({ actor_type: "agent" })), "bg-transparent");
  assert.equal(avatarBg(incidentMsg({ actor_type: "system" })), "bg-slate-600");
  assert.equal(avatarBg(incidentMsg({ actor_type: "user", source: "slack" })), "bg-emerald-600");
  assert.equal(avatarBg(incidentMsg({ actor_type: "user" })), "bg-blue-600");
});

test("avatarLetter is the first letter of display name (uppercased)", () => {
  assert.equal(avatarLetter(userMsg), "A");
  assert.equal(avatarLetter(agentMsg), "A");
  assert.equal(avatarLetter(slackMsg), "S");
});

test("sourceColor returns the per-source left border class", () => {
  assert.equal(sourceColor("agent"), "border-l-purple-500");
  assert.equal(sourceColor("system"), "border-l-blue-500");
  assert.equal(sourceColor("slack"), "border-l-emerald-500");
  assert.equal(sourceColor("mattermost"), "border-l-indigo-500");
  assert.equal(sourceColor("user"), "border-l-[var(--border-primary)]");
});

test("borderClass falls back to sourceColor except for known kinds", () => {
  assert.equal(borderClass(incidentMsg({ kind: "decision" })), "border-l-emerald-500");
  assert.equal(borderClass(incidentMsg({ kind: "action" })), "border-l-amber-500");
  assert.equal(borderClass(incidentMsg({ kind: "agent_reply" })), "border-l-purple-500");
  assert.equal(borderClass(incidentMsg({ kind: "investigation_summary" })), "border-l-cyan-500");
  assert.equal(
    borderClass(incidentMsg({ kind: "comment", source: "slack" })),
    "border-l-emerald-500",
  );
});

test("messagePermalink anchors the message id to the current page", () => {
  // jsdom-less env: window is undefined → the fallback format is used.
  const prevWindow = (globalThis as { window?: unknown }).window;
  delete (globalThis as { window?: unknown }).window;
  try {
    assert.equal(messagePermalink("abc"), "#msg-abc");
  } finally {
    if (prevWindow !== undefined) (globalThis as { window?: unknown }).window = prevWindow;
  }
});

test("shouldShowAgentAvatar is true for agent sources only", () => {
  assert.equal(shouldShowAgentAvatar(agentMsg), true);
  assert.equal(shouldShowAgentAvatar(userMsg), false);
  assert.equal(shouldShowAgentAvatar(slackMsg), false);
});

test("groupMessagesByParent puts orphan roots under the empty key", () => {
  const root = incidentMsg({ id: "r1", parent_message_id: null });
  const child = incidentMsg({ id: "c1", parent_message_id: "r1" });
  const grouped = groupMessagesByParent([root, child]);
  assert.equal(grouped.get("")?.length, 1);
  assert.equal(grouped.get("")?.[0]?.id, "r1");
  assert.equal(grouped.get("r1")?.length, 1);
  assert.equal(grouped.get("r1")?.[0]?.id, "c1");
});

test("rootMessages includes orphans whose parent id is unknown", () => {
  const root = incidentMsg({ id: "r1", parent_message_id: null });
  const orphan = incidentMsg({ id: "o1", parent_message_id: "missing" });
  const grouped = groupMessagesByParent([root, orphan]);
  const roots = rootMessages([root, orphan], grouped);
  assert.equal(roots.length, 2);
  assert.equal(roots[0]?.id, "r1");
  assert.equal(roots[1]?.id, "o1");
});

test("childrenOf returns children sorted by created_at ascending", () => {
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
  assert.deepEqual(
    ordered.map((m) => m.id),
    ["c2", "c1"],
  );
});
