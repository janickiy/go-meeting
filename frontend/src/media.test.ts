import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ConferenceMediaClient } from "./media";
import type { MediaTrack } from "./media";
import type { RealtimeEvent } from "./types";
import type { ClientRealtimeType } from "./realtime";

const peerId = "00000000-0000-4000-8000-000000000001";
const remotePeer = "00000000-0000-4000-8000-000000000002";
const remoteParticipant = "00000000-0000-4000-8000-000000000003";
const audioId = "00000000-0000-4000-8000-000000000004";
const videoId = "00000000-0000-4000-8000-000000000005";
/**
 * FakeTrack имитирует браузерную дорожку с управляемыми событиями.
 *
 * Состав:
 *   - readyState — поле или операция этого контракта.
 *   - onended — поле или операция этого контракта.
 *   - stop — поле или операция этого контракта.
 *   - applyConstraints — поле или операция этого контракта.
 *   - constructor — function Object() { [native code] }.
 */
class FakeTrack {
  readyState = "live";
  onended:
    | /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
       *
       *
       * @returns void — значение не возвращается; функция выполняет описанные действия.
       */ (() => void)
    | null = null;
  stop = vi.fn(
    /**
     * Обработчик vi.fn выполняет переданный шаг вызова vi.fn в проверках клиентского поведения.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */ () => {
      this.readyState = "ended";
    },
  );
  applyConstraints = vi.fn(
    /**
     * Обработчик vi.fn выполняет переданный шаг вызова vi.fn в проверках клиентского поведения.
     *
     * @parameters:
     *   - _constraints (MediaTrackConstraints) — ограничения захвата; имитация принимает их для совместимости с браузерным API.
     *
     * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */ async (_constraints: MediaTrackConstraints) => {},
  );
  /**
   * constructor function Object() { [native code] }.
   *
   * @parameters:
   *   - kind (string) — вид устройства, медиаисточника или события, определяющий действие.
   *   - id (string) — идентификатор ресурса или конференции данного запроса.
   *
   * @returns инициализированный экземпляр текущего класса.
   */
  constructor(
    public kind: string,
    public id: string,
  ) {}
}
/**
 * FakeStream имитирует браузерный поток и его набор дорожек.
 *
 * Состав:
 *   - constructor — function Object() { [native code] }.
 *   - getTracks — поле или операция этого контракта.
 *   - getAudioTracks — поле или операция этого контракта.
 *   - getVideoTracks — поле или операция этого контракта.
 *   - addTrack — поле или операция этого контракта.
 *   - removeTrack — поле или операция этого контракта.
 */
class FakeStream {
  /**
   * constructor function Object() { [native code] }.
   *
   * @parameters:
   *   - list (FakeTrack[]) — входное значение list текущего шага обработки (по умолчанию []).
   *
   * @returns инициализированный экземпляр текущего класса.
   */
  constructor(private list: FakeTrack[] = []) {}
  /**
   * getTracks возвращает дорожки подставного потока.
   *
   *
   * @returns вычисленное значение: [...this.list].
   */
  getTracks() {
    return [...this.list];
  }
  /**
   * getAudioTracks возвращает аудиодорожки подставного потока.
   *
   *
   * @returns вычисленное значение: this.list.filter( (track) => track.kind === "audio", ).
   */
  getAudioTracks() {
    return this.list.filter(
      /**
       * Обработчик filter проверяет, соответствует ли текущий элемент условию выборки или поиска.
       *
       * @parameters:
       *   - track — дорожка захваченного или удалённого MediaStream.
       *
       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
       */ (track) => track.kind === "audio",
    );
  }
  /**
   * getVideoTracks возвращает видеодорожки подставного потока.
   *
   *
   * @returns вычисленное значение: this.list.filter( (track) => track.kind === "video", ).
   */
  getVideoTracks() {
    return this.list.filter(
      /**
       * Обработчик filter проверяет, соответствует ли текущий элемент условию выборки или поиска.
       *
       * @parameters:
       *   - track — дорожка захваченного или удалённого MediaStream.
       *
       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
       */ (track) => track.kind === "video",
    );
  }
  /**
   * addTrack добавляет дорожку в подставной поток или соединение.
   *
   * @parameters:
   *   - track (FakeTrack) — дорожка захваченного или удалённого MediaStream.
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  addTrack(track: FakeTrack) {
    this.list.push(track);
  }
  /**
   * removeTrack удаляет дорожку подставного потока.
   *
   * @parameters:
   *   - track (FakeTrack) — дорожка захваченного или удалённого MediaStream.
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  removeTrack(track: FakeTrack) {
    this.list = this.list.filter(
      /**
       * Обработчик filter проверяет, соответствует ли текущий элемент условию выборки или поиска.
       *
       * @parameters:
       *   - t — одна дорожка проверяемого медиапотока.
       *
       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
       */ (t) => t !== track,
    );
  }
}
/**
 * FakePeer имитирует WebRTC-соединение, сигнализацию и трансиверы.
 *
 * Состав:
 *   - instances — поле или операция этого контракта.
 *   - connectionState — поле или операция этого контракта.
 *   - iceConnectionState — поле или операция этого контракта.
 *   - signalingState — поле или операция этого контракта.
 *   - localDescription — поле или операция этого контракта.
 *   - onicecandidate — поле или операция этого контракта.
 *   - ontrack — поле или операция этого контракта.
 *   - onconnectionstatechange — поле или операция этого контракта.
 *   - oniceconnectionstatechange — поле или операция этого контракта.
 *   - onsignalingstatechange — поле или операция этого контракта.
 *   - transceivers — поле или операция этого контракта.
 *   - constructor — function Object() { [native code] }.
 *   - addTrack — поле или операция этого контракта.
 *   - addTransceiver — поле или операция этого контракта.
 *   - getTransceivers — поле или операция этого контракта.
 *   - createOffer — поле или операция этого контракта.
 *   - setLocalDescription — поле или операция этого контракта.
 *   - setRemoteDescription — поле или операция этого контракта.
 *   - addIceCandidate — поле или операция этого контракта.
 *   - close — поле или операция этого контракта.
 */
