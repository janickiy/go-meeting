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
import { ReactionsPanel } from "./ReactionsPanel";

afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
});

/** panel создаёт изолированную панель и контролируемую подписку на события встречи.
 * @args connected — доступно ли realtime-соединение.
 * @return интерфейс панели, доставка события, снятие подписки и обработчик медиа.
 */
function panel(connected = true) {
  let receive: (event: RealtimeEvent) => void = () => {};
  const unsubscribe = vi.fn();
  const media = vi.fn();
  const live = {
    state: connected ? { connectionId: "connection" } : null,
    onEvent: { current: media },
    subscribe: (callback: typeof receive) => {
      receive = callback;
      return unsubscribe;
    },
  } as unknown as ReturnType<typeof useRealtime>;
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <ReactionsPanel
        conferenceId="room"
        participants={[{ id: "bob", displayName: "Боб" } as Participant]}
        live={live}
      />
    </QueryClientProvider>,
  );
  return {
    view,
    live,
    media,
    unsubscribe,
    receive: (event: Partial<RealtimeEvent>) => receive(event as RealtimeEvent),
    close: () => {
      view.unmount();
      client.clear();
    },
  };
}

it("ограничивает реакции, не заменяя обработчик медиа и не показывая удалённую функцию", () => {
  const subject = panel();
  act(() => {
    for (let i = 0; i < 20; i++)
      subject.receive({
        id: String(i),
        type: "reaction.created",
        data: { participantId: "bob", emoji: "👍" },
      });
  });
  expect(document.querySelectorAll(".reaction-bubble")).toHaveLength(6);
  expect(subject.live.onEvent.current).toBe(subject.media);
  expect(screen.getAllByRole("button")).toHaveLength(4);
  expect(screen.queryByRole("button", { name: "Поднять руку" })).toBeNull();
  act(() =>
    subject.receive({
      id: "old-client",
      type: "hand.raised",
      data: { participantId: "bob" },
    }),
  );
  expect(screen.queryByRole("list", { name: "Поднятые руки" })).toBeNull();
  expect(document.querySelectorAll(".reaction-bubble")).toHaveLength(6);
  subject.close();
  expect(subject.unsubscribe).toHaveBeenCalledTimes(1);
});

it("удаляет повторные и недопустимые реакции и очищает их по таймеру", () => {
  vi.useFakeTimers();
  const subject = panel();
  act(() => {
    const event = {
      id: "one",
      type: "reaction.created",
      data: { participantId: "bob", emoji: "👍" },
    };
    subject.receive(event);
    subject.receive(event);
    subject.receive({
      id: "invalid",
      type: "reaction.created",
      data: { participantId: "bob", emoji: "💥" },
    });
  });
  expect(document.querySelectorAll(".reaction-bubble")).toHaveLength(1);
  act(() => vi.advanceTimersByTime(3500));
  expect(document.querySelectorAll(".reaction-bubble")).toHaveLength(0);
  subject.close();
  expect(vi.getTimerCount()).toBe(0);
});

it("отправляет реакцию с защитой от повторных кликов", async () => {
  const reaction = vi
    .spyOn(api, "reaction")
    .mockResolvedValue({ status: "success" });
  const subject = panel();
  fireEvent.click(screen.getByRole("button", { name: "Реакция 👍" }));
  await waitFor(() =>
    expect(reaction).toHaveBeenCalledExactlyOnceWith("room", "👍"),
  );
  expect(screen.getByRole("button", { name: "Реакция 👍" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "Реакция 👍" }));
  expect(reaction).toHaveBeenCalledTimes(1);
  subject.close();
});

it("запрещает отправку реакций без соединения", () => {
  const reaction = vi.spyOn(api, "reaction");
  const subject = panel(false);
  for (const button of screen.getAllByRole("button"))
    expect(button).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "Реакция 👍" }));
  expect(reaction).not.toHaveBeenCalled();
  subject.close();
});
