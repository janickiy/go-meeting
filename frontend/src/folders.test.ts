import { describe, expect, it } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import {
  purgeFolder,
  purgeFolderTarget,
  updateFolder,
  updateFolderMapping,
} from "./folders";
import {
  acceptFolderEvent,
  revokePersonalConversation,
} from "./personalRealtime";
import type { PersonalFolder } from "./types";
const folder: PersonalFolder = {
  id: "folder",
  name: "Работа",
  position: 0,
  createdAt: "now",
  updatedAt: "now",
  itemCount: 2,
  conversationCount: 1,
  conferenceCount: 1,
};
describe("private folder cache", () => {
  it("purges revoked previews across all filters and candidates without leaking stale counts or affecting another account", () => {
    const client = new QueryClient();
    const removed = {
        type: "conversation",
        item: { id: "group", preview: "Private" },
      },
      retained = {
        type: "conference",
        item: { id: "group", title: "Allowed meeting" },
      };
    for (const key of ["folder-items", "folder-candidates"])
      for (const filter of [
        { type: "all" },
        { type: "conversation", search: "Private" },
      ])
        client.setQueryData([key, "alice", "folder", filter], {
          pages: [{ items: [removed, retained] }],
          pageParams: [undefined],
        });
    client.setQueryData(["folder-items", "bob", "folder"], {
      pages: [{ items: [removed] }],
    });
    client.setQueryData(["folders", "alice"], { items: [folder] });
    client.setQueryData(["folder", "alice", "folder"], { item: folder });
    revokePersonalConversation(client, "group", "alice");
    for (const key of ["folder-items", "folder-candidates"])
      for (const [, data] of client.getQueriesData<{
        pages: { items: unknown[] }[];
      }>({ queryKey: [key, "alice"] }))
        expect(data?.pages[0].items).toEqual([retained]);
    expect(client.getQueryData(["folders", "alice"])).toBeUndefined();
    expect(client.getQueryData(["folder", "alice", "folder"])).toBeUndefined();
    expect(client.getQueryData(["folder-items", "bob", "folder"])).toEqual({
      pages: [{ items: [removed] }],
    });
    client.clear();
  });
  it("matches both kind and ID when purging a meeting, then deletes only the selected folder scope", () => {
    const client = new QueryClient();
    const conversation = { type: "conversation", item: { id: "same" } },
      conference = { type: "conference", item: { id: "same" } };
    client.setQueryData(["folder-items", "alice", "folder"], {
      pages: [{ items: [conversation, conference] }],
    });
    purgeFolderTarget(client, "alice", { type: "conference", id: "same" });
    expect(client.getQueryData(["folder-items", "alice", "folder"])).toEqual({
      pages: [{ items: [conversation] }],
    });
    client.setQueryData(["folder-items", "alice", "other"], { pages: [] });
    purgeFolder(client, "alice", "folder");
    expect(
      client.getQueryData(["folder-items", "alice", "folder"]),
    ).toBeUndefined();
    expect(client.getQueryData(["folder-items", "alice", "other"])).toEqual({
      pages: [],
    });
    client.clear();
  });
  it("updates metadata but preserves independent checkbox state for every other target", () => {
    const client = new QueryClient();
    const selected = { type: "conversation" as const, id: "a" },
      other = { type: "conversation" as const, id: "b" };
    client.setQueryData(["folders", "alice", selected], {
      items: [{ ...folder, contains: false }],
    });
    client.setQueryData(["folders", "alice", other], {
      items: [{ ...folder, contains: false }],
    });
    updateFolder(client, "alice", { ...folder, itemCount: 3 }, selected, true);
    expect(
      client.getQueryData<{ items: PersonalFolder[] }>([
        "folders",
        "alice",
        selected,
      ])?.items[0].contains,
    ).toBe(true);
    expect(
      client.getQueryData<{ items: PersonalFolder[] }>([
        "folders",
        "alice",
        other,
      ])?.items[0],
    ).toMatchObject({ contains: false, itemCount: 3 });
    client.clear();
  });
  it("accepts only the agreed minimal folder event vocabulary", () => {
    for (const type of [
      "folder.created",
      "folder.updated",
      "folder.deleted",
      "folder.items.updated",
    ])
      expect(
        acceptFolderEvent(JSON.stringify({ type, data: { folderId: "f" } })),
      ).toEqual({ type, data: { folderId: "f" } });
    expect(
      acceptFolderEvent(JSON.stringify({ type: "folder.reordered", data: {} })),
    ).not.toBeNull();
    for (const raw of [
      "null",
      "invalid",
      JSON.stringify({ type: "folder.updated", data: {} }),
      JSON.stringify({ type: "folder.updated", data: { folderId: 5 } }),
      JSON.stringify({ type: "folder.created", data: [] }),
      JSON.stringify({ type: "folder.pinned", data: { folderId: "f" } }),
    ])
      expect(acceptFolderEvent(raw)).toBeNull();
  });
});
it("updates a confirmed unlink in every filter of just that folder, preserving sibling mappings", () => {
  const client = new QueryClient();
  const entry = { type: "conversation", item: { id: "chat" }, inFolder: true };
  for (const key of ["folder-items", "folder-candidates"])
    for (const id of ["one", "two"])
      for (const filter of [{ type: "all" }, { type: "conversation" }])
        client.setQueryData([key, "alice", id, filter], {
          pages: [{ items: [entry] }],
        });
  updateFolderMapping(
    client,
    "alice",
    "one",
    { type: "conversation", id: "chat" },
    false,
  );
  for (const [, data] of client.getQueriesData<{
    pages: { items: unknown[] }[];
  }>({ queryKey: ["folder-items", "alice", "one"] }))
    expect(data?.pages[0].items).toEqual([]);
  for (const [, data] of client.getQueriesData<{
    pages: { items: { inFolder?: boolean }[] }[];
  }>({ queryKey: ["folder-candidates", "alice", "one"] }))
    expect(data?.pages[0].items[0].inFolder).toBe(false);
  expect(
    client.getQueryData(["folder-items", "alice", "two", { type: "all" }]),
  ).toEqual({ pages: [{ items: [entry] }] });
  client.clear();
});
