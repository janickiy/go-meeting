import type { Participant, PresenceParticipant } from "./types";

/** Собирает состав живой комнаты по подтверждённому сервером присутствию.
 * Членство и история сами по себе не подтверждают подключение. Последний снимок
 * присутствия обновляет роли и настройки медиа, включая ещё не загруженные страницы списка.
 * @args participants — сохранённые членства в порядке API; presence — последний снимок WebSocket или отсутствие связи.
 * @return Уникальные допущенные участники онлайн; исходные массивы не изменяются.
 */
export function onlineParticipants(
  participants: readonly Participant[],
  presence: readonly PresenceParticipant[] | undefined,
): Participant[] {
  if (!presence) return [];
  const snapshot = new Map(presence.map((person) => [person.id, person]));
  const merged = new Map(participants.map((person) => [person.id, person]));
  for (const person of snapshot.values()) {
    merged.set(person.id, { ...merged.get(person.id), ...person });
  }
  return [...merged.values()].filter((person) => {
    const current = snapshot.get(person.id);
    return (
      current?.online === true &&
      person.status === "joined" &&
      (!person.admissionState || person.admissionState === "admitted")
    );
  });
}
