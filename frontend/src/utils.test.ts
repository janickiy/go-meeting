import { describe, expect, it, vi } from "vitest";
import {
  inviteCode,
  passwordLength,
  utf8Bytes,
  readSession,
  safeNext,
  saveSession,
  SESSION_KEY,
} from "./utils";

describe("session storage", /**
 * Проверка: session storage выполняет тестовый сценарий «session storage» и проверяет ожидаемые результаты.
 *
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ () => {
  it("restores only an unexpired, bounded session", /**
   * Проверка: restores only an unexpired, bounded session выполняет тестовый сценарий «restores only an unexpired, bounded session» и проверяет ожидаемые результаты.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    const session = { token: "jwt-test", expiresAt: Date.now() + 50000 };
    saveSession(session);
    expect(readSession()).toEqual(session);
    saveSession(null);
    expect(readSession()).toBeNull();
  });
  it.each([
    "{broken",
    JSON.stringify({ token: "test", expiresAt: 0 }),
    JSON.stringify({ token: "test", expiresAt: Date.now() + 7200000 }),
    JSON.stringify({ token: "x".repeat(5000), expiresAt: Date.now() + 50000 }),
  ])(
    "discards corrupt or expired storage %s",
    /**
     * Обработчик вызова выполняет переданный шаг вызова вызова в проверках клиентского поведения.
     *
     * @args
     *   - value — значение для проверки, преобразования или отображения.
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */ (value) => {
      sessionStorage.setItem(SESSION_KEY, value);
      expect(readSession()).toBeNull();
      expect(sessionStorage.getItem(SESSION_KEY)).toBeNull();
    },
  );
  it("does not fail if browser storage is blocked", /**
   * Проверка: does not fail if browser storage is blocked выполняет тестовый сценарий «does not fail if browser storage is blocked» и проверяет ожидаемые результаты.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(
      /**
       * Обработчик mockImplementation выполняет переданный шаг вызова mockImplementation в проверках клиентского поведения.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */ () => {
        throw new Error("blocked");
      },
    );
    expect(
      /**
       * Обработчик expect выполняет переданный шаг вызова expect в проверках клиентского поведения.
       *
       *
       * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
       */ () => saveSession({ token: "test", expiresAt: Date.now() + 1000 }),
    ).not.toThrow();
  });
});
describe("navigation and invitations", /**
 * Проверка: navigation and invitations выполняет тестовый сценарий «navigation and invitations» и проверяет ожидаемые результаты.
 *
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ () => {
  const code = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef";
  it.each([
    "https://evil.test",
    "//evil.test/app",
    "/\\evil.test/app",
    "/login",
    "/app\n",
  ])(
    "rejects unsafe next %s",
    /**
     * Обработчик вызова выполняет переданный шаг вызова вызова в проверках клиентского поведения.
     *
     * @args
     *   - value — значение для проверки, преобразования или отображения.
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */ (value) => {
      expect(safeNext(value)).toBe("/app");
    },
  );
  it("preserves a local invitation through authorization", /**
   * Проверка: preserves a local invitation through authorization выполняет тестовый сценарий «preserves a local invitation through authorization» и проверяет ожидаемые результаты.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    expect(safeNext(`/i/${code}`)).toBe(`/i/${code}`);
    expect(safeNext("/conferences/test?x=1")).toBe("/conferences/test?x=1");
  });
  it("accepts only codes and invitation URLs from the current origin", /**
   * Проверка: accepts only codes and invitation URLs from the current origin выполняет тестовый сценарий «accepts only codes and invitation URLs from the current origin» и проверяет ожидаемые результаты.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    expect(inviteCode(code)).toBe(code);
    expect(inviteCode(`${window.location.origin}/i/${code}`)).toBe(code);
    expect(inviteCode(`/api/v1/conference-invites/${code}`)).toBe(code);
    expect(inviteCode(`https://evil.test/i/${code}`)).toBeNull();
    expect(inviteCode("invalid")).toBeNull();
  });
  it("counts password characters consistently with Go unicode runes", /**
   * Проверка: counts password characters consistently with Go unicode runes выполняет тестовый сценарий «counts password characters consistently with Go unicode runes» и проверяет ожидаемые результаты.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    expect(passwordLength("abcdefgh")).toBe(8);
    expect(passwordLength("абвгдежз")).toBe(8);
    expect(passwordLength("😀".repeat(8))).toBe(8);
    expect(passwordLength("😀".repeat(128))).toBe(128);
    expect(passwordLength(" abcd e ")).toBe(8);
  });
  it("still counts UTF-8 bytes for the email limit", /**
   * Проверка: still counts UTF-8 bytes for the email limit выполняет тестовый сценарий «still counts UTF-8 bytes for the email limit» и проверяет ожидаемые результаты.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    expect(utf8Bytes("abcdefgh")).toBe(8);
    expect(utf8Bytes("пароль")).toBe(12);
    expect(utf8Bytes("😀")).toBe(4);
  });
});
