import { useState } from "react";
import {
  Mic,
  MicOff,
  Search,
  ShieldCheck,
  Video,
  VideoOff,
} from "lucide-react";
import type {
  ModerationAction,
  Participant,
  PresenceParticipant,
} from "../types";
import { isAdmitted } from "../collaboration";
import { initials } from "../utils";
import { onlineParticipants } from "../presence";
import { Button, ErrorNotice, Loading } from "./ui";

const roles = {
  owner: "Организатор",
  co_host: "Соорганизатор",
  participant: "Участник",
  guest: "Гость",
};
const statuses = {
  joined: "Во встрече",
  left: "Вышел",
  waiting: "Ожидает допуска",
  rejected: "Запрос отклонён",
  kicked: "Исключён",
};

/** Показывает настоящий состав комнаты и только разрешённые текущей ролью действия.
 * @args participants — загруженные членства; membership — текущий пользователь; presence — серверное WS-присутствие;
 * loading, error — состояние списка; busy — выполняемая модерация; onModerate — серверная команда, без оптимистического изменения прав.
 * @return Панель поиска, ролей и доступной модерации участников.
 */
export function ParticipantsPanel({
  participants,
  membership,
  presence,
  loading,
  error,
  busy,
  onModerate,
}: {
  participants: Participant[];
  membership: Participant;
  presence?: PresenceParticipant[];
  loading: boolean;
  error?: unknown;
  busy: boolean;
  onModerate: (participantId: string, action: ModerationAction) => void;
}) {
  const [search, setSearch] = useState("");
  const visible = onlineParticipants(participants, presence).filter((person) =>
    person.displayName
      .toLocaleLowerCase("ru")
      .includes(search.toLocaleLowerCase("ru")),
  );
  return (
    <section className="room-participants" aria-label="Участники встречи">
      <label className="room-participant-search">
        <Search size={17} aria-hidden="true" />
        <input
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder="Найти участника"
          aria-label="Поиск участника"
        />
      </label>
      <ErrorNotice error={error} />
      {loading ? (
        <Loading />
      ) : (
        <ul className="room-participant-list">
          {visible.map((person) => {
            const canModerate =
              membership.status === "joined" &&
              person.id !== membership.id &&
              person.role !== "owner" &&
              isAdmitted(person) &&
              person.status !== "kicked" &&
              (membership.role === "owner" ||
                (membership.role === "co_host" &&
                  person.role === "participant"));
            return (
              <li key={person.id} className="room-participant-row">
                <div className="room-participant-identity">
                  <span className="avatar avatar-small">
                    {initials(person.displayName)}
                  </span>
                  <div>
                    <strong>
                      {person.displayName}
                      {person.id === membership.id ? " (вы)" : ""}
                    </strong>
                    <span>
                      {roles[person.role]} · {statuses[person.status]}
                    </span>
                  </div>
                  <span className="room-participant-media">
                    {person.microphoneEnabled && !person.microphoneBlocked ? (
                      <Mic size={15} aria-label="Микрофон включён" />
                    ) : (
                      <MicOff size={15} aria-label="Микрофон выключен" />
                    )}
                    {person.cameraEnabled && !person.cameraBlocked ? (
                      <Video size={15} aria-label="Камера включена" />
                    ) : (
                      <VideoOff size={15} aria-label="Камера выключена" />
                    )}
                  </span>
                </div>
                {canModerate && (
                  <div
                    className="room-moderation"
                    aria-label={`Управление: ${person.displayName}`}
                  >
                    {(
                      [
                        ["mute", "микрофон", !!person.microphoneBlocked],
                        ["camera", "видео", !!person.cameraBlocked],
                        ["screen", "экран", !!person.screenBlocked],
                      ] as const
                    )
                      .filter(
                        ([action]) =>
                          action !== "camera" || membership.role === "owner",
                      )
                      .map(([action, label, blocked]) => (
                        <Button
                          key={action}
                          variant="secondary"
                          disabled={busy}
                          onClick={() =>
                            onModerate(person.id, { action, blocked: !blocked })
                          }
                        >
                          {blocked ? "Разрешить" : "Отключить"} {label}
                        </Button>
                      ))}
                    {membership.role === "owner" && (
                      <Button
                        variant="outline"
                        disabled={busy}
                        onClick={() =>
                          onModerate(person.id, {
                            action: "role",
                            role:
                              person.role === "co_host"
                                ? "participant"
                                : "co_host",
                          })
                        }
                      >
                        <ShieldCheck size={14} />
                        {person.role === "co_host"
                          ? "Убрать соорганизатора"
                          : "Назначить соорганизатором"}
                      </Button>
                    )}
                    <Button
                      variant="danger"
                      disabled={busy}
                      onClick={() => {
                        if (
                          window.confirm(
                            `Исключить ${person.displayName}? Повторное присоединение будет запрещено.`,
                          )
                        )
                          onModerate(person.id, { action: "kick" });
                      }}
                    >
                      Исключить
                    </Button>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
      {!loading && visible.length === 0 && (
        <p className="field-hint">Участники не найдены.</p>
      )}
    </section>
  );
}
