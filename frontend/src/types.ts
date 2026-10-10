/**
 * User описывает публичные сведения учётной записи без пароля.
 *
 * @params:
 *   - id — идентификатор ресурса или конференции данного запроса.
 *   - email — адрес электронной почты.
 *   - displayName — необязательное отображаемое имя пользователя.
 *   - createdAt — время создания.
 *   - updatedAt — время последнего сохранённого изменения.
 */
export interface User {
  id: string;
  email: string;
  displayName: string | null;
  isAdmin?: boolean;
  guestConferenceId?: string;
  createdAt: string;
  updatedAt: string;
}

/** Registered account available to the organiser's invitation search. */
export interface InvitationUser {
  id: string;
  email: string;
  displayName?: string | null;
}

export interface ConferenceInvitation {
  id: string;
  email: string;
  userId?: string | null;
  status: "queued" | "already_invited" | "left_chat";
}
/**
 * ConferenceStatus ограничивает допустимые серверные состояния встречи.
 *
 */
export type ConferenceStatus =
  "created" | "scheduled" | "active" | "finished" | "cancelled";
/**
 * Conference описывает встречу, её владельца, жизненный цикл, расписание и настройки ожидания.
 *
 * @params:
 *   - id — идентификатор ресурса или конференции данного запроса.
 *   - ownerId — идентификатор организатора.
 *   - title — название встречи или диалога.
 *   - inviteCode — код приглашения, который не заменяет авторизацию.
 *   - inviteUrl — полная ссылка приглашения.
 *   - status — HTTP-статус либо состояние встречи.
 *   - createdAt — время создания.
 *   - updatedAt — время последнего сохранённого изменения.
 *   - startedAt — время фактического начала.
 *   - finishedAt — время завершения встречи.
 *   - waitingRoomEnabled — требует допуска перед входом в комнату.
 *   - scheduledAt — однозначная ISO-временная отметка встречи.
 *   - plannedDurationMin — длительность в минутах либо null, если она не указана.
 */
export interface Conference {
  id: string;
  ownerId: string;
  title: string;
  inviteCode: string;
  inviteUrl: string;
  status: ConferenceStatus;
  createdAt: string;
  updatedAt: string;
  startedAt: string | null;
  finishedAt: string | null;
  waitingRoomEnabled?: boolean;
  scheduledAt?: string | null;
  plannedDurationMin?: number | null;
  participantCount?: number;
}
/**
 * Participant описывает членство, роль, допуск и сохранённые ограничения медиа участника.
 *
 * @params:
 *   - id — идентификатор ресурса или конференции данного запроса.
 *   - conferenceId — идентификатор конференции и области данных.
 *   - userId — идентификатор текущего авторизованного пользователя.
 *   - displayName — необязательное отображаемое имя пользователя.
 *   - role — роль участника и его полномочия.
 *   - status — HTTP-статус либо состояние встречи.
 *   - joinedAt — время присоединения.
 *   - leftAt — время выхода.
 *   - createdAt — время создания.
 *   - updatedAt — время последнего сохранённого изменения.
 *   - microphoneEnabled — признак включённого микрофона.
 *   - cameraEnabled — признак включённой камеры.
 *   - screenSharing — признак демонстрации экрана.
 *   - microphoneBlocked — серверный запрет микрофона.
 *   - cameraBlocked — серверный запрет камеры.
 *   - screenBlocked — серверный запрет экрана.
 *   - mediaPolicyVersion — версия серверной политики.
 *   - admissionState — состояние ожидания, допуска, отклонения или исключения.
 *   - admissionDecidedAt — время решения допуска.
 *   - admissionVersion — монотонная версия решения допуска.
 */
export interface Participant {
  id: string;
  conferenceId: string;
  userId: string | null;
  displayName: string;
  role: "owner" | "co_host" | "participant" | "guest";
  status: "joined" | "left" | "waiting" | "rejected" | "kicked";
  joinedAt: string | null;
  leftAt: string | null;
  createdAt: string;
  updatedAt: string;
  microphoneEnabled?: boolean;
  cameraEnabled?: boolean;
  screenSharing?: boolean;
  microphoneBlocked?: boolean;
  cameraBlocked?: boolean;
  screenBlocked?: boolean;
  mediaPolicyVersion?: number;
  admissionState?: "waiting" | "admitted" | "rejected" | "kicked";
  admissionDecidedAt?: string | null;
  admissionVersion?: number;
}

