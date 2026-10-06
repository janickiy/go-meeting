import { useEffect, useRef, useState } from "react";
import type { InfiniteData, QueryClient } from "@tanstack/react-query";
import { ApiError } from "./api";
import type { FolderPage, FolderTarget, PersonalFolder } from "./types";

/** Keep counts fresh while preserving checkbox state for every other target. */
export function updateFolder(
  client: QueryClient,
  userId: string,
  folder: PersonalFolder,
  target?: FolderTarget,
  present?: boolean,
) {
  for (const [key, data] of client.getQueriesData<{ items: PersonalFolder[] }>({
    queryKey: ["folders", userId],
  })) {
    if (!data) continue;
    const selected = key[2] as FolderTarget | undefined;
    const same =
      target && selected?.type === target.type && selected.id === target.id;
    client.setQueryData(key, {
      ...data,
      items: data.items.map((old) =>
        old.id === folder.id
          ? { ...folder, contains: same ? present : old.contains }
          : old,
      ),
    });
  }
  client.setQueryData(["folder", userId, folder.id], {
    status: "success",
    item: folder,
  });
}

export function invalidateFolders(client: QueryClient, userId: string) {
  return Promise.all(
    ["folders", "folder", "folder-items", "folder-candidates"].map((key) =>
      client.invalidateQueries({ queryKey: [key, userId] }),
    ),
  );
}
/** A confirmed write stays visible even when its follow-up refresh is offline. */
export function updateFolderMapping(
  client: QueryClient,
  userId: string,
  folderId: string,
  target: FolderTarget,
  present: boolean,
) {
  client.setQueriesData<InfiniteData<FolderPage>>(
    { queryKey: ["folder-candidates", userId, folderId] },
    (data) =>
      data
        ? {
            ...data,
            pages: data.pages.map((page) => ({
              ...page,
              items: page.items.map((entry) =>
                entry.type === target.type && entry.item.id === target.id
                  ? { ...entry, inFolder: present }
                  : entry,
              ),
            })),
          }
        : data,
  );
  if (!present)
    client.setQueriesData<InfiniteData<FolderPage>>(
      { queryKey: ["folder-items", userId, folderId] },
      (data) =>
        data
          ? {
              ...data,
              pages: data.pages.map((page) => ({
                ...page,
                items: page.items.filter(
                  (entry) =>
                    entry.type !== target.type || entry.item.id !== target.id,
                ),
              })),
            }
          : data,
    );
}
export function folderAccessDenied(error: unknown) {
  return error instanceof ApiError && [403, 404].includes(error.status);
}
/** Remove known revoked previews synchronously; inaccessible counts are reset. */
export function purgeFolderTarget(
  client: QueryClient,
  userId: string,
  target: FolderTarget,
) {
  for (const key of ["folder-items", "folder-candidates"]) {
    void client.cancelQueries({ queryKey: [key, userId] });
    client.setQueriesData<InfiniteData<FolderPage>>(
      { queryKey: [key, userId] },
      (data) =>
        data
          ? {
              ...data,
              pages: data.pages.map((page) => ({
                ...page,
                items: page.items.filter(
                  (entry) =>
                    entry.type !== target.type || entry.item.id !== target.id,
                ),
              })),
            }
          : data,
    );
  }
  // A revoked item may belong to an unloaded folder; never invent its counts.
  void client.resetQueries({ queryKey: ["folders", userId] });
  void client.resetQueries({ queryKey: ["folder", userId] });
  void invalidateFolders(client, userId);
}
export function purgeFolder(
  client: QueryClient,
  userId: string,
  folderId: string,
) {
  for (const key of ["folder", "folder-items", "folder-candidates"]) {
    void client.cancelQueries({ queryKey: [key, userId, folderId] });
    client.removeQueries({ queryKey: [key, userId, folderId] });
  }
  void client.resetQueries({ queryKey: ["folders", userId] });
}
export function useFolderSearch(text: string) {
  const [search, setSearch] = useState(text.trim());
  useEffect(() => {
    const timer = setTimeout(() => setSearch(text.trim()), 300);
    return () => clearTimeout(timer);
  }, [text]);
  return search;
}
export function useFolderVisibility() {
  const [visible, setVisible] = useState(
    () => document.visibilityState !== "hidden",
  );
  useEffect(() => {
    const update = () => setVisible(document.visibilityState !== "hidden");
    document.addEventListener("visibilitychange", update);
    return () => document.removeEventListener("visibilitychange", update);
  }, []);
  return visible;
}
export function useFolderRequest() {
  const controller = useRef<AbortController | null>(null);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      controller.current?.abort();
    };
  }, []);
  return {
    mounted,
    signal: () => {
      controller.current?.abort();
      controller.current = new AbortController();
      return controller.current.signal;
    },
  };
}
