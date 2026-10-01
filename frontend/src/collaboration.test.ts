import { describe, expect, it } from "vitest";
import {
  isAdmitted,
  localDayEnd,
  localSchedule,
  mergeChatPages,
  toLocalInput,
} from "./collaboration";
import { takeNotificationFrames } from "./notifications";
import { validateAttachment } from "./components/AttachmentUploader";
import { notificationLabel } from "./components/NotificationBell";
import type { ChatMessage, Notification, Participant } from "./types";

describe("stage five contracts", /**
 * Проверка: stage five contracts выполняет тестовый сценарий «stage five contracts» и проверяет ожидаемые результаты.
 *
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ () => {
  it("does not admit waiting/rejected/kicked or missing membership", /**
   * Проверка: does not admit waiting/rejected/kicked or missing membership выполняет тестовый сценарий «does not admit waiting/rejected/kicked or missing membership» и проверяет ожидаемые результаты.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    expect(isAdmitted(null)).toBe(false);
    for (const admissionState of ["waiting", "rejected", "kicked"])
      expect(
        isAdmitted({ status: "joined", admissionState } as Participant),
      ).toBe(false);
    expect(
      isAdmitted({ status: "left", admissionState: "admitted" } as Participant),
    ).toBe(true);
    expect(isAdmitted({ status: "waiting" } as Participant)).toBe(false);
  });
  it("merges overlapping chat pages by version and exact 64-bit sequence", /**
   * Проверка: merges overlapping chat pages by version and exact 64-bit sequence выполняет тестовый сценарий «merges overlapping chat pages by version and exact 64-bit sequence» и проверяет ожидаемые результаты.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    const first = {
      id: "a",
      sequence: "9007199254740992",
      version: 1,
      text: "old",
    } as ChatMessage;
    const second = {
      id: "b",
      sequence: "9007199254740993",
      version: 1,
    } as ChatMessage;
    expect(
      mergeChatPages([
        { items: [second, { ...first, version: 2, text: "edited" }] },
        { items: [first] },
      ]),
    ).toEqual([{ ...first, version: 2, text: "edited" }, second]);
  });
  it("serializes local schedules to UTC and rejects normalized invalid dates", /**
   * Проверка: serializes local schedules to UTC and rejects normalized invalid dates выполняет тестовый сценарий «serializes local schedules to UTC and rejects normalized invalid dates» и проверяет ожидаемые результаты.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    const value = "2026-11-12T14:35";
    expect(toLocalInput(localSchedule(value)!)).toBe(value);
    expect(localSchedule("2026-02-31T12:00")).toBeNull();
    expect(localSchedule("2026-11-12")).toBeNull();
    expect(localSchedule("garbage")).toBeNull();
    const dayEnd = new Date(localDayEnd("2026-11-12")!);
    expect([
      dayEnd.getHours(),
      dayEnd.getMinutes(),
      dayEnd.getSeconds(),
      dayEnd.getMilliseconds(),
    ]).toEqual([23, 59, 59, 999]);
    expect(localDayEnd("2026-02-31")).toBeNull();
  });
  it("parses split authenticated SSE frames and ignores malformed/non-v1 data", /**
   * Проверка: parses split authenticated SSE frames and ignores malformed/non-v1 data выполняет тестовый сценарий «parses split authenticated SSE frames and ignores malformed/non-v1 data» и проверяет ожидаемые результаты.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    const event = {
      version: 1,
      id: "event",
      type: "notification.created",
      data: { notification: { payload: { conferenceId: "room" } } },
    };
    const input = `: heartbeat\r\n\r\ndata: ${JSON.stringify(event)}\r\n\r\ndata: {"ver`;
    const parsed = takeNotificationFrames(input);
    expect(parsed.events).toEqual([event]);
    expect(parsed.rest).toBe('data: {"ver');
    expect(
      takeNotificationFrames(parsed.rest + 'sion":2}\n\ndata: invalid\n\n')
        .events,
    ).toEqual([]);
  });
  it("rejects active/empty/oversized files before uploading", /**
   * Проверка: rejects active/empty/oversized files before uploading выполняет тестовый сценарий «rejects active/empty/oversized files before uploading» и проверяет ожидаемые результаты.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    expect(validateAttachment({ name: "report.txt", size: 1024 })).toBeNull();
    expect(validateAttachment({ name: "attack.svg", size: 100 })).toMatch(
      /JPG/,
    );
    expect(validateAttachment({ name: "empty.txt", size: 0 })).toMatch(
      /Размер/,
    );
    expect(
      validateAttachment({ name: "huge.pdf", size: 10 * 1024 * 1024 + 1 }),
    ).toMatch(/Размер/);
  });
  it("distinguishes admission rejection from later removal in notifications", /**
   * Проверка: distinguishes admission rejection from later removal in notifications выполняет тестовый сценарий «distinguishes admission rejection from later removal in notifications» и проверяет ожидаемые результаты.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    expect(
      notificationLabel({
        type: "admission.decided",
        payload: { admissionState: "kicked" },
      } as Notification),
    ).toBe("Вы исключены из встречи");
  });
});
