import { describe, expect, it } from "vitest";
import { onlineParticipants } from "./presence";
import type { Participant, PresenceParticipant } from "./types";

/** Создаёт полное сохранённое членство для независимой проверки списка присутствия.
 * @args id — идентификатор участника; changes — отличия от присоединившегося и допущенного участника.
 * @return Членство без физических подключений и признака присутствия.
 */
function member(id: string, changes: Partial<Participant> = {}): Participant {
  return {
    id,
    conferenceId: "room",
    userId: `user-${id}`,
    displayName: id,
    role: "participant",
    status: "joined",
    admissionState: "admitted",
    joinedAt: "2026-10-03T12:00:00Z",
    leftAt: null,
    createdAt: "2026-10-03T12:00:00Z",
    updatedAt: "2026-10-03T12:00:00Z",
    ...changes,
  };
}

/** Дополняет членство авторитетным серверным снимком физических подключений.
 * @args person — исходное членство; changes — изменённые поля присутствия или членства.
 * @return Участник с одним действующим подключением, если изменения не задают другое состояние.
 */
function present(
  person: Participant,
  changes: Partial<PresenceParticipant> = {},
): PresenceParticipant {
  return {
    ...person,
    online: true,
    connections: 1,
    connectionIds: [`connection-${person.id}`],
    ...changes,
  };
}

describe("Список присутствующих во встрече", () => {
  it("не считает сохранённое joined-членство подключением до получения снимка", () => {
    const person = member("owner", { role: "owner" });
    expect(onlineParticipants([person], undefined)).toEqual([]);
    expect(onlineParticipants([person], [])).toEqual([]);
  });

  it("скрывает отсутствующих в снимке и явно отключённых участников", () => {
    const connected = member("connected");
    const disconnected = member("disconnected");
    const unknown = member("unknown");
    const snapshot = [
      present(connected),
      present(disconnected, {
        online: false,
        connections: 0,
        connectionIds: [],
      }),
    ];
    expect(
      onlineParticipants([connected, disconnected, unknown], snapshot).map(
        (person) => person.id,
      ),
    ).toEqual(["connected"]);
  });

  it("использует серверный online, а не устаревшее число подключений", () => {
    const person = member("disconnected");
    expect(
      onlineParticipants(
        [person],
        [
          present(person, {
            online: false,
            connections: 2,
            connectionIds: ["old-one", "old-two"],
          }),
        ],
      ),
    ).toEqual([]);
  });

  it("показывает участника с несколькими вкладками один раз и не скрывает после закрытия одной", () => {
    const person = member("colleague");
    const snapshot = present(person, {
      connections: 2,
      connectionIds: ["one", "two"],
    });
    expect(onlineParticipants([person, person], [snapshot])).toEqual([
      snapshot,
    ]);
    const remaining = present(person, {
      connections: 1,
      connectionIds: ["two"],
    });
    expect(onlineParticipants([person], [remaining])).toEqual([remaining]);
  });

  it.each(["left", "waiting", "rejected", "kicked"] as const)(
    "не возвращает членство со статусом %s даже при online=true",
    (status) => {
      const person = member("colleague");
      expect(
        onlineParticipants([person], [present(person, { status })]),
      ).toEqual([]);
    },
  );

  it.each(["waiting", "rejected", "kicked"] as const)(
    "не возвращает недопущенное joined-членство с admissionState=%s",
    (admissionState) => {
      const person = member("colleague");
      expect(
        onlineParticipants([person], [present(person, { admissionState })]),
      ).toEqual([]);
    },
  );

  it("сохраняет совместимость с подключённым joined-членством без admissionState", () => {
    const person = member("legacy", { admissionState: undefined });
    const snapshot = present(person);
    expect(onlineParticipants([person], [snapshot])).toEqual([snapshot]);
  });

  it("добавляет подключённых участников из снимка, ещё не загруженных страницами API", () => {
    const firstPage = member("first-page");
    const laterPage = member("later-page");
    const snapshot = [present(laterPage), present(firstPage)];
    expect(
      onlineParticipants([firstPage], snapshot).map((person) => person.id),
    ).toEqual(["first-page", "later-page"]);
  });

  it("использует свежие имя, роль, ограничения и допуск из снимка без изменения входных данных", () => {
    const stored = Object.freeze(member("colleague"));
    const snapshot = Object.freeze(
      present(stored, {
        displayName: "Новое имя",
        role: "co_host",
        microphoneBlocked: true,
        mediaPolicyVersion: 4,
      }),
    );
    const participants = Object.freeze([stored]);
    const presence = Object.freeze([snapshot]);
    const result = onlineParticipants(participants, presence);
    expect(result).toEqual([snapshot]);
    expect(result[0]).toMatchObject({
      displayName: "Новое имя",
      role: "co_host",
      microphoneBlocked: true,
      mediaPolicyVersion: 4,
    });
    expect(stored.displayName).toBe("colleague");
    expect(stored.role).toBe("participant");
    expect(stored.microphoneBlocked).toBeUndefined();
    expect(participants).toEqual([stored]);
    expect(presence).toEqual([snapshot]);
  });

  it("после переподключения не возвращает прежний roster вместо нового снимка", () => {
    const previous = member("previous");
    const restored = member("restored");
    expect(onlineParticipants([previous], [present(previous)])).toHaveLength(1);
    expect(onlineParticipants([previous], undefined)).toEqual([]);
    expect(
      onlineParticipants([previous], [present(restored)]).map(
        (person) => person.id,
      ),
    ).toEqual(["restored"]);
  });
});
