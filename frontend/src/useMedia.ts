import { useCallback, useEffect, useRef, useState } from "react";
import { ConferenceMediaClient, emptyMediaView } from "./media";
import type { MediaPolicy, MediaStartOptions } from "./media";
import { api } from "./api";
import type { useRealtime } from "./realtime";

/**
 * useMedia связывает состояние React с клиентом WebRTC и освобождает медиа при смене конференции или отключении.
 *
 * @args
 *   - live (ReturnType<typeof useRealtime>) — входное значение live текущего шага обработки.
 *   - conferenceId (string) — идентификатор конференции и области данных.
 *   - policy (MediaPolicy) — актуальные ограничения модерации источников.
 *
 * @returns состояние, данные или действия React-хука; ресурсы освобождаются при изменении зависимостей.
 */
export function useMedia(
  live: ReturnType<typeof useRealtime>,
  conferenceId: string,
  policy: MediaPolicy,
) {
  const [view, setView] = useState(emptyMediaView);
  const [running, setRunning] = useState(false);
  const controller = useRef<ConferenceMediaClient | null>(null);
  const mounted = useRef(true);
  const liveRef = useRef(live);
  const policyRef = useRef(policy);
  // Счётчик не сбрасывается при media.stop/start: запоздалые HTTP-запросы прежнего
  // захвата не должны перезаписывать новый снимок в том же соединении WS.
  const mediaSequence = useRef(0);
  policyRef.current = policy;
  liveRef.current = live;
  /**
   * stop закрывает WebRTC, останавливает принадлежащие клиенту дорожки и очищает таймеры и состояние.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  const stop = () => {
    controller.current?.stop();
    controller.current = null;
    setRunning(false);
  };
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      mounted.current = true;
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      return () => {
        mounted.current = false;
        controller.current?.stop();
        controller.current = null;
      };
    },
    [],
  );
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      stop();
      setView(emptyMediaView());
    },
    [live.state?.connectionId],
  );
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      controller.current?.setPolicy(policy);
    },
    [
      policy.version,
      policy.microphoneBlocked,
      policy.cameraBlocked,
      policy.screenBlocked,
    ],
  );
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      const connectionId = live.state?.connectionId;
      if (!connectionId) return;
      const snapshot = {
        connectionId,
        sequence: ++mediaSequence.current,
        microphoneEnabled: view.microphoneEnabled,
        cameraEnabled: view.cameraEnabled,
        screenSharing: view.screenSharing,
      };
      const timer = setTimeout(
        /**
         * Обработчик setTimeout выполняет отложенную либо периодическую часть операции.
         *
         *
         * @returns следующее состояние, рассчитанное из предыдущего значения.
         */ () => {
          void api.setMediaState(conferenceId, snapshot).catch(
            /**
             * Обработчик catch выполняет переданный шаг вызова catch в состоянии связи и WebRTC-медиа.
             *
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ () => {},
          );
        },
        100,
      );
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns вычисленное значение: clearTimeout(timer).
       */
      return () => clearTimeout(timer);
    },
    [
      conferenceId,
      live.state?.connectionId,
      view.microphoneEnabled,
      view.cameraEnabled,
      view.screenSharing,
    ],
  );
  live.onEvent.current =
    /**
     * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
     *
     * @args
     *   - event — проверенный конверт события комнаты.
     *
     * @returns вычисленное значение: controller.current?.handle(event).
     */ (event) => controller.current?.handle(event);
  /**
   * start подготавливает медиа-соединение и при явном разрешении захватывает устройства пользователя.
   *
   * @args
   *   - captureDevices — разрешает первоначальный захват устройств после явного действия пользователя (по умолчанию true).
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  const start = (captureDevices = true, options?: MediaStartOptions) => {
    const connectionId = liveRef.current.state?.connectionId;
    if (!connectionId) return;
    stop();
    const next = new ConferenceMediaClient(
      /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
       *
       * @args
       *   - type — машинный тип события.
       *   - data — нагрузка события, проверяемая перед чтением.
       *
       * @returns вычисленное значение: liveRef.current.send(type, data).
       */
      (type, data) => {
        if (liveRef.current.state?.connectionId !== connectionId)
          throw new Error("connection_changed");
        return liveRef.current.send(type, data);
      },
      /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
       *
       * @args
       *   - state — новое состояние источников медиа.
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      (state) => {
        if (mounted.current && controller.current === next) {
          setView(state);
          setRunning(state.active);
        }
      },
    );
    controller.current = next;
    next.setPolicy(policyRef.current);
    setRunning(true);
    if (options) void next.start(captureDevices, options);
    else void next.start(captureDevices);
  };
  const diagnostics = useCallback(async () => {
    const current = controller.current;
    if (!current) return null;
    const sample = await current.diagnostics();
    return controller.current === current ? sample : null;
  }, []);
  const configure = useCallback(
    (options: MediaStartOptions) => controller.current?.configure(options),
    [],
  );
  return {
    configure,
    view,
    running,
    start,
    stop,
    diagnostics,
    /**
     * microphone меняет активность или устройство микрофона.
     *
     * @args
     *   - enabled (boolean) — разрешает выполнение запроса или подключение при выполненных условиях доступа.
     *   - deviceId (string) — идентификатор выбранного пользователем устройства (необязательный параметр).
     *
     * @returns вычисленное значение: controller.current?.changeSource("microphone", enabled, deviceId).
     */
    microphone: (enabled: boolean, deviceId?: string) =>
      controller.current?.changeSource("microphone", enabled, deviceId),
    /**
     * camera меняет активность или устройство камеры.
     *
     * @args
     *   - enabled (boolean) — разрешает выполнение запроса или подключение при выполненных условиях доступа.
     *   - deviceId (string) — идентификатор выбранного пользователем устройства (необязательный параметр).
     *
     * @returns вычисленное значение: controller.current?.changeSource("camera", enabled, deviceId).
     */
    camera: (enabled: boolean, deviceId?: string) =>
      controller.current?.changeSource("camera", enabled, deviceId),
    /**
     * startScreen по действию пользователя запрашивает демонстрацию экрана и публикует разрешённые дорожки.
     *
     *
     * @returns вычисленное значение: controller.current?.startScreen().
     */
    startScreen: () => controller.current?.startScreen(),
    /**
     * stopScreen останавливает принадлежащие клиенту дорожки экрана и согласует снятие публикации.
     *
     *
     * @returns вычисленное значение: controller.current?.stopScreen().
     */
    stopScreen: () => controller.current?.stopScreen(),
  };
}
