import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "../api";
import type { Participant, RealtimeEvent } from "../types";
import type { useRealtime } from "../realtime";
import { HandReactionsPanel } from "./HandReactionsPanel";

afterEach(
  /**
   * Обработчик afterEach выполняет переданный шаг вызова afterEach в проверках клиентского поведения.
   *
   *
   * @returns вычисленное значение: vi.restoreAllMocks().
   */ () => vi.restoreAllMocks(),
);
it("subscribes without replacing the media handler and bounds reaction bubbles", /**
 * Проверка: subscribes without replacing the media handler and bounds reaction bubbles выполняет тестовый сценарий «subscribes without replacing the media handler and bounds reaction bubbles» и проверяет ожидаемые результаты.
 *
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ async () => {
  vi.spyOn(api, "hands").mockResolvedValue({ status: "success", items: [] });
  const hand = vi.spyOn(api, "hand").mockResolvedValue({ status: "success" });
  const media = vi.fn();
  /**
   * receive доставляет подготовленное событие тестовому клиенту.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  let receive: /**
   * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
   *
   * @args
   *   - event (RealtimeEvent) — проверенный конверт события комнаты.
   *
   * @returns void — значение не возвращается; функция выполняет описанные действия.
   */ (event: RealtimeEvent) => void = () => {};
  const unsubscribe = vi.fn();
  const live = {
    state: { connectionId: "connection" },
    onEvent: { current: media },
    /**
     * subscribe подключает обработчик состояния или событий и возвращает снятие подписки.
     *
     * @args
     *   - callback (typeof receive) — обработчик события или изменения наблюдаемого состояния.
     *
     * @returns вычисленное значение: unsubscribe.
     */
    subscribe: (callback: typeof receive) => {
      receive = callback;
      return unsubscribe;
    },
  } as unknown as ReturnType<typeof useRealtime>;
  const member = { id: "self", role: "owner", status: "joined" } as Participant;
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <HandReactionsPanel
        conferenceId="room"
        membership={member}
        participants={[{ id: "bob", displayName: "Bob" } as Participant]}
        live={live}
      />
    </QueryClientProvider>,
  );
  await screen.findByRole("button", { name: "Поднять руку" });
  act(
    /**
     * Обработчик act выполняет переданный шаг вызова act в проверках клиентского поведения.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */ () => {
      for (let index = 0; index < 20; index++)
        receive({
          id: String(index),
          type: "reaction.created",
          data: { participantId: "bob", emoji: "👍" },
        } as RealtimeEvent);
    },
  );
  expect(document.querySelectorAll(".reaction-bubble")).toHaveLength(6);
  expect(live.onEvent.current).toBe(media);
  act(
    /**
     * Обработчик act выполняет переданный шаг вызова act в проверках клиентского поведения.
     *
     *
     * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
     */ () =>
      receive({
        id: "hand",
        type: "hand.raised",
        data: { participantId: "bob", raisedAt: "2026-10-01T10:00:00Z" },
      } as RealtimeEvent),
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "Опустить руку: Bob" }),
  );
  await waitFor(
    /**
     * Обработчик waitFor выполняет переданный шаг вызова waitFor в проверках клиентского поведения.
     *
     *
     * @returns вычисленное значение: expect(hand).toHaveBeenCalledWith("room", "bob", false).
     */ () => expect(hand).toHaveBeenCalledWith("room", "bob", false),
  );
  view.unmount();
  expect(unsubscribe).toHaveBeenCalledTimes(1);
  client.clear();
});
