import { afterEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { CaptionsPanel } from "./CaptionsPanel";
import { api } from "../api";
import type { Caption, RealtimeEvent } from "../types";
import type { useRealtime } from "../realtime";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
});

/** show монтирует субтитры с разрешённым состоянием и тестовым потоком событий.
 * @args manage — полномочия организатора; finals — восстановленные SQL реплики.
 * @return Функция доставки проверяемого WS события.
 */
function show(manage = false, finals: Caption[] = []) {
  vi.spyOn(api, "captions").mockResolvedValue({
    status: "success",
    item: {
      conferenceId: "room",
      sessionId: "session",
      generation: 1,
      enabled: true,
      available: true,
      canManage: manage,
      language: "ru",
      status: "active",
    },
  });
  vi.spyOn(api, "captionFinals").mockResolvedValue({
    items: finals,
    nextCursor: 10,
    hasMore: false,
  });
  const change = vi.spyOn(api, "setCaptions").mockResolvedValue({
    status: "success",
    item: {
      conferenceId: "room",
      sessionId: "session",
      generation: 2,
      enabled: false,
      available: true,
      canManage: manage,
      language: "ru",
      status: "off",
    },
  });
  let callback: ((event: RealtimeEvent) => void) | undefined;
  const live = {
    subscribe: (fn: (event: RealtimeEvent) => void) => {
      callback = fn;
      return () => {
        callback = undefined;
      };
    },
  } as ReturnType<typeof useRealtime>;
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  clients.push(client);
  const view = render(
    <QueryClientProvider client={client}>
      <CaptionsPanel conferenceId="room" active live={live} />
    </QueryClientProvider>,
  );
  return {
    change,
    ...view,
    emit: (data: Caption) =>
      act(() =>
        callback?.({
          type: data.final ? "caption.final" : "caption.partial",
          data,
        } as RealtimeEvent),
      ),
  };
}
const row = {
  id: "line",
  conferenceId: "room",
  sessionId: "session",
  generation: 1,
  sequence: 1,
  revision: 1,
  participantId: "p",
  speaker: "Алиса",
  startMs: 0,
  endMs: 500,
  language: "ru",
  text: "черновик",
  final: false,
  cursor: 0,
  trackInstanceId: "track",
} as Caption;

describe("Живые субтитры", () => {
  it("объявляет скринридеру только принятые финальные реплики", async () => {
    const view = show();
    await screen.findByText("Распознавание включено");
    const announcement = screen.getByTestId("caption-announcement");
    view.emit(row);
    expect(announcement).toBeEmptyDOMElement();
    view.emit({
      ...row,
      text: "Готово",
      sequence: 2,
      revision: 2,
      final: true,
    });
    expect(announcement).toHaveTextContent("Алиса: Готово");
    view.emit({
      ...row,
      text: "Устаревший финал",
      sequence: 1,
      revision: 5,
      final: true,
    });
    view.emit({
      ...row,
      text: "другой черновик",
      sequence: 3,
      revision: 3,
      final: false,
    });
    expect(announcement).toHaveTextContent("Алиса: Готово");
    fireEvent.click(screen.getByRole("checkbox"));
    expect(announcement).toBeEmptyDOMElement();
    view.emit({
      ...row,
      id: "next",
      text: "Скрытая реплика",
      sequence: 4,
      revision: 4,
      final: true,
    });
    fireEvent.click(screen.getByRole("checkbox"));
    expect(announcement).toBeEmptyDOMElement();
  });
  it("заменяет текст по версии и скрывает его локально без остановки распознавания", async () => {
    const view = show();
    await screen.findByText("Распознавание включено");
    view.emit(row);
    expect(screen.getByText("черновик")).toBeVisible();
    view.emit({
      ...row,
      text: "готовая реплика",
      sequence: 2,
      revision: 2,
      final: true,
    });
    view.emit(row);
    expect(screen.queryByText("черновик")).not.toBeInTheDocument();
    expect(screen.getByText("готовая реплика")).toBeVisible();
    fireEvent.click(screen.getByRole("checkbox"));
    expect(screen.queryByText("готовая реплика")).not.toBeInTheDocument();
    expect(view.change).not.toHaveBeenCalled();
    expect(screen.getByText("Распознавание включено")).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "Отключить распознавание" }),
    ).not.toBeInTheDocument();
  });
  it("восстанавливает final без WS и выводит недоверенный текст безопасно", async () => {
    const final = {
      ...row,
      final: true,
      revision: 2,
      sequence: 2,
      text: "<img src=x onerror=alert(1)>",
    };
    const view = show(true, [final]);
    expect(await screen.findByText(final.text)).toBeVisible();
    expect(document.querySelector("img[src=x]")).toBeNull();
    view.emit({ ...row, revision: 3, sequence: 3 });
    expect(screen.queryByText("черновик")).toBeNull();
    fireEvent.click(
      screen.getByRole("button", { name: "Отключить распознавание" }),
    );
    await waitFor(() =>
      expect(view.change).toHaveBeenCalledWith("room", false, "ru"),
    );
    const signal = vi.mocked(api.captionFinals).mock.calls[0][2];
    view.unmount();
    expect(signal?.aborted).toBe(true);
  });
});
