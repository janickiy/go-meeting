import { describe, expect, it } from "vitest";
import { acceptPersonalEvent } from "./personalRealtime";
const event = (kind = "message.created", version = 1, id = "m") =>
  JSON.stringify({
    type: kind,
    data: {
      type: "direct",
      conversationId: "conversation",
      message: { id, version, conversationId: "conversation" },
    },
  });
describe("account message stream", () => {
  it("deduplicates retries and old versions while accepting edits/deletes", () => {
    const seen = new Map<string, number>();
    expect(acceptPersonalEvent(event(), seen)).not.toBeNull();
    expect(acceptPersonalEvent(event(), seen)).toBeNull();
    expect(
      acceptPersonalEvent(event("message.updated", 2), seen),
    ).not.toBeNull();
    expect(acceptPersonalEvent(event(), seen)).toBeNull();
    expect(
      acceptPersonalEvent(event("message.deleted", 3), seen),
    ).not.toBeNull();
  });
  it("bounds deduplication state and rejects malformed/cross-scope events", () => {
    const seen = new Map<string, number>();
    for (let i = 0; i < 500; i++)
      acceptPersonalEvent(event("message.created", 1, String(i)), seen);
    expect(seen.size).toBe(256);
    for (const raw of [
      "not JSON",
      "null",
      JSON.stringify({
        type: "message.created",
        data: { type: "conference", conversationId: "x" },
      }),
      event().replace(
        '"conversationId":"conversation"',
        '"conversationId":"foreign"',
      ),
    ])
      expect(acceptPersonalEvent(raw, seen)).toBeNull();
  });
});
