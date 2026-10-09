import type { InfiniteData, QueryClient } from "@tanstack/react-query";
import type {
  DirectConversation,
  FolderPage,
  Item,
  PersonalPage,
} from "./types";

/**
 * Заменяет личные данные диалога во всех загруженных списках, включая папки.
 *
 * @args client — кеш запросов; userId — владелец личных настроек; item — подтверждённый ответ сервера.
 * @return true, если снимок принят; false для запоздалого ответа с меньшим порогом истории.
 */
export function updateDirectConversation(
  client: QueryClient,
  userId: string,
  item: DirectConversation,
) {
  const knownCutoff = directConversationHistoryCutoff(client, item.id, userId);
  // Отдельный монотонный порог переживает отмену и замену снимков обычных запросов.
  client.setQueryData(
    ["personal-history-cutoff", item.id, userId],
    Math.max(knownCutoff, item.historyClearedThrough || 0),
  );
  for (const key of [
    "personal-list",
    "personal-summary",
    "folder-items",
    "folder-candidates",
  ])
    void client.cancelQueries({ queryKey: [key, userId] });
  void client.cancelQueries({ queryKey: ["personal-detail", item.id, userId] });
  if ((item.historyClearedThrough || 0) < knownCutoff) {
    // Старый ответ не должен вернуть очищенное превью, счётчик или настройки из прежнего снимка.
    for (const key of [
      "personal-list",
      "personal-summary",
      "folder-items",
      "folder-candidates",
    ])
      void client.invalidateQueries({ queryKey: [key, userId] });
    void client.invalidateQueries({
      queryKey: ["personal-detail", item.id, userId],
    });
    return false;
  }
  // Новый порог может прийти вместе с настройками, когда событие самой очистки было потеряно.
  if ((item.historyClearedThrough || 0) > knownCutoff)
    resetDirectConversationHistory(client, item.id, userId);
  const previous = client.getQueryData<Item<DirectConversation>>([
    "personal-detail",
    item.id,
    userId,
  ])?.item;
  let oldUnread = previous?.unreadCount || 0;
  for (const [, list] of client.getQueriesData<InfiniteData<PersonalPage>>({
    queryKey: ["personal-list", userId],
  }))
    for (const page of list?.pages || [])
      oldUnread = Math.max(
        oldUnread,
        page.items.find((old) => old.id === item.id)?.unreadCount || 0,
      );
  const removedUnread = Math.max(0, oldUnread - item.unreadCount);
  client.setQueryData(["personal-detail", item.id, userId], {
    status: "success",
    item,
  });
  for (const [key, current] of client.getQueriesData<
    InfiniteData<PersonalPage>
  >({ queryKey: ["personal-list", userId] })) {
    if (!current) continue;
    const unreadOnly = (key[2] as { unreadOnly?: boolean } | undefined)
      ?.unreadOnly;
    client.setQueryData(key, {
      ...current,
      pages: current.pages.map((page) => ({
        ...page,
        items: page.items
          .map((old) => (old.id === item.id ? item : old))
          .filter((old) => !unreadOnly || old.unreadCount > 0),
        unreadCount: Math.max(0, page.unreadCount - removedUnread),
      })),
    });
  }
  client.setQueryData<PersonalPage>(["personal-summary", userId], (current) =>
    current
      ? {
          ...current,
          items: current.items.map((old) => (old.id === item.id ? item : old)),
          unreadCount: Math.max(0, current.unreadCount - removedUnread),
        }
      : current,
  );
  for (const key of ["folder-items", "folder-candidates"])
    client.setQueriesData<InfiniteData<FolderPage>>(
      { queryKey: [key, userId] },
      (current) =>
        current
          ? {
              ...current,
              pages: current.pages.map((page) => ({
                ...page,
                items: page.items.map((entry) =>
                  entry.type === "conversation" && entry.item.id === item.id
                    ? { ...entry, item }
                    : entry,
                ),
              })),
            }
          : current,
    );
  return true;
}

/**
 * Возвращает известный личный порог истории для отсечения задержанных событий.
 * @args client — кеш запросов; id — диалог; userId — учётная запись владельца порога.
 * @return Номер последнего скрытого сообщения либо ноль до первой очистки.
 */
