import { describe, expect, it, vi } from "vitest";
import {
  inviteCode,
  passwordLength,
  utf8Bytes,
  readSession,
  safeNext,
  saveSession,
  SESSION_KEY,
  initials,
} from "./utils";

it.each([
  ["Анна Морозова", "АМ"],
  ["  Мария   Орлова  ", "МО"],
  ["Вася", "В"],
  ["Анна Мария Морозова", "АМ"],
  ["", "М"],
])("инициалы %s соответствуют аватару макета", (name, expected) => {
  expect(initials(name)).toBe(expected);
});

describe("session storage", /**
 * Проверяет хранение сессии.
 *
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ () => {
  it("retains an expired access cache for cookie restoration without storing credentials in localStorage", () => {
    const session = {
      token: "expired-short-access",
      expiresAt: Date.now() - 86_400_000,
    };
    saveSession(session);
    expect(readSession()).toEqual(session);
    expect(localStorage.getItem(SESSION_KEY)).toBeNull();
    saveSession(null);
  });
  it("restores only an unexpired, bounded session", /**
   * Проверяет восстановление только неистёкшей сессии в установленных пределах.
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
   * Проверяет работу при заблокированном хранилище браузера.
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
 * Проверяет навигацию и приглашения.
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
   * Проверяет сохранность локального приглашения при авторизации.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    expect(safeNext(`/i/${code}`)).toBe(`/i/${code}`);
    expect(safeNext("/conferences/test?x=1")).toBe("/conferences/test?x=1");
    expect(safeNext("/recordings/test?conference=room")).toBe(
      "/recordings/test?conference=room",
    );
    expect(safeNext("/recordings?conference=room")).toBe(
      "/recordings?conference=room",
    );
  });
  it.each([
    "/calendar?date=2026-10-09",
    "/analytics",
    "/meetings",
    "/meetings/new",
    "/meetings/45144e4e-c3d7-4eed-863e-2edc7ceec5b4",
    "/meetings/45144e4e-c3d7-4eed-863e-2edc7ceec5b4/join?camera=off",
    "/conferences/45144e4e-c3d7-4eed-863e-2edc7ceec5b4/join",
    "/history",
    "/history/45144e4e-c3d7-4eed-863e-2edc7ceec5b4",
    "/folders",
    "/folders/45144e4e-c3d7-4eed-863e-2edc7ceec5b4?sort=name",
    "/settings",
    "/app/settings",
    "/app/recordings?conference=room",
    "/notifications?unread=true",
    "/admin",
    "/calendar/",
  ])("сохраняет адрес раздела после авторизации: %s", (value) => {
    expect(safeNext(value)).toBe(value);
  });
  it.each([
    "/calendar/settings",
    "/analytics/export",
    "/meetings/room/unknown",
    "/history/room/files",
    "/app/login",
    "/settings/security",
    "/notifications/../login",
    "/meetings/%2f%2fevil.test/join",
    "/recordings/%5cevil.test",
    "/history/%0aroom",
  ])("отвергает неподдерживаемый вложенный адрес: %s", (value) => {
    expect(safeNext(value)).toBe("/app");
  });
  it.each([
    "/personal",
    "/personal?from=notification",
    "/personal/45144e4e-c3d7-4eed-863e-2edc7ceec5b4",
    "/personal/45144E4E-C3D7-4EED-863E-2EDC7CEEC5B4?from=notification",
  ])("preserves a personal destination through authorization: %s", (value) => {
    expect(safeNext(value)).toBe(value);
  });
  it.each([
    "https://evil.test/personal",
    "//evil.test/personal",
    "/\\evil.test/personal",
    "/personal\n",
    "/personal-notes",
    "/personal/not-a-conversation",
    "/personal/45144e4e-c3d7-4eed-863e-2edc7ceec5b4/messages",
    "/personal/45144e4e-c3d7-4eed-863e-2edc7ceec5b4%2fmessages",
    "/personal//evil.test",
    "/personal/../login",
  ])("rejects an unsafe or unsupported personal destination: %s", (value) => {
    expect(safeNext(value)).toBe("/app");
  });
  it("accepts only codes and invitation URLs from the current origin", /**
   * Проверяет приём только кодов и ссылок приглашения с текущего источника.
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
   * Проверяет подсчёт символов пароля в соответствии с рунами Unicode в Go.
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
   * Проверяет сохранение подсчёта байтов UTF-8 для ограничения email.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    expect(utf8Bytes("abcdefgh")).toBe(8);
    expect(utf8Bytes("пароль")).toBe(12);
    expect(utf8Bytes("😀")).toBe(4);
  });
});