/** Chat member presence is independent of meeting attendance. */
export interface ConferenceChatMember extends Participant {
  online: boolean | null;
  isGuest: boolean;
}
/**
 * ParticipantMediaState связывает признаки медиа с физическим подключением и порядковым номером изменения.
 *
 * @params:
 *   - connectionId — идентификатор физического подключения.
 *   - sequence — серверный номер последовательности; строка чата сохраняет точность BIGSERIAL.
 *   - microphoneEnabled — признак включённого микрофона.
 *   - cameraEnabled — признак включённой камеры.
 *   - screenSharing — признак демонстрации экрана.
 */
export interface ParticipantMediaState {
  connectionId: string;
  sequence: number;
  microphoneEnabled: boolean;
  cameraEnabled: boolean;
  screenSharing: boolean;
}
/**
 * ModerationAction ограничивает допустимые действия модерации и их параметры.
 *
 * @params:
 *   - action — разрешённое действие управления либо асинхронная операция.
 *   - blocked — требуемое ограничение источника.
 *   - role — роль участника и его полномочия.
 */
export interface ModerationAction {
  action: "mute" | "camera" | "screen" | "kick" | "role";
  blocked?: boolean;
  role?: "co_host" | "participant";
}
/**
 * ConferenceRecording описывает общую запись конференции, её состояние и доступные артефакты.
 *
 * @params:
 *   - uuid — внешний UUID записи.
 *   - conferenceId — идентификатор конференции и области данных.
 *   - requestedBy — идентификатор пользователя, запустившего запись; может отсутствовать у старых записей.
 *   - mode — режим записи конференции.
 *   - status — HTTP-статус либо состояние встречи.
 *   - createdAt — время создания.
 *   - startedAt — время фактического начала.
 *   - endedAt — время завершения записи.
 *   - durationSec — измеренная длительность в секундах.
 *   - errorMessage — безопасная причина отказа.
 *   - files — доступные артефакты и выданные сервером ссылки.
 */
export interface ConferenceRecording {
  uuid: string;
  conferenceId: string;
  requestedBy?: string | null;
  mode: RecordingMode;
  status:
    | "starting"
    | "recording"
    | "degraded"
    | "stopping"
    | "processing"
    | "ready"
    | "failed"
    | "cancelled";
  createdAt: string;
  startedAt?: string;
  endedAt?: string;
  durationSec?: number;
  errorMessage?: string;
  files: { fileType: string; url?: string; sizeBytes?: number }[];
}
/**
 * Invite передаёт ограниченные сведения встречи по приглашению.
 *
 * @params:
 *   - id — идентификатор ресурса или конференции данного запроса.
 *   - title — название встречи или диалога.
 *   - status — HTTP-статус либо состояние встречи.
 *   - scheduledAt — однозначная ISO-временная отметка встречи.
 *   - waitingRoomEnabled — требует допуска перед входом в комнату.
 */
export interface Invite {
  id: string;
  title: string;
  status: ConferenceStatus;
  scheduledAt?: string | null;
  waitingRoomEnabled?: boolean;
}
/**
 * LoginResponse описывает токен, его срок и пользователя успешного входа.
 *
 * @params:
 *   - accessToken — подписанный токен учётной записи.
 *   - tokenType — схема Bearer авторизации.
 *   - expiresIn — срок токена в секундах.
 *   - user — публичные сведения пользователя.
 */
export interface LoginResponse {
  accessToken: string;
  tokenType: "Bearer";
  expiresIn: number;
  user: User;
}
/**
 * Item задаёт стандартный API-ответ с одним типизированным элементом.
 *
 * @params:
 *   - status — HTTP-статус либо состояние встречи.
 *   - item — элемент списка, который обрабатывает текущий шаг.
 */
export type Item<T> = { status: string; item: T };
/**
 * Items задаёт стандартный API-ответ со списком типизированных элементов.
 *
 * @params:
 *   - status — HTTP-статус либо состояние встречи.
 *   - items — элементы результата для объединения или отображения.
 */
export type Items<T> = { status: string; items: T[] };

