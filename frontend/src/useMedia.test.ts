import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "./api";
import { emptyMediaView, type MediaView } from "./media";
import { useMedia } from "./useMedia";
import type { useRealtime } from "./realtime";

const clients = vi.hoisted(
  /**
   * Обработчик vi.hoisted выполняет переданный шаг вызова vi.hoisted в проверках клиентского поведения.
   *
   *
   * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
   */
  () =>
    [] as {
      emit: /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
       *
       * @parameters:
       *   - state (MediaView) — новое состояние источников медиа.
       *
       * @returns void — значение не возвращается; функция выполняет описанные действия.
       */ (state: MediaView) => void;
      start: ReturnType<typeof vi.fn>;
    }[],
);
vi.mock(
  "./media",
  /**
   * Обработчик vi.mock выполняет переданный шаг вызова vi.mock в проверках клиентского поведения.
   *
   * @parameters:
   *   - original — входное значение original текущего шага обработки.
   *
   * @returns Promise, который после завершения операции возвращает: объект с данными, собранными в текущей операции.
   */ async (original) => {
    const actual = await original<typeof import("./media")>();
    return {
      ...actual,
      ConferenceMediaClient: class {
        start = vi.fn(
          /**
           * Обработчик vi.fn выполняет переданный шаг вызова vi.fn в проверках клиентского поведения.
           *
           *
           * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
           */ async () => {},
        );
        /**
         * constructor function Object() { [native code] }.
         *
         * @parameters:
         *   - _send (unknown) — входное значение _send текущего шага обработки.
         *   - emit ((state: MediaView) => void) — входное значение emit текущего шага обработки.
         *
         * @returns инициализированный экземпляр текущего класса.
         */
        constructor(
          _send: unknown,
          readonly emit: /**
           * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
           *
           * @parameters:
           *   - state (MediaView) — новое состояние источников медиа.
           *
           * @returns void — значение не возвращается; функция выполняет описанные действия.
           */ (state: MediaView) => void,
        ) {
          clients.push(this);
        }
        /**
         * stop закрывает WebRTC, останавливает принадлежащие клиенту дорожки и очищает таймеры и состояние.
         *
         *
         * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
         */
        stop() {
          this.emit(actual.emptyMediaView());
        }
        /**
         * setPolicy применяет текущие ограничения модерации к локальным устройствам и экрану.
         *
         *
         * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
         */
        setPolicy() {}
        /**
         * handle ставит входящее медиа-событие в последовательную обработку, сохраняя порядок SDP и ICE.
         *
         *
         * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
         */
        handle() {}
      },
    };
  },
);

/**
 * live создаёт действующее тестовое состояние медиа.
 *
 * @parameters:
 *   - connectionId (string) — идентификатор физического подключения.
 *
 * @returns ReturnType<typeof useRealtime> — вычисленные данные текущего шага, которые использует вызывающая операция.
 */
function live(connectionId: string): ReturnType<typeof useRealtime> {
  return {
    state: { connectionId },
    onEvent: { current: null },
    send: vi.fn(),
  } as unknown as ReturnType<typeof useRealtime>;
}
/**
 * tick ожидает завершения отложенного шага тестовой операции.
 *
 *
 * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
 */
const tick = () =>
  act(
    /**
     * Обработчик act выполняет переданный шаг вызова act в проверках клиентского поведения.
     *
     *
     * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */ async () => {
      await vi.advanceTimersByTimeAsync(101);
    },
  );
