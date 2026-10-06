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
  ReactionEmoji,
  NotificationsPage,
  Notification,
  NotificationPreferences,
  IntegrationCapabilities,
  CalendarConnection,
  CalendarSync,
  TranscriptView,
  TranscriptSegment,
  SummaryView,
  OffsetPage,
  SearchFilters,
  SearchResult,
  RecordingMode,
  Caption,
  CaptionState,
  MeetingAnalytics,
  CapabilitiesResponse,
  AdminSummary,
  InvitationUser,
  ConferenceInvitation,
} from "./types";

let accessToken: string | null = null;
let identityVersion = 0;
type SessionRecovery = (
  usedToken: string,
) => boolean | void | Promise<boolean | void>;
let invalidSession: SessionRecovery = () => false;
const tokenListeners = new Set<() => void>();

export function getAccessToken(): string | null {
  return accessToken;
}

/** Token renewal is independent of the mounted account and conference. */
export function subscribeAccessToken(listener: () => void): () => void {
  tokenListeners.add(listener);
  return () => tokenListeners.delete(listener);
}

/** Returns false when the account changed while the old request was pending. */
async function recoverSession(
  usedToken: string,
  version: number,
): Promise<boolean> {
  const recovered = await invalidSession(usedToken);
  return recovered === true && !!accessToken && version === identityVersion;
}

export function refreshAccessToken(): Promise<boolean> {
  return accessToken
    ? recoverSession(accessToken, identityVersion)
    : Promise.resolve(false);
}
/**
 * configureAuth сохраняет текущий токен и обработчик ответа 401; обработчик получает именно токен, использованный неудачным запросом.
 *
 * @args
 *   - token (string | null) — токен текущей авторизации; null отключает авторизованные запросы.
 *   - onInvalid ((usedToken: string) => void) — обработчик отказа авторизации с использованным токеном (необязательный параметр).
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */
export function configureAuth(
  token: string | null,
  onInvalid?: SessionRecovery,
) {
  if (token === null) identityVersion++;
  const changed = accessToken !== token;
  accessToken = token;
  if (onInvalid) invalidSession = onInvalid;
  if (changed) tokenListeners.forEach((listener) => listener());
}
/**
 * ApiError связывает HTTP-статус с понятным пользователю сообщением ошибки API.
 *
 * @params:
 *   - constructor — создаёт экземпляр класса с переданными параметрами.
 */
export class ApiError extends Error {
  /**
   * constructor создаёт ошибку API с HTTP-статусом и понятным сообщением.
   *
   * @args
   *   - status (number) — HTTP-статус либо состояние встречи.
   *   - message (string) — понятный текст ошибки или сообщение операции.
   *
   * @returns инициализированный экземпляр текущего класса.
   */
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
  "recording is already active":
    "Запись уже запущена. Дождитесь завершения текущей записи перед запуском новой.",
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
/**
 * errorMessage выбирает безопасный текст ошибки API или сообщение о недоступной сети.
 *
 * @args
 *   - error (unknown) — пойманная ошибка API или сети.
 *
 * @returns string — безопасный текст для отображения ошибки пользователю.
 */
export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  return "Не удалось связаться с сервером. Проверьте подключение и попробуйте снова.";
}
/**
 * request отправляет JSON-запрос к API, добавляет Bearer-токен для приватного маршрута и проверяет ответ; при 401 уведомляет владельца использованной сессии.
 *
 * @args
 *   - path (string) — локальный путь API без базового префикса.
 *   - options ({ body?: unknown; method?: string; signal?: AbortSignal; public?: boolean; }) — метод, тело, отмена и признаки авторизации запроса (по умолчанию {}).
 *
 * @returns Promise<T> — Promise с результатом описанной асинхронной операции; отказ передаётся через отклонение Promise.
 */
