import assert from "node:assert/strict";
import { test } from "node:test";
import { normalizeOwnerThreadResponse } from "../src/lib/ownerThread.ts";
import type { OwnerThreadMessage } from "../src/lib/api";

const message = {
  id: "msg-1",
  type: "comment",
  source: "agent",
  message: "checked the alert",
  created_at: "2026-06-07T00:00:00Z",
  updated_at: "2026-06-07T00:00:00Z",
} satisfies OwnerThreadMessage;

test("normalizes items form", () => {
  assert.deepEqual(normalizeOwnerThreadResponse({ items: [message], total: 1 }).messages, [
    message,
  ]);
});

test("normalizes messages form", () => {
  assert.deepEqual(normalizeOwnerThreadResponse({ messages: [message] }).messages, [message]);
});

test("returns empty array when neither items nor messages is provided", () => {
  assert.deepEqual(normalizeOwnerThreadResponse({}).messages, []);
});
