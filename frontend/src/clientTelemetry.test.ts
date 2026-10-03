import { describe, expect, it, vi } from "vitest";
import {
  browserFamily,
  createClientErrorReporter,
  sanitizedStack,
  telemetryRoute,
} from "./clientTelemetry";

describe("безопасная телеметрия браузера", () => {
  it("удаляет приглашения, UUID и query; неизвестный путь не отправляется", () => {
    expect(telemetryRoute("/i/private-invite?token=secret")).toBe("/i/:code");
    expect(telemetryRoute("/meetings/private-id/join#private")).toBe(
      "/meetings/:id/join",
    );
    expect(
      telemetryRoute("/app/settings/calendar/google/callback?code=secret"),
    ).toBe("/app/settings/calendar/:provider/callback");
    expect(telemetryRoute("/private-name@example.test")).toBe("unknown");
    expect(telemetryRoute("/meetings/new")).toBe("/meetings/new");
    expect(
      telemetryRoute(
        "/recordings/private-id?conference=private-room&signature=secret",
      ),
    ).toBe("/recordings/:id");
    expect(telemetryRoute("/recordings?conference=private-room")).toBe(
      "/recordings",
    );
  });

  it("оставляет только семейство браузера", () => {
    expect(
      browserFamily("Mozilla/5.0 Chrome/120.1 Safari/537.36 Edg/120.1"),
    ).toBe("edge");
    expect(browserFamily("Firefox/130.1 private-device")).toBe("firefox");
    expect(browserFamily("Version/17.1 Safari/605.1")).toBe("safari");
    expect(browserFamily("private-device")).toBe("other");
  });

  it("выбрасывает сообщения, подписанные URL, внешние кадры и имена функций", () => {
    const error = new Error("chat transcript secret@example.test");
    error.stack = [
      "Error: chat transcript secret@example.test",
      " at privateName (https://meet.example/assets/index-abcdefgh.js:21:9)",
      "privateName@https://meet.example/assets/ConferencePage-abcd1234.js:3:5",
      " at privateName (https://other.example/assets/index-abcdefgh.js:1:1)",
      " at signed (https://meet.example/assets/index-abcdefgh.js?X-Amz-Signature=secret:2:3)",
      " at source (https://meet.example/src/private.ts:2:3)",
      " at source (https://meet.example/assets/index-abcdefgh.js:0:3)",
    ].join("\n");
    expect(sanitizedStack(error, "https://meet.example")).toEqual([
      { file: "index-abcdefgh.js", line: 21, column: 9 },
      { file: "ConferencePage-abcd1234.js", line: 3, column: 5 },
    ]);
    expect(
      sanitizedStack(
        { message: "private", stack: "private" },
        "https://meet.example",
      ),
    ).toEqual([]);
  });

  it("ограничивает число кадров и длину имени файла", () => {
    const error = new Error();
    error.stack =
      ` at f (https://meet.example/assets/${"a".repeat(200)}-abcdefgh.js:1:1)\n` +
      " at f (https://meet.example/assets/index-abcdefgh.js:1:1)\n".repeat(20);
    const frames = sanitizedStack(error, "https://meet.example");
    expect(frames).toHaveLength(5);
    expect(frames.every((frame) => frame.file === "index-abcdefgh.js")).toBe(
      true,
    );
  });

  it("по умолчанию не отправляет данные; включённый адаптер не передаёт credentials", () => {
    const send = vi.fn().mockResolvedValue({});
    const options = {
      enabled: false,
      version: "v1.2.3",
      origin: "https://meet.example",
      pathname: () => "/history/private-id",
      userAgent: "Firefox/130",
      fetch: send,
    };
    createClientErrorReporter(options)("uncaught_error", new Error("private"));
    expect(send).not.toHaveBeenCalled();
    createClientErrorReporter({ ...options, enabled: true })(
      "uncaught_error",
      new Error("private"),
    );
    expect(send).toHaveBeenCalledOnce();
    const [url, request] = send.mock.calls[0];
    expect(url).toBe("/api/v1/client-errors");
    expect(request).toMatchObject({
      credentials: "omit",
      mode: "same-origin",
      redirect: "error",
      headers: { "Content-Type": "application/json" },
    });
    expect(JSON.parse(request.body)).toEqual({
      version: "v1.2.3",
      route: "/history/:id",
      code: "uncaught_error",
      browser: "firefox",
      stack: [],
    });
    expect(request.body).not.toContain("private");
  });

  it("подавляет дубли, ограничивает пять отправок в минуту и двадцать за страницу", () => {
    let now = 0;
    let path = "/";
    const send = vi.fn().mockResolvedValue({});
    const report = createClientErrorReporter({
      enabled: true,
      version: "dev",
      origin: "https://meet.example",
      pathname: () => path,
      userAgent: "",
      fetch: send,
      now: () => now,
    });
    report("uncaught_error", null);
    report("uncaught_error", null);
    expect(send).toHaveBeenCalledTimes(1);
    const routes = [
      "/login",
      "/register",
      "/app",
      "/settings",
      "/admin",
      "/search",
    ];
    for (path of routes) report("uncaught_error", null);
    expect(send).toHaveBeenCalledTimes(5);
    for (let minute = 1; minute <= 5; minute++) {
      now = minute * 60_000;
      for (path of routes) report("uncaught_error", null);
    }
    expect(send).toHaveBeenCalledTimes(20);
  });

  it("не создаёт новый отказ при отклонённой отправке или плохом объекте ошибки", async () => {
    const send = vi.fn().mockRejectedValue(new Error("network"));
    const error = new Error();
    Object.defineProperty(error, "stack", {
      get() {
        throw new Error("private");
      },
    });
    const report = createClientErrorReporter({
      enabled: true,
      version: "dev",
      origin: "https://meet.example",
      pathname: () => "/",
      userAgent: "",
      fetch: send,
    });
    expect(() => report("unhandled_rejection", error)).not.toThrow();
    await Promise.resolve();
    expect(JSON.parse(send.mock.calls[0][1].body).stack).toEqual([]);
  });
});
