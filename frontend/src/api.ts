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
/**
 * invalidSession обрабатывает отказ запроса с использованным токеном, не завершая более новую авторизацию.
 *
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */
let invalidSession: /**
 * Вложенный обработчик выполняет шаг «Вложенный обработчик» в типизированных HTTP-запросах.
 *
 * @parameters:
 *   - usedToken (string) — токен конкретного запроса, который получил отказ авторизации.
 *
 * @returns void — значение не возвращается; функция выполняет описанные действия.
 */ (usedToken: string) => void = () => {};
/**
 * configureAuth сохраняет текущий токен и обработчик ответа 401; обработчик получает именно токен, использованный неудачным запросом.
 *
 * @parameters:
 *   - token (string | null) — токен текущей авторизации; null отключает авторизованные запросы.
 *   - onInvalid ((usedToken: string) => void) — обработчик отказа авторизации с использованным токеном (необязательный параметр).
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */
export function configureAuth(
  token: string | null,
  onInvalid?: /**
   * Вложенный обработчик выполняет шаг «Вложенный обработчик» в типизированных HTTP-запросах.
   *
   * @parameters:
   *   - usedToken (string) — токен конкретного запроса, который получил отказ авторизации.
   *
   * @returns void — значение не возвращается; функция выполняет описанные действия.
   */ (usedToken: string) => void,
) {
  accessToken = token;
  if (onInvalid) invalidSession = onInvalid;
}
/**
 * ApiError связывает HTTP-статус с понятным пользователю сообщением ошибки API.
 *
 * Состав:
 *   - constructor — function Object() { [native code] }.
 */
export class ApiError extends Error {
  /**
   * constructor function Object() { [native code] }.
   *
   * @parameters:
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
 * @parameters:
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
 * @parameters:
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
  const data: unknown = await response.json().catch(
    /**
     * Обработчик catch выполняет переданный шаг вызова catch в типизированных HTTP-запросах.
     *
     *
     * @returns вычисленное значение: null.
     */ () => null,
  );
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
  /**
   * myConferences читает страницу встреч пользователя с серверными фильтрами и курсором продолжения.
   *
   * @parameters:
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
   * @parameters:
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
  /**
   * admit отправляет решение организатора о допуске или отклонении участника.
   *
   * @parameters:
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
   * @parameters:
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
   * @parameters:
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
   * @parameters:
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
   * @parameters:
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  iceConfig: (signal?: AbortSignal) =>
    request<{ iceServers: RTCIceServer[] }>("/webrtc/config", { signal }),
  /**
   * register отправляет данные регистрации с нормализацией необязательного отображаемого имени.
   *
   * @parameters:
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
   * @parameters:
   *   - email (string) — адрес электронной почты.
   *   - password (string) — пароль из формы; не предназначен для журналирования.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  login: (email: string, password: string) =>
    request<LoginResponse>("/auth/login", {
      method: "POST",
      body: { email, password },
      public: true,
    }),
  /**
   * me читает публичные сведения текущей авторизованной учётной записи.
   *
   * @parameters:
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  me: (signal?: AbortSignal) => request<{ user: User }>("/auth/me", { signal }),
  /**
   * logout отправляет запрос завершения авторизации.
   *
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  logout: () => request("/auth/logout", { method: "POST" }),
  /**
   * conferences читает ограниченную страницу доступных конференций.
   *
   * @parameters:
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
   * @parameters:
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
   * @parameters:
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
   * @parameters:
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
   * @parameters:
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
   * recordings читает доступные записи конференции с их текущими состояниями.
   *
   * @parameters:
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  recordings: (id: string, signal?: AbortSignal) =>
    request<Items<ConferenceRecording>>(
      `/conferences/${encodeURIComponent(id)}/recordings`,
      { signal },
    ),
  /**
   * startRecording запрашивает начало общей записи конференции.
   *
   * @parameters:
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  startRecording: (id: string) =>
    request<Item<ConferenceRecording>>(
      `/conferences/${encodeURIComponent(id)}/recordings`,
      { method: "POST", body: { segmentDurationSec: 5 } },
    ),
  /**
   * stopRecording запрашивает остановку указанной записи и её последующую финализацию.
   *
   * @parameters:
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
   * @parameters:
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
   * @parameters:
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
   * @parameters:
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
   * @parameters:
   *   - code (string) — проверенный код приглашения.
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  invite: (code: string, signal?: AbortSignal) =>
    request<Item<Invite>>(`/conference-invites/${encodeURIComponent(code)}`, {
      signal,
    }),
  /**
   * joinInvite запрашивает вход авторизованного пользователя по коду приглашения.
   *
   * @parameters:
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
   * @parameters:
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
   * @parameters:
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
   * @parameters:
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
   * @parameters:
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
   * @parameters:
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
   * @parameters:
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
   * @parameters:
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
   * @parameters:
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
   * @parameters:
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
   * hands читает актуальный снимок поднятых рук комнаты.
   *
   * @parameters:
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - signal (AbortSignal) — сигнал отмены запроса или потока (необязательный параметр).
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  hands: (id: string, signal?: AbortSignal) =>
    request<Items<RaisedHand>>(`/conferences/${encodeURIComponent(id)}/hands`, {
      signal,
    }),
  /**
   * hand изменяет состояние собственной руки либо опускает разрешённую чужую руку.
   *
   * @parameters:
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *   - participantId (string) — идентификатор членства целевого участника.
   *   - raised (boolean) — true поднимает руку, false опускает её.
   *
   * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
   */
  hand: (id: string, participantId: string, raised: boolean) =>
    request(
      `/conferences/${encodeURIComponent(id)}/participants/${encodeURIComponent(participantId)}/hand`,
      { method: "PUT", body: { raised } },
    ),
  /**
   * reaction отправляет одну временную реакцию из разрешённого набора.
   *
   * @parameters:
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
   * @parameters:
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
   * @parameters:
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
   * notificationEvents открывает fetch SSE-поток с JWT в заголовке и поддержкой отмены.
   *
   * @parameters:
   *   - signal (AbortSignal) — сигнал отмены запроса или потока.
   *
   * @returns Promise, который после завершения операции возвращает: вычисленное значение: response.
   */
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
/**
 * uploadAttachment передаёт байты файла через XMLHttpRequest с авторизацией, отслеживает прогресс и поддерживает отмену загрузки.
 *
 * @parameters:
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
   * @parameters:
   *   - percent (number) — доля завершённой загрузки от 0 до 100.
   *
   * @returns void — значение не возвращается; функция выполняет описанные действия.
   */ (percent: number) => void,
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
  return new Promise(
    /**
     * Вложенный обработчик выполняет шаг «Вложенный обработчик» в типизированных HTTP-запросах.
     *
     * @parameters:
     *   - resolve — завершает ожидающий Promise успешным результатом.
     *   - reject — завершает ожидающий Promise ошибкой.
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */ (resolve, reject) => {
      const usedToken = accessToken;
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
         * @parameters:
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
         */ () => {
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
