import { useState } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "../api";
import type { DirectConversation, User } from "../types";
import { DirectUserInfoModal } from "./DirectConversationActions";

const authState = vi.hoisted(() => ({ user: undefined as User | undefined }));
vi.mock("../auth", () => ({ useAuth: () => authState }));

const actor: User = {
  id: "alice",
  email: "alice@example.test",
  displayName: "Алиса",
  createdAt: "2026-10-09T00:00:00Z",
  updatedAt: "2026-10-09T00:00:00Z",
};
const conversation: DirectConversation = {
  id: "direct",
  type: "direct",
  peer: { id: "bob", displayName: "Борис" },
  createdAt: "2026-10-09T00:00:00Z",
  lastMessageAt: null,
  lastMessageId: null,
  preview: "Личная переписка",
  unreadCount: 2,
};
const clients: QueryClient[] = [];
type PresenceResponse = Awaited<ReturnType<typeof api.personalPeerPresence>>;

/**
 * Создаёт подтверждённый ответ только для указанной области личного диалога.
 * @args online — статус собеседника; item — проверяемый диалог.
 * @return Ответ защищённого API со статусом и идентификаторами.
 */
function response(online: boolean, item = conversation): PresenceResponse {
  return {
    status: "success",
    item: { conversationId: item.id, peerId: item.peer.id, online },
  };
}

/**
 * Позволяет проверить загрузку и поздний ответ отменённого запроса.
 * @return Promise и функция его завершения.
 */
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((finish) => {
    resolve = finish;
  });
  return { promise, resolve };
}

/**
 * Закрывает окно реальным размонтированием, как родительские компоненты приложения.
 * @args item — текущий диалог; onClose — наблюдение за закрытием.
 * @return Окно и кнопка его повторного открытия.
 */
function Harness({
  item,
  onClose,
}: {
  item: DirectConversation;
  onClose: () => void;
}) {
  const [open, setOpen] = useState(true);
  return (
    <>
      <button onClick={() => setOpen(true)}>Показать информацию</button>
      {open && (
        <DirectUserInfoModal
          conversation={item}
          onClose={() => {
            setOpen(false);
            onClose();
          }}
        />
      )}
    </>
  );
}

/**
 * Подключает отдельный кеш и позволяет менять область без размонтирования окна.
 * @args item — начальный диалог; client — необязательный заранее заполненный кеш.
 * @return Кеш, наблюдатель закрытия и функция смены диалога.
 */
function show(item = conversation, client = new QueryClient()) {
  clients.push(client);
  const onClose = vi.fn();
  const ui = (current: DirectConversation) => (
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <Harness item={current} onClose={onClose} />
      </MemoryRouter>
    </QueryClientProvider>
  );
  const view = render(ui(item));
  return {
    client,
    onClose,
    unmount: view.unmount,
    change: (current = item) => view.rerender(ui(current)),
  };
}

/**
 * Ждёт реальный интервал React Query, включая завершение отмены и сборку кеша.
 * @args milliseconds — длительность опроса в миллисекундах.
 * @return Завершённое обновление React после тиков интервала.
 */
async function poll(milliseconds = 1000) {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, milliseconds));
  });
}

beforeEach(() => {
  authState.user = { ...actor };
  vi.spyOn(api, "personalPeerPresence").mockResolvedValue(response(false));
});

afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
});

