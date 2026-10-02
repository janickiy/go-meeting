import { describe, expect, it } from "vitest";
import {
  notificationLink,
  recordingTime,
  searchResultLink,
} from "./intelligence";
import { notificationLabel } from "./components/NotificationBell";
import type { Notification, SearchResult } from "./types";

describe("ссылки на материалы", () => {
  it("форматирует время и отбрасывает недопустимые значения", () => {
    expect(recordingTime(3_661_999)).toBe("1:01:01");
    expect(recordingTime(-10)).toBe("00:00");
    expect(recordingTime(Infinity)).toBe("00:00");
  });
  it("кодирует идентификаторы и сохраняет миллисекунды", () => {
    const link = searchResultLink({
      type: "transcript",
      conferenceId: "room/a",
      recordingId: "record&b",
      segmentId: "segment/1",
      startMs: 42_500,
    } as SearchResult);
    expect(link).toContain("/conferences/room%2Fa?");
    const params = new URLSearchParams(link.split("?")[1]);
    expect(params.get("recording")).toBe("record&b");
    expect(params.get("t")).toBe("42500");
    expect(params.get("segment")).toBe("segment/1");
  });
  it("не переносит бесконечное смещение и выбирает вкладку итогов", () => {
    const link = searchResultLink({
      type: "summary",
      conferenceId: "room",
      recordingId: "record",
      startMs: Infinity,
    } as SearchResult);
    expect(link).toContain("tab=summary");
    expect(link).not.toContain("&t=");
  });
  it("уведомление открывает конкретную запись и показывает понятную подпись", () => {
    const notification = {
      type: "summary.ready",
      payload: { conferenceId: "room", recordingId: "record" },
    } as Notification;
    expect(notificationLabel(notification)).toBe("Итоги встречи готовы");
    expect(notificationLink(notification)).toContain(
      "recording=record&tab=summary",
    );
    expect(
      notificationLabel({ ...notification, type: "transcript.ready" }),
    ).toBe("Расшифровка встречи готова");
  });
  it("ошибка обработки не называется готовностью и сохраняет нужную вкладку", () => {
    const notification = {
      type: "processing.failed",
      payload: {
        conferenceId: "room",
        recordingId: "record",
        summaryId: "summary",
      },
    } as Notification;
    expect(notificationLabel(notification)).toBe(
      "Обработка материалов не завершена",
    );
    expect(notificationLink(notification)).toContain("tab=summary");
    expect(notificationLabel({ ...notification, type: "summary.failed" })).toBe(
      "Не удалось подготовить итоги встречи",
    );
    expect(
      notificationLabel({ ...notification, type: "transcript.failed" }),
    ).toBe("Не удалось подготовить расшифровку");
  });
});