/**
 * PresenceParticipant добавляет к членству онлайн-присутствие и число физических подключений.
 *
 * @params:
 *   - online — наличие действующей физической сессии.
 *   - connections — число действующих физических сессий.
 *   - connectionIds — идентификаторы подключений.
 */
export interface PresenceParticipant extends Participant {
  online: boolean;
  connections: number;
  connectionIds: string[];
}
/**
 * RealtimeState описывает начальный снимок комнаты, идентичность подключения и видимый состав участников.
 *
 * @params:
 *   - connectionId — идентификатор физического подключения.
 *   - participantId — идентификатор членства целевого участника.
 *   - status — HTTP-статус либо состояние встречи.
 *   - participants — разрешённый состав участников.
 */
export interface RealtimeState {
  connectionId: string;
  participantId: string;
  status: ConferenceStatus;
  participants: PresenceParticipant[];
}
/**
 * RealtimeEvent описывает версионный конверт доверенного серверного события.
 *
 * @params:
 *   - version — версия изменения или протокола.
 *   - id — идентификатор ресурса или конференции данного запроса.
 *   - type — машинный тип события.
 *   - conferenceId — идентификатор конференции и области данных.
 *   - timestamp — время серверного события.
 *   - data — нагрузка события, проверяемая перед чтением.
 *   - replyTo — идентификатор исходного сообщения или запроса.
 */
export interface RealtimeEvent {
  version: 1;
  id: string;
  type: string;
  conferenceId: string;
  timestamp: string;
  data: unknown;
  replyTo?: string;
}
/**
 * Signal описывает адресацию и нагрузку WebRTC-сигнализации.
 *
 * @params:
 *   - targetConnectionId — адресат физического подключения.
 *   - senderConnectionId — подтверждённое сервером подключение отправителя.
 *   - senderParticipantId — подтверждённое членство отправителя.
 *   - sdp — описание согласуемого WebRTC-сеанса.
 *   - candidate — кандидат ICE или null после завершения сбора.
 */
export interface Signal {
  targetConnectionId: string;
  senderConnectionId?: string;
  senderParticipantId?: string;
  sdp?: string;
  candidate?: RTCIceCandidateInit;
}

/**
 * CursorItems добавляет курсор следующей страницы к списку API.
 *
 * @params:
 *   - nextCursor — граница следующей страницы либо отсутствие продолжения.
 */
export interface CursorItems<T> extends Items<T> {
  nextCursor: string | null;
}
/**
 * ConferenceFilters задаёт серверные фильтры текущего пользователя по списку, области, состоянию и датам.
 *
 * @params:
 *   - view — раздел будущих, активных или прошедших встреч.
 *   - scope — область собственных или доступных встреч.
 *   - from — нижняя временная граница фильтра.
 *   - to — верхняя граница фильтра либо локальный путь согласно типу.
 *   - status — HTTP-статус либо состояние встречи.
 */
export interface ConferenceFilters {
  view?: "upcoming" | "active" | "past";
  scope?: "all" | "owned" | "participating";
  from?: string;
  to?: string;
  status?: ConferenceStatus;
}
/**
 * ConferenceInput задаёт входные параметры создания немедленной либо запланированной встречи.
 *
 * @params:
 *   - title — название встречи или диалога.
 *   - waitingRoomEnabled — требует допуска перед входом в комнату.
 *   - scheduledAt — однозначная ISO-временная отметка встречи.
 *   - plannedDurationMin — длительность в минутах либо null, если она не указана.
 */
export interface ConferenceInput {
  title: string;
  waitingRoomEnabled?: boolean;
  scheduledAt?: string | null;
  plannedDurationMin?: number | null;
}
/**
 * ConferenceHistory описывает сводку завершённой встречи, участников, записей и доступность чата.
 *
 * @params:
 *   - conference — поле или операция этого контракта.
 *   - owner — публичные сведения организатора.
 *   - durationSec — измеренная длительность в секундах.
 *   - participantCount — число видимых исторических членств.
 *   - participants — разрешённый состав участников.
 *   - participantsTruncated — признак ограниченной первой части участников.
 *   - recordings — поле или операция этого контракта.
 *   - chatAvailable — доступ текущего пользователя к постоянному чату.
 *   - chatReadOnly — запрещает изменение чата после завершения встречи.
 */
