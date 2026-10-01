import type { ClientRealtimeType } from "./realtime";
import type { RealtimeEvent } from "./types";

/**
 * MediaSource различает микрофон, камеру, экран и звук экрана при публикации медиа.
 *
 */
export type MediaSource =
  "microphone" | "camera" | "video/screen" | "audio/screen";
/**
 * MediaPolicy описывает ограничения источников медиа, которые клиент применяет после серверной модерации.
 *
 * Состав:
 *   - version — версия изменения или протокола.
 *   - microphoneBlocked — серверный запрет микрофона.
 *   - cameraBlocked — серверный запрет камеры.
 *   - screenBlocked — серверный запрет экрана.
 */
export interface MediaPolicy {
  version?: number;
  microphoneBlocked?: boolean;
  cameraBlocked?: boolean;
  screenBlocked?: boolean;
}
/**
 * MediaTrack связывает дорожку с серверным участником, источником и физическим подключением.
 *
 * Состав:
 *   - id — идентификатор ресурса или конференции данного запроса.
 *   - streamId — поле или операция этого контракта.
 *   - mediaPeerId — поле или операция этого контракта.
 *   - participantId — идентификатор членства целевого участника.
 *   - kind — поле или операция этого контракта.
 *   - source — семантический источник медиа: микрофон, камера либо экран.
 */
export interface MediaTrack {
  id: string;
  streamId: string;
  mediaPeerId: string;
  participantId: string;
  kind: "audio" | "video";
  source: MediaSource;
}
/**
 * RemoteMedia собирает удалённые потоки и источники одного физического подключения.
 *
 * Состав:
 *   - id — идентификатор ресурса или конференции данного запроса.
 *   - mediaPeerId — поле или операция этого контракта.
 *   - participantId — идентификатор членства целевого участника.
 *   - stream — поле или операция этого контракта.
 *   - kinds — поле или операция этого контракта.
 *   - screen — поле или операция этого контракта.
 */
export interface RemoteMedia {
  id: string;
  mediaPeerId: string;
  participantId: string;
  stream: MediaStream;
  kinds: string[];
  screen: boolean;
}
/**
 * MediaView описывает отображаемый снимок состояния медиа, устройств, потоков и ошибок.
 *
 * Состав:
 *   - active — поле или операция этого контракта.
 *   - status — HTTP-статус либо состояние встречи.
 *   - localStream — поле или операция этого контракта.
 *   - localScreen — поле или операция этого контракта.
 *   - microphoneEnabled — признак включённого микрофона.
 *   - cameraEnabled — признак включённой камеры.
 *   - screenSharing — признак демонстрации экрана.
 *   - controlBusy — поле или операция этого контракта.
 *   - remoteStreams — поле или операция этого контракта.
 *   - mediaPeerId — поле или операция этого контракта.
 *   - workerId — поле или операция этого контракта.
 *   - connectionState — поле или операция этого контракта.
 *   - iceState — поле или операция этого контракта.
 *   - negotiationState — поле или операция этого контракта.
 *   - error — пойманная ошибка API или сети.
 */
export interface MediaView {
  active: boolean;
  status: string;
  localStream: MediaStream | null;
  localScreen: MediaStream | null;
  microphoneEnabled: boolean;
  cameraEnabled: boolean;
  screenSharing: boolean;
  controlBusy: boolean;
  remoteStreams: RemoteMedia[];
  mediaPeerId: string;
  workerId: string;
  connectionState: string;
  iceState: string;
  negotiationState: string;
  error: string | null;
}
/**
 * Send задаёт контракт отправки разрешённой сигнализации физического соединения.
 *
 */
type Send =
  /**
   * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
   *
   * @parameters:
   *   - type (ClientRealtimeType) — машинный тип события.
   *   - data (unknown) — нагрузка события, проверяемая перед чтением.
   *
   * @returns string — результат указанного контракта; реализация предоставляется вызывающим компонентом.
   */ (type: ClientRealtimeType, data: unknown) => string;
const uuid =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
/**
 * initial создаёт исходный снимок медиа без соединения и захваченных устройств.
 *
 *
 * @returns MediaView — новое исходное состояние без активного подключения.
 */
const initial = (): MediaView => ({
  active: false,
  status: "Камера и микрофон выключены",
  localStream: null,
  localScreen: null,
  microphoneEnabled: false,
  cameraEnabled: false,
  screenSharing: false,
  controlBusy: false,
  remoteStreams: [],
  mediaPeerId: "",
  workerId: "",
  connectionState: "new",
  iceState: "new",
  negotiationState: "stable",
  error: null,
});
/**
 * record проверяет, является ли неизвестная нагрузка обычным объектом для дальнейшего чтения полей.
 *
 * @parameters:
 *   - value (unknown) — значение для проверки, преобразования или отображения.
 *
 * @returns Record<string, unknown> | null — true для обычного ненулевого объекта и false для остальных значений.
 */
function record(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}
/**
 * tracks проверяет и нормализует описания медиа-дорожек входящего события.
 *
 * @parameters:
 *   - value (unknown) — значение для проверки, преобразования или отображения.
 *
 * @returns MediaTrack[] | null — набор элементов указанного типа, полученных описанной операцией.
 */
function tracks(value: unknown): MediaTrack[] | null {
  if (!Array.isArray(value) || value.length > 128) return null;
  const result: MediaTrack[] = [];
  const ids = new Set<string>();
  for (const item of value) {
    const t = record(item);
    if (
      !t ||
      typeof t.id !== "string" ||
      !uuid.test(t.id) ||
      ids.has(t.id) ||
      typeof t.mediaPeerId !== "string" ||
      !uuid.test(t.mediaPeerId) ||
      t.streamId !==
        (String(t.source).endsWith("/screen")
          ? `${t.mediaPeerId}-screen`
          : t.mediaPeerId) ||
      typeof t.participantId !== "string" ||
      !uuid.test(t.participantId) ||
      !(
        (t.kind === "audio" &&
          (t.source === "microphone" || t.source === "audio/screen")) ||
        (t.kind === "video" &&
          (t.source === "camera" || t.source === "video/screen"))
      )
    )
      return null;
    ids.add(t.id);
    result.push(t as unknown as MediaTrack);
  }
  return result;
}
/**
 * videoCapture проверяет предложенный профиль захвата видео перед применением к устройству.
 *
 * @parameters:
 *   - value (unknown) — значение для проверки, преобразования или отображения.
 *
 * @returns объект с данными, собранными в текущей операции.
 */
function videoCapture(value: unknown) {
  if (value === undefined)
    return { maxWidth: 1280, maxHeight: 720, maxFrameRate: 30 };
  const target = record(value);
  if (
    !target ||
    !Number.isInteger(target.maxWidth) ||
    Number(target.maxWidth) < 1 ||
    Number(target.maxWidth) > 1280 ||
    !Number.isInteger(target.maxHeight) ||
    Number(target.maxHeight) < 1 ||
    Number(target.maxHeight) > 720 ||
    !Number.isInteger(target.maxFrameRate) ||
    Number(target.maxFrameRate) < 1 ||
    Number(target.maxFrameRate) > 30
  )
    throw new Error("invalid_video_capture");
  return {
    maxWidth: Number(target.maxWidth),
    maxHeight: Number(target.maxHeight),
    maxFrameRate: Number(target.maxFrameRate),
  };
}

