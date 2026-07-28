import { describe, expect, it } from "vitest";
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

describe("normalizeOwnerThreadResponse", () => {
  it("normalizes items form", () => {
    expect(normalizeOwnerThreadResponse({ items: [message], total: 1 }).messages).toEqual([
      message,
    ]);
  });

  it("normalizes messages form", () => {
    expect(normalizeOwnerThreadResponse({ messages: [message] }).messages).toEqual([message]);
  });

  it("returns empty array when neither items nor messages is provided", () => {
    expect(normalizeOwnerThreadResponse({}).messages).toEqual([]);
  });
});
