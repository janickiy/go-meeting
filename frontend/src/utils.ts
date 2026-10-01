export const SESSION_KEY = "meet.session.v1";
export interface Session {
  token: string;
  expiresAt: number;
}
export function readSession(): Session | null {
  try {
    const data: unknown = JSON.parse(
      sessionStorage.getItem(SESSION_KEY) || "null",
    );
    if (
      data &&
      typeof data === "object" &&
      "token" in data &&
      typeof data.token === "string" &&
      data.token.length > 0 &&
      data.token.length <= 4096 &&
      "expiresAt" in data &&
      typeof data.expiresAt === "number" &&
      Number.isFinite(data.expiresAt) &&
      data.expiresAt > Date.now() &&
      data.expiresAt <= Date.now() + 3600000
    )
      return data as Session;
    sessionStorage.removeItem(SESSION_KEY);
  } catch {
    try {
      sessionStorage.removeItem(SESSION_KEY);
    } catch {
      /* Storage may be blocked; use memory only. */
    }
  }
  return null;
}
export function saveSession(session: Session | null) {
  try {
    if (session) sessionStorage.setItem(SESSION_KEY, JSON.stringify(session));
    else sessionStorage.removeItem(SESSION_KEY);
  } catch {
    /* Do not persist credentials elsewhere. */
  }
}
export function safeNext(value: string | null): string {
  if (
    !value ||
    !value.startsWith("/") ||
    value.startsWith("//") ||
    value.includes("\\") ||
    /[\u0000-\u001f]/.test(value)
  )
    return "/app";
  try {
    const url = new URL(value, "https://meet.invalid");
    if (
      url.origin !== "https://meet.invalid" ||
      !/^\/(?:app(?:\/[^]*)?|conferences(?:\/[^]*)?|i\/[A-Za-z0-9_-]{32})$/.test(
        url.pathname,
      )
    )
      return "/app";
    return url.pathname + url.search;
  } catch {
    return "/app";
  }
}
export function inviteCode(value: string): string | null {
  const trimmed = value.trim();
  if (/^[A-Za-z0-9_-]{32}$/.test(trimmed)) return trimmed;
  try {
    const url = new URL(trimmed, window.location.origin);
    if (url.origin !== window.location.origin) return null;
    const match = url.pathname.match(
      /^\/(?:i|api\/v1\/conference-invites)\/([A-Za-z0-9_-]{32})\/?$/,
    );
    return match?.[1] || null;
  } catch {
    return null;
  }
}
export const inviteLink = (code: string) =>
  `${window.location.origin}/i/${encodeURIComponent(code)}`;
export const formatDate = (value: string) =>
  new Intl.DateTimeFormat("ru-RU", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
export const utf8Bytes = (value: string) =>
  new TextEncoder().encode(value).length;
export const passwordLength = (value: string) => Array.from(value).length;
export const initials = (name: string) =>
  Array.from(name.trim())[0]?.toUpperCase() || "М";
