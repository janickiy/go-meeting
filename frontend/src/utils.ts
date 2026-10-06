import type { User } from "./types";

export const SESSION_KEY = "meet.session.v1";
export const LOGOUT_KEY = "meet.auth.logout.v1";
/**
 * Session связывает сохранённый токен, пользователя и сведения клиентской авторизации.
 *
 * @params:
 *   - token — токен текущей авторизации; null отключает авторизованные запросы.
 *   - expiresAt — поле или операция этого контракта.
 */
export interface Session {
  token: string;
  expiresAt: number;
  user?: User;
}
/**
 * readSession читает и проверяет сохранённую клиентскую сессию и отвергает повреждённое значение.
 *
 *
 * @returns Session | null — вычисленное значение: data as Session; null.
 */
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
      data.expiresAt > 0 &&
      data.expiresAt <= Date.now() + 3600000
    ) {
      const session: Session = { token: data.token, expiresAt: data.expiresAt };
      if ("user" in data && data.user && typeof data.user === "object") {
        const user = data.user as Partial<User>;
        if (
          typeof user.id === "string" &&
          typeof user.email === "string" &&
          (typeof user.displayName === "string" || user.displayName === null) &&
          typeof user.createdAt === "string" &&
          typeof user.updatedAt === "string"
        )
          session.user = user as User;
      }
      // An expired access token is only a cache. The HttpOnly cookie restores the session.
      return session;
    }
    sessionStorage.removeItem(SESSION_KEY);
  } catch {
    try {
      sessionStorage.removeItem(SESSION_KEY);
    } catch {
      /* Хранилище может быть заблокировано; используем только память. */
    }
  }
  return null;
}
/**
 * saveSession сохраняет либо удаляет сессию в локальном хранилище браузера.
 *
 * @args
 *   - session (Session | null) — проверенная клиентская сессия либо null для удаления.
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */
export function saveSession(session: Session | null) {
  try {
    if (session) sessionStorage.setItem(SESSION_KEY, JSON.stringify(session));
    else sessionStorage.removeItem(SESSION_KEY);
  } catch {
    /* Не сохраняем учётные данные в других местах. */
  }
}

/** Contains only an explicit logout marker; credentials never enter localStorage. */
export function hasLogoutMarker(): boolean {
  try {
    return localStorage.getItem(LOGOUT_KEY) !== null;
  } catch {
    return false;
  }
}

export function markLogout(): void {
  try {
    localStorage.setItem(LOGOUT_KEY, `${Date.now()}:${crypto.randomUUID()}`);
  } catch {
    /* The current tab still clears its in-memory session. */
  }
}

export function clearLogoutMarker(): void {
  try {
    localStorage.removeItem(LOGOUT_KEY);
  } catch {
    /* Storage may be unavailable. */
  }
}
/**
 * safeNext проверяет локальный путь возврата после авторизации и исключает внешний переход.
 *
 * @args
 *   - value (string | null) — значение для проверки, преобразования или отображения.
 *
 * @returns string — вычисленное значение: "/app"; url.pathname + url.search.
 */
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
      !/^\/(?:app(?:\/[^]*)?|conferences(?:\/[^]*)?|recordings(?:\/[^]*)?|personal(?:\/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})?|i\/[A-Za-z0-9_-]{32})$/.test(
        url.pathname,
      )
    )
      return "/app";
    return url.pathname + url.search;
  } catch {
    return "/app";
  }
}
/**
 * inviteCode извлекает допустимый код приглашения из кода или ссылки.
 *
 * @args
 *   - value (string) — значение для проверки, преобразования или отображения.
 *
 * @returns string | null — вычисленное значение: trimmed; null; match?.[1] || null.
 */
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
/**
 * inviteLink строит ссылку приглашения для текущего адреса приложения.
 *
 * @args
 *   - code (string) — проверенный код приглашения.
 *
 * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
 */
export const inviteLink = (code: string) =>
  `${window.location.origin}/i/${encodeURIComponent(code)}`;
/**
 * formatDate форматирует временную отметку для отображения даты и времени встречи.
 *
 * @args
 *   - value (string) — значение для проверки, преобразования или отображения.
 *
 * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
 */
export const formatDate = (value: string) =>
  new Intl.DateTimeFormat("ru-RU", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
/**
 * utf8Bytes считает длину строки в байтах UTF-8.
 *
 * @args
 *   - value (string) — значение для проверки, преобразования или отображения.
 *
 * @returns вычисленное значение: new TextEncoder().encode(value).length.
 */
export const utf8Bytes = (value: string) =>
  new TextEncoder().encode(value).length;
/**
 * passwordLength считает символы Unicode пароля без привязки к числу байтов.
 *
 * @args
 *   - value (string) — значение для проверки, преобразования или отображения.
 *
 * @returns вычисленное значение: Array.from(value).length.
 */
export const passwordLength = (value: string) => Array.from(value).length;
/**
 * initials выбирает инициалы имени для аватара участника.
 *
 * @args
 *   - name (string) — отображаемое имя пользователя для инициалов.
 *
 * @returns вычисленное значение: Array.from(name.trim())[0]?.toUpperCase() || "М".
 */
export const initials = (name: string) =>
  Array.from(name.trim())[0]?.toUpperCase() || "М";