async function request<T>(
  path: string,
  options: {
    body?: unknown;
    method?: string;
    signal?: AbortSignal;
    public?: boolean;
    sessionCookie?: boolean;
    recover?: boolean;
    token?: string | null;
  } = {},
  retry = true,
): Promise<T> {
  const usedToken =
    options.token !== undefined
      ? options.token
      : options.public
        ? null
        : accessToken;
  const version = identityVersion;
  const headers = new Headers({ Accept: "application/json" });
  if (options.body !== undefined)
    headers.set("Content-Type", "application/json");
  if (usedToken) headers.set("Authorization", `Bearer ${usedToken}`);
  const response = await fetch(`/api/v1${path}`, {
    method: options.method || "GET",
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
    signal: options.signal,
    credentials: options.sessionCookie ? "same-origin" : "omit",
  });
  if (response.status === 204) return undefined as T;
  const data: unknown = await response.json().catch(
    /**
     * Обработчик catch выполняет переданный шаг вызова catch в типизированных HTTP-запросах.
     *
     *
     * @returns вычисленное значение: null.
     */ () => null,
  );
  if (!response.ok) {
    if (
      response.status === 401 &&
      usedToken &&
      retry &&
      options.recover !== false
    ) {
      if (await recoverSession(usedToken, version)) {
        options.signal?.throwIfAborted();
        return request<T>(path, options, false);
      }
    }
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
  /** Читает эффективные серверные функции без адресов и секретов провайдеров. */
  capabilities: (signal?: AbortSignal) =>
    request<CapabilitiesResponse>("/capabilities", { signal }),
  /** Читает только безопасные агрегаты после отдельной серверной проверки admin. */
  adminSummary: (signal?: AbortSignal) =>
    request<Item<AdminSummary>>("/admin/summary", { signal }),
  /**
   * myConferences читает страницу встреч пользователя с серверными фильтрами и курсором продолжения.
   *
   * @args
   *   - filters (ConferenceFilters) — серверные фильтры списка встреч (по умолчанию {}).
   *   - cursor (string) — непрозрачный курсор продолжения страницы (необязательный параметр).
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
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
  /**
   * myMembership читает собственное членство и состояние допуска в конференции.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  myMembership: (id: string, signal?: AbortSignal) =>
    request<Item<Participant>>(
      `/conferences/${encodeURIComponent(id)}/participants/me`,
      { signal },
    ),
  /** Searches registered accounts inside an authorised meeting invitation flow. */
  invitationUsers: (id: string, query: string, signal?: AbortSignal) =>
    request<Items<InvitationUser>>(
      `/conferences/${encodeURIComponent(id)}/invitation-users?${new URLSearchParams({ query })}`,
      { signal },
    ),
  /** Queues meeting invitation emails and the recipients' account notifications. */
  inviteParticipants: (
    id: string,
    recipients: { emails?: string[]; userIds?: string[] },
  ) =>
    request<Items<ConferenceInvitation>>(
      `/conferences/${encodeURIComponent(id)}/invitations`,
      { method: "POST", body: recipients },
    ),
  /**
   * admit отправляет решение организатора о допуске или отклонении участника.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - participantId (string) — идентификатор членства целевого участника.
   *   - decision ("admit" | "reject") — решение admit или reject.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  admit: (id: string, participantId: string, decision: "admit" | "reject") =>
    request<Item<Participant>>(
      `/conferences/${encodeURIComponent(id)}/participants/${encodeURIComponent(participantId)}/admission`,
      { method: "POST", body: { decision } },
    ),
  /**
   * schedule изменяет UTC-расписание ещё не начавшейся встречи.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - scheduledAt (string) — однозначная ISO-временная отметка встречи.
   *   - plannedDurationMin (number | null) — длительность в минутах либо null, если она не указана.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  schedule: (
    id: string,
    scheduledAt: string,
    plannedDurationMin: number | null,
  ) =>
    request<Item<Conference>>(
      `/conferences/${encodeURIComponent(id)}/schedule`,
      { method: "PUT", body: { scheduledAt, plannedDurationMin } },
    ),
  /**
   * history читает авторизованную сводку завершённой встречи и её записей.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  history: (id: string, signal?: AbortSignal) =>
    request<Item<ConferenceHistory>>(
      `/conferences/${encodeURIComponent(id)}/history`,
      { signal },
    ),
  /**
   * wsTicket запрашивает краткоживущий одноразовый билет подключения к комнате.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  wsTicket: (id: string, signal?: AbortSignal) =>
    request<{ ticket: string; expiresAt: string }>(
      `/conferences/${encodeURIComponent(id)}/ws-ticket`,
      { method: "POST", signal },
    ),
  /**
   * iceConfig читает доступные настройки ICE для установления WebRTC-соединения.
   *
   * @args
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  iceConfig: (signal?: AbortSignal) =>
    request<{ iceServers: RTCIceServer[] }>("/webrtc/config", { signal }),
  /**
   * register отправляет данные регистрации с нормализацией необязательного отображаемого имени.
   *
   * @args
   *   - email (string) — адрес электронной почты.
   *   - password (string) — пароль из формы; не предназначен для журналирования.
   *   - displayName (string) — необязательное отображаемое имя пользователя.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  register: (email: string, password: string, displayName: string) =>
    request<{ user: User }>("/auth/register", {
      method: "POST",
      body: { email, password, displayName: displayName.trim() || null },
      public: true,
    }),
  /**
   * login отправляет учётные данные и получает токен и сведения пользователя.
   *
   * @args
   *   - email (string) — адрес электронной почты.
   *   - password (string) — пароль из формы; не предназначен для журналирования.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  login: (email: string, password: string, signal?: AbortSignal) =>
    request<LoginResponse>("/auth/login", {
      method: "POST",
      body: { email, password },
      public: true,
      sessionCookie: true,
      signal,
    }),
  /** Restores an account using its persistent HttpOnly cookie. */
  refreshSession: (signal?: AbortSignal) =>
    request<LoginResponse>("/auth/refresh", {
      method: "POST",
      body: {},
      public: true,
      sessionCookie: true,
      recover: false,
      signal,
    }),
  /** Upgrades a still-valid pre-existing account token to a persistent session. */
  establishSession: (signal?: AbortSignal) =>
    request<LoginResponse>("/auth/session", {
      method: "POST",
      body: {},
      sessionCookie: true,
      recover: false,
      signal,
    }),
  /**
   * me читает публичные сведения текущей авторизованной учётной записи.
   *
   * @args
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  me: (signal?: AbortSignal) => request<{ user: User }>("/auth/me", { signal }),
  /** Обновляет имя текущего пользователя, оставляя токен и остальные поля без изменений. */
  updateProfile: (displayName: string) =>
    request<{ status: string; user: User }>("/auth/me", {
      method: "PATCH",
      body: { displayName },
    }),
  /**
   * logout отправляет запрос завершения авторизации.
   *
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  logout: (token: string | null = accessToken) =>
    request("/auth/logout", {
      method: "POST",
      body: {},
      token,
      sessionCookie: true,
      recover: false,
    }),
  /**
   * conferences читает ограниченную страницу доступных конференций.
   *
   * @args
   *   - offset — смещение страницы списка (по умолчанию 0).
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  conferences: (offset = 0, signal?: AbortSignal) =>
    request<Items<Conference>>(`/conferences?limit=20&offset=${offset}`, {
      signal,
    }),
  /**
   * conference читает разрешённые сведения одной конференции.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  conference: (id: string, signal?: AbortSignal) =>
    request<Item<Conference>>(`/conferences/${encodeURIComponent(id)}`, {
      signal,
    }),
  /**
   * participants читает страницу участников, отфильтрованную сервером по полномочиям текущего пользователя.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - offset — смещение страницы списка (по умолчанию 0).
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  participants: (id: string, offset = 0, signal?: AbortSignal) =>
    request<Items<Participant>>(
      `/conferences/${encodeURIComponent(id)}/participants?limit=100&offset=${offset}`,
      { signal },
    ),
  /**
   * setMediaState сохраняет фактические признаки источников медиа конкретного физического подключения.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - state (ParticipantMediaState) — новое состояние источников медиа.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  setMediaState: (id: string, state: ParticipantMediaState) =>
    request<Item<Participant>>(
      `/conferences/${encodeURIComponent(id)}/participants/me/media`,
      { method: "PUT", body: state },
    ),
  /**
   * moderate отправляет действие модерации над участником конференции.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - participantId (string) — идентификатор членства целевого участника.
   *   - action (ModerationAction) — разрешённое действие управления либо асинхронная операция.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  moderate: (id: string, participantId: string, action: ModerationAction) =>
    request<Item<Participant>>(
      `/conferences/${encodeURIComponent(id)}/participants/${encodeURIComponent(participantId)}/moderation`,
      { method: "POST", body: action },
    ),
  /**
   * recordings читает защищённую страницу записей выбранной конференции.
   * @args id — конференция; signal — отмена запроса; pagination — лимит и смещение.
   * Без pagination сохраняет прежний контракт для панели действующей встречи.
   * @return записи и разрешённые backend ссылки на их реальные артефакты.
   */
  recordings: (
    id: string,
    signal?: AbortSignal,
    pagination?: { limit: number; offset: number },
  ) => {
    const params = pagination
      ? new URLSearchParams({
          limit: String(pagination.limit),
          offset: String(pagination.offset),
        })
      : null;
    return request<Items<ConferenceRecording>>(
      `/conferences/${encodeURIComponent(id)}/recordings${params ? `?${params}` : ""}`,
      { signal },
    );
  },
  /**
   * Читает отдельную приватную запись, в том числе по ссылке из поиска.
   * @args id, recordingId — идентификаторы встречи и записи; signal — отмена запроса.
   * @return Запись со ссылками, разрешёнными текущему пользователю.
   */
  recording: (id: string, recordingId: string, signal?: AbortSignal) =>
    request<Item<ConferenceRecording>>(
      `/conferences/${encodeURIComponent(id)}/recordings/${encodeURIComponent(recordingId)}`,
      { signal },
    ),
  /**
   * Читает состояние расшифровки без запуска обработки.
   * @args id, recordingId — идентификаторы; signal — отмена запроса.
   * @return Nullable расшифровка и проверенные сервером возможности.
   */
  transcript: (id: string, recordingId: string, signal?: AbortSignal) =>
    request<TranscriptView>(
      `/conferences/${encodeURIComponent(id)}/recordings/${encodeURIComponent(recordingId)}/transcript`,
      { signal },
    ),
  /**
   * Загружает ограниченную страницу сегментов в хронологическом порядке.
   * @args id, recordingId — идентификаторы; offset — смещение; signal — отмена.
   * @return Страница текста с временными метками.
   */
  transcriptSegments: (
    id: string,
    recordingId: string,
    offset: number,
    signal?: AbortSignal,
  ) =>
    request<OffsetPage<TranscriptSegment>>(
      `/conferences/${encodeURIComponent(id)}/recordings/${encodeURIComponent(recordingId)}/transcript/segments?limit=100&offset=${offset}`,
      { signal },
    ),
  /**
   * Явно запрашивает разрешённый организатору повтор распознавания.
   * @args id, recordingId — идентификаторы встречи и записи.
   * @return Обновлённое состояние очереди либо отказ авторизации/лимита.
   */
  retryTranscript: (id: string, recordingId: string) =>
    request<TranscriptView>(
      `/conferences/${encodeURIComponent(id)}/recordings/${encodeURIComponent(recordingId)}/transcript/retry`,
      { method: "POST", body: {} },
    ),
  /**
   * Читает сохранённые итоги ИИ, не вызывая провайдера.
   * @args id, recordingId — идентификаторы; signal — отмена запроса.
   * @return Nullable итоги и право повторного запуска.
   */
  summary: (id: string, recordingId: string, signal?: AbortSignal) =>
    request<SummaryView>(
      `/conferences/${encodeURIComponent(id)}/recordings/${encodeURIComponent(recordingId)}/summary`,
      { signal },
    ),
  /**
   * Ставит новую генерацию итогов по явной команде организатора.
   * @args id, recordingId — идентификаторы встречи и записи.
   * @return Принятое задание либо безопасная ошибка лимита.
   */
  regenerateSummary: (id: string, recordingId: string) =>
    request<SummaryView>(
      `/conferences/${encodeURIComponent(id)}/recordings/${encodeURIComponent(recordingId)}/summary/regenerate`,
      { method: "POST", body: {} },
    ),
  /**
   * Передаёт фильтры полнотекстового поиска с авторизацией в заголовке.
   * @args filters — запрос и UTC-даты; offset — смещение; signal — отмена.
   * @return Страница доступных результатов без выдачи URL хранилища.
   */
  search: (filters: SearchFilters, offset: number, signal?: AbortSignal) => {
    const params = new URLSearchParams({
      q: filters.q,
      source: filters.source,
      limit: "20",
      offset: String(offset),
    });
    if (filters.conferenceId) params.set("conferenceId", filters.conferenceId);
    if (filters.from) params.set("from", filters.from);
    if (filters.to) params.set("to", filters.to);
    if (filters.mode) params.set("mode", filters.mode);
    if (filters.membership) params.set("membership", filters.membership);
    if (filters.participantId)
      params.set("participantId", filters.participantId);
    return request<
      OffsetPage<SearchResult> & {
        effectiveMode?: string;
        fallbackReason?: string;
      }
    >(`/search?${params}`, { signal });
  },
  /**
   * Читает предпочтения внешних уведомлений текущего пользователя.
   * @args signal — необязательная отмена запроса.
   * @return Независимые флаги событий и каналов.
   */
  notificationPreferences: (signal?: AbortSignal) =>
    request<NotificationPreferences>("/notifications/preferences", { signal }),
  /**
   * Сохраняет только явно выбранные пользователем предпочтения.
   * @args body — полный набор флагов без идентификатора другого пользователя.
   * @return Подтверждённые сервером настройки.
   */
  saveNotificationPreferences: (body: NotificationPreferences) =>
    request<NotificationPreferences>("/notifications/preferences", {
      method: "PUT",
      body,
    }),
  /**
   * Читает доступность интеграций без секретов провайдеров.
   * @args signal — отмена запроса.
   * @return Режимы noop/mock/http и доступность OAuth.
   */
  integrationCapabilities: (signal?: AbortSignal) =>
    request<IntegrationCapabilities>("/integrations/capabilities", { signal }),
  /**
   * Перечисляет собственные подключения календаря.
   * @args signal — отмена запроса.
   * @return Безопасные метаданные без OAuth-токенов.
   */
  calendars: (signal?: AbortSignal) =>
    request<Items<CalendarConnection>>("/integrations/calendars", { signal }),
  /**
   * Создаёт демонстрационное подключение только при разрешении сервера.
   * @return Тестовое подключение либо отказ для рабочей среды.
   */
  connectMockCalendar: () =>
    request<Item<CalendarConnection>>("/integrations/calendars/mock", {
      method: "POST",
      body: {},
    }),
  /**
   * Начинает серверный OAuth/PKCE без передачи провайдерского токена в браузер.
   * @args provider — известный серверу адаптер календаря.
   * @return URL выдачи разрешения и одноразовое состояние.
   */
  calendarConnect: (provider: string) =>
    request<{ authUrl: string; state: string }>(
      `/integrations/calendars/${encodeURIComponent(provider)}/connect`,
    ),
  /**
   * Обменивает одноразовый код в контексте действующей пользовательской сессии.
   * @args provider — адаптер; code, state — параметры возврата OAuth.
   * @return Только публичные метаданные подключения.
   */
  calendarCallback: (provider: string, code: string, state: string) =>
    request<Item<CalendarConnection>>(
      `/integrations/calendars/${encodeURIComponent(provider)}/callback`,
      { method: "POST", body: { code, state } },
    ),
  /**
   * Отзывает собственное подключение календаря.
   * @args id — идентификатор подключения, не секрет провайдера.
   * @return Подтверждение 204 без JSON-тела.
   */
  disconnectCalendar: (id: string) =>
    request<void>(`/integrations/calendars/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),
  /**
   * Читает статус автоматической синхронизации для организатора.
   * @args id — конференция; signal — отмена запроса.
   * @return Состояния календарных событий без credentials.
   */
  calendarSync: (id: string, signal?: AbortSignal) =>
    request<Items<CalendarSync>>(
      `/conferences/${encodeURIComponent(id)}/calendar`,
      { signal },
    ),
  /**
   * startRecording запрашивает начало общей записи конференции.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  startRecording: (id: string, mode: RecordingMode = "composite") =>
    request<Item<ConferenceRecording>>(
      `/conferences/${encodeURIComponent(id)}/recordings`,
      { method: "POST", body: { segmentDurationSec: 5, mode } },
    ),
  /** Читает состояние распознавания; @args id — встреча; signal — отмена; @return приватный снимок. */
  captions: (id: string, signal?: AbortSignal) =>
    request<Item<CaptionState>>(
      `/conferences/${encodeURIComponent(id)}/captions`,
      { signal },
    ),
  /** Переключает передачу звука; @args id — встреча, enabled — согласие, language — auto/ru/en; @return состояние. */
  setCaptions: (id: string, enabled: boolean, language: string) =>
    request<Item<CaptionState>>(
      `/conferences/${encodeURIComponent(id)}/captions`,
      { method: "PUT", body: { enabled, language } },
    ),
  /** Восстанавливает финалы; @args id — встреча, afterCursor — последняя версия, signal — отмена; @return страница. */
  captionFinals: (id: string, afterCursor: number, signal?: AbortSignal) =>
    request<{ items: Caption[]; nextCursor: number; hasMore: boolean }>(
      `/conferences/${encodeURIComponent(id)}/captions/segments?afterCursor=${afterCursor}&limit=100`,
      { signal },
    ),
  /** Читает агрегаты; @args id — встреча, signal — отмена; @return разрешённая аналитика. */
  analytics: (id: string, signal?: AbortSignal) =>
    request<Item<MeetingAnalytics>>(
      `/conferences/${encodeURIComponent(id)}/analytics`,
      { signal },
    ),
  /** Ставит переиндексацию; @args id,rid — встреча и запись; @return подтверждение постановки. */
  reindex: (id: string, rid: string) =>
    request(
      `/conferences/${encodeURIComponent(id)}/recordings/${encodeURIComponent(rid)}/search/reindex`,
      { method: "POST", body: {} },
    ),
  /**
   * stopRecording запрашивает остановку указанной записи и её последующую финализацию.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - recordingId (string) — идентификатор записи конференции.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  stopRecording: (id: string, recordingId: string) =>
    request<Item<ConferenceRecording>>(
      `/conferences/${encodeURIComponent(id)}/recordings/${encodeURIComponent(recordingId)}/stop`,
      { method: "POST", body: {} },
    ),
  /**
   * create создаёт немедленную или запланированную встречу по переданным параметрам.
   *
   * @args
   *   - input (string | ConferenceInput) — название либо полный набор параметров встречи.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  create: (input: string | ConferenceInput) =>
    request<Item<Conference>>("/conferences", {
      method: "POST",
      body: typeof input === "string" ? { title: input } : input,
    }),
  /**
   * transition отправляет разрешённое действие запуска, завершения или отмены встречи.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - action ("start" | "finish" | "cancel") — разрешённое действие управления либо асинхронная операция.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  transition: (id: string, action: "start" | "finish" | "cancel") =>
    request<Item<Conference>>(
      `/conferences/${encodeURIComponent(id)}/${action}`,
      { method: "POST" },
    ),
  /**
   * membership создаёт или восстанавливает членство текущего пользователя в конференции.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - action ("join" | "leave") — разрешённое действие управления либо асинхронная операция.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  membership: (id: string, action: "join" | "leave") =>
    request<Item<Participant>>(
      `/conferences/${encodeURIComponent(id)}/${action}`,
      { method: "POST" },
    ),
  /**
   * invite получает ограниченные сведения встречи по коду приглашения.
   *
   * @args
   *   - code (string) — проверенный код приглашения.
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  invite: (code: string, signal?: AbortSignal) =>
    request<Item<Invite>>(`/conference-invites/${encodeURIComponent(code)}`, {
      signal,
      public: true,
    }),
  joinGuest: (code: string, displayName: string, signal?: AbortSignal) =>
    request<LoginResponse & Item<Participant>>(
      `/conference-invites/${encodeURIComponent(code)}/guest`,
      { method: "POST", body: { displayName }, signal },
    ),
  /**
   * joinInvite запрашивает вход авторизованного пользователя по коду приглашения.
   *
   * @args
   *   - code (string) — проверенный код приглашения.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  joinInvite: (code: string) =>
    request<Item<Participant>>(
      `/conference-invites/${encodeURIComponent(code)}/join`,
      { method: "POST" },
    ),
  /**
   * messages читает страницу постоянных сообщений конференции.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - before (string) — непрозрачный курсор более ранних сообщений (необязательный параметр).
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  messages: (id: string, before?: string, signal?: AbortSignal) =>
    request<ChatPage>(
      `/conferences/${encodeURIComponent(id)}/messages?limit=50${before ? `&before=${encodeURIComponent(before)}` : ""}`,
      { signal },
    ),
  /**
   * sendMessage отправляет сообщение с ключом повторной операции, ответом и подготовленными вложениями.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - body ({ clientRequestId: string; text: string; replyTo?: string; attachmentIds?: string[]; }) — типизированное тело запроса.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
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
  /**
   * editMessage изменяет текст собственного сообщения.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - messageId (string) — идентификатор сообщения в этой конференции.
   *   - text (string) — обычный текст сообщения.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  editMessage: (id: string, messageId: string, text: string) =>
    request<Item<ChatMessage>>(
      `/conferences/${encodeURIComponent(id)}/messages/${encodeURIComponent(messageId)}`,
      { method: "PATCH", body: { text } },
    ),
  /**
   * deleteMessage запрашивает мягкое удаление разрешённого сообщения.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - messageId (string) — идентификатор сообщения в этой конференции.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  deleteMessage: (id: string, messageId: string) =>
    request<Item<ChatMessage>>(
      `/conferences/${encodeURIComponent(id)}/messages/${encodeURIComponent(messageId)}`,
      { method: "DELETE" },
    ),
  /**
   * chatRead читает сохранённую границу прочтения и число непрочитанных.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  chatRead: (id: string, signal?: AbortSignal) =>
    request<Item<ChatReadState>>(
      `/conferences/${encodeURIComponent(id)}/chat/read`,
      { signal },
    ),
  /**
   * markChatRead продвигает серверную отметку прочтения до указанного сообщения.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - messageId (string) — идентификатор сообщения в этой конференции.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  markChatRead: (id: string, messageId: string) =>
    request<Item<ChatReadState>>(
      `/conferences/${encodeURIComponent(id)}/chat/read`,
      { method: "PUT", body: { messageId } },
    ),
  /**
   * initAttachment регистрирует метаданные файла до передачи его байтов.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - body ({ clientRequestId: string; filename: string; size: number; mimeType: string; }) — типизированное тело запроса.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
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
  /**
   * finalizeAttachment подтверждает завершённую загрузку файла перед отправкой сообщения.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - attachmentId (string) — идентификатор подготовленного или прикреплённого вложения.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  finalizeAttachment: (id: string, attachmentId: string) =>
    request<Item<ChatAttachment>>(
      `/conferences/${encodeURIComponent(id)}/attachments/${encodeURIComponent(attachmentId)}/finalize`,
      { method: "POST", body: {} },
    ),
  /**
   * attachmentDownload запрашивает временную ссылку разрешённого скачивания вложения.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - attachmentId (string) — идентификатор подготовленного или прикреплённого вложения.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  attachmentDownload: (id: string, attachmentId: string) =>
    request<{ url: string; expiresAt: string }>(
      `/conferences/${encodeURIComponent(id)}/attachments/${encodeURIComponent(attachmentId)}/download`,
    ),
  /**
   * reaction отправляет одну временную реакцию из разрешённого набора.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - emoji (ReactionEmoji) — одна из четырёх допустимых реакций.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  reaction: (id: string, emoji: ReactionEmoji) =>
    request(`/conferences/${encodeURIComponent(id)}/reactions`, {
      method: "POST",
      body: { emoji },
    }),
  /**
   * notifications читает страницу личных уведомлений и число непрочитанных.
   *
   * @args
   *   - cursor (string) — непрозрачный курсор продолжения страницы (необязательный параметр).
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  notifications: (cursor?: string, signal?: AbortSignal) =>
    request<NotificationsPage>(
      `/notifications?limit=30${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`,
      { signal },
    ),
  /**
   * readNotification идемпотентно отмечает личное уведомление прочитанным.
   *
   * @args
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  readNotification: (id: string) =>
    request<Item<Notification>>(
      `/notifications/${encodeURIComponent(id)}/read`,
      { method: "POST" },
    ),
  /**
   * notificationEvents открывает поток SSE через fetch с JWT в заголовке и поддержкой отмены.
   *
   * @args
   *   - signal (AbortSignal) — сигнал отмены запроса или потока.
   *
   * @returns Promise, который после завершения операции возвращает: вычисленное значение: response.
   */
  notificationEvents: async (signal: AbortSignal) => {
    const usedToken = accessToken;
    const version = identityVersion;
    const open = () =>
      fetch("/api/v1/notifications/events", {
        headers: {
          Accept: "text/event-stream",
          ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
        },
        credentials: "omit",
        signal,
      });
    let response = await open();
    if (
      response.status === 401 &&
      usedToken &&
      (await recoverSession(usedToken, version))
    ) {
      signal.throwIfAborted();
      response = await open();
    }
    if (!response.ok) {
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
/**
 * uploadAttachment передаёт байты файла через XMLHttpRequest с авторизацией, отслеживает прогресс и поддерживает отмену загрузки.
 *
 * @args
 *   - url (string) — адрес запроса или ресурса.
 *   - file (File) — выбранный пользователем файл для проверки или передачи.
 *   - onProgress ((percent: number) => void) — обработчик числа переданных байтов или процента передачи.
 *   - signal (AbortSignal) — сигнал отмены запроса или потока.
 *
 * @returns Promise<void> — Promise с результатом описанной асинхронной операции; отказ передаётся через отклонение Promise.
 */
export function uploadAttachment(
  url: string,
  file: File,
  onProgress: /**
   * Вложенный обработчик выполняет шаг «Вложенный обработчик» в типизированных HTTP-запросах.
   *
   * @args
   *   - percent (number) — доля завершённой загрузки от 0 до 100.
   *
   * @returns void — значение не возвращается; функция выполняет описанные действия.
   */ (percent: number) => void,
  signal: AbortSignal,
  retry = true,
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
  return new Promise(
    /**
     * Вложенный обработчик выполняет шаг «Вложенный обработчик» в типизированных HTTP-запросах.
     *
     * @args
     *   - resolve — завершает ожидающий Promise успешным результатом.
     *   - reject — завершает ожидающий Promise ошибкой.
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */ (resolve, reject) => {
      const usedToken = accessToken;
      const version = identityVersion;
      const xhr = new XMLHttpRequest();
      /**
       * abort отменяет передачу или ожидание текущей операции.
       *
       *
       * @returns вычисленное значение: xhr.abort().
       */
      const abort = () => xhr.abort();
      xhr.open("PUT", target.pathname);
      xhr.timeout = 120000;
      if (usedToken)
        xhr.setRequestHeader("Authorization", `Bearer ${usedToken}`);
      xhr.setRequestHeader(
        "Content-Type",
        file.type || "application/octet-stream",
      );
      xhr.upload.onprogress =
        /**
         * Вложенный обработчик выполняет шаг «Вложенный обработчик» в типизированных HTTP-запросах.
         *
         * @args
         *   - event — проверенный конверт события комнаты.
         *
         * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
         */ (event) => {
          if (event.lengthComputable)
            onProgress(Math.round((event.loaded / event.total) * 100));
        };
      xhr.onload =
        /**
         * Вложенный обработчик выполняет шаг «Вложенный обработчик» в типизированных HTTP-запросах.
         *
         *
         * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
         */ async () => {
          if (xhr.status === 401 && usedToken && retry) {
            try {
              if (await recoverSession(usedToken, version)) {
                signal.throwIfAborted();
                await uploadAttachment(url, file, onProgress, signal, false);
                resolve();
                return;
              }
            } catch (error) {
              reject(error);
              return;
            }
          }
          if (xhr.status >= 200 && xhr.status < 300) resolve();
          else
            reject(
              new ApiError(
                xhr.status,
                statuses[xhr.status] || "Не удалось загрузить файл.",
              ),
            );
        };
      xhr.onerror = xhr.ontimeout =
        /**
         * Вложенный обработчик выполняет шаг «Вложенный обработчик» в типизированных HTTP-запросах.
         *
         *
         * @returns вычисленное значение: reject(new Error("upload_unavailable")).
         */ () => reject(new Error("upload_unavailable"));
      xhr.onabort =
        /**
         * Вложенный обработчик выполняет шаг «Вложенный обработчик» в типизированных HTTP-запросах.
         *
         *
         * @returns вычисленное значение: reject(new DOMException("Upload aborted", "AbortError")).
         */ () => reject(new DOMException("Upload aborted", "AbortError"));
      xhr.onloadend =
        /**
         * Вложенный обработчик выполняет шаг «Вложенный обработчик» в типизированных HTTP-запросах.
         *
         *
         * @returns вычисленное значение: signal.removeEventListener("abort", abort).
         */ () => signal.removeEventListener("abort", abort);
      signal.addEventListener("abort", abort, { once: true });
      if (signal.aborted) {
        signal.removeEventListener("abort", abort);
        reject(new DOMException("Upload aborted", "AbortError"));
        return;
      }
      xhr.send(file);
    },
  );
}
export const statusLabels: Record<ConferenceStatus, string> = {
  created: "Ожидает начала",
  scheduled: "Запланирована",
  active: "Идёт сейчас",
  finished: "Завершена",
  cancelled: "Отменена",
};