// One controller = one Stage 2 connection and one server-side MediaPeer.
// No permission request or automatic capture occurs in the constructor.
/**
 * ConferenceMediaClient управляет физическим WebRTC-соединением, захватом устройств, экраном и последовательным согласованием SDP.
 *
 * Состав:
 *   - view — раздел будущих, активных или прошедших встреч.
 *   - disposed — поле или операция этого контракта.
 *   - started — поле или операция этого контракта.
 *   - pc — поле или операция этого контракта.
 *   - local — поле или операция этого контракта.
 *   - screen — поле или операция этого контракта.
 *   - policy — актуальные ограничения модерации источников.
 *   - capture — поле или операция этого контракта.
 *   - sources — поле или операция этого контракта.
 *   - localRevision — поле или операция этого контракта.
 *   - preferredInputs — поле или операция этого контракта.
 *   - answeredLocalRevision — поле или операция этого контракта.
 *   - joinedRequest — поле или операция этого контракта.
 *   - negotiation — поле или операция этого контракта.
 *   - wantedRevision — поле или операция этого контракта.
 *   - answeredRevision — поле или операция этого контракта.
 *   - catalog — поле или операция этого контракта.
 *   - ownPublished — поле или операция этого контракта.
 *   - catalogRevision — поле или операция этого контракта.
 *   - received — поле или операция этого контракта.
 *   - streams — поле или операция этого контракта.
 *   - remoteICE — поле или операция этого контракта.
 *   - localICE — поле или операция этого контракта.
 *   - remoteDescriptionReady — поле или операция этого контракта.
 *   - hasSentOffer — поле или операция этого контракта.
 *   - readyRequests — поле или операция этого контракта.
 *   - queue — поле или операция этого контракта.
 *   - timer — поле или операция этого контракта.
 *   - disconnectedTimer — поле или операция этого контракта.
 *   - constructor — function Object() { [native code] }.
 *   - snapshot — поле или операция этого контракта.
 *   - update — поле или операция этого контракта.
 *   - deadline — поле или операция этого контракта.
 *   - fail — поле или операция этого контракта.
 *   - stop — поле или операция этого контракта.
 *   - start — поле или операция этого контракта.
 *   - watchTrack — поле или операция этого контракта.
 *   - refreshLocal — поле или операция этого контракта.
 *   - mutate — поле или операция этого контракта.
 *   - replaceSource — поле или операция этого контракта.
 *   - setPolicy — поле или операция этого контракта.
 *   - changeSource — поле или операция этого контракта.
 *   - startScreen — поле или операция этого контракта.
 *   - stopScreen — поле или операция этого контракта.
 *   - handle — поле или операция этого контракта.
 *   - process — поле или операция этого контракта.
 *   - createPeer — поле или операция этого контракта.
 *   - sendICE — поле или операция этого контракта.
 *   - offer — поле или операция этого контракта.
 *   - syncRemoteStreams — поле или операция этого контракта.
 */
export class ConferenceMediaClient {
  private view = initial();
  private disposed = false;
  private started = false;
  private pc: RTCPeerConnection | null = null;
  private local: MediaStream | null = null;
  private screen: MediaStream | null = null;
  private policy: MediaPolicy = {};
  private capture = videoCapture(undefined);
  private sources = new Map<
    MediaSource,
    {
      sender: RTCRtpSender;
      transceiver: RTCRtpTransceiver;
      track: MediaStreamTrack | null;
    }
  >();
  private localRevision = 0;
  private preferredInputs = new Map<MediaSource, string>();
  private answeredLocalRevision = -1;
  private joinedRequest = "";
  private negotiation: {
    id: string;
    request: string;
    revision: number;
    localRevision: number;
  } | null = null;
  private wantedRevision = 0;
  private answeredRevision = -1;
  private catalog: MediaTrack[] = [];
  private ownPublished = new Map<string, MediaTrack>();
  private catalogRevision = -1;
  // A preallocated browser receiver may retain its own track ID. SDP msid's
  // stream ID + kind binds it to the worker's canonical publication metadata.
  private received = new Map<
    string,
    { track: MediaStreamTrack; streamIds: string[] }
  >();
  private streams = new Map<string, MediaStream>();
  private remoteICE: (RTCIceCandidateInit | null)[] = [];
  private localICE: (RTCIceCandidateInit | null)[] = [];
  private remoteDescriptionReady = false;
  private hasSentOffer = false;
  private readyRequests = new Map<string, ReturnType<typeof setTimeout>>();
  private queue = Promise.resolve();
  private timer?: ReturnType<typeof setTimeout>;
  private disconnectedTimer?: ReturnType<typeof setTimeout>;

  /**
   * constructor function Object() { [native code] }.
   *
   * @parameters:
   *   - send (Send) — входное значение send текущего шага обработки.
   *   - onChange ((view: MediaView) => void) — обработчик изменения управляемого значения.
   *
   * @returns инициализированный экземпляр текущего класса.
   */
  constructor(
    private send: Send,
    private onChange: /**
     * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
     *
     * @parameters:
     *   - view (MediaView) — раздел будущих, активных или прошедших встреч.
     *
     * @returns void — значение не возвращается; функция выполняет описанные действия.
     */ (view: MediaView) => void,
  ) {}

