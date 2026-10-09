import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "../api";
import type { DirectConversation, User } from "../types";
import { DirectUserInfoModal } from "./DirectConversationActions";
import { PersonalPeerStatus } from "./PersonalPeerStatus";
import { PersonalPeerPresenceProvider } from "../usePersonalPeerPresence";

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
  unreadCount: 0,
};
const clients: QueryClient[] = [];
type PresenceResponse = Awaited<ReturnType<typeof api.personalPeerPresence>>;

/**
 * Подтверждает присутствие только в заданной области диалога и собеседника.
 * @args online — статус собеседника; item — проверяемый диалог.
 * @return Ответ защищённого API с идентификаторами области.
 */
function response(online: boolean, item = conversation): PresenceResponse {
  return {
    status: "success",
    item: { conversationId: item.id, peerId: item.peer.id, online },
  };
}

/**
 * Позволяет завершить запрос после смены области или размонтирования.
 * @return Незавершённый запрос и функции успешного или ошибочного завершения.
 */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((finish, fail) => {
    resolve = finish;
    reject = fail;
  });
  return { promise, resolve, reject };
}

/**
 * Подключает отдельный кеш и меняет диалог без размонтирования заголовка.
 * @args item — начальный диалог; client — необязательный заполненный кеш.
 * @return Кеш, наблюдатель отказа доступа и управление текущей областью.
 */
function show(item = conversation, client = new QueryClient()) {
  clients.push(client);
  const onAccessDenied = vi.fn();
  const ui = (current: DirectConversation, modal = false) => (
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <PersonalPeerPresenceProvider conversation={current}>
          <PersonalPeerStatus
            conversation={current}
            onAccessDenied={onAccessDenied}
          />
          {modal && (
            <DirectUserInfoModal conversation={current} onClose={() => {}} />
          )}
        </PersonalPeerPresenceProvider>
      </MemoryRouter>
    </QueryClientProvider>
  );
  const view = render(ui(item));
  return {
    client,
    onAccessDenied,
    unmount: view.unmount,
    change: (current = item, modal = false) =>
      view.rerender(ui(current, modal)),
  };
}

