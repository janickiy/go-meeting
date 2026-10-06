import { useEffect, useRef, useState } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { CalendarDays } from "lucide-react";
import { api, ApiError } from "../api";
import { useAuth } from "../auth";
import { chatTime } from "../chatPresentation";
import {
  folderAccessDenied,
  invalidateFolders,
  purgeFolderTarget,
  updateFolder,
  updateFolderMapping,
  useFolderRequest,
  useFolderSearch,
  useFolderVisibility,
} from "../folders";
import type { FolderItem, FolderItemFilters, FolderTarget } from "../types";
import { ConversationAvatar } from "./GroupChats";
import { Button, ErrorNotice, Loading, Modal, StatusBadge } from "./ui";
import "./folders.css";

export function folderItemName(entry: FolderItem) {
  return entry.type === "conference"
    ? entry.item.title
    : entry.item.type === "group"
      ? entry.item.name
      : entry.item.peer.displayName;
}
export function folderItemTarget(entry: FolderItem): FolderTarget {
  return { type: entry.type, id: entry.item.id };
}
export function folderItemHref(entry: FolderItem) {
  return entry.type === "conversation"
    ? `/personal/${entry.item.id}`
    : `/${["finished", "cancelled"].includes(entry.item.status) ? "history" : "meetings"}/${entry.item.id}`;
}
export function FolderItemLabel({ entry }: { entry: FolderItem }) {
  const { user } = useAuth();
  return (
    <>
      {entry.type === "conversation" ? (
        <ConversationAvatar conversation={entry.item} />
      ) : (
        <span className="folder-meeting-icon">
          <CalendarDays size={22} aria-hidden="true" />
        </span>
      )}
      <span className="folder-item-copy">
        <strong>{folderItemName(entry)}</strong>
        <small>
          {entry.type === "conversation" &&
          entry.item.type === "group" &&
          entry.item.preview &&
          entry.item.lastSender
            ? `${entry.item.lastSender.id === user?.id ? "Вы" : entry.item.lastSender.displayName}: `
            : ""}
          {entry.type === "conversation"
            ? entry.item.preview ||
              (entry.item.type === "group"
                ? "Групповая переписка"
                : "Личная переписка")
            : new Date(
                entry.item.scheduledAt || entry.item.createdAt,
              ).toLocaleString("ru-RU", {
                day: "numeric",
                month: "short",
                hour: "2-digit",
                minute: "2-digit",
              })}
        </small>
        {entry.type === "conversation" && (
          <small>
            {chatTime(entry.item.lastMessageAt || entry.item.createdAt)}
          </small>
        )}
        {entry.type === "conference" &&
          typeof entry.item.participantCount === "number" && (
            <small>{entry.item.participantCount} участн.</small>
          )}
      </span>
      {entry.type === "conference" && (
        <StatusBadge status={entry.item.status} />
      )}
      {entry.type === "conversation" && entry.item.unreadCount > 0 && (
        <span
          className="count-badge"
          aria-label={`${entry.item.unreadCount} непрочитанных сообщений`}
        >
          {entry.item.unreadCount}
        </span>
      )}
    </>
  );
}
export function FolderItemPicker({
  folderId,
  onClose,
  onUnavailable,
}: {
  folderId: string;
  onClose: () => void;
  onUnavailable: () => void;
}) {
  const { user } = useAuth();
  return (
    <FolderItemPickerContent
      key={`${user?.id}:${folderId}`}
      folderId={folderId}
      onClose={onClose}
      onUnavailable={onUnavailable}
    />
  );
}
function FolderItemPickerContent({
  folderId,
  onClose,
  onUnavailable,
}: {
  folderId: string;
  onClose: () => void;
  onUnavailable: () => void;
}) {
  const { user } = useAuth();
  const client = useQueryClient();
  const request = useFolderRequest();
  const gate = useRef(false);
  const visible = useFolderVisibility();
  const [type, setType] = useState<FolderItemFilters["type"]>("all"),
    [text, setText] = useState("");
  const search = useFolderSearch(text);
  const [pending, setPending] = useState<{
    target: FolderTarget;
    present: boolean;
  } | null>(null);
  const query = useInfiniteQuery({
    queryKey: ["folder-candidates", user?.id, folderId, { type, search }],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      api.folderCandidates(folderId, { type, search }, pageParam, signal),
    getNextPageParam: (page) => page.nextCursor || undefined,
    retry: false,
    refetchInterval: visible ? 15000 : false,
  });
  const unavailable = folderAccessDenied(query.error);
  useEffect(() => {
    if (unavailable) onUnavailable();
  }, [unavailable, onUnavailable]);
  const write = useMutation({
    mutationFn: ({
      target,
      present,
    }: {
      target: FolderTarget;
      present: boolean;
    }) => api.setFolderItem(folderId, target, present, request.signal()),
    onSuccess: async ({ item }, { target, present }) => {
      if (!request.mounted.current || !user) return;
      updateFolder(client, user.id, item, target, present);
      updateFolderMapping(client, user.id, folderId, target, present);
      await invalidateFolders(client, user.id);
    },
    onError: (error, { target }) => {
      if (!request.mounted.current || !user) return;
      if (error instanceof ApiError && error.status === 404) {
        onUnavailable();
        return;
      }
      if (error instanceof ApiError && error.status === 403) {
        purgeFolderTarget(client, user.id, target);
        void query.refetch();
      }
    },
    onSettled: () => {
      gate.current = false;
      if (request.mounted.current) setPending(null);
    },
  });
  const entries = Array.from(
    new Map(
      (query.data?.pages.flatMap((page) => page.items) || []).map((entry) => [
        `${entry.type}:${entry.item.id}`,
        entry,
      ]),
    ).values(),
  );
  return (
    <Modal
      title="Добавить в папку"
      className="folder-modal folder-candidate-modal"
      onClose={() => {
        if (!gate.current) onClose();
      }}
    >
      <p className="folder-hint">
        Чаты и встречи останутся на своих страницах. Выбор сохраняется сразу.
      </p>
      <div
        className="folder-candidate-filters"
        role="group"
        aria-label="Тип элементов"
      >
        {(
          [
            ["all", "Все"],
            ["conversation", "Чаты"],
            ["conference", "Встречи"],
          ] as const
        ).map(([value, label]) => (
          <button
            type="button"
            key={value}
            aria-pressed={type === value}
            disabled={write.isPending}
            onClick={() => setType(value)}
          >
            {label}
          </button>
        ))}
      </div>
      <label className="field">
        Поиск чатов и встреч
        <input
          value={text}
          disabled={write.isPending}
          onChange={(event) => setText(event.target.value)}
          placeholder="Название или собеседник"
        />
      </label>
      <ErrorNotice error={query.error} />
      {query.isError && !unavailable && (
        <Button variant="outline" onClick={() => void query.refetch()}>
          Повторить загрузку
        </Button>
      )}
      {query.isPending && <Loading />}
      <div
        className="folder-checkboxes folder-candidates"
        aria-busy={write.isPending || undefined}
      >
        {!unavailable &&
          entries.map((entry) => {
            const target = folderItemTarget(entry);
            return (
              <label key={`${entry.type}:${entry.item.id}`}>
                <input
                  type="checkbox"
                  aria-label={folderItemName(entry)}
                  checked={
                    pending?.target.type === entry.type &&
                    pending.target.id === entry.item.id
                      ? pending.present
                      : entry.inFolder === true
                  }
                  disabled={write.isPending}
                  onChange={(event) => {
                    if (gate.current) return;
                    gate.current = true;
                    const change = { target, present: event.target.checked };
                    setPending(change);
                    write.mutate(change);
                  }}
                />
                <FolderItemLabel entry={entry} />
              </label>
            );
          })}
        {!query.isPending && !query.isError && !entries.length && (
          <p className="muted">
            {search
              ? "Ничего не найдено."
              : "Доступных чатов и встреч пока нет."}
          </p>
        )}
      </div>
      <ErrorNotice error={write.error} />
      <div className="folder-picker-actions">
        {query.hasNextPage && (
          <Button
            variant="outline"
            busy={query.isFetchingNextPage}
            disabled={write.isPending}
            onClick={() => void query.fetchNextPage()}
          >
            Ещё элементы
          </Button>
        )}
        <Button disabled={write.isPending} onClick={onClose}>
          Готово
        </Button>
      </div>
    </Modal>
  );
}