describe("подтверждённое присутствие в информации личного собеседника", () => {
  it("показывает загрузку, затем online → offline при опросе и прекращает запросы после закрытия", async () => {
    const pending = deferred<PresenceResponse>();
    vi.mocked(api.personalPeerPresence)
      .mockImplementationOnce(() => pending.promise)
      .mockResolvedValue(response(false));
    const state = show();
    expect(screen.getByRole("status")).toHaveTextContent("Проверяем статус…");
    expect(api.personalPeerPresence).toHaveBeenCalledWith(
      "direct",
      expect.any(AbortSignal),
    );
    await act(async () => pending.resolve(response(true)));
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(/^В сети$/),
    );
    expect(screen.getByRole("status")).toHaveClass(
      "personal-user-presence-online",
    );
    await poll();
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(/^не в сети\.$/),
    );
    expect(screen.getByRole("status")).not.toHaveClass(
      "personal-user-presence-online",
    );
    fireEvent.click(screen.getByRole("button", { name: "Закрыть" }));
    expect(state.onClose).toHaveBeenCalledOnce();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    const requests = vi.mocked(api.personalPeerPresence).mock.calls.length;
    await poll(1500);
    expect(api.personalPeerPresence).toHaveBeenCalledTimes(requests);
    await waitFor(() =>
      expect(
        state.client.getQueriesData({ queryKey: ["personal-peer-presence"] }),
      ).toEqual([]),
    );
  });

  it("не принимает ошибку сервиса за offline и не показывает прежний online после ошибки опроса", async () => {
    vi.mocked(api.personalPeerPresence)
      .mockResolvedValueOnce(response(true))
      .mockRejectedValue(new ApiError(503, "Сервис присутствия недоступен"));
    show();
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(/^В сети$/),
    );
    await poll();
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(
        /^Статус временно недоступен\.$/,
      ),
    );
    expect(screen.getByRole("status")).not.toHaveClass(
      "personal-user-presence-online",
    );
    expect(screen.getByRole("status")).not.toHaveTextContent(/^не в сети\.$/);
    expect(screen.getByRole("dialog")).toBeVisible();
    expect(api.personalPeerPresence).toHaveBeenCalledTimes(2);
  });

  it.each([
    [
      "другой диалог",
      { conversationId: "foreign", peerId: "bob", online: true },
    ],
    [
      "другой собеседник",
      { conversationId: "direct", peerId: "foreign", online: true },
    ],
    [
      "небулевый статус",
      { conversationId: "direct", peerId: "bob", online: "false" },
    ],
    ["неполный ответ", undefined],
  ])("считает неподтверждённым DTO: %s", async (_label, item) => {
    vi.mocked(api.personalPeerPresence).mockResolvedValue({
      status: "success",
      item,
    } as PresenceResponse);
    show();
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(
        /^Статус временно недоступен\.$/,
      ),
    );
    expect(screen.getByRole("status")).not.toHaveClass(
      "personal-user-presence-online",
    );
  });

  it.each(["учётная запись", "диалог", "собеседник"] as const)(
    "не переносит online в новую область: %s",
    async (scope) => {
      vi.mocked(api.personalPeerPresence).mockResolvedValueOnce(response(true));
      const state = show();
      await waitFor(() =>
        expect(screen.getByRole("status")).toHaveTextContent(/^В сети$/),
      );
      const next = deferred<PresenceResponse>();
      vi.mocked(api.personalPeerPresence).mockImplementation(
        () => next.promise,
      );
      const nextConversation = {
        ...conversation,
        ...(scope === "диалог" ? { id: "another-direct" } : {}),
        peer:
          scope === "собеседник"
            ? { id: "charlie", displayName: "Константин" }
            : conversation.peer,
      };
      if (scope === "учётная запись")
        authState.user = { ...actor, id: "another-account" };
      state.change(nextConversation);
      expect(screen.getByRole("status")).toHaveTextContent(
        /^Проверяем статус…$/,
      );
      expect(screen.getByRole("status")).not.toHaveClass(
        "personal-user-presence-online",
      );
      await act(async () => next.resolve(response(false, nextConversation)));
      await waitFor(() =>
        expect(screen.getByRole("status")).toHaveTextContent(/^не в сети\.$/),
      );
      expect(
        state.client.getQueryData([
          "personal-peer-presence",
          nextConversation.id,
          authState.user!.id,
          nextConversation.peer.id,
        ]),
      ).toEqual(response(false, nextConversation));
    },
  );

  it("не показывает кешированный online при повторном открытии и отменяет незавершённый запрос при закрытии", async () => {
    vi.mocked(api.personalPeerPresence).mockResolvedValueOnce(response(true));
    const state = show();
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(/^В сети$/),
    );
    fireEvent.click(screen.getByRole("button", { name: "Закрыть" }));
    await poll(0);
    await waitFor(() =>
      expect(
        state.client.getQueriesData({ queryKey: ["personal-peer-presence"] }),
      ).toEqual([]),
    );
    const pending = deferred<PresenceResponse>();
    vi.mocked(api.personalPeerPresence).mockImplementation(
      () => pending.promise,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Показать информацию" }),
    );
    expect(screen.getByRole("status")).toHaveTextContent(/^Проверяем статус…$/);
    const signal = vi.mocked(api.personalPeerPresence).mock.calls.at(-1)![1]!;
    expect(signal.aborted).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "Закрыть" }));
    expect(signal.aborted).toBe(true);
    await act(async () => pending.resolve(response(true)));
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    const requests = vi.mocked(api.personalPeerPresence).mock.calls.length;
    await poll(1500);
    expect(api.personalPeerPresence).toHaveBeenCalledTimes(requests);
  });

  it.each([403, 404])(
    "при %s закрывает окно, очищает только отозванную переписку и прекращает опрос",
    async (status) => {
      const retained = { ...conversation, id: "retained", unreadCount: 1 };
      const client = new QueryClient();
      const page = { items: [conversation, retained], unreadCount: 3 };
      client.setQueryData(["personal-list", actor.id, {}], {
        pages: [page],
        pageParams: [undefined],
      });
      client.setQueryData(["personal-summary", actor.id], page);
      for (const key of [
        "personal-detail",
        "personal-chat",
        "personal-chat-read",
      ])
        client.setQueryData([key, conversation.id, actor.id], {
          item: conversation,
        });
      client.setQueryData(
        ["personal-peer-presence", conversation.id, actor.id, "old-peer"],
        response(true),
      );
      client.setQueryData(
        ["personal-peer-presence", retained.id, actor.id, "retained-peer"],
        response(true, retained),
      );
      client.setQueryData(["folder-items", actor.id, "folder", {}], {
        pages: [
          {
            items: [
              { type: "conversation", item: conversation },
              { type: "conversation", item: retained },
            ],
          },
        ],
        pageParams: [undefined],
      });
      vi.mocked(api.personalPeerPresence).mockRejectedValue(
        new ApiError(status, "Нет доступа"),
      );
      const state = show(conversation, client);
      await waitFor(() => expect(state.onClose).toHaveBeenCalledOnce());
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(client.getQueryData(["personal-summary", actor.id])).toEqual({
        items: [retained],
        unreadCount: 1,
      });
      expect(client.getQueryData(["personal-list", actor.id, {}])).toEqual({
        pages: [{ items: [retained], unreadCount: 1 }],
        pageParams: [undefined],
      });
      expect(
        client.getQueryData(["folder-items", actor.id, "folder", {}]),
      ).toEqual({
        pages: [{ items: [{ type: "conversation", item: retained }] }],
        pageParams: [undefined],
      });
      for (const key of [
        "personal-detail",
        "personal-chat",
        "personal-chat-read",
        "personal-peer-presence",
      ])
        expect(
          client.getQueriesData({ queryKey: [key, conversation.id, actor.id] }),
        ).toEqual([]);
      expect(
        client.getQueryData([
          "personal-peer-presence",
          retained.id,
          actor.id,
          "retained-peer",
        ]),
      ).toEqual(response(true, retained));
      await poll(1500);
      expect(api.personalPeerPresence).toHaveBeenCalledOnce();
    },
  );

  it.each(["без пользователя", "гость встречи"])(
    "не читает защищённое присутствие: %s",
    async (kind) => {
      authState.user =
        kind === "без пользователя"
          ? undefined
          : { ...actor, guestConferenceId: "guest-meeting" };
      show();
      await poll(1500);
      expect(api.personalPeerPresence).not.toHaveBeenCalled();
      expect(screen.getByRole("status")).not.toHaveClass(
        "personal-user-presence-online",
      );
    },
  );
});
