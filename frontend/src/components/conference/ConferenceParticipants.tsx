import { Users } from "lucide-react";
import { isAdmitted } from "../../collaboration";
import { initials } from "../../utils";
import { Button, ErrorNotice, Loading } from "../ui";
import type { ConferenceViewProps } from "./types";

/**
 * Отображает состав встречи и разрешённые сервером команды модерации.
 * @args props — общие данные, команды и состояние интерфейса, созданные корневой страницей.
 * @return Представление раздела с прежними условиями доступа и монтирования панелей.
 */
export function ConferenceParticipants(props: ConferenceViewProps) {
  const { user, membership, admitted, participants, closed, displayedPeople } =
    props.data;
  const { moderation } = props.commands;
  const roleNames = {
    owner: "Организатор",
    co_host: "Соорганизатор",
    participant: "Участник",
    guest: "Гость",
  };
  const presence = {
    joined: "Присоединился",
    left: "Вышел",
    waiting: "Ожидает",
    rejected: "Отклонён",
    kicked: "Исключён",
  };
  return (
    <>
      {admitted && (
        <section
          className="content-card participants-card"
          id="conference-participants"
        >
          <div className="section-heading">
            <h2>
              <Users size={20} />
              Участники{" "}
              <span className="count-badge">
                {displayedPeople.length}
                {closed && participants.hasNextPage ? "+" : ""}
              </span>
            </h2>
            <span className="muted small">Обновляется автоматически</span>
          </div>
          <ErrorNotice error={participants.error} />
          {participants.isPending ? (
            <Loading />
          ) : (
            <div className="participant-list">
              {displayedPeople.map((person) => (
                <div className="participant-row" key={person.id}>
                  <span
                    className={`avatar ${person.role === "owner" ? "avatar-owner" : ""}`}
                  >
                    {initials(person.displayName)}
                  </span>
                  {!closed &&
                    membership?.status === "joined" &&
                    person.id !== membership.id &&
                    person.role !== "owner" &&
                    isAdmitted(person) &&
                    person.status !== "kicked" &&
                    (membership.role === "owner" ||
                      (membership.role === "co_host" &&
                        person.role === "participant")) && (
                      <div
                        className="participant-controls"
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
                              action !== "camera" ||
                              membership.role === "owner",
                          )
                          .map(([action, label, blocked]) => (
                            <Button
                              key={action}
                              variant="secondary"
                              disabled={moderation.isPending}
                              onClick={() =>
                                moderation.mutate({
                                  participantId: person.id,
                                  action: { action, blocked: !blocked },
                                })
                              }
                            >
                              {blocked ? "Разрешить" : "Отключить"} {label}
                            </Button>
                          ))}
                        {membership.role === "owner" && (
                          <Button
                            variant="outline"
                            disabled={moderation.isPending}
                            onClick={() =>
                              moderation.mutate({
                                participantId: person.id,
                                action: {
                                  action: "role",
                                  role:
                                    person.role === "co_host"
                                      ? "participant"
                                      : "co_host",
                                },
                              })
                            }
                          >
                            {person.role === "co_host"
                              ? "Убрать соорганизатора"
                              : "Назначить соорганизатором"}
                          </Button>
                        )}
                        <Button
                          variant="danger"
                          disabled={moderation.isPending}
                          onClick={() => {
                            if (
                              window.confirm(
                                `Исключить ${person.displayName}? Повторное присоединение будет запрещено.`,
                              )
                            )
                              moderation.mutate({
                                participantId: person.id,
                                action: { action: "kick" },
                              });
                          }}
                        >
                          Исключить
                        </Button>
                      </div>
                    )}
                  <div className="participant-name">
                    <strong>
                      {person.displayName}
                      {person.userId === user?.id && (
                        <span className="muted"> (вы)</span>
                      )}
                    </strong>
                    <span>{roleNames[person.role]}</span>
                    {person.status === "joined" && (
                      <span className="participant-media-status">
                        {person.microphoneBlocked
                          ? "Звук запрещён"
                          : person.microphoneEnabled
                            ? "Микрофон включён"
                            : "Микрофон выключен"}
                        {" · "}
                        {person.cameraBlocked
                          ? "Видео запрещено"
                          : person.cameraEnabled
                            ? "Камера включена"
                            : "Камера выключена"}
                        {person.screenSharing ? " · Показывает экран" : ""}
                      </span>
                    )}
                  </div>
                  <span
                    className={`participant-status ${person.status === "joined" ? "participant-joined" : ""}`}
                  >
                    <span className="presence-dot" />
                    {person.joinedAt ||
                    ["waiting", "rejected", "kicked"].includes(person.status)
                      ? presence[person.status]
                      : "Ещё не присоединялся"}
                  </span>
                </div>
              ))}
            </div>
          )}
          {participants.hasNextPage && (
            <Button
              variant="outline"
              busy={participants.isFetchingNextPage}
              onClick={() => {
                void participants.fetchNextPage();
              }}
            >
              Загрузить ещё участников
            </Button>
          )}
          <p className="field-hint">
            {closed
              ? "Сохранён состав участников завершённой встречи."
              : "Показаны только участники онлайн. После 5 секунд без связи участник исчезает из списка."}
          </p>
        </section>
      )}
    </>
  );
}