export interface ConferenceHistory {
  conference: Conference;
  owner: { id: string; displayName: string | null };
  durationSec: number | null;
  participantCount: number;
  participants: Participant[];
  participantsTruncated: boolean;
  recordings: {
    total: number;
    ready: number;
    processing: number;
    failed: number;
  };
  chatAvailable: boolean;
  chatReadOnly: boolean;
}
/**
 * ChatAttachment описывает публичные метаданные вложения без внутреннего ключа объекта и токена загрузки.
 *
 * @params:
 *   - id — идентификатор ресурса или конференции данного запроса.
 *   - filename — проверенное имя вложения без внутреннего пути.
 *   - mimeType — проверенный тип содержимого.
 *   - size — размер в байтах.
 *   - status — HTTP-статус либо состояние встречи.
 */
export interface ChatAttachment {
  id: string;
  filename: string;
  mimeType: string;
  size: number;
  status: string;
}
/**
 * ChatMessage описывает сообщение, автора, версию, ответ и вложения; sequence хранится строкой для точности BIGSERIAL.
 *
 * @params:
 *   - id — идентификатор ресурса или конференции данного запроса.
 *   - sequence — серверный номер последовательности; строка чата сохраняет точность BIGSERIAL.
 *   - conferenceId — идентификатор конференции и области данных.
 *   - senderId — идентификатор отправителя.
 *   - senderName — имя отправителя.
 *   - text — обычный текст сообщения.
 *   - replyTo — идентификатор исходного сообщения или запроса.
 *   - replyPreview — краткое представление исходного сообщения.
 *   - createdAt — время создания.
 *   - updatedAt — время последнего сохранённого изменения.
 *   - deletedAt — время мягкого удаления либо null.
 *   - version — версия изменения или протокола.
 *   - attachments — метаданные прикреплённых файлов.
 */
export interface ChatMessage {
  id: string;
  sequence: string;
  conferenceId?: string;
  conversationId?: string;
  senderId: string;
  senderName: string;
  text: string;
  replyTo?: string | null;
  replyPreview?: {
    id: string;
    text: string;
    senderName: string;
    deleted: boolean;
  } | null;
  createdAt: string;
  updatedAt: string;
  deletedAt: string | null;
  version: number;
  /** Reconciles the sender's optimistic row with REST and account events. */
  clientRequestId?: string;
  attachments: ChatAttachment[];
  /** Personal bookmark, projected only into the acting member's REST reads. */
  important?: boolean;
}

export interface ConferenceChatPreferences {
  notificationsEnabled: boolean;
}

export interface ConferenceChatInfo extends ConferenceChatPreferences {
  conferenceId: string;
  title: string;
  description: string;
  inviteUrl: string;
  participantCount: number;
  canEdit: boolean;
  canInvite: boolean;
}

export interface ConferenceChatMaterial {
  message: ChatMessage;
  attachment?: ChatAttachment;
  url?: string;
}
/**
 * ChatPage добавляет непрочитанные и границу прочтения к странице чата.
 *
 * @params:
 *   - unreadCount — число доступных непрочитанных элементов.
 *   - lastReadMessageId — последнее сообщение границы прочтения.
 */
export interface ChatPage extends CursorItems<ChatMessage> {
  unreadCount: number;
  lastReadMessageId: string | null;
}
/**
 * ChatReadState описывает сохранённую границу прочтения и число чужих непрочитанных сообщений.
 *
 * @params:
 *   - lastReadMessageId — последнее сообщение границы прочтения.
 *   - unreadCount — число доступных непрочитанных элементов.
 */
export interface ChatReadState {
  lastReadMessageId: string | null;
  unreadCount: number;
}
/**
 * ReactionEmoji ограничивает допустимые временные реакции четырьмя разрешёнными эмодзи.
 *
 */
export type ReactionEmoji = "👍" | "👏" | "❤️" | "😂";
/**
 * Notification описывает личное уведомление с ссылочной нагрузкой и отметкой прочтения.
 *
 * @params:
 *   - id — идентификатор ресурса или конференции данного запроса.
 *   - userId — идентификатор текущего авторизованного пользователя.
 *   - type — машинный тип события.
 *   - version — версия изменения или протокола.
 *   - payload — ссылки и состояние уведомления без выдачи прав на ресурс.
 *   - createdAt — время создания.
 *   - readAt — время прочтения либо null.
 */
