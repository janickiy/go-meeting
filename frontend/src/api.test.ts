import { afterEach, describe, expect, it, vi } from "vitest";
import {
  api,
  ApiError,
  configureAuth,
  personalChatAPI,
  uploadAttachment,
} from "./api";

afterEach(
  /**
   * Обработчик afterEach выполняет переданный шаг вызова afterEach в проверках клиентского поведения.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    configureAuth(
      null,
      /**
       * Обработчик configureAuth выполняет переданный шаг вызова configureAuth в проверках клиентского поведения.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */ () => {},
    );
    vi.unstubAllGlobals();
  },
);
/**
 * fetchResponse создаёт изолированный HTTP-ответ для проверки клиента API.
 *
 * @args
 *   - status (number) — HTTP-статус либо состояние встречи.
 *   - body (unknown) — типизированное тело запроса.
 *
 * @returns вычисленное значение: fetch.
 */
function fetchResponse(status: number, body: unknown) {
  const fetch = vi.fn().mockImplementation(
    /**
     * Обработчик mockImplementation выполняет переданный шаг вызова mockImplementation в проверках клиентского поведения.
     *
     *
     * @returns Promise, который после завершения операции возвращает: вычисленные данные текущего шага, которые использует вызывающая операция.
     */
    async () =>
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
  );
  vi.stubGlobal("fetch", fetch);
  return fetch;
}
describe("API contract", /**
 * Проверяет контракт API.
 *
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ () => {
  it("binds personal list cursors to server filters without changing legacy defaults", async () => {
    const fetch = fetchResponse(200, { status: "success", items: [] });
    const controller = new AbortController();
    await api.personalConversations(undefined, controller.signal);
    expect(fetch.mock.calls[0][0]).toBe("/api/v1/conversations?limit=50");
    await api.personalConversations("cursor+next", controller.signal, 50, {
      type: "group",
      unreadOnly: true,
      search: " Команда ",
    });
    const query = new URL(fetch.mock.calls[1][0], "http://local.invalid")
      .searchParams;
    expect(Object.fromEntries(query)).toEqual({
      limit: "50",
      before: "cursor+next",
      type: "group",
      unreadOnly: "true",
      search: "Команда",
    });
    expect(fetch.mock.calls[1][1].signal).toBe(controller.signal);
  });
  it("renews private binary authorization once without exposing a token in its URL", async () => {
    configureAuth("old-binary", async () => {
      configureAuth("new-binary");
      return true;
    });
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(new Response(null, { status: 401 }))
      .mockResolvedValueOnce(
        new Response(new Uint8Array([1, 2, 3]), {
          headers: { "Content-Type": "image/png" },
        }),
      );
    vi.stubGlobal("fetch", fetch);
    const controller = new AbortController();
    const image = await api.groupAvatar("group/id", controller.signal);
    expect(image.size).toBe(3);
    expect(image.type).toBe("image/png");
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(fetch.mock.calls[1][0]).toBe(
      "/api/v1/conversations/group%2Fid/avatar/content",
    );
    expect(fetch.mock.calls[1][1].headers.get("Authorization")).toBe(
      "Bearer new-binary",
    );
    expect(fetch.mock.calls[1][1]).toMatchObject({
      credentials: "omit",
      signal: controller.signal,
    });
  });
  it("rejects private avatar MIME and streaming size before exposing bytes", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(
        new Response("<svg/>", {
          headers: { "Content-Type": "image/svg+xml" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(new Uint8Array(2 * 1024 * 1024 + 1), {
          headers: { "Content-Type": "image/png" },
        }),
      );
    vi.stubGlobal("fetch", fetch);
    await expect(api.groupAvatar("group")).rejects.toMatchObject({
      status: 502,
    });
    await expect(api.groupAvatar("group")).rejects.toMatchObject({
      status: 413,
    });
  });
  it("uploads avatar raw and downloads private group attachments with Bearer and cancellation", async () => {
    configureAuth("group-private");
    const fetch = fetchResponse(200, { status: "success", item: {} });
    const controller = new AbortController();
    const file = new File(["png"], "photo.png", { type: "image/png" });
    await api.putGroupAvatar("group", file, controller.signal);
    expect(fetch.mock.calls[0][1]).toMatchObject({
      method: "PUT",
      body: file,
      credentials: "omit",
      signal: controller.signal,
    });
    expect(fetch.mock.calls[0][1].headers.get("Content-Type")).toBe(
      "image/png",
    );
    expect(fetch.mock.calls[0][1].headers.get("Authorization")).toBe(
      "Bearer group-private",
    );
    await expect(
      api.putGroupAvatar(
        "group",
        new File(["svg"], "evil.svg", { type: "image/svg+xml" }),
      ),
    ).rejects.toBeInstanceOf(ApiError);
    expect(fetch).toHaveBeenCalledTimes(1);
    fetch.mockResolvedValueOnce(
      new Response("Private attachment", {
        headers: { "Content-Type": "text/plain" },
      }),
    );
    const blob = await personalChatAPI.attachmentContent(
      "group",
      "file",
      controller.signal,
    );
    expect(blob.size).toBe(18);
    expect(fetch.mock.calls[1][0]).toBe(
      "/api/v1/conversations/group/attachments/file/content",
    );
    expect(fetch.mock.calls[1][1].headers.get("Authorization")).toBe(
      "Bearer group-private",
    );
  });
  it("объясняет конфликт повторного запуска записи без технических деталей", async () => {
    fetchResponse(409, {
      status: "error",
      message: "recording is already active",
    });
    await expect(api.startRecording("room", "composite")).rejects.toMatchObject(
      {
        status: 409,
        message:
          "Запись уже запущена. Дождитесь завершения текущей записи перед запуском новой.",
      },
    );
  });
  it("передаёт лимит и смещение записей третьим аргументом с авторизацией и отменой", async () => {
    configureAuth("recordings-test-token");
    const fetch = fetchResponse(200, { status: "success", items: [] });
    const abort = new AbortController();
    await api.recordings("conference/id", abort.signal, {
      limit: 20,
      offset: 40,
    });
    const [url, options] = fetch.mock.calls[0];
    expect(url).toBe(
      "/api/v1/conferences/conference%2Fid/recordings?limit=20&offset=40",
    );
    expect(url).not.toContain("recordings-test-token");
    expect(options.signal).toBe(abort.signal);
    expect(options.headers.get("Authorization")).toBe(
      "Bearer recordings-test-token",
    );
    expect(options.credentials).toBe("omit");
  });
  it("сохраняет прежнюю сигнатуру списка записей без параметров пагинации", async () => {
    const fetch = fetchResponse(200, { status: "success", items: [] });
    const abort = new AbortController();
    await api.recordings("conference/id", abort.signal);
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/conferences/conference%2Fid/recordings",
      expect.objectContaining({ signal: abort.signal }),
    );
  });
  it("принимает успешный ответ 204 при отзыве календаря", async () => {
    configureAuth("private-test-token");
    const fetch = vi
      .fn()
      .mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetch);
    await expect(
      api.disconnectCalendar("calendar/id"),
    ).resolves.toBeUndefined();
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/integrations/calendars/calendar%2Fid",
      expect.objectContaining({
        method: "DELETE",
        credentials: "omit",
      }),
    );
    expect(
      (fetch.mock.calls[0][1].headers as Headers).get("Authorization"),
    ).toBe("Bearer private-test-token");
  });
  it("opens notification SSE with bearer header and an abortable fetch, not a query token", /**
   * Проверяет открытие SSE-уведомлений с Bearer в заголовке и отменяемым fetch, без токена в строке запроса.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    configureAuth("private-test-token");
    const fetch = vi.fn().mockResolvedValue(
      new Response(": heartbeat\n\n", {
        headers: { "Content-Type": "text/event-stream" },
      }),
    );
    vi.stubGlobal("fetch", fetch);
    const abort = new AbortController();
    await api.notificationEvents(abort.signal);
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/notifications/events",
      expect.objectContaining({
        signal: abort.signal,
        credentials: "omit",
        headers: {
          Accept: "text/event-stream",
          Authorization: "Bearer private-test-token",
        },
      }),
    );
  });
  it("never leaks an upload bearer token to a foreign or unexpected URL", /**
   * Проверяет, что токен загрузки не отправляется на посторонний или неожиданный URL.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    configureAuth("private-test-token");
    const xhr = vi.fn();
    vi.stubGlobal("XMLHttpRequest", xhr);
    const file = new File(["safe"], "notes.txt", { type: "text/plain" });
    for (const url of [
      "https://foreign.invalid/api/v1/conferences/a/attachments/b/content",
      "/api/v1/auth/login",
      "/api/v1/conferences/a/attachments/b/content?token=x",
    ])
      await expect(
        uploadAttachment(
          url,
          file,
          /**
           * Обработчик uploadAttachment выполняет переданный шаг вызова uploadAttachment в проверках клиентского поведения.
           *
           *
           * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
           */ () => {},
          new AbortController().signal,
        ),
      ).rejects.toBeInstanceOf(ApiError);
    expect(xhr).not.toHaveBeenCalled();
  });
  it("issues WebSocket tickets with bearer auth only in headers", /**
   * Проверяет выдачу билетов WebSocket с Bearer-авторизацией только в заголовках.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    configureAuth("private-test-token");
    const fetch = fetchResponse(201, {
      ticket: "short-ticket",
      expiresAt: "future",
    });
    await api.wsTicket("room");
    const [url, options] = fetch.mock.calls[0];
    expect(url).toBe("/api/v1/conferences/room/ws-ticket");
    expect(url).not.toContain("private-test-token");
    expect(options.method).toBe("POST");
    expect(options.headers.get("Authorization")).toBe(
      "Bearer private-test-token",
    );
  });
  it("sends only title for conference creation, with the active bearer token", /**
   * Проверяет передачу только названия конференции и текущего токена Bearer при создании встречи.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    configureAuth("private-test-token");
    const fetch = fetchResponse(201, { item: { title: "Обсуждение" } });
    await api.create("Обсуждение");
    const [url, options] = fetch.mock.calls[0];
    expect(url).toBe("/api/v1/conferences");
    expect(JSON.parse(options.body)).toEqual({ title: "Обсуждение" });
    expect(options.headers.get("Authorization")).toBe(
      "Bearer private-test-token",
    );
    expect(options.credentials).toBe("omit");
  });
  it("updates only the current display name with bearer auth", async () => {
    configureAuth("profile-token");
    const user = {
      id: "user-1",
      email: "person@example.test",
      displayName: "Новое имя",
      isAdmin: true,
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-02T00:00:00Z",
    };
    const fetch = fetchResponse(200, { status: "success", user });
    await expect(api.updateProfile("Новое имя")).resolves.toEqual({
      status: "success",
      user,
    });
    const [url, options] = fetch.mock.calls[0];
    expect(url).toBe("/api/v1/auth/me");
    expect(options.method).toBe("PATCH");
    expect(JSON.parse(options.body)).toEqual({ displayName: "Новое имя" });
    expect(options.headers.get("Authorization")).toBe("Bearer profile-token");
  });
  it("never attaches a bearer token to login or registration", /**
   * Проверяет отсутствие токена Bearer при входе и регистрации.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    configureAuth("private-test-token");
    const fetch = fetchResponse(200, { user: {} });
    await api.login("user@example.test", "password-test");
    await api.register("user@example.test", "password-test", "  ");
    expect(
      fetch.mock.calls.every(
        /**
         * Обработчик every проверяет, соответствует ли текущий элемент условию выборки или поиска.
         *
         * @args
         *   - [, options] — элементы записи набора, извлечённые по указанным позициям.
         *
         * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
         */
        ([, options]) => !options.headers.has("Authorization"),
      ),
    ).toBe(true);
    expect(JSON.parse(fetch.mock.calls[1][1].body).displayName).toBeNull();
  });
  it("passes the used token to the expiry handler and does not retry a mutation", /**
   * Проверяет передачу использованного токена обработчику истечения и отсутствие повтора изменения данных.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const expired = vi.fn();
    configureAuth("old-token", expired);
    const fetch = fetchResponse(401, { message: "expired" });
    await expect(api.create("Test")).rejects.toBeInstanceOf(ApiError);
    expect(expired).toHaveBeenCalledWith("old-token");
    expect(fetch).toHaveBeenCalledTimes(1);
  });
  it("does not log out a session on invalid login credentials", /**
   * Проверяет сохранность текущей сессии при неверных данных входа.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const expired = vi.fn();
    configureAuth("old-token", expired);
    fetchResponse(401, { message: "invalid email or password" });
    await expect(api.login("test@example.test", "wrong")).rejects.toThrow(
      "Неверный email или пароль.",
    );
    expect(expired).not.toHaveBeenCalled();
  });
  it("does not expose backend SQL or private error details", /**
   * Проверяет отсутствие SQL и приватных деталей серверной ошибки в ответе.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    fetchResponse(500, { message: "postgres: password=secret, SQL SELECT..." });
    await expect(api.me()).rejects.toThrow("Сервис временно недоступен.");
  });
  it("handles non-JSON upstream errors", /**
   * Проверяет обработку ошибки вышестоящего сервера без JSON.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response("<html>gateway error</html>", { status: 502 }),
        ),
    );
    await expect(api.me()).rejects.toBeInstanceOf(ApiError);
  });
});

describe("persistent account recovery", () => {
  it("uses the cookie only for auth mutations and never puts it into JavaScript credentials", async () => {
    configureAuth("current-access");
    const fetch = fetchResponse(200, {
      accessToken: "fresh",
      expiresIn: 3600,
      user: { id: "person" },
    });
    await api.login("person@example.test", "password-test");
    await api.refreshSession();
    await api.establishSession();
    await api.logout("captured-expired-access");
    expect(
      fetch.mock.calls.every(
        ([, options]) => options.credentials === "same-origin",
      ),
    ).toBe(true);
    expect(fetch.mock.calls[0][1].headers.has("Authorization")).toBe(false);
    expect(fetch.mock.calls[1][1].headers.has("Authorization")).toBe(false);
    expect(fetch.mock.calls[2][1].headers.get("Authorization")).toBe(
      "Bearer current-access",
    );
    expect(fetch.mock.calls[3][1].headers.get("Authorization")).toBe(
      "Bearer captured-expired-access",
    );
  });

  it("retries a middleware-rejected mutation once with the renewed bearer", async () => {
    const recover = vi.fn(async () => {
      configureAuth("new-access");
      return true;
    });
    configureAuth("old-access", recover);
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ message: "expired" }), { status: 401 }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ item: { id: "meeting" } }), {
          status: 201,
        }),
      );
    vi.stubGlobal("fetch", fetch);
    await expect(api.create("Meeting")).resolves.toEqual({
      item: { id: "meeting" },
    });
    expect(recover).toHaveBeenCalledOnce();
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(fetch.mock.calls[1][1].headers.get("Authorization")).toBe(
      "Bearer new-access",
    );
    expect(fetch.mock.calls[1][1].body).toBe(fetch.mock.calls[0][1].body);
  });

  it("does not repeat a request indefinitely when the renewed access is also rejected", async () => {
    const recover = vi.fn(async () => {
      configureAuth("new-access");
      return true;
    });
    configureAuth("old-access", recover);
    const fetch = fetchResponse(401, { message: "unauthorized" });
    await expect(api.me()).rejects.toMatchObject({ status: 401 });
    expect(recover).toHaveBeenCalledOnce();
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it("does not retry an old account's mutation after explicit logout and another login", async () => {
    let resolve!: (value: boolean) => void;
    const pending = new Promise<boolean>((done) => {
      resolve = done;
    });
    configureAuth("old-account", () => pending);
    const fetch = fetchResponse(401, { message: "expired" });
    const request = api.create("Old account meeting");
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledOnce());
    configureAuth(null);
    configureAuth("other-account");
    resolve(true);
    await expect(request).rejects.toMatchObject({ status: 401 });
    expect(fetch).toHaveBeenCalledOnce();
  });

  it("renews and reconnects a rejected notification stream with the new bearer", async () => {
    configureAuth("old-access", async () => {
      configureAuth("new-access");
      return true;
    });
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ message: "expired" }), { status: 401 }),
      )
      .mockResolvedValueOnce(
        new Response(": heartbeat\n\n", {
          headers: { "Content-Type": "text/event-stream" },
        }),
      );
    vi.stubGlobal("fetch", fetch);
    await api.notificationEvents(new AbortController().signal);
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(fetch.mock.calls[1][1].headers.Authorization).toBe(
      "Bearer new-access",
    );
  });

  it("does not invoke recovery recursively for auth refresh, migration or logout", async () => {
    const recover = vi.fn();
    configureAuth("old-access", recover);
    fetchResponse(401, { message: "unauthorized" });
    await expect(api.refreshSession()).rejects.toMatchObject({ status: 401 });
    await expect(api.establishSession()).rejects.toMatchObject({ status: 401 });
    await expect(api.logout()).rejects.toMatchObject({ status: 401 });
    expect(recover).not.toHaveBeenCalled();
  });
});
describe("private folders API", () => {
  it("binds each cursor to its folder, type and search, and forwards AbortSignal", async () => {
    const fetch = fetchResponse(200, { items: [] });
    const controller = new AbortController();
    configureAuth("folder-test-token");
    await api.folderItems(
      "folder/id",
      { type: "all", search: "Работа & дом" },
      "cursor+next",
      controller.signal,
    );
    const url = new URL(fetch.mock.calls[0][0], "http://test.invalid");
    expect(url.pathname).toBe("/api/v1/folders/folder%2Fid/items");
    expect(Object.fromEntries(url.searchParams)).toEqual({
      limit: "50",
      type: "all",
      search: "Работа & дом",
      before: "cursor+next",
    });
    expect(fetch.mock.calls[0][1].signal).toBe(controller.signal);
    expect(fetch.mock.calls[0][1].headers.get("Authorization")).toBe(
      "Bearer folder-test-token",
    );
    await api.folderCandidates(
      "folder",
      { type: "conference", search: "План" },
      undefined,
      controller.signal,
    );
    expect(
      Object.fromEntries(
        new URL(fetch.mock.calls[1][0], "http://test.invalid").searchParams,
      ),
    ).toEqual({
      limit: "50",
      folderId: "folder",
      type: "conference",
      search: "План",
    });
  });
  it("uses one bounded checkbox projection and changes only the selected mapping or folder", async () => {
    const fetch = fetchResponse(200, { status: "success", items: [] });
    const signal = new AbortController().signal;
    await api.folders({ type: "conversation", id: "chat" }, signal);
    expect(fetch.mock.calls[0][0]).toBe(
      "/api/v1/folders?itemKind=conversation&itemId=chat",
    );
    await api.setFolderItem(
      "folder",
      { type: "conference", id: "meeting" },
      true,
      signal,
    );
    expect(fetch.mock.calls[1][0]).toBe(
      "/api/v1/folders/folder/items/conference/meeting",
    );
    expect(fetch.mock.calls[1][1].method).toBe("PUT");
    await api.setFolderItem(
      "folder",
      { type: "conference", id: "meeting" },
      false,
      signal,
    );
    expect(fetch.mock.calls[2][1].method).toBe("DELETE");
    await api.orderFolders(["a", "b"], signal);
    expect(fetch.mock.calls[3][0]).toBe("/api/v1/folders/order");
    expect(fetch.mock.calls[3][1].body).toBe(
      JSON.stringify({ ids: ["a", "b"] }),
    );
    await api.deleteFolder("folder", signal);
    expect(fetch.mock.calls[4][0]).toBe("/api/v1/folders/folder");
  });
});
