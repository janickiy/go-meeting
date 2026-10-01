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

describe("session storage", () => {
  it("restores only an unexpired, bounded session", () => {
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
  ])("discards corrupt or expired storage %s", (value) => {
    sessionStorage.setItem(SESSION_KEY, value);
    expect(readSession()).toBeNull();
    expect(sessionStorage.getItem(SESSION_KEY)).toBeNull();
  });
  it("does not fail if browser storage is blocked", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    expect(() =>
      saveSession({ token: "test", expiresAt: Date.now() + 1000 }),
    ).not.toThrow();
  });
});
describe("navigation and invitations", () => {
  const code = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef";
  it.each([
    "https://evil.test",
    "//evil.test/app",
    "/\\evil.test/app",
    "/login",
    "/app\n",
  ])("rejects unsafe next %s", (value) => {
    expect(safeNext(value)).toBe("/app");
  });
  it("preserves a local invitation through authorization", () => {
    expect(safeNext(`/i/${code}`)).toBe(`/i/${code}`);
    expect(safeNext("/conferences/test?x=1")).toBe("/conferences/test?x=1");
  });
  it("accepts only codes and invitation URLs from the current origin", () => {
    expect(inviteCode(code)).toBe(code);
    expect(inviteCode(`${window.location.origin}/i/${code}`)).toBe(code);
    expect(inviteCode(`/api/v1/conference-invites/${code}`)).toBe(code);
    expect(inviteCode(`https://evil.test/i/${code}`)).toBeNull();
    expect(inviteCode("invalid")).toBeNull();
  });
  it("counts password characters consistently with Go unicode runes", () => {
    expect(passwordLength("abcdefgh")).toBe(8);
    expect(passwordLength("абвгдежз")).toBe(8);
    expect(passwordLength("😀".repeat(8))).toBe(8);
    expect(passwordLength("😀".repeat(128))).toBe(128);
    expect(passwordLength(" abcd e ")).toBe(8);
  });
  it("still counts UTF-8 bytes for the email limit", () => {
    expect(utf8Bytes("abcdefgh")).toBe(8);
    expect(utf8Bytes("пароль")).toBe(12);
    expect(utf8Bytes("😀")).toBe(4);
  });
});