export interface Notification {
  id: string;
  userId: string;
  type: string;
  version: 1;
  payload: {
    conferenceId: string;
    conversationId?: string;
    conversationType?: "direct" | "group";
    isReply?: boolean;
    messageId?: string;
    recordingId?: string;
    transcriptId?: string;
    summaryId?: string;
    scheduledAt?: string;
    admissionState?: string;
  };
  createdAt: string;
  readAt: string | null;
}
/**
 * NotificationsPage добавляет число непрочитанных к странице личных уведомлений.
 *
 * @params:
 *   - unreadCount — число доступных непрочитанных элементов.
 */
export interface NotificationsPage extends CursorItems<Notification> {
  unreadCount: number;
}

export type ProviderMode = "noop" | "mock" | "http";
export interface NotificationPreferences {
  invitation: boolean;
  reminder: boolean;
  recording: boolean;
  summary: boolean;
  email: boolean;
  push: boolean;
}
export interface IntegrationCapabilities {
  email: ProviderMode | "smtp";
  push: ProviderMode;
  calendar: ProviderMode;
  calendarOAuthConfigured: boolean;
  mockConnectAllowed: boolean;
}
export interface CalendarConnection {
  id: string;
  provider: string;
  calendarId: string;
  status: "connected" | "revoked";
  expiresAt?: string;
  createdAt: string;
  updatedAt: string;
}
export interface CalendarSync {
  provider: string;
  externalCalendarId: string;
  externalEventId: string;
  syncStatus: string;
  lastSyncedAt?: string;
}
export type ContentStatus = "queued" | "processing" | "ready" | "failed";
export interface Transcript {
  id: string;
  conferenceId: string;
  recordingId: string;
  status: ContentStatus;
  language: string;
  provider: string;
  createdAt: string;
  updatedAt: string;
  errorCode?: string;
  errorMessage?: string;
}
export interface TranscriptView extends Item<Transcript | null> {
  enabled: boolean;
  canRetry: boolean;
  providerMode: ProviderMode;
}
export interface TranscriptSegment {
  id: string;
  transcriptId: string;
  startMs: number;
  endMs: number;
  speakerId?: string | null;
  speakerLabel?: string | null;
  text: string;
  confidence?: number | null;
}
export interface OffsetPage<T> extends Items<T> {
  limit: number;
  offset: number;
  total: number;
}
export interface MeetingSummary {
  id: string;
  conferenceId: string;
  transcriptId: string;
  status: ContentStatus;
  summary: string;
  keyPoints: string[];
  actionItems: {
    text: string;
    assignee: string | null;
    dueDate: string | null;
    sourceSegmentIds: string[];
  }[];
  topics: string[];
  provider: string;
  model: string;
  promptVersion: string;
  schemaVersion: string | number;
  errorCode?: string;
}
export interface SummaryView extends Item<MeetingSummary | null> {
  enabled: boolean;
  canRegenerate: boolean;
  providerMode: ProviderMode;
}
export type SearchSource = "all" | "conference" | "transcript" | "summary";
export interface SearchFilters {
  mode?: "keyword" | "semantic" | "hybrid";
  membership?: "all" | "owned" | "participating";
  participantId?: string;
  q: string;
  source: SearchSource;
  conferenceId?: string;
  from?: string;
  to?: string;
}
export interface SearchResult {
  speaker?: string;
  speakerId?: string;
  type: Exclude<SearchSource, "all">;
  conferenceId: string;
  conferenceTitle: string;
  recordingId?: string;
  transcriptId?: string;
  segmentId?: string;
  startMs?: number;
  snippet: string;
  rank: number;
}

/** RecordingMode ограничивает выбор серверными стратегиями, без параметров FFmpeg. */
export type RecordingMode =
  "composite" | "audio_only" | "individual_tracks" | "screen_focus";