beforeEach(
  /**
   * Обработчик beforeEach выполняет переданный шаг вызова beforeEach в проверках клиентского поведения.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    vi.useFakeTimers();
    clients.length = 0;
    vi.spyOn(api, "setMediaState").mockResolvedValue(
      {} as Awaited<ReturnType<typeof api.setMediaState>>,
    );
  },
);
afterEach(
  /**
   * Обработчик afterEach выполняет переданный шаг вызова afterEach в проверках клиентского поведения.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    cleanup();
    vi.useRealTimers();
    vi.restoreAllMocks();
  },
);

describe("per-connection media snapshots", /**
 * Проверка: per-connection media snapshots выполняет тестовый сценарий «per-connection media snapshots» и проверяет ожидаемые результаты.
 *
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ () => {
  it("increments sequence across capture stop/restart on the same connection", /**
   * Проверка: increments sequence across capture stop/restart on the same connection выполняет тестовый сценарий «increments sequence across capture stop/restart on the same connection» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const { result } = renderHook(
      /**
       * Обработчик renderHook выполняет переданный шаг вызова renderHook в проверках клиентского поведения.
       *
       *
       * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
       */ () => useMedia(live("tab-a"), "room", {}),
    );
    await tick();
    act(
      /**
       * Обработчик act выполняет переданный шаг вызова act в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: result.current.start(false).
       */ () => result.current.start(false),
    );
    expect(clients[0].start).toHaveBeenCalledWith(false);
    act(
      /**
       * Обработчик act выполняет переданный шаг вызова act в проверках клиентского поведения.
       *
       *
       * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
       */ () =>
        clients[0].emit({
          ...emptyMediaView(),
          active: true,
          microphoneEnabled: true,
        }),
    );
    await tick();
    act(
      /**
       * Обработчик act выполняет переданный шаг вызова act в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: result.current.stop().
       */ () => result.current.stop(),
    );
    await tick();
    act(
      /**
       * Обработчик act выполняет переданный шаг вызова act в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: result.current.start().
       */ () => result.current.start(),
    );
    act(
      /**
       * Обработчик act выполняет переданный шаг вызова act в проверках клиентского поведения.
       *
       *
       * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
       */ () =>
        clients[1].emit({
          ...emptyMediaView(),
          active: true,
          cameraEnabled: true,
        }),
    );
    await tick();
    const states = vi.mocked(api.setMediaState).mock.calls.map(
      /**
       * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
       *
       * @parameters:
       *   - [, state] — элементы записи набора, извлечённые по указанным позициям.
       *
       * @returns преобразованное значение текущего элемента для результирующего набора.
       */ ([, state]) => state,
    );
    expect(
      states.map(
        /**
         * Обработчик states.map преобразует один элемент набора в представление или данные следующего шага.
         *
         * @parameters:
         *   - state — новое состояние источников медиа.
         *
         * @returns преобразованное значение текущего элемента для результирующего набора.
         */ (state) => state.connectionId,
      ),
    ).toEqual(["tab-a", "tab-a", "tab-a", "tab-a"]);
    expect(
      states.map(
        /**
         * Обработчик states.map преобразует один элемент набора в представление или данные следующего шага.
         *
         * @parameters:
         *   - state — новое состояние источников медиа.
         *
         * @returns преобразованное значение текущего элемента для результирующего набора.
         */ (state) => state.sequence,
      ),
    ).toEqual([1, 2, 3, 4]);
    expect(states[2].microphoneEnabled).toBe(false);
    expect(states[3].cameraEnabled).toBe(true);
  });

  it("never relabels an old snapshot with a reconnected tab's identity", /**
   * Проверка: never relabels an old snapshot with a reconnected tab's identity выполняет тестовый сценарий «never relabels an old snapshot with a reconnected tab's identity» и проверяет ожидаемые результаты.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const { result, rerender } = renderHook(
      /**
       * Обработчик renderHook выполняет переданный шаг вызова renderHook в проверках клиентского поведения.
       *
       * @parameters:
       *   - объект параметров: id — идентификатор ресурса или конференции данного запроса.
       *
       * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
       */
      ({ id }) => useMedia(live(id), "room", {}),
      {
        initialProps: { id: "old-tab" },
      },
    );
    act(
      /**
       * Обработчик act выполняет переданный шаг вызова act в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: result.current.start().
       */ () => result.current.start(),
    );
    act(
      /**
       * Обработчик act выполняет переданный шаг вызова act в проверках клиентского поведения.
       *
       *
       * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
       */ () =>
        clients[0].emit({
          ...emptyMediaView(),
          active: true,
          screenSharing: true,
        }),
    );
    await tick();
    rerender({ id: "new-tab" });
    await tick();
    const calls = vi.mocked(api.setMediaState).mock.calls;
    expect(calls[0][1]).toMatchObject({
      connectionId: "old-tab",
      screenSharing: true,
    });
    expect(calls.at(-1)?.[1]).toMatchObject({
      connectionId: "new-tab",
      screenSharing: false,
    });
    act(
      /**
       * Обработчик act выполняет переданный шаг вызова act в проверках клиентского поведения.
       *
       *
       * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
       */ () =>
        clients[0].emit({
          ...emptyMediaView(),
          active: true,
          screenSharing: true,
        }),
    );
    await tick();
    expect(api.setMediaState).toHaveBeenCalledTimes(2);
  });
});
