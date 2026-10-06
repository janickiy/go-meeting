import { describe, expect, it } from "vitest";
import {
  acceptPersonalEvent,
  revokePersonalConversation,
} from "./personalRealtime";
import { QueryClient } from "@tanstack/react-query";
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
  it("accepts group messages and typed membership lifecycle without accepting conference events", () => {
    const seen = new Map<string, number>();
    expect(
      acceptPersonalEvent(
        event().replace('"type":"direct"', '"type":"group"'),
        seen,
      ),
    ).not.toBeNull();
    for (const type of [
      "conversation.member.added",
      "conversation.member.updated",
      "conversation.member.removed",
    ]) {
      expect(
        acceptPersonalEvent(
          JSON.stringify({
            type,
            data: { conversationId: "group", type: "group", userId: "member" },
          }),
          seen,
        ),
      ).not.toBeNull();
      expect(
        acceptPersonalEvent(
          JSON.stringify({
            type,
            data: { conversationId: "group", type: "group" },
          }),
          seen,
        ),
      ).toBeNull();
    }
  });
  it("immediately removes revoked previews from every cached filter and private scope while offline", () => {
    const client = new QueryClient();
    const removed = { id: "group", unreadCount: 3, preview: "Private preview" },
      retained = { id: "direct", unreadCount: 2, preview: "Retained" };
    for (const filter of [{}, { type: "group" }, { unreadOnly: true }])
      client.setQueryData(["personal-list", "user", filter], {
        pages: [{ items: [removed, retained], unreadCount: 5 }],
        pageParams: [undefined],
      });
    client.setQueryData(["personal-summary", "user"], {
      items: [removed],
      unreadCount: 5,
    });
    for (const key of [
      "personal-chat",
      "personal-chat-read",
      "personal-detail",
      "group-members",
    ])
      client.setQueryData([key, "group", "user"], { item: removed });
    revokePersonalConversation(client, "group", "user");
    for (const [, data] of client.getQueriesData<{
      pages: { items: (typeof removed)[]; unreadCount: number }[];
    }>({ queryKey: ["personal-list", "user"] })) {
      expect(data?.pages[0].items).toEqual([retained]);
      expect(data?.pages[0].unreadCount).toBe(2);
    }
    expect(client.getQueryData(["personal-summary", "user"])).toEqual({
      items: [],
      unreadCount: 2,
    });
    for (const key of [
      "personal-chat",
      "personal-chat-read",
      "personal-detail",
      "group-members",
    ])
      expect(client.getQueryData([key, "group", "user"])).toBeUndefined();
    client.clear();
  });
});
