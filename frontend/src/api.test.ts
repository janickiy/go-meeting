import { afterEach, describe, expect, it, vi } from "vitest";
import { api, ApiError, configureAuth, uploadAttachment } from "./api";

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
 * Проверка: API contract выполняет тестовый сценарий «API contract» и проверяет ожидаемые результаты.
 *
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ () => {
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
   * Проверка: opens notification SSE with bearer header and an abortable fetch, not a query token выполняет тестовый сценарий «opens notification SSE with bearer header and an abortable fetch, not a query token» и проверяет ожидаемые результаты.
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
   * Проверка: never leaks an upload bearer token to a foreign or unexpected URL выполняет тестовый сценарий «never leaks an upload bearer token to a foreign or unexpected URL» и проверяет ожидаемые результаты.
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
   * Проверка: issues WebSocket tickets with bearer auth only in headers выполняет тестовый сценарий «issues WebSocket tickets with bearer auth only in headers» и проверяет ожидаемые результаты.
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
   * Проверка: sends only title for conference creation, with the active bearer token выполняет тестовый сценарий «sends only title for conference creation, with the active bearer token» и проверяет ожидаемые результаты.
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
   * Проверка: never attaches a bearer token to login or registration выполняет тестовый сценарий «never attaches a bearer token to login or registration» и проверяет ожидаемые результаты.
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
   * Проверка: passes the used token to the expiry handler and does not retry a mutation выполняет тестовый сценарий «passes the used token to the expiry handler and does not retry a mutation» и проверяет ожидаемые результаты.
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
   * Проверка: does not log out a session on invalid login credentials выполняет тестовый сценарий «does not log out a session on invalid login credentials» и проверяет ожидаемые результаты.
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
   * Проверка: does not expose backend SQL or private error details выполняет тестовый сценарий «does not expose backend SQL or private error details» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    fetchResponse(500, { message: "postgres: password=secret, SQL SELECT..." });
    await expect(api.me()).rejects.toThrow("Сервис временно недоступен.");
  });
  it("handles non-JSON upstream errors", /**
   * Проверка: handles non-JSON upstream errors выполняет тестовый сценарий «handles non-JSON upstream errors» и проверяет ожидаемые результаты.
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