/** Caption содержит серверную идентичность реплики, порядок версий и время внутри встречи. */
export interface Caption {
  id: string;
  conferenceId: string;
  sessionId: string;
  participantId: string;
  speaker: string;
  trackInstanceId: string;
  generation: number;
  cursor: number;
  sequence: number;
  revision: number;
  startMs: number;
  endMs: number;
  text: string;
  language: string;
  final: boolean;
}
/** CaptionState отделяет согласие на распознавание от локального отображения субтитров. */
export interface CaptionState {
  conferenceId: string;
  sessionId: string;
  enabled: boolean;
  available: boolean;
  canManage: boolean;
  language: string;
  status: string;
  generation: number;
  canonicalRecordingId?: string;
}
/** MeetingAnalytics содержит агрегаты без оценки продуктивности или ранжирования участников. */
export interface MeetingAnalytics {
  enabled: boolean;
  durationMs: number;
  participantCount: number;
  recordingAvailable: boolean;
  transcriptAvailable: boolean;
  timeline: { atMs: number; count: number }[];
  participants: {
    participantId: string;
    displayName: string;
    participationMs: number;
    speakingMs: number;
    observedAudioMs: number;
    screenMs: number;
    messageCount: number;
  }[];
}

/** ProductCapabilities содержит только безопасные признаки включённых серверных функций. */
export interface ProductCapabilities {
  liveCaptions: boolean;
  transcription: boolean;
  aiSummary: boolean;
  semanticSearch: boolean;
  meetingAnalytics: boolean;
  recordingModes: RecordingMode[];
}

export interface CapabilitiesResponse {
  status: string;
  capabilities: ProductCapabilities;
  buildVersion: string;
}

/** AdminSummary содержит только агрегаты и коды отказов, без персональных данных. */
export interface AdminSummary {
  asOf: string;
  activeConferences: number;
  joinedParticipants: number;
  activeRecordings: number;
  queuedJobs: number;
  failedJobs24h: number;
  failedRecordings24h: number;
  failedTranscriptions24h: number;
  apiReady: boolean;
  mediaWorkerReady: boolean;
  dependencies: Record<string, boolean>;
  recentFailures: { kind: string; code: string; at: string }[];
}

export interface PersonalPeer {
  id: string;
  displayName: string;
}
/** Подтверждённое сервером присутствие собеседника в доступной личной переписке.
 * @params conversationId — проверенный диалог; peerId — собеседник, определённый сервером;
 * online — наличие живой физической сессии; отсутствие подтверждения передаётся ошибкой API.
 */
export interface PersonalPeerPresence {
  conversationId: string;
  peerId: string;
  online: boolean;
}
interface ConversationSummary {
  id: string;
  createdAt: string;
  lastMessageAt: string | null;
  lastMessageId: string | null;
  preview: string;
  unreadCount: number;
}
export interface DirectConversation extends ConversationSummary {
  type: "direct";
  peer: PersonalPeer;
  /** Личная настройка уведомлений; отсутствие поля в старом ответе означает «включены». */
  notificationsEnabled?: boolean;
  /** Личный порог видимой истории; сообщения с меньшим номером больше недоступны. */
  historyClearedThrough?: number;
}
/** Подтверждает личное скрытие диалога и точный порог, закрывающий запоздалые сообщения. */
export interface DirectConversationHideReceipt {
  status: string;
  hidden: true;
  historyClearedThrough: number;
}
export type GroupRole = "owner" | "admin" | "member";
export interface GroupConversation extends ConversationSummary {
  type: "group";
  name: string;
  description: string;
  createdBy: string;
  updatedAt: string;
  memberCount: number;
  myRole: GroupRole;
  avatarVersion: string | null;
  lastSender: PersonalPeer | null;
}
export interface GroupMember extends PersonalPeer {
  role: GroupRole;
  online?: boolean | null;
}
export type PersonalConversation = DirectConversation | GroupConversation;
export interface ConversationFilters {
  type?: "direct" | "group";
  unreadOnly?: boolean;
  search?: string;
}
export interface PersonalPage extends CursorItems<PersonalConversation> {
  unreadCount: number;
}

export type FolderItemKind = "conversation" | "conference";
export interface FolderTarget {
  type: FolderItemKind;
  id: string;
}
export interface PersonalFolder {
  id: string;
  name: string;
  position: number;
  createdAt: string;
  updatedAt: string;
  itemCount: number;
  conversationCount: number;
  conferenceCount: number;
  contains?: boolean;
}
export type FolderItem =
  | { type: "conversation"; item: PersonalConversation; inFolder?: boolean }
  | { type: "conference"; item: Conference; inFolder?: boolean };
export interface FolderItemFilters {
  type: FolderItemKind | "all";
  search?: string;
}
export interface FolderPage {
  items: FolderItem[];
  nextCursor?: string;
}
