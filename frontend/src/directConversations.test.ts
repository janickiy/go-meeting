import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import {
  directConversationHistoryCutoff,
  resetDirectConversationHistory,
  updateDirectConversation,
} from "./directConversations";
import type { DirectConversation, FolderPage, PersonalPage } from "./types";

const initial: DirectConversation = {
  id: "chat",
  type: "direct",
  peer: { id: "bob", displayName: "Борис" },
  createdAt: "2026-10-09",
  lastMessageAt: null,
  lastMessageId: null,
  unreadCount: 2,
  preview: "Старое сообщение",
};
describe("личные кеши диалога", () => {
  it.each(["detail", "list", "summary", "folder-items", "folder-candidates"])(
    "не принимает старый HTTP-снимок при более новом пороге в %s",
    (source) => {
      const client = new QueryClient();
      const newer = {
        ...initial,
        historyClearedThrough: 15,
        preview: "",
        unreadCount: 0,
      };
      if (source === "detail")
        client.setQueryData(["personal-detail", "chat", "alice"], {
          status: "success",
          item: newer,
        });
      else if (source === "list")
        client.setQueryData(["personal-list", "alice", {}], {
          pages: [{ items: [newer], unreadCount: 0 }],
          pageParams: [undefined],
        });
      else if (source === "summary")
        client.setQueryData(["personal-summary", "alice"], {
          items: [newer],
          unreadCount: 0,
        });
      else
        client.setQueryData([source, "alice", "folder"], {
          pages: [{ items: [{ type: "conversation", item: newer }] }],
          pageParams: [undefined],
        });
      expect(directConversationHistoryCutoff(client, "chat", "alice")).toBe(15);
      expect(
        updateDirectConversation(client, "alice", {
          ...initial,
          historyClearedThrough: 5,
        }),
      ).toBe(false);
      expect(directConversationHistoryCutoff(client, "chat", "alice")).toBe(15);
      expect(
        client.getQueryData(["personal-history-cutoff", "chat", "alice"]),
      ).toBe(15);
      const detail = client.getQueryData<{ item: DirectConversation }>([
        "personal-detail",
        "chat",
        "alice",
      ]);
      expect(detail?.item.preview).not.toBe("Старое сообщение");
      client.clear();
    },
  );
  it("сразу убирает превью во всех фильтрах и папках, не меняя другую учётную запись", () => {
    const client = new QueryClient();
    const peerList = {
      pages: [{ items: [initial], unreadCount: 2 }],
      pageParams: [undefined],
    };
    for (const userId of ["alice", "bob"])
      for (const filter of [{}, { unreadOnly: true }])
        client.setQueryData(["personal-list", userId, filter], peerList);
    client.setQueryData(["personal-summary", "alice"], {
      items: [initial],
      unreadCount: 2,
    });
    for (const key of ["folder-items", "folder-candidates"])
      client.setQueryData([key, "alice", "folder"], {
        pages: [
          { items: [{ type: "conversation", item: initial, inFolder: true }] },
        ],
        pageParams: [undefined],
      });
    updateDirectConversation(client, "alice", {
      ...initial,
      historyClearedThrough: 9,
      unreadCount: 0,
      preview: "",
    });
    for (const [key, data] of client.getQueriesData<{ pages: PersonalPage[] }>({
      queryKey: ["personal-list", "alice"],
    })) {
      if ((key[2] as { unreadOnly?: boolean })?.unreadOnly)
        expect(data?.pages[0].items).toEqual([]);
      else expect(data?.pages[0].items[0].preview).toBe("");
      expect(data?.pages[0].unreadCount).toBe(0);
    }
    for (const key of ["folder-items", "folder-candidates"]) {
      const item = client.getQueryData<{ pages: FolderPage[] }>([
        key,
        "alice",
        "folder",
      ])?.pages[0].items[0];
      expect(item?.type === "conversation" && item.item.preview).toBe("");
      expect(item?.inFolder).toBe(true);
    }
    expect(client.getQueryData(["personal-list", "bob", {}])).toEqual(peerList);
    client.clear();
  });
  it("отменяет незавершённый запрос истории до ответа и освобождает курсоры чтения", async () => {
    const client = new QueryClient();
    let finish!: (data: unknown) => void;
    let requestSignal!: AbortSignal;
    const pending = client
      .fetchQuery({
        queryKey: ["personal-chat", "chat", "alice"],
        queryFn: ({ signal }) => {
          requestSignal = signal;
          return new Promise((resolve) => {
            finish = resolve;
          });
        },
      })
      .catch(() => undefined);
    client.setQueryData(["personal-chat-read", "chat", "alice"], {
      oldCursor: "9",
    });
    resetDirectConversationHistory(client, "chat", "alice");
    expect(requestSignal.aborted).toBe(true);
    finish({ pages: [{ items: [{ text: "Старая история" }] }] });
    await pending;
    expect(
      client.getQueryData(["personal-chat", "chat", "alice"]),
    ).toBeUndefined();
    expect(
      client.getQueryData(["personal-chat-read", "chat", "alice"]),
    ).toBeUndefined();
    expect(
      client.getQueryData(["personal-thread-version", "chat", "alice"]),
    ).toBe(1);
    client.clear();
  });
});
