import type {
  Conference,
  ConferenceStatus,
  Invite,
  Item,
  Items,
  LoginResponse,
  Participant,
  User,
} from "./types";

let accessToken: string | null = null;
let invalidSession: (usedToken: string) => void = () => {};
export function configureAuth(
  token: string | null,
  onInvalid?: (usedToken: string) => void,
) {
  accessToken = token;
  if (onInvalid) invalidSession = onInvalid;
}
export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}
const messages: Record<string, string> = {
  "password must contain 8 to 128 characters":
    "Пароль должен содержать от 8 до 128 символов.",
  "invalid email or password": "Неверный email или пароль.",
  "email is already registered":
    "Этот email уже зарегистрирован. Попробуйте войти.",
  "conference is closed": "Эта конференция уже завершена или отменена.",
  "conference status does not allow this transition":
    "Состояние конференции изменилось. Обновите страницу.",
  "inviteCode is required for a new membership":
    "Для входа в эту конференцию нужна ссылка-приглашение.",
};
const statuses: Record<number, string> = {
  400: "Проверьте данные запроса.",
  401: "Войдите в аккаунт. Возможно, срок сессии истёк.",
  403: "У вас нет доступа к этому действию.",
  404: "Конференция или приглашение не найдены.",
  409: "Действие недоступно в текущем состоянии.",
  422: "Проверьте введённые данные.",
  429: "Слишком много попыток. Подождите минуту и попробуйте снова.",
  500: "Сервис временно недоступен. Попробуйте позже.",
};
export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  return "Не удалось связаться с сервером. Проверьте подключение и попробуйте снова.";
}
async function request<T>(
  path: string,
  options: {
    body?: unknown;
    method?: string;
    signal?: AbortSignal;
    public?: boolean;
  } = {},
): Promise<T> {
  const usedToken = options.public ? null : accessToken;
  const headers = new Headers({ Accept: "application/json" });
  if (options.body !== undefined)
    headers.set("Content-Type", "application/json");
  if (usedToken) headers.set("Authorization", `Bearer ${usedToken}`);
  const response = await fetch(`/api/v1${path}`, {
    method: options.method || "GET",
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
    signal: options.signal,
    credentials: "omit",
  });
  const data: unknown = await response.json().catch(() => null);
  if (!response.ok) {
    if (response.status === 401 && usedToken) invalidSession(usedToken);
    const message =
      data &&
      typeof data === "object" &&
      "message" in data &&
      typeof data.message === "string"
        ? data.message
        : "";
    throw new ApiError(
      response.status,
      messages[message] ||
        statuses[response.status] ||
        "Не удалось выполнить запрос.",
    );
  }
  if (!data) throw new ApiError(502, "Сервер вернул некорректный ответ.");
  return data as T;
}
export const api = {
  wsTicket: (id: string, signal?: AbortSignal) =>
    request<{ ticket: string; expiresAt: string }>(
      `/conferences/${encodeURIComponent(id)}/ws-ticket`,
      { method: "POST", signal },
    ),
  iceConfig: (signal?: AbortSignal) =>
    request<{ iceServers: RTCIceServer[] }>("/webrtc/config", { signal }),
  register: (email: string, password: string, displayName: string) =>
    request<{ user: User }>("/auth/register", {
      method: "POST",
      body: { email, password, displayName: displayName.trim() || null },
      public: true,
    }),
  login: (email: string, password: string) =>
    request<LoginResponse>("/auth/login", {
      method: "POST",
      body: { email, password },
      public: true,
    }),
  me: (signal?: AbortSignal) => request<{ user: User }>("/auth/me", { signal }),
  logout: () => request("/auth/logout", { method: "POST" }),
  conferences: (offset = 0, signal?: AbortSignal) =>
    request<Items<Conference>>(`/conferences?limit=20&offset=${offset}`, {
      signal,
    }),
  conference: (id: string, signal?: AbortSignal) =>
    request<Item<Conference>>(`/conferences/${encodeURIComponent(id)}`, {
      signal,
    }),
  participants: (id: string, offset = 0, signal?: AbortSignal) =>
    request<Items<Participant>>(
      `/conferences/${encodeURIComponent(id)}/participants?limit=100&offset=${offset}`,
      { signal },
    ),
  create: (title: string) =>
    request<Item<Conference>>("/conferences", {
      method: "POST",
      body: { title },
    }),
  transition: (id: string, action: "start" | "finish" | "cancel") =>
    request<Item<Conference>>(
      `/conferences/${encodeURIComponent(id)}/${action}`,
      { method: "POST" },
    ),
  membership: (id: string, action: "join" | "leave") =>
    request<Item<Participant>>(
      `/conferences/${encodeURIComponent(id)}/${action}`,
      { method: "POST" },
    ),
  invite: (code: string, signal?: AbortSignal) =>
    request<Item<Invite>>(`/conference-invites/${encodeURIComponent(code)}`, {
      signal,
    }),
  joinInvite: (code: string) =>
    request<Item<Participant>>(
      `/conference-invites/${encodeURIComponent(code)}/join`,
      { method: "POST" },
    ),
};
export const statusLabels: Record<ConferenceStatus, string> = {
  created: "Ожидает начала",
  active: "Идёт сейчас",
  finished: "Завершена",
  cancelled: "Отменена",
};