export function directConversationHistoryCutoff(
  client: QueryClient,
  id: string,
  userId: string,
) {
  let cutoff = Math.max(
    client.getQueryData<number>(["personal-history-cutoff", id, userId]) || 0,
    client.getQueryData<Item<DirectConversation>>([
      "personal-detail",
      id,
      userId,
    ])?.item.historyClearedThrough || 0,
  );
  for (const [, data] of client.getQueriesData<InfiniteData<PersonalPage>>({
    queryKey: ["personal-list", userId],
  }))
    for (const page of data?.pages || [])
      for (const item of page.items)
        if (item.type === "direct" && item.id === id)
          cutoff = Math.max(cutoff, item.historyClearedThrough || 0);
  for (const item of client.getQueryData<PersonalPage>([
    "personal-summary",
    userId,
  ])?.items || [])
    if (item.type === "direct" && item.id === id)
      cutoff = Math.max(cutoff, item.historyClearedThrough || 0);
  for (const key of ["folder-items", "folder-candidates"])
    for (const [, data] of client.getQueriesData<InfiniteData<FolderPage>>({
      queryKey: [key, userId],
    }))
      for (const page of data?.pages || [])
        for (const entry of page.items)
          if (
            entry.type === "conversation" &&
            entry.item.type === "direct" &&
            entry.item.id === id
          )
            cutoff = Math.max(cutoff, entry.item.historyClearedThrough || 0);
  return cutoff;
}

/**
 * Запоминает точный порог сервера до удаления остальных личных кешей.
 * @args client — кеш; id — диалог; userId — владелец; cutoff — проверенный номер последнего скрытого сообщения.
 * @return true, если порог повысился; старые и неверные значения не ослабляют защиту.
 */
export function rememberDirectConversationHistoryCutoff(
  client: QueryClient,
  id: string,
  userId: string,
  cutoff: number,
) {
  if (!Number.isSafeInteger(cutoff) || cutoff < 0) return false;
  const known = directConversationHistoryCutoff(client, id, userId);
  client.setQueryData(
    ["personal-history-cutoff", id, userId],
    Math.max(known, cutoff),
  );
  return cutoff > known;
}

/**
 * Убирает старые сообщения, ссылки и курсоры чтения из кеша и пересоздаёт редактор диалога.
 * Пересоздание освобождает загрузки, черновики, ответы и локальные ссылки вложений.
 *
 * @args client — кеш запросов; id — личный диалог; userId — текущая учётная запись.
 */
export function resetDirectConversationHistory(
  client: QueryClient,
  id: string,
  userId: string,
) {
  for (const key of ["personal-chat", "personal-chat-read"]) {
    void client.cancelQueries({ queryKey: [key, id, userId] });
    client.removeQueries({ queryKey: [key, id, userId] });
  }
  client.setQueryData<number>(
    ["personal-thread-version", id, userId],
    (version) => (version || 0) + 1,
  );
}

/**
 * Синхронизирует только настройку, которую сервер проверил непосредственно перед отправкой кадра.
 * Старый локальный снимок не может отменить это разрешение и подавить актуальное уведомление.
 * @args client — кеш запросов; id — диалог; userId — владелец настройки; enabled — свежий серверный признак.
 */
export function observeDirectConversationNotifications(
  client: QueryClient,
  id: string,
  userId: string,
  enabled: boolean,
) {
  const detail = client.getQueryData<Item<DirectConversation>>([
    "personal-detail",
    id,
    userId,
  ]);
  if (detail?.item.type === "direct")
    client.setQueryData(["personal-detail", id, userId], {
      ...detail,
      item: { ...detail.item, notificationsEnabled: enabled },
    });
  const update = (item: PersonalPage["items"][number]) =>
    item.id === id && item.type === "direct"
      ? { ...item, notificationsEnabled: enabled }
      : item;
  client.setQueriesData<InfiniteData<PersonalPage>>(
    { queryKey: ["personal-list", userId] },
    (current) =>
      current
        ? {
            ...current,
            pages: current.pages.map((page) => ({
              ...page,
              items: page.items.map(update),
            })),
          }
        : current,
  );
  client.setQueryData<PersonalPage>(["personal-summary", userId], (current) =>
    current ? { ...current, items: current.items.map(update) } : current,
  );
  for (const key of ["folder-items", "folder-candidates"])
    client.setQueriesData<InfiniteData<FolderPage>>(
      { queryKey: [key, userId] },
      (current) =>
        current
          ? {
              ...current,
              pages: current.pages.map((page) => ({
                ...page,
                items: page.items.map((entry) =>
                  entry.type === "conversation"
                    ? { ...entry, item: update(entry.item) }
                    : entry,
                ),
              })),
            }
          : current,
    );
}
