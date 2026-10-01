import { describe, expect, it } from "vitest";
import { parseRealtime, websocketURL } from "./realtime";
describe("Realtime protocol", /**
 * Проверка: Realtime protocol выполняет тестовый сценарий «Realtime protocol» и проверяет ожидаемые результаты.
 *
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ () => {
  it("uses a short ticket and wss for HTTPS, never a JWT query parameter", /**
   * Проверка: uses a short ticket and wss for HTTPS, never a JWT query parameter выполняет тестовый сценарий «uses a short ticket and wss for HTTPS, never a JWT query parameter» и проверяет ожидаемые результаты.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    const url = new URL(
      websocketURL(
        "conference-id",
        "single-use-ticket",
        "https://meet.example:18482",
      ),
    );
    expect(url.protocol).toBe("wss:");
    expect(url.pathname).toBe("/api/v1/conferences/conference-id/ws");
    expect([...url.searchParams.keys()]).toEqual(["ticket"]);
  });
  it("rejects foreign conference/version/malformed data", /**
   * Проверка: rejects foreign conference/version/malformed data выполняет тестовый сценарий «rejects foreign conference/version/malformed data» и проверяет ожидаемые результаты.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    const event = {
      version: 1,
      id: "id",
      type: "conference.state",
      conferenceId: "room",
      timestamp: "now",
      data: {},
    };
    expect(parseRealtime(JSON.stringify(event), "room")?.type).toBe(
      "conference.state",
    );
    expect(parseRealtime(JSON.stringify(event), "other")).toBeNull();
    expect(
      parseRealtime(JSON.stringify({ ...event, version: 2 }), "room"),
    ).toBeNull();
    expect(parseRealtime("not JSON", "room")).toBeNull();
  });
});
