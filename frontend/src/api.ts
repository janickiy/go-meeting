import type {
  Conference,
  ConferenceStatus,
  Invite,
  Item,
  Items,
  LoginResponse,
  Participant,
  User,
  ParticipantMediaState,
  ModerationAction,
  ConferenceRecording,
  ConferenceFilters,
  ConferenceInput,
  ConferenceHistory,
  CursorItems,
  ChatPage,
  ChatMessage,
  ChatReadState,
  ChatAttachment,
  RaisedHand,
  ReactionEmoji,
  NotificationsPage,
  Notification,
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
  "conference is read-only":
    "Встреча завершена. Чат доступен только для чтения.",
};
const statuses: Record<number, string> = {
  400: "Проверьте данные запроса.",
  401: "Войдите в аккаунт. Возможно, срок сессии истёк.",
  403: "У вас нет доступа к этому действию.",
  404: "Конференция или приглашение не найдены.",
  409: "Действие недоступно в текущем состоянии.",
  413: "Файл слишком большой. Максимальный размер — 10 МБ.",
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
  myConferences: (
    filters: ConferenceFilters = {},
    cursor?: string,
    signal?: AbortSignal,
  ) => {
    const params = new URLSearchParams({ limit: "20" });
    for (const [key, value] of Object.entries(filters))
      if (value) params.set(key, value);
    if (cursor) params.set("cursor", cursor);
    return request<CursorItems<Conference>>(`/me/conferences?${params}`, {
      signal,
    });
  },
  myMembership: (id: string, signal?: AbortSignal) =>
    request<Item<Participant>>(
      `/conferences/${encodeURIComponent(id)}/participants/me`,
      { signal },
    ),
  admit: (id: string, participantId: string, decision: "admit" | "reject") =>
    request<Item<Participant>>(
      `/conferences/${encodeURIComponent(id)}/participants/${encodeURIComponent(participantId)}/admission`,
      { method: "POST", body: { decision } },
    ),
  schedule: (
    id: string,
    scheduledAt: string,
    plannedDurationMin: number | null,
  ) =>
    request<Item<Conference>>(
      `/conferences/${encodeURIComponent(id)}/schedule`,
      { method: "PUT", body: { scheduledAt, plannedDurationMin } },
    ),
  history: (id: string, signal?: AbortSignal) =>
    request<Item<ConferenceHistory>>(
      `/conferences/${encodeURIComponent(id)}/history`,
      { signal },
    ),
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
  setMediaState: (id: string, state: ParticipantMediaState) =>
    request<Item<Participant>>(
      `/conferences/${encodeURIComponent(id)}/participants/me/media`,
      { method: "PUT", body: state },
    ),
  moderate: (id: string, participantId: string, action: ModerationAction) =>
    request<Item<Participant>>(
      `/conferences/${encodeURIComponent(id)}/participants/${encodeURIComponent(participantId)}/moderation`,
      { method: "POST", body: action },
    ),
  recordings: (id: string, signal?: AbortSignal) =>
    request<Items<ConferenceRecording>>(
      `/conferences/${encodeURIComponent(id)}/recordings`,
      { signal },
    ),
  startRecording: (id: string) =>
    request<Item<ConferenceRecording>>(
      `/conferences/${encodeURIComponent(id)}/recordings`,
      { method: "POST", body: { segmentDurationSec: 5 } },
    ),
  stopRecording: (id: string, recordingId: string) =>
    request<Item<ConferenceRecording>>(
      `/conferences/${encodeURIComponent(id)}/recordings/${encodeURIComponent(recordingId)}/stop`,
      { method: "POST", body: {} },
    ),
  create: (input: string | ConferenceInput) =>
    request<Item<Conference>>("/conferences", {
      method: "POST",
      body: typeof input === "string" ? { title: input } : input,
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
  messages: (id: string, before?: string, signal?: AbortSignal) =>
    request<ChatPage>(
      `/conferences/${encodeURIComponent(id)}/messages?limit=50${before ? `&before=${encodeURIComponent(before)}` : ""}`,
      { signal },
    ),
  sendMessage: (
    id: string,
    body: {
      clientRequestId: string;
      text: string;
      replyTo?: string;
      attachmentIds?: string[];
    },
  ) =>
    request<Item<ChatMessage>>(
      `/conferences/${encodeURIComponent(id)}/messages`,
      { method: "POST", body },
    ),
  editMessage: (id: string, messageId: string, text: string) =>
    request<Item<ChatMessage>>(
      `/conferences/${encodeURIComponent(id)}/messages/${encodeURIComponent(messageId)}`,
      { method: "PATCH", body: { text } },
    ),
  deleteMessage: (id: string, messageId: string) =>
    request<Item<ChatMessage>>(
      `/conferences/${encodeURIComponent(id)}/messages/${encodeURIComponent(messageId)}`,
      { method: "DELETE" },
    ),
  chatRead: (id: string, signal?: AbortSignal) =>
    request<Item<ChatReadState>>(
      `/conferences/${encodeURIComponent(id)}/chat/read`,
      { signal },
    ),
  markChatRead: (id: string, messageId: string) =>
    request<Item<ChatReadState>>(
      `/conferences/${encodeURIComponent(id)}/chat/read`,
      { method: "PUT", body: { messageId } },
    ),
  initAttachment: (
    id: string,
    body: {
      clientRequestId: string;
      filename: string;
      size: number;
      mimeType: string;
    },
  ) =>
    request<Item<ChatAttachment> & { uploadUrl: string }>(
      `/conferences/${encodeURIComponent(id)}/attachments/init`,
      { method: "POST", body },
    ),
  finalizeAttachment: (id: string, attachmentId: string) =>
    request<Item<ChatAttachment>>(
      `/conferences/${encodeURIComponent(id)}/attachments/${encodeURIComponent(attachmentId)}/finalize`,
      { method: "POST", body: {} },
    ),
  attachmentDownload: (id: string, attachmentId: string) =>
    request<{ url: string; expiresAt: string }>(
      `/conferences/${encodeURIComponent(id)}/attachments/${encodeURIComponent(attachmentId)}/download`,
    ),
  hands: (id: string, signal?: AbortSignal) =>
    request<Items<RaisedHand>>(`/conferences/${encodeURIComponent(id)}/hands`, {
      signal,
    }),
  hand: (id: string, participantId: string, raised: boolean) =>
    request(
      `/conferences/${encodeURIComponent(id)}/participants/${encodeURIComponent(participantId)}/hand`,
      { method: "PUT", body: { raised } },
    ),
  reaction: (id: string, emoji: ReactionEmoji) =>
    request(`/conferences/${encodeURIComponent(id)}/reactions`, {
      method: "POST",
      body: { emoji },
    }),
  notifications: (cursor?: string, signal?: AbortSignal) =>
    request<NotificationsPage>(
      `/notifications?limit=30${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`,
      { signal },
    ),
  readNotification: (id: string) =>
    request<Item<Notification>>(
      `/notifications/${encodeURIComponent(id)}/read`,
      { method: "POST" },
    ),
  notificationEvents: async (signal: AbortSignal) => {
    const usedToken = accessToken;
    const response = await fetch("/api/v1/notifications/events", {
      headers: {
        Accept: "text/event-stream",
        ...(usedToken ? { Authorization: `Bearer ${usedToken}` } : {}),
      },
      credentials: "omit",
      signal,
    });
    if (!response.ok) {
      if (response.status === 401 && usedToken) invalidSession(usedToken);
      throw new ApiError(
        response.status,
        statuses[response.status] || "Уведомления временно недоступны.",
      );
    }
    if (
      !response.body ||
      !response.headers.get("Content-Type")?.startsWith("text/event-stream")
    )
      throw new ApiError(502, "Некорректный поток уведомлений.");
    return response;
  },
};
export function uploadAttachment(
  url: string,
  file: File,
  onProgress: (percent: number) => void,
  signal: AbortSignal,
): Promise<void> {
  const target = new URL(url, window.location.origin);
  if (
    target.origin !== window.location.origin ||
    !/^\/api\/v1\/conferences\/[^/]+\/attachments\/[^/]+\/content$/.test(
      target.pathname,
    ) ||
    target.search ||
    target.hash
  )
    return Promise.reject(new ApiError(502, "Некорректный адрес загрузки."));
  return new Promise((resolve, reject) => {
    const usedToken = accessToken;
    const xhr = new XMLHttpRequest();
    const abort = () => xhr.abort();
    xhr.open("PUT", target.pathname);
    xhr.timeout = 120000;
    if (usedToken) xhr.setRequestHeader("Authorization", `Bearer ${usedToken}`);
    xhr.setRequestHeader(
      "Content-Type",
      file.type || "application/octet-stream",
    );
    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable)
        onProgress(Math.round((event.loaded / event.total) * 100));
    };
    xhr.onload = () => {
      if (xhr.status === 401 && usedToken) invalidSession(usedToken);
      if (xhr.status >= 200 && xhr.status < 300) resolve();
      else
        reject(
          new ApiError(
            xhr.status,
            statuses[xhr.status] || "Не удалось загрузить файл.",
          ),
        );
    };
    xhr.onerror = xhr.ontimeout = () => reject(new Error("upload_unavailable"));
    xhr.onabort = () =>
      reject(new DOMException("Upload aborted", "AbortError"));
    xhr.onloadend = () => signal.removeEventListener("abort", abort);
    signal.addEventListener("abort", abort, { once: true });
    if (signal.aborted) {
      signal.removeEventListener("abort", abort);
      reject(new DOMException("Upload aborted", "AbortError"));
      return;
    }
    xhr.send(file);
  });
}
export const statusLabels: Record<ConferenceStatus, string> = {
  created: "Ожидает начала",
  scheduled: "Запланирована",
  active: "Идёт сейчас",
  finished: "Завершена",
  cancelled: "Отменена",
};
