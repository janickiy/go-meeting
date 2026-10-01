import { afterEach, describe, expect, it, vi } from "vitest";
import { api, ApiError, configureAuth, uploadAttachment } from "./api";

afterEach(() => {
  configureAuth(null, () => {});
  vi.unstubAllGlobals();
});
function fetchResponse(status: number, body: unknown) {
  const fetch = vi.fn().mockImplementation(
    async () =>
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
  );
  vi.stubGlobal("fetch", fetch);
  return fetch;
}
describe("API contract", () => {
  it("opens notification SSE with bearer header and an abortable fetch, not a query token", async () => {
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
  it("never leaks an upload bearer token to a foreign or unexpected URL", async () => {
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
        uploadAttachment(url, file, () => {}, new AbortController().signal),
      ).rejects.toBeInstanceOf(ApiError);
    expect(xhr).not.toHaveBeenCalled();
  });
  it("issues WebSocket tickets with bearer auth only in headers", async () => {
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
  it("sends only title for conference creation, with the active bearer token", async () => {
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
  it("never attaches a bearer token to login or registration", async () => {
    configureAuth("private-test-token");
    const fetch = fetchResponse(200, { user: {} });
    await api.login("user@example.test", "password-test");
    await api.register("user@example.test", "password-test", "  ");
    expect(
      fetch.mock.calls.every(
        ([, options]) => !options.headers.has("Authorization"),
      ),
    ).toBe(true);
    expect(JSON.parse(fetch.mock.calls[1][1].body).displayName).toBeNull();
  });
  it("passes the used token to the expiry handler and does not retry a mutation", async () => {
    const expired = vi.fn();
    configureAuth("old-token", expired);
    const fetch = fetchResponse(401, { message: "expired" });
    await expect(api.create("Test")).rejects.toBeInstanceOf(ApiError);
    expect(expired).toHaveBeenCalledWith("old-token");
    expect(fetch).toHaveBeenCalledTimes(1);
  });
  it("does not log out a session on invalid login credentials", async () => {
    const expired = vi.fn();
    configureAuth("old-token", expired);
    fetchResponse(401, { message: "invalid email or password" });
    await expect(api.login("test@example.test", "wrong")).rejects.toThrow(
      "Неверный email или пароль.",
    );
    expect(expired).not.toHaveBeenCalled();
  });
  it("does not expose backend SQL or private error details", async () => {
    fetchResponse(500, { message: "postgres: password=secret, SQL SELECT..." });
    await expect(api.me()).rejects.toThrow("Сервис временно недоступен.");
  });
  it("handles non-JSON upstream errors", async () => {
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
