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

afterEach(() => vi.restoreAllMocks());
it("subscribes without replacing the media handler and bounds reaction bubbles", async () => {
  vi.spyOn(api, "hands").mockResolvedValue({ status: "success", items: [] });
  const hand = vi.spyOn(api, "hand").mockResolvedValue({ status: "success" });
  const media = vi.fn();
  let receive: (event: RealtimeEvent) => void = () => {};
  const unsubscribe = vi.fn();
  const live = {
    state: { connectionId: "connection" },
    onEvent: { current: media },
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
  act(() => {
    for (let index = 0; index < 20; index++)
      receive({
        id: String(index),
        type: "reaction.created",
        data: { participantId: "bob", emoji: "👍" },
      } as RealtimeEvent);
  });
  expect(document.querySelectorAll(".reaction-bubble")).toHaveLength(6);
  expect(live.onEvent.current).toBe(media);
  act(() =>
    receive({
      id: "hand",
      type: "hand.raised",
      data: { participantId: "bob", raisedAt: "2026-10-01T10:00:00Z" },
    } as RealtimeEvent),
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "Опустить руку: Bob" }),
  );
  await waitFor(() => expect(hand).toHaveBeenCalledWith("room", "bob", false));
  view.unmount();
  expect(unsubscribe).toHaveBeenCalledTimes(1);
  client.clear();
});