  /**
   * snapshot возвращает текущий снимок состояния медиа для отображения интерфейса.
   *
   *
   * @returns текущий снимок состояния медиа, потоков и устройств.
   */
  snapshot() {
    return this.view;
  }
  /**
   * update объединяет изменение со снимком медиа и уведомляет подписчика состояния.
   *
   * @parameters:
   *   - patch (Partial<MediaView>) — частичное изменение снимка медиа.
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  private update(patch: Partial<MediaView>) {
    this.view = { ...this.view, ...patch };
    this.onChange(this.view);
  }
  /**
   * deadline ограничивает ожидание согласования таймером и выводит понятную ошибку по истечении срока.
   *
   * @parameters:
   *   - message (string) — понятный текст ошибки или сообщение операции.
   *   - ms — длительность ожидания в миллисекундах (по умолчанию 15000).
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  private deadline(message: string, ms = 15000) {
    clearTimeout(this.timer);
    this.timer = setTimeout(
      /**
       * Обработчик setTimeout выполняет отложенную либо периодическую часть операции.
       *
       *
       * @returns следующее состояние, рассчитанное из предыдущего значения.
       */ () => this.fail(message),
      ms,
    );
  }
  /**
   * fail фиксирует ошибку медиа и обновляет отображаемое состояние клиента.
   *
   * @parameters:
   *   - message (string) — понятный текст ошибки или сообщение операции.
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  private fail(message: string) {
    if (this.disposed) return;
    this.stop();
    this.update({ status: "Медиасвязь остановлена", error: message });
  }
  /**
   * stop закрывает WebRTC, останавливает принадлежащие клиенту дорожки и очищает таймеры и состояние.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  stop() {
    if (this.disposed) return;
    this.disposed = true;
    clearTimeout(this.timer);
    clearTimeout(this.disconnectedTimer);
    for (const timer of this.readyRequests.values()) clearTimeout(timer);
    this.readyRequests.clear();
    if (this.view.mediaPeerId || this.joinedRequest) {
      try {
        this.send(
          "media.leave",
          this.view.mediaPeerId ? { mediaPeerId: this.view.mediaPeerId } : {},
        );
      } catch {
        /* WS cleanup also closes the worker peer. */
      }
    }
    this.pc?.close();
    this.pc = null;
    for (const track of this.local?.getTracks() || []) track.stop();
    for (const track of this.screen?.getTracks() || []) track.stop();
    this.local = null;
    this.screen = null;
    this.sources.clear();
    this.received.clear();
    this.ownPublished.clear();
    this.streams.clear();
    this.remoteICE = [];
    this.localICE = [];
    this.update({
      active: false,
      localStream: null,
      localScreen: null,
      microphoneEnabled: false,
      cameraEnabled: false,
      screenSharing: false,
      controlBusy: false,
      remoteStreams: [],
      status: "Камера и микрофон выключены",
      connectionState: "closed",
      iceState: "closed",
    });
  }

  /**
   * start подготавливает медиа-соединение и при явном разрешении захватывает устройства пользователя.
   *
   * @parameters:
   *   - captureDevices — разрешает первоначальный захват устройств после явного действия пользователя (по умолчанию true).
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  async start(captureDevices = true) {
    if (this.started || this.disposed) return;
    this.started = true;
    this.update({
      active: true,
      status: captureDevices
        ? "Запрашиваем доступ к камере и микрофону…"
        : "Подключаем медиасвязь без устройств…",
      error: null,
    });
    try {
      if (captureDevices && !navigator.mediaDevices?.getUserMedia)
        throw new Error("secure_context");
      const stream =
        !captureDevices ||
        (this.policy.microphoneBlocked && this.policy.cameraBlocked)
          ? new MediaStream()
          : await navigator.mediaDevices.getUserMedia({
              audio: this.policy.microphoneBlocked
                ? false
                : {
                    echoCancellation: true,
                    noiseSuppression: true,
                    autoGainControl: true,
                  },
              video: this.policy.cameraBlocked
                ? false
                : {
                    width: { ideal: 1280, max: 1280 },
                    height: { ideal: 720, max: 720 },
                    frameRate: { ideal: 30, max: 30 },
                  },
            });
      if (this.disposed) {
        stream.getTracks().forEach(
          /**
           * Обработчик forEach выполняет переданный шаг вызова forEach в состоянии связи и WebRTC-медиа.
           *
           * @parameters:
           *   - track — дорожка захваченного или удалённого MediaStream.
           *
           * @returns вычисленное значение: track.stop().
           */ (track) => track.stop(),
        );
        return;
      }
      for (const track of stream.getTracks()) {
        if (
          (track.kind === "audio" && this.policy.microphoneBlocked) ||
          (track.kind === "video" && this.policy.cameraBlocked)
        ) {
          track.stop();
          stream.removeTrack(track);
        }
      }
      this.local = stream;
      this.update({
        localStream: stream.getTracks().length ? stream : null,
        microphoneEnabled: stream.getTracks().some(
          /**
           * Обработчик some проверяет, соответствует ли текущий элемент условию выборки или поиска.
           *
           * @parameters:
           *   - t — одна дорожка проверяемого медиапотока.
           *
           * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
           */ (t) => t.kind === "audio",
        ),
        cameraEnabled: stream.getTracks().some(
          /**
           * Обработчик some проверяет, соответствует ли текущий элемент условию выборки или поиска.
           *
           * @parameters:
           *   - t — одна дорожка проверяемого медиапотока.
           *
           * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
           */ (t) => t.kind === "video",
        ),
        status: "Подключаемся к media-worker…",
      });
      this.deadline(
        "Media-worker не ответил. Проверьте сервис и повторите подключение.",
      );
      this.joinedRequest = this.send("media.join", {});
    } catch (error) {
      const info = record(error);
      const name = typeof info?.name === "string" ? info.name : "";
      this.fail(
        error instanceof Error && error.message === "secure_context"
          ? "Камера доступна только на HTTPS или localhost. Откройте защищённую страницу."
          : name === "NotAllowedError" || name === "SecurityError"
            ? "Доступ к камере и микрофону запрещён. Разрешите их для этого сайта в браузере и повторите."
            : name === "NotFoundError"
              ? "Камера или микрофон не найдены. Подключите устройства и повторите."
              : name === "NotReadableError"
                ? "Не удалось открыть камеру или микрофон. Возможно, устройство занято другим приложением."
                : "Не удалось включить медиасвязь. Проверьте устройства и media-worker, затем повторите.",
      );
    }
  }

  /**
   * watchTrack следит за завершением захваченной дорожки и согласует отключение её семантического источника.
   *
   * @parameters:
   *   - source (MediaSource) — семантический источник медиа: микрофон, камера либо экран.
   *   - track (MediaStreamTrack) — дорожка захваченного или удалённого MediaStream.
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  private watchTrack(source: MediaSource, track: MediaStreamTrack) {
    track.onended =
      /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */ () => {
        if (this.disposed || this.sources.get(source)?.track !== track) return;
        for (const publication of this.ownPublished.values()) {
          if (publication.source === source)
            this.send("media.unpublish", {
              mediaPeerId: this.view.mediaPeerId,
              trackId: publication.id,
            });
        }
        if (source === "video/screen") void this.stopScreen();
        else
          void this.mutate(
            /**
             * Обработчик mutate выполняет переданный шаг вызова mutate в состоянии связи и WebRTC-медиа.
             *
             *
             * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ async () => {
              if (this.sources.get(source)?.track === track)
                await this.replaceSource(source, null);
            },
          );
      };
  }
  /**
   * refreshLocal обновляет локальные потоки и признаки активных источников для интерфейса.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  private refreshLocal() {
    this.update({
      localStream: this.local?.getTracks().some(
        /**
         * Обработчик some проверяет, соответствует ли текущий элемент условию выборки или поиска.
         *
         * @parameters:
         *   - track — дорожка захваченного или удалённого MediaStream.
         *
         * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
         */ (track) => track.readyState === "live",
      )
        ? this.local
        : null,
      localScreen: this.screen,
      microphoneEnabled:
        this.sources.get("microphone")?.track?.readyState === "live",
      cameraEnabled: this.sources.get("camera")?.track?.readyState === "live",
      screenSharing:
        this.sources.get("video/screen")?.track?.readyState === "live",
    });
  }
  /**
   * mutate сериализует изменение источников медиа, чтобы конкурирующие действия не нарушали порядок согласования.
   *
   * @parameters:
   *   - action (() => Promise<void>) — разрешённое действие управления либо асинхронная операция.
   *
   * @returns вычисленное значение: this.queue.
   */
  private mutate(
    action: /**
     * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
     *
     *
     * @returns Promise<void> — Promise с результатом описанной асинхронной операции; отказ передаётся через отклонение Promise.
     */ () => Promise<void>,
  ) {
    const next = this.queue.then(
      /**
       * Обработчик then выполняет переданный шаг вызова then в состоянии связи и WebRTC-медиа.
       *
       *
       * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */ async () => {
        if (this.disposed) return;
        await action();
        if (this.disposed) return;
        this.refreshLocal();
        this.localRevision++;
        await this.offer();
      },
    );
    this.queue = next.catch(
      /**
       * Обработчик next.catch обрабатывает отказ асинхронной операции в соответствии с текущим состоянием интерфейса.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */ () => {
        if (!this.disposed)
          this.update({
            error:
              "Не удалось изменить медиапоток. Повторите действие или переподключитесь.",
          });
      },
    );
    return this.queue;
  }
  /**
   * replaceSource заменяет или отключает дорожку конкретного источника через соответствующий RTCRtpSender.
   *
   * @parameters:
   *   - source (MediaSource) — семантический источник медиа: микрофон, камера либо экран.
   *   - track (MediaStreamTrack | null) — дорожка захваченного или удалённого MediaStream.
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  private async replaceSource(
    source: MediaSource,
    track: MediaStreamTrack | null,
  ) {
    const slot = this.sources.get(source);
    if (!slot || this.disposed) {
      track?.stop();
      return;
    }
    const previous = slot.track;
    await slot.sender.replaceTrack(track);
    if (this.disposed) {
      track?.stop();
      return;
    }
    slot.track = track;
    slot.transceiver.direction = track ? "sendrecv" : "recvonly";
    const stream = source.endsWith("/screen") ? this.screen : this.local;
    if (previous) {
      previous.onended = null;
      stream?.removeTrack(previous);
      previous.stop();
    }
    if (track) {
      if (!stream?.getTracks().includes(track)) stream?.addTrack(track);
      this.watchTrack(source, track);
    }
  }
  /**
   * setPolicy применяет текущие ограничения модерации к локальным устройствам и экрану.
   *
   * @parameters:
   *   - policy (MediaPolicy) — актуальные ограничения модерации источников.
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  setPolicy(policy: MediaPolicy) {
    // Delayed roster fetches must not undo a newer realtime restriction.
    if ((policy.version ?? 0) < (this.policy.version ?? 0)) return;
    this.policy = { ...policy };
    if (!this.pc || this.disposed) return;
    const blocked: MediaSource[] = [];
    if (policy.microphoneBlocked) blocked.push("microphone", "audio/screen");
    if (policy.cameraBlocked) blocked.push("camera");
    if (policy.screenBlocked || policy.cameraBlocked)
      blocked.push("video/screen", "audio/screen");
    if (
      blocked.some(
        /**
         * Обработчик blocked.some проверяет условие поиска элемента или соответствия элементов набора.
         *
         * @parameters:
         *   - source — семантический источник медиа: микрофон, камера либо экран.
         *
         * @returns логический признак соответствия элемента условию.
         */ (source) => this.sources.get(source)?.track,
      )
    )
      void this.mutate(
        /**
         * Обработчик mutate выполняет переданный шаг вызова mutate в состоянии связи и WebRTC-медиа.
         *
         *
         * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
         */ async () => {
          for (const source of blocked) await this.replaceSource(source, null);
          if (policy.screenBlocked || policy.cameraBlocked) this.screen = null;
        },
      );
  }
  /**
   * changeSource включает, отключает или меняет устройство конкретного разрешённого источника медиа.
   *
   * @parameters:
   *   - source ("microphone" | "camera") — семантический источник медиа: микрофон, камера либо экран.
   *   - enabled (boolean) — разрешает выполнение запроса или подключение при выполненных условиях доступа.
   *   - deviceId (string) — идентификатор выбранного пользователем устройства (необязательный параметр).
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  async changeSource(
    source: "microphone" | "camera",
    enabled: boolean,
    deviceId?: string,
  ) {
    if (this.disposed || !this.pc || this.view.controlBusy) return;
    deviceId = deviceId || this.preferredInputs.get(source);
    if (
      enabled &&
      (source === "microphone"
        ? this.policy.microphoneBlocked
        : this.policy.cameraBlocked)
    ) {
      this.update({
        error: "Организатор запретил включение этого устройства.",
      });
      return;
    }
    this.update({ controlBusy: true, error: null });
    let captured: MediaStream | null = null;
    try {
      if (enabled)
        captured = await navigator.mediaDevices.getUserMedia({
          audio:
            source === "microphone"
              ? {
                  echoCancellation: true,
                  noiseSuppression: true,
                  ...(deviceId ? { deviceId: { exact: deviceId } } : {}),
                }
              : false,
          video:
            source === "camera"
              ? {
                  width: {
                    ideal: this.capture.maxWidth,
                    max: this.capture.maxWidth,
                  },
                  height: {
                    ideal: this.capture.maxHeight,
                    max: this.capture.maxHeight,
                  },
                  frameRate: {
                    ideal: this.capture.maxFrameRate,
                    max: this.capture.maxFrameRate,
                  },
                  ...(deviceId ? { deviceId: { exact: deviceId } } : {}),
                }
              : false,
        });
      if (
        this.disposed ||
        (enabled &&
          (source === "microphone"
            ? this.policy.microphoneBlocked
            : this.policy.cameraBlocked))
      ) {
        captured?.getTracks().forEach(
          /**
           * Обработчик forEach выполняет переданный шаг вызова forEach в состоянии связи и WebRTC-медиа.
           *
           * @parameters:
           *   - track — дорожка захваченного или удалённого MediaStream.
           *
           * @returns вычисленное значение: track.stop().
           */ (track) => track.stop(),
        );
        return;
      }
      const track =
        captured?.getTracks().find(
          /**
           * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
           *
           * @parameters:
           *   - t — одна дорожка проверяемого медиапотока.
           *
           * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
           */
          (t) => t.kind === (source === "microphone" ? "audio" : "video"),
        ) || null;
      if (enabled && !track) throw new Error("missing_track");
      await this.mutate(
        /**
         * Обработчик mutate выполняет переданный шаг вызова mutate в состоянии связи и WebRTC-медиа.
         *
         *
         * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
         */ async () => {
          if (
            enabled &&
            (source === "microphone"
              ? this.policy.microphoneBlocked
              : this.policy.cameraBlocked)
          ) {
            track?.stop();
            return;
          }
          await this.replaceSource(source, track);
          if (track && deviceId) this.preferredInputs.set(source, deviceId);
        },
      );
    } catch {
      captured?.getTracks().forEach(
        /**
         * Обработчик forEach выполняет переданный шаг вызова forEach в состоянии связи и WebRTC-медиа.
         *
         * @parameters:
         *   - track — дорожка захваченного или удалённого MediaStream.
         *
         * @returns вычисленное значение: track.stop().
         */ (track) => track.stop(),
      );
      if (!this.disposed)
        this.update({
          error:
            "Не удалось открыть устройство. Проверьте разрешения браузера и выбор камеры или микрофона.",
        });
    } finally {
      for (const track of captured?.getTracks() || [])
        if (this.sources.get(source)?.track !== track) track.stop();
      if (!this.disposed) this.update({ controlBusy: false });
    }
  }
  /**
   * startScreen по действию пользователя запрашивает демонстрацию экрана и публикует разрешённые дорожки.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  async startScreen() {
    if (
      this.disposed ||
      !this.pc ||
      this.view.controlBusy ||
      this.view.screenSharing
    )
      return;
    if (this.policy.screenBlocked || this.policy.cameraBlocked) {
      this.update({
        error:
          "Организатор остановил демонстрацию экрана. Дождитесь разрешения.",
      });
      return;
    }
    this.update({ controlBusy: true, error: null });
    let stream: MediaStream | null = null;
    try {
      // Called directly by the button so transient user activation is retained.
      stream = await navigator.mediaDevices.getDisplayMedia({
        video: { frameRate: { ideal: 15, max: 30 } },
        audio: !this.policy.microphoneBlocked,
      });
      if (
        this.disposed ||
        this.policy.screenBlocked ||
        this.policy.cameraBlocked
      ) {
        stream.getTracks().forEach(
          /**
           * Обработчик forEach выполняет переданный шаг вызова forEach в состоянии связи и WebRTC-медиа.
           *
           * @parameters:
           *   - t — одна дорожка проверяемого медиапотока.
           *
           * @returns вычисленное значение: t.stop().
           */ (t) => t.stop(),
        );
        return;
      }
      const captured = stream;
      await this.mutate(
        /**
         * Обработчик mutate выполняет переданный шаг вызова mutate в состоянии связи и WebRTC-медиа.
         *
         *
         * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
         */ async () => {
          if (this.policy.screenBlocked || this.policy.cameraBlocked) {
            captured.getTracks().forEach(
              /**
               * Обработчик forEach выполняет переданный шаг вызова forEach в состоянии связи и WebRTC-медиа.
               *
               * @parameters:
               *   - t — одна дорожка проверяемого медиапотока.
               *
               * @returns вычисленное значение: t.stop().
               */ (t) => t.stop(),
            );
            return;
          }
          try {
            this.screen = new MediaStream();
            const video = captured.getTracks().find(
              /**
               * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
               *
               * @parameters:
               *   - t — одна дорожка проверяемого медиапотока.
               *
               * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
               */ (t) => t.kind === "video",
            );
            if (!video) throw new Error("missing_screen");
            await this.replaceSource("video/screen", video);
            await this.replaceSource(
              "audio/screen",
              this.policy.microphoneBlocked
                ? null
                : captured.getTracks().find(
                    /**
                     * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
                     *
                     * @parameters:
                     *   - t — одна дорожка проверяемого медиапотока.
                     *
                     * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
                     */ (t) => t.kind === "audio",
                  ) || null,
            );
          } catch (error) {
            // A second sender may reject after the first has accepted capture.
            // Stop physical capture even if rollback itself encounters an error.
            captured.getTracks().forEach(
              /**
               * Обработчик forEach выполняет переданный шаг вызова forEach в состоянии связи и WebRTC-медиа.
               *
               * @parameters:
               *   - track — дорожка захваченного или удалённого MediaStream.
               *
               * @returns вычисленное значение: track.stop().
               */ (track) => track.stop(),
            );
            for (const source of [
              "video/screen",
              "audio/screen",
            ] as MediaSource[]) {
              const slot = this.sources.get(source);
              if (!slot) continue;
              slot.track = null;
              slot.transceiver.direction = "recvonly";
              await slot.sender.replaceTrack(null).catch(
                /**
                 * Обработчик catch выполняет переданный шаг вызова catch в состоянии связи и WebRTC-медиа.
                 *
                 *
                 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                 */ () => {},
              );
            }
            this.screen = null;
            this.refreshLocal();
            this.localRevision++;
            await this.offer();
            throw error;
          }
        },
      );
      if (this.disposed)
        captured.getTracks().forEach(
          /**
           * Обработчик forEach выполняет переданный шаг вызова forEach в состоянии связи и WebRTC-медиа.
           *
           * @parameters:
           *   - t — одна дорожка проверяемого медиапотока.
           *
           * @returns вычисленное значение: t.stop().
           */ (t) => t.stop(),
        );
    } catch {
      stream?.getTracks().forEach(
        /**
         * Обработчик forEach выполняет переданный шаг вызова forEach в состоянии связи и WebRTC-медиа.
         *
         * @parameters:
         *   - t — одна дорожка проверяемого медиапотока.
         *
         * @returns вычисленное значение: t.stop().
         */ (t) => t.stop(),
      );
      if (!this.disposed)
        this.update({
          error:
            "Демонстрация не началась. Выберите окно или экран и разрешите доступ.",
        });
    } finally {
      for (const track of stream?.getTracks() || [])
        if (
          ![...this.sources.values()].some(
            /**
             * Обработчик some проверяет, соответствует ли текущий элемент условию выборки или поиска.
             *
             * @parameters:
             *   - slot — входное значение slot текущего шага обработки.
             *
             * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
             */ (slot) => slot.track === track,
          )
        )
          track.stop();
      if (!this.disposed) this.update({ controlBusy: false });
    }
  }
  /**
   * stopScreen останавливает принадлежащие клиенту дорожки экрана и согласует снятие публикации.
   *
   *
   * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
   */
  stopScreen() {
    return this.mutate(
      /**
       * Обработчик mutate выполняет переданный шаг вызова mutate в состоянии связи и WebRTC-медиа.
       *
       *
       * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */ async () => {
        await this.replaceSource("video/screen", null);
        await this.replaceSource("audio/screen", null);
        this.screen = null;
      },
    );
  }

  /**
   * handle ставит входящее медиа-событие в последовательную обработку, сохраняя порядок SDP и ICE.
   *
   * @parameters:
   *   - event (RealtimeEvent) — проверенный конверт события комнаты.
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  handle(event: RealtimeEvent) {
    if (
      this.disposed ||
      (!event.type.startsWith("media.") &&
        event.type !== "error" &&
        event.type !== "ack")
    )
      return;
    this.queue = this.queue
      .then(
        /**
         * Обработчик then выполняет переданный шаг вызова then в состоянии связи и WebRTC-медиа.
         *
         *
         * @returns вычисленное значение: this.process(event).
         */ () => this.process(event),
      )
      .catch(
        /**
         * Обработчик catch выполняет переданный шаг вызова catch в состоянии связи и WebRTC-медиа.
         *
         *
         * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
         */ () => {
          this.fail(
            "Ошибка обмена медиа. Переподключите камеру и микрофон; может требоваться TURN.",
          );
        },
      );
  }
  /**
   * process разбирает тип медиа-события, применяет сигнализацию и обновляет локальный снимок.
   *
   * @parameters:
   *   - event (RealtimeEvent) — проверенный конверт события комнаты.
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  private async process(event: RealtimeEvent) {
    if (this.disposed) return;
    const data = record(event.data);
    if (!data) return;
    if (event.type === "ack") {
      if (event.replyTo && data.type === "media.ready") {
        clearTimeout(this.readyRequests.get(event.replyTo));
        this.readyRequests.delete(event.replyTo);
      }
      return;
    }
    if (event.type === "error") {
      if (
        data.code === "media_policy_blocked" &&
        event.replyTo === this.negotiation?.request &&
        this.pc
      ) {
        // A policy can win a race with an offer already sent by this tab.
        // Keep receiving the conference while discarding rejected capture.
        await this.pc.setLocalDescription({ type: "rollback" });
        this.negotiation = null;
        for (const source of this.sources.keys())
          await this.replaceSource(source, null);
        this.screen = null;
        this.refreshLocal();
        this.localRevision++;
        this.update({
          error:
            "Организатор изменил разрешения. Передача устройств остановлена; приём конференции продолжается.",
        });
        await this.offer();
        return;
      }
      if (
        event.replyTo === this.negotiation?.request &&
        data.code === "screen_sharing_conflict" &&
        this.pc
      ) {
        await this.pc.setLocalDescription({ type: "rollback" });
        this.negotiation = null;
        await this.replaceSource("video/screen", null);
        await this.replaceSource("audio/screen", null);
        this.screen = null;
        this.refreshLocal();
        this.localRevision++;
        this.update({
          error:
            "Экран уже показывает другой участник. Дождитесь окончания его демонстрации.",
        });
        await this.offer();
        return;
      }
      if (
        event.replyTo === this.joinedRequest ||
        event.replyTo === this.negotiation?.request ||
        (event.replyTo && this.readyRequests.has(event.replyTo))
      )
        this.fail(
          "Медиасервис отклонил подключение. Проверьте доступ к конференции и повторите.",
        );
      return;
    }
    if (event.type === "media.joined") {
      if (!this.started || this.pc || event.replyTo !== this.joinedRequest)
        return;
      const list = tracks(data.tracks);
      if (
        typeof data.mediaPeerId !== "string" ||
        !uuid.test(data.mediaPeerId) ||
        typeof data.workerId !== "string" ||
        data.workerId.length > 128 ||
        !Number.isInteger(data.maxPeers) ||
        Number(data.maxPeers) < 1 ||
        Number(data.maxPeers) > 32 ||
        !Array.isArray(data.iceServers) ||
        data.iceServers.length > 16 ||
        !list
      )
        throw new Error("invalid_join");
      const joinedPolicy = record(data.policy);
      if (joinedPolicy) this.setPolicy(joinedPolicy as MediaPolicy);
      for (const track of this.local?.getTracks() || []) {
        if (
          (track.kind === "audio" && this.policy.microphoneBlocked) ||
          (track.kind === "video" && this.policy.cameraBlocked)
        ) {
          track.stop();
          this.local?.removeTrack(track);
        }
      }
      const capture = videoCapture(data.videoCapture);
      this.capture = capture;
      this.update({
        mediaPeerId: data.mediaPeerId,
        workerId: data.workerId,
        status: "Согласуем медиасвязь…",
      });
      this.catalog = list;
      const video = this.local?.getTracks().find(
        /**
         * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
         *
         * @parameters:
         *   - track — дорожка захваченного или удалённого MediaStream.
         *
         * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
         */ (track) => track.kind === "video",
      );
      if (video)
        await video.applyConstraints({
          width: { ideal: capture.maxWidth, max: capture.maxWidth },
          height: { ideal: capture.maxHeight, max: capture.maxHeight },
          frameRate: { ideal: capture.maxFrameRate, max: capture.maxFrameRate },
        });
      // Applying browser constraints is async. A stop/reconnect while pending
      // must not resurrect a PeerConnection or publish the captured stream.
      if (this.disposed) return;
      if (data.iceTransportPolicy !== undefined && data.iceTransportPolicy !== "all" && data.iceTransportPolicy !== "relay") throw new Error("invalid_ice_policy");
      this.createPeer(data.iceServers as RTCIceServer[], Number(data.maxPeers), data.iceTransportPolicy === "relay" ? "relay" : "all");
      this.refreshLocal();
      await this.offer();
      return;
    }
    if (!this.pc || data.mediaPeerId !== this.view.mediaPeerId) return;
    if (event.type === "media.policy") {
      const policy = record(data.policy);
      if (policy) this.setPolicy(policy as MediaPolicy);
      return;
    } else if (event.type === "media.answer") {
      if (
        data.negotiationId !== this.negotiation?.id ||
        typeof data.sdp !== "string" ||
        data.sdp.length > 262144
      )
        return;
      const offer = this.negotiation!;
      await this.pc.setRemoteDescription({ type: "answer", sdp: data.sdp });
      if (this.disposed) return;
      this.remoteDescriptionReady = true;
      for (const candidate of this.remoteICE.splice(0))
        await this.pc.addIceCandidate(candidate || undefined);
      if (this.disposed) return;
      // RTP for a newly negotiated sender must not precede the browser applying
      // its answer. WS command serialization places ready before the next offer.
      if (this.readyRequests.size >= 32) throw new Error("ready_limit");
      const readyRequest = this.send("media.ready", {
        mediaPeerId: this.view.mediaPeerId,
        negotiationId: offer.id,
      });
      this.readyRequests.set(
        readyRequest,
        setTimeout(
          /**
           * Обработчик setTimeout выполняет отложенную либо периодическую часть операции.
           *
           *
           * @returns следующее состояние, рассчитанное из предыдущего значения.
           */ () => {
            this.fail(
              "Media-worker не подтвердил готовность медиасвязи. Подключите камеру и микрофон снова.",
            );
          },
          15000,
        ),
      );
      this.answeredRevision = offer.revision;
      this.answeredLocalRevision = offer.localRevision;
      this.negotiation = null;
      this.update({
        negotiationState: this.pc.signalingState,
        status:
          this.pc.connectionState === "connected"
            ? "Медиасвязь подключена"
            : "Устанавливаем медиасвязь…",
      });
      if (this.pc.connectionState !== "connected")
        this.deadline(
          "ICE-соединение не установилось. Проверьте доступность медиа-портов и STUN/TURN.",
          25000,
        );
      else clearTimeout(this.timer);
      if (
        this.wantedRevision > this.answeredRevision ||
        this.localRevision > this.answeredLocalRevision
      )
        await this.offer();
    } else if (event.type === "media.renegotiate") {
      if (Number.isInteger(data.revision) && Number(data.revision) >= 0) {
        this.wantedRevision = Math.max(
          this.wantedRevision,
          Number(data.revision),
        );
        if (!this.negotiation && this.wantedRevision > this.answeredRevision)
          await this.offer();
      }
    } else if (event.type === "media.ice") {
      if (data.candidate !== null && !record(data.candidate))
        throw new Error("invalid_ice");
      const candidate = data.candidate as RTCIceCandidateInit | null;
      if (this.remoteDescriptionReady)
        await this.pc.addIceCandidate(candidate || undefined);
      else if (this.remoteICE.length < 128) this.remoteICE.push(candidate);
      else throw new Error("ice_limit");
    } else if (event.type === "media.tracks") {
      const list = tracks(data.tracks);
      if (
        !list ||
        !Number.isInteger(data.revision) ||
        Number(data.revision) < 0
      )
        throw new Error("invalid_tracks");
      if (Number(data.revision) < this.catalogRevision) return;
      this.catalogRevision = Number(data.revision);
      const ids = new Set(
        list.map(
          /**
           * Обработчик list.map преобразует один элемент набора в представление или данные следующего шага.
           *
           * @parameters:
           *   - entry — состояние видимости одного наблюдаемого элемента.
           *
           * @returns преобразованное значение текущего элемента для результирующего набора.
           */ (entry) => entry.id,
        ),
      );
      for (const previous of this.catalog) {
        if (ids.has(previous.id)) continue;
        for (const [key, received] of this.received) {
          if (
            (key === previous.id ||
              (received.track.kind === previous.kind &&
                received.streamIds.includes(previous.streamId))) &&
            !list.some(
              /**
               * Обработчик list.some проверяет условие поиска элемента или соответствия элементов набора.
               *
               * @parameters:
               *   - entry — состояние видимости одного наблюдаемого элемента.
               *
               * @returns логический признак соответствия элемента условию.
               */
              (entry) =>
                entry.kind === received.track.kind &&
                received.streamIds.includes(entry.streamId),
            )
          )
            this.received.delete(key);
        }
      }
      this.catalog = list;
      this.syncRemoteStreams();
    } else if (event.type === "media.state") {
      if (data.state === "failed" || data.state === "closed")
        this.fail(
          "Media-worker отключил медиасвязь. Повторите подключение камеры и микрофона.",
        );
    } else if (event.type === "media.left") {
      this.stop();
    } else if (event.type === "media.published") {
      const list = tracks([data.track]);
      if (!list || list[0].mediaPeerId !== this.view.mediaPeerId)
        throw new Error("invalid_published_track");
      this.ownPublished.set(list[0].id, list[0]);
      const localTrack = this.sources.get(list[0].source)?.track;
      if (!localTrack || localTrack.readyState === "ended")
        this.send("media.unpublish", {
          mediaPeerId: this.view.mediaPeerId,
          trackId: list[0].id,
        });
    } else if (
      event.type === "media.unpublished" &&
      typeof data.trackId === "string"
    ) {
      this.ownPublished.delete(data.trackId);
    }
  }

  /**
   * createPeer создаёт RTCPeerConnection с ICE-настройками и обработчиками удалённых дорожек и состояния.
   *
   * @parameters:
   *   - iceServers (RTCIceServer[]) — проверенные серверные настройки ICE.
   *   - maxPeers (number) — серверное ограничение числа подключений.
   *   - iceTransportPolicy (RTCIceTransportPolicy) — all или relay, выданный сервером режим ICE.
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  private createPeer(iceServers: RTCIceServer[], maxPeers: number, iceTransportPolicy: RTCIceTransportPolicy = "all") {
    // SFU always negotiates BUNDLE. Share one ICE transport even before the
    // first answer: gathering per receive slot can otherwise exceed the
    // signaling rate limit as the room's preallocated transceiver count grows.
    const pc = new RTCPeerConnection({
      iceServers,
      iceTransportPolicy,
      bundlePolicy: "max-bundle",
    });
    this.pc = pc;
    for (const source of [
      "microphone",
      "camera",
      "video/screen",
      "audio/screen",
    ] as MediaSource[]) {
      const kind =
        source === "microphone" || source === "audio/screen"
          ? "audio"
          : "video";
      const screen = source.endsWith("/screen");
      const track = screen
        ? null
        : this.local!.getTracks().find(
            /**
             * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
             *
             * @parameters:
             *   - t — одна дорожка проверяемого медиапотока.
             *
             * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
             */ (t) => t.kind === kind,
          ) || null;
      const transceiver = pc.addTransceiver(track || kind, {
        direction: track ? "sendrecv" : "recvonly",
        ...(track ? { streams: [this.local!] } : {}),
      });
      this.sources.set(source, {
        sender: transceiver.sender,
        transceiver,
        track,
      });
      if (track) this.watchTrack(source, track);
    }
    // The browser owns every offer. Stable receive slots prevent worker-driven
    // glare and make late publishers subscribable without server-side offers.
    for (let i = 0; i < 2 * (maxPeers - 1); i++) {
      pc.addTransceiver("audio", { direction: "recvonly" });
      pc.addTransceiver("video", { direction: "recvonly" });
    }
    for (const transceiver of pc.getTransceivers()) {
      const kind = transceiver.receiver.track.kind;
      const codecs = RTCRtpReceiver.getCapabilities?.(kind)?.codecs.filter(
        /**
         * Обработчик filter проверяет, соответствует ли текущий элемент условию выборки или поиска.
         *
         * @parameters:
         *   - codec — входное значение codec текущего шага обработки.
         *
         * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
         */
        (codec) =>
          codec.mimeType.toLowerCase() ===
          (kind === "audio" ? "audio/opus" : "video/vp8"),
      );
      if (
        codecs?.length &&
        typeof transceiver.setCodecPreferences === "function"
      )
        transceiver.setCodecPreferences(codecs);
    }
    pc.onicecandidate =
      /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
       *
       * @parameters:
       *   - event — проверенный конверт события комнаты.
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */ (event) => {
        if (this.disposed) return;
        const candidate = event.candidate?.toJSON() || null;
        if (!this.hasSentOffer) {
          if (this.localICE.length < 128) this.localICE.push(candidate);
          else
            this.fail(
              "Слишком много ICE-кандидатов. Проверьте сетевую конфигурацию.",
            );
        } else this.sendICE(candidate);
      };
    pc.ontrack =
      /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
       *
       * @parameters:
       *   - event — проверенный конверт события комнаты.
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */ (event) => {
        if (this.disposed) return;
        const streamIds =
          event.streams?.map(
            /**
             * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
             *
             * @parameters:
             *   - stream — поток браузерных медиа-дорожек.
             *
             * @returns преобразованное значение текущего элемента для результирующего набора.
             */ (stream) => stream.id,
          ) || [];
        for (const [id, previous] of this.received) {
          if (
            previous.track.kind === event.track.kind &&
            previous.streamIds.some(
              /**
               * Обработчик some проверяет, соответствует ли текущий элемент условию выборки или поиска.
               *
               * @parameters:
               *   - id — идентификатор ресурса или конференции данного запроса.
               *
               * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
               */ (id) => streamIds.includes(id),
            )
          )
            this.received.delete(id);
        }
        if (!this.received.has(event.track.id) && this.received.size >= 128) {
          this.fail("Превышен лимит удалённых медиа-треков.");
          return;
        }
        this.received.set(event.track.id, {
          track: event.track,
          streamIds,
        });
        event.track.onended =
          /**
           * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
           *
           *
           * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
           */ () => {
            if (this.received.get(event.track.id)?.track !== event.track)
              return;
            this.received.delete(event.track.id);
            this.syncRemoteStreams();
          };
        this.syncRemoteStreams();
      };
    pc.onconnectionstatechange =
      /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */ () => {
        if (this.disposed) return;
        const state = pc.connectionState;
        this.update({ connectionState: state });
        if (state === "connected") {
          if (!this.negotiation) clearTimeout(this.timer);
          clearTimeout(this.disconnectedTimer);
          this.update({ status: "Медиасвязь подключена" });
        } else if (state === "failed" || state === "closed") {
          this.fail(
            "Медиасвязь прервана. Подключите камеру и микрофон снова; может требоваться TURN.",
          );
        } else if (state === "disconnected") {
          this.update({ status: "Медиасвязь потеряна…" });
          clearTimeout(this.disconnectedTimer);
          this.disconnectedTimer = setTimeout(
            /**
             * Обработчик setTimeout выполняет отложенную либо периодическую часть операции.
             *
             *
             * @returns следующее состояние, рассчитанное из предыдущего значения.
             */
            () =>
              this.fail(
                "Медиасвязь не восстановилась. Подключите камеру и микрофон снова.",
              ),
            8000,
          );
        }
      };
    pc.oniceconnectionstatechange =
      /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */ () => {
        if (!this.disposed) this.update({ iceState: pc.iceConnectionState });
      };
    pc.onsignalingstatechange =
      /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */ () => {
        if (!this.disposed)
          this.update({ negotiationState: pc.signalingState });
      };
  }
  /**
   * sendICE отправляет кандидат ICE через проверенный канал сигнализации комнаты.
   *
   * @parameters:
   *   - candidate (RTCIceCandidateInit | null) — кандидат ICE или null после завершения сбора.
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  private sendICE(candidate: RTCIceCandidateInit | null) {
    try {
      this.send("media.ice", { mediaPeerId: this.view.mediaPeerId, candidate });
    } catch {
      this.fail("Realtime-связь потеряна. Подключите медиасвязь снова.");
    }
  }
  /**
   * offer создаёт SDP-предложение с семантическими источниками и отправляет его серверу.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  private async offer() {
    const pc = this.pc;
    if (
      this.disposed ||
      !pc ||
      this.negotiation ||
      pc.signalingState !== "stable"
    )
      return;
    const id = crypto.randomUUID();
    const negotiation = {
      id,
      request: "",
      revision: this.wantedRevision,
      localRevision: this.localRevision,
    };
    this.negotiation = negotiation;
    await pc.setLocalDescription(await pc.createOffer());
    if (this.disposed || this.pc !== pc) return;
    negotiation.request = this.send("media.offer", {
      mediaPeerId: this.view.mediaPeerId,
      negotiationId: id,
      sdp: pc.localDescription!.sdp,
      publications: [...this.sources]
        .filter(
          /**
           * Обработчик filter проверяет, соответствует ли текущий элемент условию выборки или поиска.
           *
           * @parameters:
           *   - [, slot] — элементы записи набора, извлечённые по указанным позициям.
           *
           * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
           */
          ([, slot]) =>
            slot.track?.readyState === "live" && slot.transceiver.mid !== null,
        )
        .map(
          /**
           * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
           *
           * @parameters:
           *   - [source, slot] — элементы записи набора, извлечённые по указанным позициям.
           *
           * @returns новый объект вычисленных данных.
           */ ([source, slot]) => ({
            mid: slot.transceiver.mid!,
            source,
            trackId: slot.track!.id,
          }),
        ),
    });
    this.hasSentOffer = true;
    for (const candidate of this.localICE.splice(0)) this.sendICE(candidate);
    this.deadline(
      "Ответ на медиа offer не получен. Повторите подключение камеры и микрофона.",
    );
  }
  /**
   * syncRemoteStreams обновляет удалённые потоки по действующим дорожкам и серверным сведениям источников.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  private syncRemoteStreams() {
    if (this.disposed) return;
    const grouped = new Map<
      string,
      {
        mediaPeerId: string;
        participantId: string;
        screen: boolean;
        tracks: MediaStreamTrack[];
      }
    >();
    for (const entry of this.catalog) {
      if (entry.mediaPeerId === this.view.mediaPeerId) continue;
      const track =
        this.received.get(entry.id)?.track ||
        [...this.received.values()].find(
          /**
           * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
           *
           * @parameters:
           *   - received — входное значение received текущего шага обработки.
           *
           * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
           */
          (received) =>
            received.track.kind === entry.kind &&
            received.streamIds.includes(entry.streamId),
        )?.track;
      if (!track || track.readyState === "ended") continue;
      const group = grouped.get(entry.streamId) || {
        mediaPeerId: entry.mediaPeerId,
        participantId: entry.participantId,
        screen: entry.source.endsWith("/screen"),
        tracks: [],
      };
      group.tracks.push(track);
      grouped.set(entry.streamId, group);
    }
    for (const [id, stream] of this.streams) {
      const active = grouped.get(id)?.tracks || [];
      stream.getTracks().forEach(
        /**
         * Обработчик forEach выполняет переданный шаг вызова forEach в состоянии связи и WebRTC-медиа.
         *
         * @parameters:
         *   - track — дорожка захваченного или удалённого MediaStream.
         *
         * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
         */ (track) => {
          if (!active.includes(track)) stream.removeTrack(track);
        },
      );
      if (!active.length) this.streams.delete(id);
    }
    const remoteStreams: RemoteMedia[] = [];
    for (const [id, group] of grouped) {
      let stream = this.streams.get(id);
      if (!stream) {
        stream = new MediaStream();
        this.streams.set(id, stream);
      }
      for (const track of group.tracks)
        if (!stream.getTracks().includes(track)) stream.addTrack(track);
      remoteStreams.push({
        id,
        mediaPeerId: group.mediaPeerId,
        participantId: group.participantId,
        stream,
        kinds: group.tracks.map(
          /**
           * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
           *
           * @parameters:
           *   - track — дорожка захваченного или удалённого MediaStream.
           *
           * @returns преобразованное значение текущего элемента для результирующего набора.
           */ (track) => track.kind,
        ),
        screen: group.screen,
      });
    }
    this.update({ remoteStreams });
  }
}

/**
 * emptyMediaView возвращает новое пустое состояние медиа без общего изменяемого объекта.
 *
 *
 * @returns новое независимое пустое состояние медиа.
 */
export function emptyMediaView() {
  return initial();
}