/**
 * Ждёт реальный период опроса и завершение уведомлений React Query.
 * @args milliseconds — длительность ожидания в миллисекундах.
 * @return Завершённый цикл обновления React.
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

describe("подтверждённое присутствие в заголовке личного диалога", () => {
  it("показывает ожидание, затем зелёный online и обновляет offline без открытия информации", async () => {
    const pending = deferred<PresenceResponse>();
    vi.mocked(api.personalPeerPresence)
      .mockImplementationOnce(() => pending.promise)
      .mockResolvedValue(response(false));
    show();
    expect(screen.getByRole("status")).toHaveTextContent(/^Проверяем статус…$/);
    expect(api.personalPeerPresence).toHaveBeenCalledWith(
      conversation.id,
      expect.any(AbortSignal),
    );
    await act(async () => pending.resolve(response(true)));
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(/^Онлайн$/),
    );
    expect(screen.getByRole("status")).toHaveClass(
      "personal-peer-status-online",
    );
    await poll();
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(/^Не в сети$/),
    );
    expect(screen.getByRole("status")).not.toHaveClass(
      "personal-peer-status-online",
    );
    expect(screen.getByRole("status")).toHaveAttribute("aria-live", "polite");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("при ошибке следующего опроса не оставляет прежний зелёный online", async () => {
    vi.mocked(api.personalPeerPresence)
      .mockResolvedValueOnce(response(true))
      .mockRejectedValue(new ApiError(503, "Сервис присутствия недоступен"));
    const state = show();
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(/^Онлайн$/),
    );
    await poll();
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(
        /^Статус недоступен$/,
      ),
    );
    expect(screen.getByRole("status")).not.toHaveClass(
      "personal-peer-status-online",
    );
    expect(screen.getByRole("status")).not.toHaveTextContent(/^Не в сети$/);
    expect(state.onAccessDenied).not.toHaveBeenCalled();
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
      { conversationId: "direct", peerId: "bob", online: "true" },
    ],
    ["неполный ответ", undefined],
  ])(
    "не показывает online из неподтверждённого DTO: %s",
    async (_label, item) => {
      vi.mocked(api.personalPeerPresence).mockResolvedValue({
        status: "success",
        item,
      } as PresenceResponse);
      show();
      await waitFor(() =>
        expect(screen.getByRole("status")).toHaveTextContent(
          /^Статус недоступен$/,
        ),
      );
      expect(screen.getByRole("status")).not.toHaveClass(
        "personal-peer-status-online",
      );
    },
  );

  it.each(["учётная запись", "диалог", "собеседник"] as const)(
    "не переносит online в новую область: %s",
    async (scope) => {
      vi.mocked(api.personalPeerPresence).mockResolvedValueOnce(response(true));
      const state = show();
      await waitFor(() =>
        expect(screen.getByRole("status")).toHaveTextContent(/^Онлайн$/),
      );
      const pending = deferred<PresenceResponse>();
      vi.mocked(api.personalPeerPresence).mockImplementation(
        () => pending.promise,
      );
      const next = {
        ...conversation,
        ...(scope === "диалог" ? { id: "next-direct" } : {}),
        peer:
          scope === "собеседник"
            ? { id: "charlie", displayName: "Константин" }
            : conversation.peer,
      };
      if (scope === "учётная запись")
        authState.user = { ...actor, id: "next-account" };
      state.change(next);
      expect(screen.getByRole("status")).toHaveTextContent(
        /^Проверяем статус…$/,
      );
      expect(screen.getByRole("status")).not.toHaveClass(
        "personal-peer-status-online",
      );
      await act(async () => pending.resolve(response(false, next)));
      await waitFor(() =>
        expect(screen.getByRole("status")).toHaveTextContent(/^Не в сети$/),
      );
      expect(
        state.client.getQueryData([
          "personal-peer-presence",
          next.id,
          authState.user!.id,
          next.peer.id,
        ]),
      ).toEqual(response(false, next));
    },
  );

  it("отменяет прежний запрос при смене участника и игнорирует его поздний отказ доступа", async () => {
    const previous = deferred<PresenceResponse>();
    const next = deferred<PresenceResponse>();
    vi.mocked(api.personalPeerPresence)
      .mockImplementationOnce(() => previous.promise)
      .mockImplementationOnce(() => next.promise);
    const state = show();
    const signal = vi.mocked(api.personalPeerPresence).mock.calls[0][1]!;
    authState.user = { ...actor, id: "next-account" };
    state.change();
    expect(signal.aborted).toBe(true);
    await act(async () =>
      previous.reject(new ApiError(403, "Прежний доступ отозван")),
    );
    expect(state.onAccessDenied).not.toHaveBeenCalled();
    expect(screen.getByRole("status")).toHaveTextContent(/^Проверяем статус…$/);
    await act(async () => next.resolve(response(false)));
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(/^Не в сети$/),
    );
    expect(
      state.client.getQueryData([
        "personal-peer-presence",
        conversation.id,
        "next-account",
        conversation.peer.id,
      ]),
    ).toEqual(response(false));
  });

  it("при закрытии диалога отменяет запрос и прекращает опрос без позднего online", async () => {
    const pending = deferred<PresenceResponse>();
    vi.mocked(api.personalPeerPresence).mockImplementation(
      () => pending.promise,
    );
    const state = show();
    const signal = vi.mocked(api.personalPeerPresence).mock.calls[0][1]!;
    state.unmount();
    expect(signal.aborted).toBe(true);
    await act(async () => pending.resolve(response(true)));
    await poll(1200);
    expect(api.personalPeerPresence).toHaveBeenCalledOnce();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    await waitFor(() =>
      expect(
        state.client.getQueriesData({ queryKey: ["personal-peer-presence"] }),
      ).toEqual([]),
    );
  });

  it.each([403, 404])(
    "при отказе %s очищает только текущую область и прекращает опрос",
    async (status) => {
      const client = new QueryClient();
      const otherAccount = [
        "personal-peer-presence",
        conversation.id,
        "other-account",
        conversation.peer.id,
      ];
      const otherConversation = [
        "personal-peer-presence",
        "retained",
        actor.id,
        "retained-peer",
      ];
      const retained = {
        ...conversation,
        id: "retained",
        peer: { id: "retained-peer", displayName: "Другой" },
      };
      client.setQueryData(otherAccount, response(true));
      client.setQueryData(otherConversation, response(true, retained));
      vi.mocked(api.personalPeerPresence).mockRejectedValue(
        new ApiError(status, "Нет доступа"),
      );
      const state = show(conversation, client);
      await waitFor(() => expect(state.onAccessDenied).toHaveBeenCalledOnce());
      expect(screen.getByRole("status")).not.toHaveClass(
        "personal-peer-status-online",
      );
      expect(
        client
          .getQueriesData({
            queryKey: ["personal-peer-presence", conversation.id, actor.id],
          })
          .filter(([, data]) => data !== undefined),
      ).toEqual([]);
      expect(client.getQueryData(otherAccount)).toEqual(response(true));
      expect(client.getQueryData(otherConversation)).toEqual(
        response(true, retained),
      );
      await poll(1200);
      expect(api.personalPeerPresence).toHaveBeenCalledOnce();
    },
  );

  it("разделяет запрос с окном информации и оставляет один опрос после его закрытия", async () => {
    const pending = deferred<PresenceResponse>();
    vi.mocked(api.personalPeerPresence)
      .mockImplementationOnce(() => pending.promise)
      .mockResolvedValue(response(false));
    const state = show();
    state.change(conversation, true);
    expect(api.personalPeerPresence).toHaveBeenCalledOnce();
    await act(async () => pending.resolve(response(true)));
    await waitFor(() =>
      expect(screen.getAllByRole("status")[0]).toHaveTextContent(/^Онлайн$/),
    );
    expect(screen.getAllByRole("status")[1]).toHaveTextContent(/^В сети$/);
    await poll(1200);
    expect(api.personalPeerPresence).toHaveBeenCalledTimes(2);
    state.change(conversation, false);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent(/^Не в сети$/);
    const reads = vi.mocked(api.personalPeerPresence).mock.calls.length;
    await poll(1100);
    expect(api.personalPeerPresence).toHaveBeenCalledTimes(reads + 1);
    expect(
      state.client.getQueriesData({ queryKey: ["personal-peer-presence"] }),
    ).toHaveLength(1);
  });

  it.each(["без пользователя", "гость встречи"])(
    "не читает защищённое присутствие: %s",
    async (kind) => {
      authState.user =
        kind === "без пользователя"
          ? undefined
          : { ...actor, guestConferenceId: "guest-meeting" };
      show();
      await poll(1100);
      expect(api.personalPeerPresence).not.toHaveBeenCalled();
      expect(screen.getByRole("status")).not.toHaveClass(
        "personal-peer-status-online",
      );
    },
  );
  it("не показывает кеш аккаунта после перехода в гостевой режим с тем же идентификатором", async () => {
    vi.mocked(api.personalPeerPresence).mockResolvedValue(response(true));
    const state = show();
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(/^Онлайн$/),
    );
    authState.user = { ...actor, guestConferenceId: "guest-meeting" };
    state.change();
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(
        /^Статус недоступен$/,
      ),
    );
    await poll(1100);
    expect(api.personalPeerPresence).toHaveBeenCalledOnce();
  });
});