class FakePeer {
  static instances: FakePeer[] = [];
  connectionState = "new";
  iceConnectionState = "new";
  signalingState = "stable";
  localDescription: RTCSessionDescriptionInit | null = null;
  onicecandidate:
    | /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
       *
       * @parameters:
       *   - event ({ candidate: { toJSON(): object } | null }) — проверенный конверт события комнаты.
       *
       * @returns void — значение не возвращается; функция выполняет описанные действия.
       */ ((event: {
        candidate: {
          /**
           * toJSON возвращает сериализуемую нагрузку подставного объекта.
           *
           *
           * @returns object — результат указанного контракта; реализация предоставляется вызывающим компонентом.
           */ toJSON(): object;
        } | null;
      }) => void)
    | null = null;
  ontrack:
    | /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
       *
       * @parameters:
       *   - event ({ track: FakeTrack; streams?: { id: string }[] }) — проверенный конверт события комнаты.
       *
       * @returns void — значение не возвращается; функция выполняет описанные действия.
       */ ((event: { track: FakeTrack; streams?: { id: string }[] }) => void)
    | null = null;
  onconnectionstatechange:
    | /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
       *
       *
       * @returns void — значение не возвращается; функция выполняет описанные действия.
       */ (() => void)
    | null = null;
  oniceconnectionstatechange:
    | /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
       *
       *
       * @returns void — значение не возвращается; функция выполняет описанные действия.
       */ (() => void)
    | null = null;
  onsignalingstatechange:
    | /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
       *
       *
       * @returns void — значение не возвращается; функция выполняет описанные действия.
       */ (() => void)
    | null = null;
  transceivers: {
    receiver: { track: { kind: string } };
    setCodecPreferences: ReturnType<typeof vi.fn>;
    direction: string;
    mid: string;
    sender: { replaceTrack: ReturnType<typeof vi.fn> };
  }[] = [];
  /**
   * constructor function Object() { [native code] }.
   *
   * @parameters:
   *   - config (RTCConfiguration) — параметры создания тестируемого компонента.
   *
   * @returns инициализированный экземпляр текущего класса.
   */
  constructor(public config: RTCConfiguration) {
    FakePeer.instances.push(this);
  }
  /**
   * addTrack добавляет дорожку в подставной поток или соединение.
   *
   * @parameters:
   *   - track (FakeTrack) — дорожка захваченного или удалённого MediaStream.
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  addTrack(track: FakeTrack) {
    this.addTransceiver(track.kind, { direction: "sendrecv" });
  }
  /**
   * addTransceiver создаёт подставной трансивер для проверки семантических источников.
   *
   * @parameters:
   *   - track (string | FakeTrack) — дорожка захваченного или удалённого MediaStream.
   *   - options ({ direction: string }) — метод, тело, отмена и признаки авторизации запроса.
   *
   * @returns вычисленное значение: item.
   */
  addTransceiver(track: string | FakeTrack, options: { direction: string }) {
    const item = {
      receiver: {
        track: { kind: typeof track === "string" ? track : track.kind },
      },
      mid: String(this.transceivers.length),
      sender: {
        replaceTrack: vi.fn(
          /**
           * Обработчик vi.fn выполняет переданный шаг вызова vi.fn в проверках клиентского поведения.
           *
           * @parameters:
           *   - _track (unknown) — медиа-дорожка; имитация принимает её для совместимости с браузерным контрактом.
           *
           * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
           */ async (_track: unknown) => {},
        ),
      },
      setCodecPreferences: vi.fn(
        /**
         * Обработчик vi.fn выполняет переданный шаг вызова vi.fn в проверках клиентского поведения.
         *
         * @parameters:
         *   - _codecs (unknown[]) — список кодеков; имитация принимает его для совместимости.
         *
         * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
         */ (_codecs: unknown[]) => {},
      ),
      direction: options.direction,
    };
    this.transceivers.push(item);
    return item;
  }
  /**
   * getTransceivers возвращает трансиверы подставного соединения.
   *
   *
   * @returns вычисленное значение: this.transceivers.
   */
  getTransceivers() {
    return this.transceivers;
  }
  createOffer = vi.fn(
    /**
     * Обработчик vi.fn выполняет переданный шаг вызова vi.fn в проверках клиентского поведения.
     *
     *
     * @returns Promise: новый объект вычисленных данных.
     */ async () => ({
      type: "offer",
      sdp: "test-sdp-not-logged",
    }),
  );
  setLocalDescription = vi.fn(
    /**
     * Обработчик vi.fn выполняет переданный шаг вызова vi.fn в проверках клиентского поведения.
     *
     * @parameters:
     *   - sdp (RTCSessionDescriptionInit) — описание согласуемого WebRTC-сеанса.
     *
     * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */ async (sdp: RTCSessionDescriptionInit) => {
      this.localDescription = sdp;
      this.signalingState =
        sdp.type === "rollback" ? "stable" : "have-local-offer";
    },
  );
  setRemoteDescription = vi.fn(
    /**
     * Обработчик vi.fn выполняет переданный шаг вызова vi.fn в проверках клиентского поведения.
     *
     *
     * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */ async () => {
      this.signalingState = "stable";
    },
  );
  addIceCandidate = vi.fn(
    /**
     * Обработчик vi.fn выполняет переданный шаг вызова vi.fn в проверках клиентского поведения.
     *
     *
     * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */ async () => {},
  );
  close = vi.fn(
    /**
     * Обработчик vi.fn выполняет переданный шаг вызова vi.fn в проверках клиентского поведения.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */ () => {
      this.connectionState = "closed";
    },
  );
}
/**
 * event вызывает обработчик подставного браузерного объекта.
 *
 * @parameters:
 *   - type (string) — машинный тип события.
 *   - data (unknown) — нагрузка события, проверяемая перед чтением.
 *   - replyTo (string) — идентификатор исходного сообщения или запроса (необязательный параметр).
 *
 * @returns RealtimeEvent — объект с данными, собранными в текущей операции.
 */
function event(type: string, data: unknown, replyTo?: string): RealtimeEvent {
  return {
    version: 1,
    id: crypto.randomUUID(),
    type,
    conferenceId: "room",
    timestamp: new Date().toISOString(),
    data,
    replyTo,
  };
}
/**
 * setup собирает изолированное окружение теста и подставные зависимости.
 *
 *
 * @returns объект с данными, собранными в текущей операции.
 */
function setup() {
  const localTracks = [
    new FakeTrack("audio", "local-audio"),
    new FakeTrack("video", "local-video"),
  ];
  const capture = vi.fn(
    /**
     * Обработчик vi.fn выполняет переданный шаг вызова vi.fn в проверках клиентского поведения.
     *
     * @parameters:
     *   - _constraints (MediaStreamConstraints) — ограничения захвата; имитация принимает их для совместимости с браузерным API.
     *
     * @returns Promise, который после завершения операции возвращает: вычисленное значение: new FakeStream(localTracks).
     */
    async (_constraints: MediaStreamConstraints) => new FakeStream(localTracks),
  );
  vi.stubGlobal("navigator", { mediaDevices: { getUserMedia: capture } });
  const send = vi.fn(
    /**
     * Обработчик vi.fn выполняет переданный шаг вызова vi.fn в проверках клиентского поведения.
     *
     * @parameters:
     *   - _type (ClientRealtimeType) — тип сигнализации; в этой имитации не используется.
     *   - _data (unknown) — нагрузка сигнализации; в этой имитации не используется.
     *
     * @returns вычисленное значение: crypto.randomUUID().
     */ (_type: ClientRealtimeType, _data: unknown) => crypto.randomUUID(),
  );
  const change = vi.fn();
  return {
    client: new ConferenceMediaClient(send, change),
    send,
    change,
    capture,
    localTracks,
  };
}
/**
 * joined создаёт событие успешного тестового входа.
 *
 * @parameters:
 *   - fixture (ReturnType<typeof setup>) — изолированные ресурсы тестового сценария.
 *
 * @returns Promise, который после завершения операции возвращает: вычисленное значение: FakePeer.instances[0].
 */
async function joined(fixture: ReturnType<typeof setup>) {
  await fixture.client.start();
  fixture.client.handle(
    event(
      "media.joined",
      {
        mediaPeerId: peerId,
        workerId: "worker-test",
        maxPeers: 10,
        iceServers: [],
        tracks: [],
      },
      fixture.send.mock.results[0].value,
    ),
  );
  await vi.waitFor(
    /**
     * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
     *
     *
     * @returns вычисленное значение: expect(FakePeer.instances).toHaveLength(1).
     */ () => expect(FakePeer.instances).toHaveLength(1),
  );
  await vi.waitFor(
    /**
     * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
     *
     *
     * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
     */ () =>
      expect(fixture.send).toHaveBeenCalledWith(
        "media.offer",
        expect.objectContaining({ mediaPeerId: peerId }),
      ),
  );
  return FakePeer.instances[0];
}
/**
 * offer создаёт SDP-предложение с семантическими источниками и отправляет его серверу.
 *
 * @parameters:
 *   - fixture (ReturnType<typeof setup>) — изолированные ресурсы тестового сценария.
 *
 * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
 */
function offer(fixture: ReturnType<typeof setup>) {
  return [...fixture.send.mock.calls].reverse().find(
    /**
     * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
     *
     * @parameters:
     *   - [type] — элементы записи набора, извлечённые по указанным позициям.
     *
     * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
     */ ([type]) => type === "media.offer",
  )?.[1] as { negotiationId: string };
}
beforeEach(
  /**
   * Обработчик beforeEach выполняет переданный шаг вызова beforeEach в проверках клиентского поведения.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    FakePeer.instances = [];
    vi.stubGlobal("RTCPeerConnection", FakePeer);
    vi.stubGlobal("MediaStream", FakeStream);
    vi.stubGlobal("RTCRtpReceiver", {
      /**
       * getCapabilities возвращает подготовленные возможности кодеков тестового браузера.
       *
       * @parameters:
       *   - kind (string) — вид устройства, медиаисточника или события, определяющий действие.
       *
       * @returns новый объект вычисленных данных.
       */
      getCapabilities: (kind: string) => ({
        codecs: [
          { mimeType: kind === "audio" ? "audio/opus" : "video/VP8" },
          { mimeType: "video/H264" },
        ],
      }),
    });
  },
);
afterEach(
  /**
   * Обработчик afterEach выполняет переданный шаг вызова afterEach в проверках клиентского поведения.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  },
);

describe("SFU media client", /**
 * Проверка: SFU media client выполняет тестовый сценарий «SFU media client» и проверяет ожидаемые результаты.
 *
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ () => {
  it("requests camera/microphone only on explicit start, bounds capture, and cleans up once", /**
   * Проверка: requests camera/microphone only on explicit start, bounds capture, and cleans up once выполняет тестовый сценарий «requests camera/microphone only on explicit start, bounds capture, and cleans up once» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    expect(f.capture).not.toHaveBeenCalled();
    await joined(f);
    expect(FakePeer.instances[0].config.bundlePolicy).toBe("max-bundle");
    expect(f.capture).toHaveBeenCalledOnce();
    expect(f.capture.mock.calls[0][0]).toMatchObject({
      video: {
        width: { max: 1280 },
        height: { max: 720 },
        frameRate: { max: 30 },
      },
    });
    expect(
      FakePeer.instances[0].transceivers.filter(
        /**
         * Обработчик filter проверяет, соответствует ли текущий элемент условию выборки или поиска.
         *
         * @parameters:
         *   - t — одна дорожка проверяемого медиапотока.
         *
         * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
         */
        (t) => t.direction === "recvonly",
      ),
    ).toHaveLength(38);
    expect(
      FakePeer.instances[0].transceivers.every(
        /**
         * Обработчик every проверяет, соответствует ли текущий элемент условию выборки или поиска.
         *
         * @parameters:
         *   - t — одна дорожка проверяемого медиапотока.
         *
         * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
         */
        (t) => t.setCodecPreferences.mock.calls[0][0].length === 1,
      ),
    ).toBe(true);
    f.client.stop();
    f.client.stop();
    f.localTracks.forEach(
      /**
       * Обработчик forEach выполняет переданный шаг вызова forEach в проверках клиентского поведения.
       *
       * @parameters:
       *   - t — одна дорожка проверяемого медиапотока.
       *
       * @returns вычисленное значение: expect(t.stop).toHaveBeenCalledOnce().
       */ (t) => expect(t.stop).toHaveBeenCalledOnce(),
    );
    expect(
      f.send.mock.calls.filter(
        /**
         * Обработчик filter проверяет, соответствует ли текущий элемент условию выборки или поиска.
         *
         * @parameters:
         *   - [type] — элементы записи набора, извлечённые по указанным позициям.
         *
         * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
         */ ([type]) => (type as unknown) === "media.leave",
      ),
    ).toHaveLength(1);
    expect(f.client.snapshot().active).toBe(false);
  });
  it("stops capture that resolves after the user leaves instead of joining a zombie peer", /**
   * Проверка: stops capture that resolves after the user leaves instead of joining a zombie peer выполняет тестовый сценарий «stops capture that resolves after the user leaves instead of joining a zombie peer» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    let resolve!: /**
     * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
     *
     * @parameters:
     *   - stream (FakeStream) — поток браузерных медиа-дорожек.
     *
     * @returns void — значение не возвращается; функция выполняет описанные действия.
     */ (stream: FakeStream) => void;
    f.capture.mockImplementation(
      /**
       * Обработчик mockImplementation выполняет переданный шаг вызова mockImplementation в проверках клиентского поведения.
       *
       *
       * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
       */
      () =>
        new Promise(
          /**
           * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
           *
           * @parameters:
           *   - r — один элемент проверяемого результата.
           *
           * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
           */ (r) => {
            resolve = r;
          },
        ),
    );
    const pending = f.client.start();
    f.client.stop();
    resolve(new FakeStream(f.localTracks));
    await pending;
    expect(f.send).not.toHaveBeenCalled();
    f.localTracks.forEach(
      /**
       * Обработчик forEach выполняет переданный шаг вызова forEach в проверках клиентского поведения.
       *
       * @parameters:
       *   - t — одна дорожка проверяемого медиапотока.
       *
       * @returns вычисленное значение: expect(t.stop).toHaveBeenCalledOnce().
       */ (t) => expect(t.stop).toHaveBeenCalledOnce(),
    );
  });
  it("reports denied permissions safely without opening a worker peer", /**
   * Проверка: reports denied permissions safely without opening a worker peer выполняет тестовый сценарий «reports denied permissions safely without opening a worker peer» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    f.capture.mockRejectedValue(
      new DOMException("private device detail", "NotAllowedError"),
    );
    await f.client.start();
    expect(f.client.snapshot().error).toContain(
      "Доступ к камере и микрофону запрещён",
    );
    expect(f.client.snapshot().error).not.toContain("private");
    expect(f.send).not.toHaveBeenCalled();
  });
  it("joins receive-only without requesting devices and can enable a single source later", /**
   * Проверка: joins receive-only without requesting devices and can enable a single source later выполняет тестовый сценарий «joins receive-only without requesting devices and can enable a single source later» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    await f.client.start(false);
    expect(f.capture).not.toHaveBeenCalled();
    f.client.handle(
      event(
        "media.joined",
        {
          mediaPeerId: peerId,
          workerId: "worker-test",
          maxPeers: 10,
          iceServers: [],
          tracks: [],
        },
        f.send.mock.results[0].value,
      ),
    );
    await vi.waitFor(
      /**
       * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(FakePeer.instances).toHaveLength(1).
       */ () => expect(FakePeer.instances).toHaveLength(1),
    );
    const pc = FakePeer.instances[0];
    expect(
      pc.transceivers.every(
        /**
         * Обработчик every проверяет, соответствует ли текущий элемент условию выборки или поиска.
         *
         * @parameters:
         *   - t — одна дорожка проверяемого медиапотока.
         *
         * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
         */ (t) => t.direction === "recvonly",
      ),
    ).toBe(true);
    expect(f.client.snapshot().microphoneEnabled).toBe(false);
    expect(f.client.snapshot().cameraEnabled).toBe(false);
    f.capture.mockResolvedValue(
      new FakeStream([new FakeTrack("audio", "mic-only")]),
    );
    await f.client.changeSource("microphone", true);
    expect(f.capture.mock.calls[0][0]).toMatchObject({ video: false });
    expect(f.client.snapshot().microphoneEnabled).toBe(true);
    expect(f.client.snapshot().cameraEnabled).toBe(false);
    f.client.stop();
  });
  it("cancels an in-flight media join with authenticated session cleanup", /**
   * Проверка: cancels an in-flight media join with authenticated session cleanup выполняет тестовый сценарий «cancels an in-flight media join with authenticated session cleanup» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    await f.client.start();
    expect(f.send).toHaveBeenCalledWith("media.join", {});
    f.client.stop();
    expect(f.send).toHaveBeenCalledWith("media.leave", {});
    f.client.handle(
      event(
        "media.joined",
        {
          mediaPeerId: peerId,
          workerId: "worker-test",
          maxPeers: 10,
          iceServers: [],
          tracks: [],
        },
        f.send.mock.results[0].value,
      ),
    );
    await new Promise(
      /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
       *
       * @parameters:
       *   - r — один элемент проверяемого результата.
       *
       * @returns вычисленное значение: setTimeout(r, 5).
       */ (r) => setTimeout(r, 5),
    );
    expect(FakePeer.instances).toHaveLength(0);
  });
  it("applies the worker's lower video target before creating an offer", /**
   * Проверка: applies the worker's lower video target before creating an offer выполняет тестовый сценарий «applies the worker's lower video target before creating an offer» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    await f.client.start();
    f.client.handle(
      event(
        "media.joined",
        {
          mediaPeerId: peerId,
          workerId: "worker-test",
          maxPeers: 10,
          iceServers: [],
          tracks: [],
          videoCapture: { maxWidth: 640, maxHeight: 360, maxFrameRate: 15 },
        },
        f.send.mock.results[0].value,
      ),
    );
    await vi.waitFor(
      /**
       * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
       */ () =>
        expect(f.send).toHaveBeenCalledWith(
          "media.offer",
          expect.objectContaining({ mediaPeerId: peerId }),
        ),
    );
    expect(f.localTracks[1].applyConstraints).toHaveBeenCalledWith({
      width: { ideal: 640, max: 640 },
      height: { ideal: 360, max: 360 },
      frameRate: { ideal: 15, max: 15 },
    });
    const offerIndex = f.send.mock.calls.findIndex(
      /**
       * Обработчик findIndex проверяет, соответствует ли текущий элемент условию выборки или поиска.
       *
       * @parameters:
       *   - [type] — элементы записи набора, извлечённые по указанным позициям.
       *
       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
       */
      ([type]) => type === "media.offer",
    );
    expect(
      f.localTracks[1].applyConstraints.mock.invocationCallOrder[0],
    ).toBeLessThan(f.send.mock.invocationCallOrder[offerIndex]);
    f.client.stop();
  });
  it.each([
    { maxWidth: 1920, maxHeight: 720, maxFrameRate: 30 },
    { maxWidth: 1280, maxHeight: 1080, maxFrameRate: 30 },
    { maxWidth: 1280, maxHeight: 720, maxFrameRate: 60 },
    { maxWidth: 640, maxHeight: 360, maxFrameRate: 0 },
  ])(
    "rejects an unsafe worker video target: %j",
    /**
     * Обработчик вызова выполняет переданный шаг вызова вызова в проверках клиентского поведения.
     *
     * @parameters:
     *   - videoCapture — профиль разрешения и частоты захвата видео.
     *
     * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */ async (videoCapture) => {
      const f = setup();
      await f.client.start();
      f.client.handle(
        event(
          "media.joined",
          {
            mediaPeerId: peerId,
            workerId: "worker-test",
            maxPeers: 10,
            iceServers: [],
            tracks: [],
            videoCapture,
          },
          f.send.mock.results[0].value,
        ),
      );
      await vi.waitFor(
        /**
         * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
         *
         *
         * @returns вычисленное значение: expect(f.client.snapshot().active).toBe(false).
         */ () => expect(f.client.snapshot().active).toBe(false),
      );
      expect(FakePeer.instances).toHaveLength(0);
      expect(f.localTracks[1].applyConstraints).not.toHaveBeenCalled();
      expect(f.send).toHaveBeenCalledWith("media.leave", {});
    },
  );
  it("does not recreate media after stopping during async camera constraints", /**
   * Проверка: does not recreate media after stopping during async camera constraints выполняет тестовый сценарий «does not recreate media after stopping during async camera constraints» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    let resolve!: /**
     * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
     *
     *
     * @returns void — значение не возвращается; функция выполняет описанные действия.
     */ () => void;
    f.localTracks[1].applyConstraints.mockImplementation(
      /**
       * Обработчик mockImplementation выполняет переданный шаг вызова mockImplementation в проверках клиентского поведения.
       *
       *
       * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
       */
      () =>
        new Promise<void>(
          /**
           * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
           *
           * @parameters:
           *   - r — один элемент проверяемого результата.
           *
           * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
           */ (r) => {
            resolve = r;
          },
        ),
    );
    await f.client.start();
    f.client.handle(
      event(
        "media.joined",
        {
          mediaPeerId: peerId,
          workerId: "worker-test",
          maxPeers: 10,
          iceServers: [],
          tracks: [],
        },
        f.send.mock.results[0].value,
      ),
    );
    await vi.waitFor(
      /**
       * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(f.localTracks[1].applyConstraints).toHaveBeenCalledOnce().
       */ () =>
        expect(f.localTracks[1].applyConstraints).toHaveBeenCalledOnce(),
    );
    f.client.stop();
    resolve();
    await new Promise(
      /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
       *
       * @parameters:
       *   - r — один элемент проверяемого результата.
       *
       * @returns вычисленное значение: setTimeout(r, 5).
       */ (r) => setTimeout(r, 5),
    );
    expect(FakePeer.instances).toHaveLength(0);
    expect(
      f.send.mock.calls.some(
        /**
         * Обработчик some проверяет, соответствует ли текущий элемент условию выборки или поиска.
         *
         * @parameters:
         *   - [type] — элементы записи набора, извлечённые по указанным позициям.
         *
         * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
         */ ([type]) => type === "media.offer",
      ),
    ).toBe(false);
    expect(f.send).toHaveBeenCalledWith("media.leave", { mediaPeerId: peerId });
  });
  it("serializes browser-owned offers, coalesces revisions, and ignores stale answers", /**
   * Проверка: serializes browser-owned offers, coalesces revisions, and ignores stale answers выполняет тестовый сценарий «serializes browser-owned offers, coalesces revisions, and ignores stale answers» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    const pc = await joined(f);
    const first = offer(f);
    for (const revision of [1, 2, 3])
      f.client.handle(
        event("media.renegotiate", { mediaPeerId: peerId, revision }),
      );
    f.client.handle(
      event("media.answer", {
        mediaPeerId: peerId,
        negotiationId: crypto.randomUUID(),
        sdp: "stale",
      }),
    );
    await new Promise(
      /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
       *
       * @parameters:
       *   - r — один элемент проверяемого результата.
       *
       * @returns вычисленное значение: setTimeout(r, 5).
       */ (r) => setTimeout(r, 5),
    );
    expect(pc.createOffer).toHaveBeenCalledOnce();
    expect(pc.setRemoteDescription).not.toHaveBeenCalled();
    expect(
      f.send.mock.calls.some(
        /**
         * Обработчик some проверяет, соответствует ли текущий элемент условию выборки или поиска.
         *
         * @parameters:
         *   - [type] — элементы записи набора, извлечённые по указанным позициям.
         *
         * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
         */ ([type]) => type === "media.ready",
      ),
    ).toBe(false);
    f.client.handle(
      event("media.answer", {
        mediaPeerId: peerId,
        negotiationId: first.negotiationId,
        sdp: "answer",
      }),
    );
    await vi.waitFor(
      /**
       * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(pc.createOffer).toHaveBeenCalledTimes(2).
       */ () => expect(pc.createOffer).toHaveBeenCalledTimes(2),
    );
    const readyIndex = f.send.mock.calls.findIndex(
      /**
       * Обработчик findIndex проверяет, соответствует ли текущий элемент условию выборки или поиска.
       *
       * @parameters:
       *   - [type] — элементы записи набора, извлечённые по указанным позициям.
       *
       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
       */
      ([type]) => type === "media.ready",
    );
    const nextOfferIndex = f.send.mock.calls.findIndex(
      /**
       * Обработчик findIndex проверяет, соответствует ли текущий элемент условию выборки или поиска.
       *
       * @parameters:
       *   - [type, data] — элементы записи набора, извлечённые по указанным позициям.
       *
       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
       */
      ([type, data]) =>
        type === "media.offer" &&
        (data as { negotiationId: string }).negotiationId !==
          first.negotiationId,
    );
    expect(f.send.mock.calls[readyIndex]).toEqual([
      "media.ready",
      { mediaPeerId: peerId, negotiationId: first.negotiationId },
    ]);
    expect(pc.setRemoteDescription.mock.invocationCallOrder[0]).toBeLessThan(
      f.send.mock.invocationCallOrder[readyIndex],
    );
    expect(readyIndex).toBeLessThan(nextOfferIndex);
    f.client.handle(
      event("media.answer", {
        mediaPeerId: peerId,
        negotiationId: offer(f).negotiationId,
        sdp: "answer-2",
      }),
    );
    await vi.waitFor(
      /**
       * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(pc.setRemoteDescription).toHaveBeenCalledTimes(2).
       */ () => expect(pc.setRemoteDescription).toHaveBeenCalledTimes(2),
    );
    expect(pc.createOffer).toHaveBeenCalledTimes(2);
    f.client.stop();
  });
  it("buffers ICE until the matching remote SDP and supports end-of-candidates", /**
   * Проверка: buffers ICE until the matching remote SDP and supports end-of-candidates выполняет тестовый сценарий «buffers ICE until the matching remote SDP and supports end-of-candidates» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    const pc = await joined(f);
    f.client.handle(
      event("media.ice", {
        mediaPeerId: peerId,
        candidate: { candidate: "candidate:test" },
      }),
    );
    f.client.handle(
      event("media.ice", { mediaPeerId: peerId, candidate: null }),
    );
    await new Promise(
      /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
       *
       * @parameters:
       *   - r — один элемент проверяемого результата.
       *
       * @returns вычисленное значение: setTimeout(r, 5).
       */ (r) => setTimeout(r, 5),
    );
    expect(pc.addIceCandidate).not.toHaveBeenCalled();
    f.client.handle(
      event("media.answer", {
        mediaPeerId: peerId,
        negotiationId: offer(f).negotiationId,
        sdp: "answer",
      }),
    );
    await vi.waitFor(
      /**
       * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(pc.addIceCandidate).toHaveBeenCalledTimes(2).
       */ () => expect(pc.addIceCandidate).toHaveBeenCalledTimes(2),
    );
    await vi.waitFor(
      /**
       * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
       */ () =>
        expect(f.send).toHaveBeenCalledWith(
          "media.ready",
          expect.objectContaining({ mediaPeerId: peerId }),
        ),
    );
    const readyIndex = f.send.mock.calls.findIndex(
      /**
       * Обработчик findIndex проверяет, соответствует ли текущий элемент условию выборки или поиска.
       *
       * @parameters:
       *   - [type] — элементы записи набора, извлечённые по указанным позициям.
       *
       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
       */
      ([type]) => type === "media.ready",
    );
    expect(pc.addIceCandidate.mock.invocationCallOrder[1]).toBeLessThan(
      f.send.mock.invocationCallOrder[readyIndex],
    );
    pc.onicecandidate!({ candidate: null });
    expect(f.send).toHaveBeenCalledWith("media.ice", {
      mediaPeerId: peerId,
      candidate: null,
    });
    f.client.stop();
  });
  it("stops media when the correlated readiness command is rejected", /**
   * Проверка: stops media when the correlated readiness command is rejected выполняет тестовый сценарий «stops media when the correlated readiness command is rejected» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    await joined(f);
    f.client.handle(
      event("media.answer", {
        mediaPeerId: peerId,
        negotiationId: offer(f).negotiationId,
        sdp: "answer",
      }),
    );
    await vi.waitFor(
      /**
       * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
       */ () =>
        expect(f.send).toHaveBeenCalledWith(
          "media.ready",
          expect.objectContaining({ mediaPeerId: peerId }),
        ),
    );
    const readyIndex = f.send.mock.calls.findIndex(
      /**
       * Обработчик findIndex проверяет, соответствует ли текущий элемент условию выборки или поиска.
       *
       * @parameters:
       *   - [type] — элементы записи набора, извлечённые по указанным позициям.
       *
       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
       */
      ([type]) => type === "media.ready",
    );
    f.client.handle(
      event(
        "error",
        { code: "media_negotiation_conflict" },
        f.send.mock.results[readyIndex].value,
      ),
    );
    await vi.waitFor(
      /**
       * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(f.client.snapshot().active).toBe(false).
       */ () => expect(f.client.snapshot().active).toBe(false),
    );
    expect(f.client.snapshot().error).toContain(
      "Медиасервис отклонил подключение",
    );
  });
  it("clears the readiness deadline on its matching acknowledgement", /**
   * Проверка: clears the readiness deadline on its matching acknowledgement выполняет тестовый сценарий «clears the readiness deadline on its matching acknowledgement» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    vi.useFakeTimers();
    const f = setup();
    const pc = await joined(f);
    f.client.handle(
      event("media.answer", {
        mediaPeerId: peerId,
        negotiationId: offer(f).negotiationId,
        sdp: "answer",
      }),
    );
    await vi.advanceTimersByTimeAsync(1);
    const readyIndex = f.send.mock.calls.findIndex(
      /**
       * Обработчик findIndex проверяет, соответствует ли текущий элемент условию выборки или поиска.
       *
       * @parameters:
       *   - [type] — элементы записи набора, извлечённые по указанным позициям.
       *
       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
       */
      ([type]) => type === "media.ready",
    );
    expect(readyIndex).toBeGreaterThan(-1);
    f.client.handle(
      event(
        "ack",
        { type: "media.ready" },
        f.send.mock.results[readyIndex].value,
      ),
    );
    await vi.advanceTimersByTimeAsync(1);
    pc.connectionState = "connected";
    pc.onconnectionstatechange!();
    await vi.advanceTimersByTimeAsync(16000);
    expect(f.client.snapshot().active).toBe(true);
    expect(f.client.snapshot().error).toBeNull();
    f.client.stop();
  });
  it("uses authoritative track metadata for remote streams and removes stopped publishers", /**
   * Проверка: uses authoritative track metadata for remote streams and removes stopped publishers выполняет тестовый сценарий «uses authoritative track metadata for remote streams and removes stopped publishers» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    const pc = await joined(f);
    const catalog: MediaTrack[] = [
      {
        id: audioId,
        streamId: remotePeer,
        mediaPeerId: remotePeer,
        participantId: remoteParticipant,
        kind: "audio",
        source: "microphone",
      },
      {
        id: videoId,
        streamId: remotePeer,
        mediaPeerId: remotePeer,
        participantId: remoteParticipant,
        kind: "video",
        source: "camera",
      },
    ];
    // Chromium's preallocated RTCRtpReceiver can keep its generated ID instead
    // of the worker's SDP track ID, while SDP stream ID remains authoritative.
    pc.ontrack!({
      track: new FakeTrack("audio", "browser-audio-id"),
      streams: [{ id: remotePeer }],
    });
    pc.ontrack!({
      track: new FakeTrack("video", "browser-video-id"),
      streams: [{ id: remotePeer }],
    });
    expect(f.client.snapshot().remoteStreams).toHaveLength(0);
    f.client.handle(
      event("media.tracks", {
        mediaPeerId: peerId,
        revision: 2,
        tracks: catalog,
      }),
    );
    await vi.waitFor(
      /**
       * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(f.client.snapshot().remoteStreams).toHaveLength(1).
       */ () => expect(f.client.snapshot().remoteStreams).toHaveLength(1),
    );
    expect(f.client.snapshot().remoteStreams[0].kinds).toEqual([
      "audio",
      "video",
    ]);
    f.client.handle(
      event("media.tracks", { mediaPeerId: peerId, revision: 3, tracks: [] }),
    );
    await vi.waitFor(
      /**
       * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(f.client.snapshot().remoteStreams).toHaveLength(0).
       */ () => expect(f.client.snapshot().remoteStreams).toHaveLength(0),
    );
    f.client.handle(
      event("media.tracks", {
        mediaPeerId: peerId,
        revision: 2,
        tracks: catalog,
      }),
    );
    await new Promise(
      /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
       *
       * @parameters:
       *   - r — один элемент проверяемого результата.
       *
       * @returns вычисленное значение: setTimeout(r, 5).
       */ (r) => setTimeout(r, 5),
    );
    expect(f.client.snapshot().remoteStreams).toHaveLength(0);
    f.client.stop();
  });
  it("ignores stale joined responses and unrelated server errors", /**
   * Проверка: ignores stale joined responses and unrelated server errors выполняет тестовый сценарий «ignores stale joined responses and unrelated server errors» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    await f.client.start();
    f.client.handle(
      event(
        "media.joined",
        {
          mediaPeerId: peerId,
          workerId: "worker-test",
          maxPeers: 10,
          iceServers: [],
          tracks: [],
        },
        "stale-request",
      ),
    );
    f.client.handle(
      event("error", { code: "private_code" }, "not-this-media-request"),
    );
    await new Promise(
      /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
       *
       * @parameters:
       *   - r — один элемент проверяемого результата.
       *
       * @returns вычисленное значение: setTimeout(r, 5).
       */ (r) => setTimeout(r, 5),
    );
    expect(FakePeer.instances).toHaveLength(0);
    expect(f.client.snapshot().error).toBeNull();
    f.client.stop();
  });
  it("reports stopped local tracks with the worker-assigned track identity", /**
   * Проверка: reports stopped local tracks with the worker-assigned track identity выполняет тестовый сценарий «reports stopped local tracks with the worker-assigned track identity» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    await joined(f);
    f.client.handle(
      event("media.published", {
        mediaPeerId: peerId,
        track: {
          id: videoId,
          streamId: peerId,
          mediaPeerId: peerId,
          participantId: remoteParticipant,
          kind: "video",
          source: "camera",
        },
      }),
    );
    await new Promise(
      /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
       *
       * @parameters:
       *   - r — один элемент проверяемого результата.
       *
       * @returns вычисленное значение: setTimeout(r, 5).
       */ (r) => setTimeout(r, 5),
    );
    f.localTracks[1].readyState = "ended";
    f.localTracks[1].onended!();
    expect(f.send).toHaveBeenCalledWith("media.unpublish", {
      mediaPeerId: peerId,
      trackId: videoId,
    });
    f.client.stop();
  });
  it("releases devices and requires an explicit restart after ICE failure", /**
   * Проверка: releases devices and requires an explicit restart after ICE failure выполняет тестовый сценарий «releases devices and requires an explicit restart after ICE failure» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    const pc = await joined(f);
    pc.connectionState = "failed";
    pc.onconnectionstatechange!();
    expect(f.client.snapshot().active).toBe(false);
    expect(f.client.snapshot().localStream).toBeNull();
    expect(f.client.snapshot().error).toContain("Медиасвязь прервана");
    f.localTracks.forEach(
      /**
       * Обработчик forEach выполняет переданный шаг вызова forEach в проверках клиентского поведения.
       *
       * @parameters:
       *   - track — дорожка захваченного или удалённого MediaStream.
       *
       * @returns вычисленное значение: expect(track.stop).toHaveBeenCalledOnce().
       */ (track) => expect(track.stop).toHaveBeenCalledOnce(),
    );
    await f.client.start();
    expect(f.capture).toHaveBeenCalledOnce();
  });
  it("toggles and replaces capture without creating a new peer or leaking tracks", /**
   * Проверка: toggles and replaces capture without creating a new peer or leaking tracks выполняет тестовый сценарий «toggles and replaces capture without creating a new peer or leaking tracks» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    const pc = await joined(f);
    await f.client.changeSource("camera", false);
    expect(f.localTracks[1].stop).toHaveBeenCalledOnce();
    expect(f.client.snapshot().cameraEnabled).toBe(false);
    const replacement = new FakeTrack("video", "replacement-camera");
    f.capture.mockResolvedValueOnce(new FakeStream([replacement]));
    await f.client.changeSource("camera", true, "device-b");
    expect(f.capture).toHaveBeenLastCalledWith(
      expect.objectContaining({
        audio: false,
        video: expect.objectContaining({ deviceId: { exact: "device-b" } }),
      }),
    );
    expect(pc.transceivers[1].sender.replaceTrack).toHaveBeenLastCalledWith(
      replacement,
    );
    expect(f.client.snapshot().cameraEnabled).toBe(true);
    expect(FakePeer.instances).toHaveLength(1);
    f.client.stop();
    expect(replacement.stop).toHaveBeenCalledOnce();
  });
  it("blocks capture on moderator policy without automatically enabling hardware on unblock", /**
   * Проверка: blocks capture on moderator policy without automatically enabling hardware on unblock выполняет тестовый сценарий «blocks capture on moderator policy without automatically enabling hardware on unblock» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    await joined(f);
    f.client.setPolicy({ microphoneBlocked: true, cameraBlocked: true });
    await vi.waitFor(
      /**
       * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(f.client.snapshot().cameraEnabled).toBe(false).
       */ () => expect(f.client.snapshot().cameraEnabled).toBe(false),
    );
    expect(f.client.snapshot().microphoneEnabled).toBe(false);
    await f.client.changeSource("camera", true);
    expect(f.capture).toHaveBeenCalledOnce();
    f.client.setPolicy({});
    expect(f.client.snapshot().cameraEnabled).toBe(false);
    f.client.stop();
  });
  it("keeps camera while screen shares and handles native screen end without requiring audio", /**
   * Проверка: keeps camera while screen shares and handles native screen end without requiring audio выполняет тестовый сценарий «keeps camera while screen shares and handles native screen end without requiring audio» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    await joined(f);
    const screen = new FakeTrack("video", "screen-video");
    Object.assign(navigator.mediaDevices, {
      getDisplayMedia: vi.fn(
        /**
         * Обработчик vi.fn выполняет переданный шаг вызова vi.fn в проверках клиентского поведения.
         *
         *
         * @returns Promise, который после завершения операции возвращает: вычисленное значение: new FakeStream([screen]).
         */ async () => new FakeStream([screen]),
      ),
    });
    await f.client.startScreen();
    expect(f.client.snapshot().screenSharing).toBe(true);
    expect(f.client.snapshot().cameraEnabled).toBe(true);
    expect(f.client.snapshot().localScreen?.getTracks()).toEqual([screen]);
    screen.readyState = "ended";
    screen.onended!();
    await vi.waitFor(
      /**
       * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(f.client.snapshot().screenSharing).toBe(false).
       */ () => expect(f.client.snapshot().screenSharing).toBe(false),
    );
    expect(f.client.snapshot().localScreen).toBeNull();
    expect(f.localTracks[1].stop).not.toHaveBeenCalled();
    f.client.stop();
  });
  it("releases a screen capture that completes after leave", /**
   * Проверка: releases a screen capture that completes after leave выполняет тестовый сценарий «releases a screen capture that completes after leave» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    await joined(f);
    const screen = new FakeTrack("video", "late-screen");
    let resolve!: /**
     * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
     *
     * @parameters:
     *   - stream (FakeStream) — поток браузерных медиа-дорожек.
     *
     * @returns void — значение не возвращается; функция выполняет описанные действия.
     */ (stream: FakeStream) => void;
    Object.assign(navigator.mediaDevices, {
      getDisplayMedia: vi.fn(
        /**
         * Обработчик vi.fn выполняет переданный шаг вызова vi.fn в проверках клиентского поведения.
         *
         *
         * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
         */
        () =>
          new Promise<FakeStream>(
            /**
             * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
             *
             * @parameters:
             *   - r — один элемент проверяемого результата.
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ (r) => {
              resolve = r;
            },
          ),
      ),
    });
    const pending = f.client.startScreen();
    f.client.stop();
    resolve(new FakeStream([screen]));
    await pending;
    expect(screen.stop).toHaveBeenCalled();
    expect(f.client.snapshot().localScreen).toBeNull();
  });
  it("rejects stale policy and stops screen audio on forced mute", /**
   * Проверка: rejects stale policy and stops screen audio on forced mute выполняет тестовый сценарий «rejects stale policy and stops screen audio on forced mute» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    await joined(f);
    const video = new FakeTrack("video", "screen-v");
    const audio = new FakeTrack("audio", "screen-a");
    Object.assign(navigator.mediaDevices, {
      getDisplayMedia: vi.fn(
        /**
         * Обработчик vi.fn выполняет переданный шаг вызова vi.fn в проверках клиентского поведения.
         *
         *
         * @returns Promise, который после завершения операции возвращает: вычисленное значение: new FakeStream([video, audio]).
         */ async () => new FakeStream([video, audio]),
      ),
    });
    await f.client.startScreen();
    f.client.setPolicy({ version: 3, microphoneBlocked: true });
    f.client.setPolicy({ version: 2, microphoneBlocked: false });
    await vi.waitFor(
      /**
       * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(audio.stop).toHaveBeenCalled().
       */ () => expect(audio.stop).toHaveBeenCalled(),
    );
    expect(video.stop).not.toHaveBeenCalled();
    await f.client.changeSource("microphone", true);
    expect(f.capture).toHaveBeenCalledOnce();
    f.client.stop();
  });
  it("cleans partially installed screen capture if a second sender rejects", /**
   * Проверка: cleans partially installed screen capture if a second sender rejects выполняет тестовый сценарий «cleans partially installed screen capture if a second sender rejects» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    const pc = await joined(f);
    const video = new FakeTrack("video", "screen-v");
    const audio = new FakeTrack("audio", "screen-a");
    Object.assign(navigator.mediaDevices, {
      getDisplayMedia: vi.fn(
        /**
         * Обработчик vi.fn выполняет переданный шаг вызова vi.fn в проверках клиентского поведения.
         *
         *
         * @returns Promise, который после завершения операции возвращает: вычисленное значение: new FakeStream([video, audio]).
         */ async () => new FakeStream([video, audio]),
      ),
    });
    pc.transceivers[3].sender.replaceTrack.mockRejectedValueOnce(
      new Error("replace failed"),
    );
    await f.client.startScreen();
    expect(video.stop).toHaveBeenCalled();
    expect(audio.stop).toHaveBeenCalled();
    expect(f.client.snapshot().localScreen).toBeNull();
    expect(f.client.snapshot().screenSharing).toBe(false);
    expect(f.client.snapshot().cameraEnabled).toBe(true);
    f.client.stop();
  });
  it("uses the newest receiver for a replaced publication and ignores old track end", /**
   * Проверка: uses the newest receiver for a replaced publication and ignores old track end выполняет тестовый сценарий «uses the newest receiver for a replaced publication and ignores old track end» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    const pc = await joined(f);
    f.client.handle(
      event("media.tracks", {
        mediaPeerId: peerId,
        revision: 1,
        tracks: [
          {
            id: videoId,
            streamId: remotePeer,
            mediaPeerId: remotePeer,
            participantId: remoteParticipant,
            kind: "video",
            source: "camera",
          },
        ],
      }),
    );
    const old = new FakeTrack("video", "browser-track");
    pc.ontrack!({ track: old, streams: [{ id: remotePeer }] });
    await vi.waitFor(
      /**
       * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(f.client.snapshot().remoteStreams).toHaveLength(1).
       */ () => expect(f.client.snapshot().remoteStreams).toHaveLength(1),
    );
    const fresh = new FakeTrack("video", "browser-track");
    pc.ontrack!({ track: fresh, streams: [{ id: remotePeer }] });
    old.onended!();
    expect(
      f.client.snapshot().remoteStreams[0].stream.getVideoTracks(),
    ).toEqual([fresh]);
    f.client.stop();
  });
  it("keeps screen video if optional screen audio ends", /**
   * Проверка: keeps screen video if optional screen audio ends выполняет тестовый сценарий «keeps screen video if optional screen audio ends» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    await joined(f);
    const video = new FakeTrack("video", "screen-v");
    const audio = new FakeTrack("audio", "screen-a");
    Object.assign(navigator.mediaDevices, {
      getDisplayMedia: vi.fn(
        /**
         * Обработчик vi.fn выполняет переданный шаг вызова vi.fn в проверках клиентского поведения.
         *
         *
         * @returns Promise, который после завершения операции возвращает: вычисленное значение: new FakeStream([video, audio]).
         */ async () => new FakeStream([video, audio]),
      ),
    });
    await f.client.startScreen();
    audio.readyState = "ended";
    audio.onended!();
    await vi.waitFor(
      /**
       * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(f.client.snapshot().localScreen?.getAudioTracks()).toHaveLength( 0, ).
       */ () =>
        expect(f.client.snapshot().localScreen?.getAudioTracks()).toHaveLength(
          0,
        ),
    );
    expect(f.client.snapshot().screenSharing).toBe(true);
    expect(video.stop).not.toHaveBeenCalled();
    f.client.stop();
  });
  it("preserves the peer as receive-only when a moderator policy races an offer", /**
   * Проверка: preserves the peer as receive-only when a moderator policy races an offer выполняет тестовый сценарий «preserves the peer as receive-only when a moderator policy races an offer» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const f = setup();
    const pc = await joined(f);
    const index = f.send.mock.calls.findIndex(
      /**
       * Обработчик findIndex проверяет, соответствует ли текущий элемент условию выборки или поиска.
       *
       * @parameters:
       *   - [type] — элементы записи набора, извлечённые по указанным позициям.
       *
       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
       */
      ([type]) => type === "media.offer",
    );
    f.client.handle(
      event(
        "error",
        { code: "media_policy_blocked" },
        f.send.mock.results[index].value,
      ),
    );
    await vi.waitFor(
      /**
       * Обработчик vi.waitFor выполняет переданный шаг вызова vi.waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(pc.createOffer).toHaveBeenCalledTimes(2).
       */ () => expect(pc.createOffer).toHaveBeenCalledTimes(2),
    );
    expect(pc.close).not.toHaveBeenCalled();
    expect(f.client.snapshot().active).toBe(true);
    expect(f.client.snapshot().cameraEnabled).toBe(false);
    expect(f.client.snapshot().microphoneEnabled).toBe(false);
    expect(f.send).toHaveBeenLastCalledWith(
      "media.offer",
      expect.objectContaining({ publications: [] }),
    );
    f.client.stop();
  });
});
